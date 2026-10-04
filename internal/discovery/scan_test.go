package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/config"
	"gitdash/internal/testutil"
)

func cfgRoots(roots ...string) config.Config {
	cfg := config.Defaults()
	cfg.Roots = roots
	return cfg
}

func TestDetectByMarker(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "projects", "api")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	p := projects[0]
	if p.Path != proj || p.Name != "api" || !p.HasRepo || p.IsWorktree {
		t.Errorf("p = %+v", p)
	}
}

func TestPruneHiddenAndExcluded(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".hidden", "proj")
	excluded := filepath.Join(root, "api", "node_modules", "dep")
	for _, dir := range []string{hidden, excluded} {
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", "", false)
	}

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("projects = %d, want 0 (poda)", len(projects))
	}
}

func TestUnlimitedDepth(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "a", "b", "c", "d", "proj")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "proj" {
		t.Errorf("projects = %+v", projects)
	}
}

func TestNestedValid(t *testing.T) {
	root := t.TempDir()
	mono := filepath.Join(root, "mono")
	sub := filepath.Join(mono, "sub")
	for _, dir := range []string{mono, sub} {
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", "", false)
	}

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2 (mono + sub)", len(projects))
	}
}

// Scan returns the projects sorted by path and not in walk order (which depends on the filesystem); without this, inverting the comparator would go unnoticed and the TUI would see repos reshuffled between scans.
func TestScanSortsForPath(t *testing.T) {
	root := t.TempDir()
	creados := []string{"zeta", "alfa", "middle"}
	for _, name := range creados {
		dir := filepath.Join(root, name)
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", "", false)
	}

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 3 {
		t.Fatalf("projects = %d, want 3", len(projects))
	}
	want := []string{
		filepath.Join(root, "alfa"),
		filepath.Join(root, "middle"),
		filepath.Join(root, "zeta"),
	}
	for i, w := range want {
		if projects[i].Path != w {
			t.Errorf("projects[%d] = %q, want %q (ordered by path)", i, projects[i].Path, w)
		}
	}
}

// With several roots the global order is still by path, not root by root: a root can interleave its projects with the other one's. The roots are created under a common parent and in reversed order on purpose, so that interleaving is observable: with per-root order "a-root" would come out last and the test would fail.
func TestScanSortsBetweenRoots(t *testing.T) {
	base := t.TempDir()
	second := filepath.Join(base, "a-root")
	first := filepath.Join(base, "z-root")
	for _, dir := range []string{
		filepath.Join(first, "b"),
		filepath.Join(first, "d"),
		filepath.Join(second, "a"),
		filepath.Join(second, "c"),
	} {
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", "", false)
	}

	projects, err := Scan(cfgRoots(first, second))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 4 {
		t.Fatalf("projects = %d, want 4", len(projects))
	}
	want := []string{
		filepath.Join(second, "a"),
		filepath.Join(second, "c"),
		filepath.Join(first, "b"),
		filepath.Join(first, "d"),
	}
	for i, w := range want {
		if projects[i].Path != w {
			t.Errorf("projects[%d] = %q, want %q", i, projects[i].Path, w)
		}
	}
}

func TestWorktree(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main-repo")
	testutil.Init(t, main)
	testutil.Marker(t, main, "", "", "", false)
	testutil.CommitFiles(t, main, map[string]string{"a.txt": "a"}, "init")

	wt := filepath.Join(root, "wt-proj")
	testutil.MakeWorktree(t, main, wt, "wt-branch")
	testutil.Marker(t, wt, "", "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	var found bool
	for _, p := range projects {
		if p.Path == wt {
			found = true
			if !p.IsWorktree || !p.HasRepo {
				t.Errorf("worktree mal clasificado: %+v", p)
			}
		}
	}
	if !found {
		t.Error("worktree not discovered")
	}
}

func TestMarkerWithoutRepo(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "plain")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Marker(t, proj, "", "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].HasRepo {
		t.Errorf("projects = %+v, want 1 with no repo", projects)
	}
}

func TestMarkerMetadata(t *testing.T) {
	root := t.TempDir()
	withMeta := filepath.Join(root, "dirname")
	empty := filepath.Join(root, "emptymarker")
	bad := filepath.Join(root, "badmarker")
	for _, dir := range []string{withMeta, empty, bad} {
		testutil.Init(t, dir)
	}
	testutil.Marker(t, withMeta, "api", "vsocial", "", false)
	testutil.Marker(t, empty, "", "", "", false)
	testutil.Marker(t, bad, "", "", "", true)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 3 {
		t.Fatalf("projects = %d, want 3", len(projects))
	}
	byPath := map[string]Project{}
	for _, p := range projects {
		byPath[p.Name] = p
	}
	if p := byPath["api"]; p.PrimaryGroup != "vsocial" {
		t.Errorf("name/primary = %q/%q", p.Name, p.PrimaryGroup)
	}
	if p := byPath["emptymarker"]; p.PrimaryGroup != "" {
		t.Errorf("primary = %q, want empty", p.PrimaryGroup)
	}
	if p := byPath["badmarker"]; p.MarkerErr == "" {
		t.Error("malformed marker with no visible error")
	}
}

func TestMarkerGroups(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	oldKey := filepath.Join(root, "oldkey")
	secOnly := filepath.Join(root, "seconly")
	for _, dir := range []string{nested, oldKey, secOnly} {
		testutil.Init(t, dir)
	}
	testutil.Marker(t, nested, "api", "vsocial", "backend", false)
	if err := os.WriteFile(filepath.Join(oldKey, ".gitdash.toml"), []byte("group = \"backend\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Marker(t, secOnly, "solo", "", "infra", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Project{}
	for _, p := range projects {
		byPath[p.Name] = p
	}
	if p := byPath["api"]; p.PrimaryGroup != "vsocial" || p.SecondaryGroup != "backend" {
		t.Errorf("primary/secondary = %q/%q", p.PrimaryGroup, p.SecondaryGroup)
	}
	if p := byPath["oldkey"]; p.PrimaryGroup != "" || p.SecondaryGroup != "" {
		t.Errorf("old group grouped: %q/%q", p.PrimaryGroup, p.SecondaryGroup)
	}
	if p := byPath["solo"]; p.PrimaryGroup != "" || p.SecondaryGroup != "" {
		t.Errorf("secondary without primary: %q/%q", p.PrimaryGroup, p.SecondaryGroup)
	}
}

func TestUnreadableRootNotAborts(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "ok")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", "", false)

	_, err := Scan(cfgRoots(root, filepath.Join(root, "fantasma")))
	if err == nil {
		t.Error("expected an aggregated error for an unreadable root")
	}
}

// Scan tolerates bad roots: it reports them as one aggregated error and carries on. A missing root and a root that is a FILE (not a directory) have to end up in the same sack and with the same warning text, because for the user they are the same thing: "this config path cannot be walked".
func TestScanToleratesRootsUseless(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "proyecto")
	testutil.Init(t, good)
	testutil.Marker(t, good, "", "", "", false)

	noDir := filepath.Join(root, "a-file")
	if err := os.WriteFile(noDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := cfgRoots(root, filepath.Join(root, "nonexistent"), noDir)
	projects, err := Scan(cfg)

	if len(projects) != 1 || projects[0].Path != good {
		t.Errorf("projects = %+v, want only %s (a bad root does not abort the rest)", projects, good)
	}
	if err == nil {
		t.Fatal("Scan = nil error, want a notice about the unreadable roots")
	}
	for _, want := range []string{"nonexistent", "a-file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %q", err, want)
		}
	}
	if n := strings.Count(err.Error(), "unreadable root"); n != 2 {
		t.Errorf("the error mentions %d unreadable roots, want 2 (the ones that are not directories)", n)
	}
}

func TestClassifyGitWithoutGit(t *testing.T) {
	dir := t.TempDir()
	if k, main := classifyGit(filepath.Join(dir, ".git")); k != gitNone || main != "" {
		t.Errorf("classifyGit(inexistente) = %v/%q, want gitNone/\"\"", k, main)
	}

	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if k, main := classifyGit(filepath.Join(repo, ".git")); k != gitDir || main != "" {
		t.Errorf("classifyGit(dir) = %v/%q, want gitDir/%q", k, main, "")
	}

	raro := filepath.Join(dir, "raro")
	if err := os.WriteFile(raro, []byte("I am not a gitdir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if k, main := classifyGit(raro); k != gitNone || main != "" {
		t.Errorf("classifyGit(file without gitdir:) = %v/%q, want gitNone/%q", k, main, "")
	}
}

func TestClassifyGitWorktree(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main-repo")
	wt := filepath.Join(dir, "feature")
	gitdir := filepath.Join(main, ".git", "worktrees", "feature")
	if err := os.MkdirAll(filepath.Join(wt), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitdir, 0o755); err != nil {
		t.Fatal(err)
	}
	pointer := filepath.Join(wt, ".git")
	if err := os.WriteFile(pointer, []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	k, repoPrincipal := classifyGit(pointer)
	if k != gitFile {
		t.Fatalf("classifyGit = %v, want gitFile (it is a worktree)", k)
	}
	if want := main; repoPrincipal != want {
		t.Errorf("repo principal = %q, want %q", repoPrincipal, want)
	}
}

func TestMarkerPromptWithoutMarkerNotIsError(t *testing.T) {
	dir := t.TempDir()
	p, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err != nil {
		t.Errorf("MarkerPrompt without a marker = %v, want nil (no prompt is no error)", err)
	}
	if p != "" {
		t.Errorf("prompt = %q, want empty", p)
	}
}

// A directory without read permission is the real case behind the walk's error branch: the user has a repo inside something unreadable and Scan has to SKIP it instead of aborting the whole scan (aborting would lose every visible repo because of a stranger's directory).
func TestScanJumpsDirectoryUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("as root directory permissions do not prevent reading")
	}
	root := t.TempDir()
	good := filepath.Join(root, "proyecto")
	testutil.Init(t, good)
	testutil.Marker(t, good, "", "", "", false)

	bloqueado := filepath.Join(root, "no-access")
	if err := os.MkdirAll(bloqueado, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bloqueado, 0o755) })

	projects, err := Scan(cfgRoots(root))
	if len(projects) != 1 || projects[0].Path != good {
		t.Errorf("projects = %+v, want only %s (an unreadable dir does not abort)", projects, good)
	}
	if err != nil {
		t.Errorf("Scan = %v, want nil (the unreadable dir is skipped silently)", err)
	}
}

func TestMarkerPromptMalformedIsError(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "name = [roto\n")
	p, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err == nil {
		t.Fatal("MarkerPrompt with a malformed marker = nil, want an error (the file is broken)")
	}
	if !strings.Contains(err.Error(), "marker") {
		t.Errorf("error = %q, want it to name the marker", err)
	}
	if p != "" {
		t.Errorf("prompt = %q with an error, want empty", p)
	}
}

func TestClassifyGitWithFileUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("as root file permissions do not prevent reading")
	}
	dir := t.TempDir()
	git := filepath.Join(dir, ".git")
	if err := os.WriteFile(git, []byte("gitdir: /no/can/read\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(git, 0o644) })

	if k, main := classifyGit(git); k != gitNone || main != "" {
		t.Errorf("classifyGit(unreadable) = %v/%q, want gitNone/%q", k, main, "")
	}
}

// Triggered with a directory instead of with permissions on purpose: root reads a 0o000 file, so a permissions test would say one thing locally and another in CI, while this one does not depend on who runs it.
func TestMarkerPromptWithMarkerNotReadableIsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gitdash.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err == nil {
		t.Fatal("MarkerPrompt with a marker that is a directory = nil, want an error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want EISDIR and not ENOENT: a directory is NOT a missing marker", err)
	}
	if p != "" {
		t.Errorf("prompt = %q with an error, want empty", p)
	}
}

// The warning matters: without it a relative root with a broken cwd returns zero projects and NO error, which reads as "I have no repos" instead of "I cannot even tell where I am".
func TestScanWithCwdDeletedReportsTheRoot(t *testing.T) {
	roto := t.TempDir()
	t.Chdir(roto)
	if err := os.RemoveAll(roto); err != nil {
		t.Fatal(err)
	}

	projects, err := Scan(cfgRoots("."))
	if err == nil {
		t.Fatalf("Scan with the cwd deleted = nil, want an error (0 projects and no notice looks like an empty root): %+v", projects)
	}
	if len(projects) != 0 {
		t.Errorf("projects = %+v, want ninguno", projects)
	}
	if !strings.Contains(err.Error(), ".") {
		t.Errorf("error = %q, want it to name the root it could not resolve", err)
	}
}

// No 0o000 is needed (root reads it): a DIRECTORY named after the marker gives EISDIR, and the unit is the only way that error reaches the model (Scan never calls it unreadable, hasMarker requires a file).
func TestParseMarkerUnreadablePropagatesTheError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gitdash.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := parseMarker(filepath.Join(dir, ".gitdash.toml"))
	if err == nil {
		t.Fatal("parseMarker = nil, want the read error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want EISDIR and not ENOENT", err)
	}
	p := inspect(dir, ".gitdash.toml")
	if p.MarkerErr == "" {
		t.Errorf("MarkerErr empty, want the read error: %+v", p)
	}
	if p.Name != filepath.Base(dir) {
		t.Errorf("Name = %q, want the directory's name (the marker gave none)", p.Name)
	}
}
