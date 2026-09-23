package gitstatus

import (
	"strings"
	"sync"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/testutil"
)

// porcelainClean es la salida típica de un repo limpio sincronizado.
const porcelainClean = `# branch.oid 3f4e0c0f6a4e3c5a5d5b5e5a5d5b5e5a5d5b5e5a
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
`

func TestParseCleanS5_1(t *testing.T) {
	st, files := ParsePorcelain(porcelainClean)
	if st.Branch != "main" || st.Upstream != "origin/main" || !st.HasUpstream {
		t.Errorf("st = %+v", st)
	}
	if st.Ahead != 0 || st.Behind != 0 || st.Dirty() != 0 || len(files) != 0 {
		t.Errorf("S5.1 falla: %+v files=%v", st, files)
	}
	if got := st.Derive(); got != StateClean {
		t.Errorf("derive = %v, want clean", got)
	}
}

func TestParseAheadBehindDivergedS5_2(t *testing.T) {
	ahead := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +2 -0", 1)
	st, _ := ParsePorcelain(ahead)
	if st.Ahead != 2 || st.Behind != 0 {
		t.Errorf("ahead: %+v", st)
	}
	if st.Derive() != StateAhead {
		t.Errorf("ahead derive = %v", st.Derive())
	}

	behind := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +0 -3", 1)
	st, _ = ParsePorcelain(behind)
	if st.Ahead != 0 || st.Behind != 3 {
		t.Errorf("behind: %+v", st)
	}
	if st.Derive() != StateBehind {
		t.Errorf("behind derive = %v", st.Derive())
	}

	diverged := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +2 -3", 1)
	st, _ = ParsePorcelain(diverged)
	if st.Derive() != StateDiverged {
		t.Errorf("diverged derive = %v", st.Derive())
	}
}

func TestParseDirtyS5_3(t *testing.T) {
	out := porcelainClean + `1 .M NRM 100644 100644 100644 abc def src/main.go
? README.md
? docs/
2 R. N... 100644 100644 100644 abc def R100 new.txt	old.txt
`
	st, files := ParsePorcelain(out)
	if st.TrackedChanges != 2 || st.Untracked != 2 {
		t.Errorf("S5.3: tracked=%d untracked=%d", st.TrackedChanges, st.Untracked)
	}
	if st.Derive() != StateDirty {
		t.Errorf("derive = %v, want dirty", st.Derive())
	}
	if len(files) != 4 {
		t.Fatalf("files = %d, want 4", len(files))
	}
	if files[0].Path != "src/main.go" || files[0].Code != ".M" {
		t.Errorf("files[0] = %+v", files[0])
	}
	if files[1].Code != "??" {
		t.Errorf("files[1] = %+v", files[1])
	}
	if files[3].Path != "new.txt" {
		t.Errorf("rename path = %q, want new.txt", files[3].Path)
	}
}

func TestParseNoUpstreamS5_4(t *testing.T) {
	st, _ := ParsePorcelain("# branch.oid abc\n# branch.head main\n")
	if st.HasUpstream {
		t.Error("S5.4: upstream detectado sin línea")
	}
	if st.Derive() != StateNoUpstream {
		t.Errorf("derive = %v, want no-upstream", st.Derive())
	}
}

func TestParseDetachedS5_5(t *testing.T) {
	out := strings.Replace(porcelainClean, "# branch.head main", "# branch.head (detached)", 1)
	st, _ := ParsePorcelain(out)
	if !st.Detached || st.Branch != "" {
		t.Errorf("S5.5: %+v", st)
	}
}

func TestParseUnbornBranch(t *testing.T) {
	st, _ := ParsePorcelain("# branch.oid (initial)\n# branch.head (main)\n")
	if st.Branch != "main" || st.Detached {
		t.Errorf("unborn: %+v", st)
	}
}

func TestParseLog(t *testing.T) {
	out := "abc1234\x001700000000\x00feat: uno\ndef5678\x001699000000\x00fix: dos\n"
	commits := ParseLog(out)
	if len(commits) != 2 {
		t.Fatalf("commits = %d", len(commits))
	}
	if commits[0].Sha != "abc1234" || commits[0].Subject != "feat: uno" || commits[0].When != 1700000000 {
		t.Errorf("commits[0] = %+v", commits[0])
	}
}

// --- tests de recolección real con repos fixture ---

func TestCollectCleanS5_1(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	st := Collect(t.Context(), dir, "")
	if st.Err != "" {
		t.Fatalf("S5.6: err = %q", st.Err)
	}
	if st.Status.Derive() != StateClean || st.LastCommit == 0 {
		t.Errorf("snap = %+v", snapSummary(st))
	}
}

func TestCollectDirtyS5_3(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	// base.txt está trackeado: modificarlo cuenta como cambio tracked.
	testutil.WriteUncommitted(t, dir, map[string]string{"base.txt": "changed"})
	testutil.WriteUntracked(t, dir, map[string]string{"un1.txt": "x", "un2.txt": "y"})

	st := Collect(t.Context(), dir, "")
	if st.Status.TrackedChanges != 1 || st.Status.Untracked != 2 {
		t.Errorf("S5.3: tracked=%d untracked=%d", st.Status.TrackedChanges, st.Status.Untracked)
	}
	if st.Status.Derive() != StateDirty {
		t.Errorf("derive = %v", st.Status.Derive())
	}
}

func TestCollectAheadBehindDivergedS5_2(t *testing.T) {
	ahead, _ := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, ahead, map[string]string{"extra.txt": "x"}, "local 1")
	testutil.CommitFiles(t, ahead, map[string]string{"extra2.txt": "x"}, "local 2")

	st := Collect(t.Context(), ahead, "")
	if st.Status.Ahead != 2 || st.Status.Derive() != StateAhead {
		t.Errorf("S5.2 ahead: %+v", snapSummary(st))
	}

	behind, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 3, "behind-")
	testutil.FetchLocal(t, behind) // sin fetch el repo no ve el behind
	st = Collect(t.Context(), behind, "")
	if st.Status.Behind != 3 || st.Status.Derive() != StateBehind {
		t.Errorf("S5.2 behind: %+v", snapSummary(st))
	}

	diverged, origin2 := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, diverged, map[string]string{"l.txt": "l"}, "local")
	testutil.PushUpstreamCommits(t, origin2, 2, "div-")
	testutil.FetchLocal(t, diverged)
	st = Collect(t.Context(), diverged, "")
	if st.Status.Ahead != 1 || st.Status.Behind != 2 || st.Status.Derive() != StateDiverged {
		t.Errorf("S5.2 diverged: %+v", snapSummary(st))
	}
}

func TestCollectNoUpstreamS5_4(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	st := Collect(t.Context(), dir, "")
	if st.Status.HasUpstream || st.Status.Derive() != StateNoUpstream {
		t.Errorf("S5.4: %+v", snapSummary(st))
	}
}

func TestCollectDetachedS5_5(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	testutil.Detach(t, dir)
	st := Collect(t.Context(), dir, "")
	if !st.Status.Detached {
		t.Fatalf("S5.5: %+v", st.Status)
	}
	if len(st.Status.Branch) != 7 {
		t.Errorf("branch detached = %q, want sha corto", st.Status.Branch)
	}
}

func TestCollectCorruptS5_6(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	// .git corrupto: el binario git falla y el error viaja en el Snapshot.
	testutil.BreakGit(t, dir)
	st := Collect(t.Context(), dir, "")
	if st.Err == "" {
		t.Fatal("S5.6: corrupto sin error")
	}
	if st.State(true) != StateError {
		t.Errorf("state = %v, want error", st.State(true))
	}
}

func TestStreamPool(t *testing.T) {
	a, _ := testutil.NewRepo(t, true)
	b, _ := testutil.NewRepo(t, false)
	testutil.WriteUncommitted(t, b, map[string]string{"m.txt": "m"})

	projects := []discovery.Project{
		{Path: a, HasRepo: true},
		{Path: b, HasRepo: true},
	}
	got := map[string]State{}
	var mu sync.Mutex
	// emit se invoca concurrentemente (contrato de StreamPool): proteger el mapa.
	StreamPool(t.Context(), projects, "", 2, func(path string, st Snapshot) {
		mu.Lock()
		got[path] = st.State(true)
		mu.Unlock()
	})
	if len(got) != 2 {
		t.Fatalf("emit = %d paths, want 2", len(got))
	}
	if got[a] != StateClean || got[b] != StateDirty {
		t.Errorf("estados = %v", got)
	}
}

func snapSummary(s Snapshot) string {
	return s.Err + "|" + s.Status.Derive().String()
}
