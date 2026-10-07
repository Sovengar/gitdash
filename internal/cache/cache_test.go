package cache

import (
	"encoding/json"
	"fmt"
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
		got[0].SecondaryGroup != "backend" || !got[0].HasRepo {
		t.Errorf("got[0] = %+v", got[0])
	}
	if !got[1].IsWorktree {
		t.Errorf("worktree lost: %+v", got[1])
	}
}

func TestLoadDiscardsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	alive := t.TempDir()
	if err := os.WriteFile(filepath.Join(alive, ".gitdash.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, []discovery.Project{
		{Path: alive, Name: "live"},
		{Path: filepath.Join(t.TempDir(), "cleared"), Name: "dead"},
	}); err != nil {
		t.Fatal(err)
	}

	got := Load(path, ".gitdash.toml")
	if len(got) != 1 || got[0].Name != "live" {
		t.Errorf("got = %+v", got)
	}
}

func TestLoadCorruptSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("got = %+v, want nil without crashing", got)
	}
}

func TestLoadWrongVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"repos":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("future version accepted: %+v", got)
	}
}

func TestLoadV2Ignored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	raw := `{"version":2,"repos":[{"path":"/x","name":"api","group":"vsocial","has_repo":true}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("cache v2 accepted: %+v", got)
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
		t.Errorf("invalid json: %v", err)
	}
}

// Every test of the package passes a path by hand, so the function resolving $XDG_CACHE_HOME never ran; that is the one deciding where repos.json lives, the file that makes the app paint instantly instead of staying empty while it scans.
func TestPathRespectsXDGCacheHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if want := filepath.Join(dir, DirName, FileName); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// Without XDG_CACHE_HOME nor HOME there is no user directory and Path has to say so: returning an invented path would make Save write somewhere nobody reads, in silence.
func TestPathWithoutDirectoryOfUserGivesError(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if got, err := Path(); err == nil {
		t.Errorf("Path without HOME nor XDG = %q, want error", got)
	}
}

// An entry without Path cannot be validated against the marker (.Stat on "" is the process working directory, not a repo) and is useless for navigation, so it is dropped BEFORE the marker is looked at; moving the filter after would let a corrupt entry in repos.json start the dashboard with a row that does not exist on disk.
func TestLoadDiscardsInputWithoutPath(t *testing.T) {
	dir := t.TempDir()
	live := t.TempDir()
	if err := os.WriteFile(filepath.Join(live, ".gitdash.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "repos.json")
	raw := fmt.Sprintf(`{"version":%d,"repos":[
		{"path":%q,"name":"live"},
		{"path":"","name":"no-path"},
		{"name":"no-field"}
	]}`, version, live)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(path, ".gitdash.toml")
	if len(got) != 1 {
		t.Fatalf("Load returned %d repos, want 1: %+v", len(got), got)
	}
	if got[0].Name != "live" {
		t.Errorf("it kept %q, want the one with a marker", got[0].Name)
	}
}

// The checked case is a path whose parent directory is a FILE, which makes MkdirAll fail with no permissions and no root involved.
func TestSavePropagatesErrorOfDirectory(t *testing.T) {
	block := filepath.Join(t.TempDir(), "i-am-a-file")
	if err := os.WriteFile(block, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Save(filepath.Join(block, "repos.json"), nil)
	if err == nil {
		t.Fatal("Save = nil, want error: it cannot create a directory under a file")
	}
}

func TestSaveCreatesTheMissingDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", "repos.json")
	if err := Save(path, []discovery.Project{
		{Path: "/x", Name: "x", HasRepo: true},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load(path, ".gitdash.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file was not written: %v", err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("the file is not valid JSON: %v", err)
	}
	if f.Version != version {
		t.Errorf("version = %d, want %d", f.Version, version)
	}
	if len(f.Repos) != 1 || f.Repos[0].Name != "x" {
		t.Errorf("repos = %+v, want the only saved project", f.Repos)
	}
	_ = got
}
