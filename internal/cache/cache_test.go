package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gitdash/internal/discovery"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	projects := []discovery.Project{
		{Path: t.TempDir(), Name: "api", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: t.TempDir(), Name: "wt", IsWorktree: true, HasRepo: true},
	}
	for _, p := range projects {
		if err := os.WriteFile(filepath.Join(p.Path, ".gitdash.toml"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(path, projects); err != nil {
		t.Fatal(err)
	}

	got := Load(path, ".gitdash.toml")
	if len(got) != 2 {
		t.Fatalf("load = %d, want 2", len(got))
	}
	if got[0].Name != "api" || got[0].PrimaryGroup != "vsocial" ||
		got[0].SecondaryGroup != "backend" || !got[0].HasRepo { // S22.2
		t.Errorf("got[0] = %+v", got[0])
	}
	if !got[1].IsWorktree {
		t.Errorf("worktree perdido: %+v", got[1])
	}
}

func TestLoadDiscardsStaleS11_2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	alive := t.TempDir()
	if err := os.WriteFile(filepath.Join(alive, ".gitdash.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, []discovery.Project{
		{Path: alive, Name: "vivo"},
		{Path: filepath.Join(t.TempDir(), "borrado"), Name: "muerto"},
	}); err != nil {
		t.Fatal(err)
	}

	got := Load(path, ".gitdash.toml")
	if len(got) != 1 || got[0].Name != "vivo" {
		t.Errorf("S11.2: got = %+v", got)
	}
}

func TestLoadCorruptSilentS11_3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("S11.3: got = %+v, want nil sin crash", got)
	}
}

func TestLoadWrongVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"repos":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("versión futura aceptada: %+v", got)
	}
}

// S22.1: cache v2 (con clave group) se ignora silenciosamente.
func TestLoadV2IgnoredS22_1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	raw := `{"version":2,"repos":[{"path":"/x","name":"api","group":"vsocial","has_repo":true}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("S22.1: cache v2 aceptado: %+v", got)
	}
}

func TestSaveValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "repos.json")
	if err := Save(path, nil); err != nil {
		t.Fatal(err)
	}
	var f File
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != version {
		t.Errorf("json inválido: %v", err)
	}
}
