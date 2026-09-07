package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"gitdash/internal/config"
	"gitdash/internal/testutil"
)

func cfgRoots(roots ...string) config.Config {
	cfg := config.Defaults()
	cfg.Roots = roots
	return cfg
}

func TestDetectByMarkerS2_1(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "projects", "api")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", false)

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

func TestPruneHiddenAndExcludedS2_2(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".hidden", "proj")
	excluded := filepath.Join(root, "api", "node_modules", "dep")
	for _, dir := range []string{hidden, excluded} {
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", false)
	}

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("projects = %d, want 0 (poda)", len(projects))
	}
}

func TestUnlimitedDepthS2_3(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "a", "b", "c", "d", "proj")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "proj" {
		t.Errorf("projects = %+v", projects)
	}
}

func TestNestedValidS2_4(t *testing.T) {
	root := t.TempDir()
	mono := filepath.Join(root, "mono")
	sub := filepath.Join(mono, "sub")
	for _, dir := range []string{mono, sub} {
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", false)
	}

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2 (mono + sub)", len(projects))
	}
}

func TestWorktreeS3_2(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main-repo")
	testutil.Init(t, main)
	testutil.Marker(t, main, "", "", false)
	testutil.CommitFiles(t, main, map[string]string{"a.txt": "a"}, "init")

	wt := filepath.Join(root, "wt-proj")
	testutil.MakeWorktree(t, main, wt, "wt-branch")
	testutil.Marker(t, wt, "", "", false)

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
		t.Error("worktree no descubierto")
	}
}

func TestMarkerWithoutRepoS3_3(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "plain")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Marker(t, proj, "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].HasRepo {
		t.Errorf("projects = %+v, want 1 sin repo", projects)
	}
}

func TestMarkerMetadataS4(t *testing.T) {
	root := t.TempDir()
	withMeta := filepath.Join(root, "dirname")
	empty := filepath.Join(root, "emptymarker")
	bad := filepath.Join(root, "badmarker")
	for _, dir := range []string{withMeta, empty, bad} {
		testutil.Init(t, dir)
	}
	testutil.Marker(t, withMeta, "api", "vsocial", false)
	testutil.Marker(t, empty, "", "", false)
	testutil.Marker(t, bad, "", "", true)

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
	if p := byPath["api"]; p.Group != "vsocial" {
		t.Errorf("S4.1: name/group = %q/%q", p.Name, p.Group)
	}
	if p := byPath["emptymarker"]; p.Group != "" {
		t.Errorf("S4.2: group = %q, want vacío", p.Group)
	}
	if p := byPath["badmarker"]; p.MarkerErr == "" {
		t.Error("S4.3: marcador malformado sin error visible")
	}
}

func TestIlegibleRootNoAborta(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "ok")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", false)

	_, err := Scan(cfgRoots(root, filepath.Join(root, "fantasma")))
	if err == nil {
		t.Error("se esperaba error agregado por root ilegible")
	}
}
