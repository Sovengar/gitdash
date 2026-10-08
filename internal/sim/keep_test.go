package sim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeImage(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("jpg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKeepCopiesFlatAndNamesItByRepoAndSequence(t *testing.T) {
	media := t.TempDir()
	workdir := filepath.Join(t.TempDir(), "my repo")
	image := writeImage(t, t.TempDir(), "render.jpg")

	got, err := Keep(image, media, workdir)
	if err != nil {
		t.Fatalf("Keep: %v", err)
	}
	if filepath.Dir(got) != media {
		t.Errorf("kept at %q, want the media dir's root (flat: the cache is ours, git-sim's nesting is not)", got)
	}
	if !strings.HasPrefix(filepath.Base(got), "my-repo-") {
		t.Errorf("kept name = %q, want the repo's slug (spaces readable, no path riding along)", filepath.Base(got))
	}
	data, err := os.ReadFile(got)
	if err != nil || string(data) != "jpg-bytes" {
		t.Errorf("kept contents = %q (err %v), want the rendered image's bytes", data, err)
	}
}

// A render also leaves an animated mp4 and a dozen svg texts nobody opens: without this deletion the cache grows one video per simulation.
func TestKeepDeletesTheRunSubtree(t *testing.T) {
	media := t.TempDir()
	workdir := filepath.Join(t.TempDir(), "demo")
	video := filepath.Join(media, subtree, "demo", "videos", "1080p60", "movie.mp4")
	writeImage(t, filepath.Dir(video), "movie.mp4")
	image := writeImage(t, filepath.Join(media, subtree, "demo", "images"), "render.jpg")

	if _, err := Keep(image, media, workdir); err != nil {
		t.Fatalf("Keep: %v", err)
	}
	if _, err := os.Stat(filepath.Join(media, subtree, "demo")); !os.IsNotExist(err) {
		t.Errorf("the run subtree survived: err=%v, the mp4 and texts would accumulate forever", err)
	}
}

func TestKeepWithoutSourceImage(t *testing.T) {
	if _, err := Keep(filepath.Join(t.TempDir(), "nope.jpg"), t.TempDir(), "/tmp/demo"); err == nil {
		t.Error("a missing image gave nil")
	}
}

// EISDIR-free refusal: the media dir is a FILE, so writing the copy under it cannot succeed whatever the runner's privileges are.
func TestKeepWithoutMediaDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	image := writeImage(t, t.TempDir(), "render.jpg")

	if _, err := Keep(image, blocker, "/tmp/demo"); err == nil {
		t.Error("an uncreatable destination gave nil")
	}
}

func TestKeepWithoutExtensionFallsBackToJpg(t *testing.T) {
	media := t.TempDir()
	image := writeImage(t, t.TempDir(), "noext")

	got, err := Keep(image, media, filepath.Join(t.TempDir(), "demo"))
	if err != nil {
		t.Fatalf("Keep: %v", err)
	}
	if filepath.Ext(got) != ".jpg" {
		t.Errorf("kept = %q, want the .jpg fallback (a viewer keys on the extension)", got)
	}
}

func TestKeepBoundsTheCache(t *testing.T) {
	media := t.TempDir()
	image := writeImage(t, t.TempDir(), "render.jpg")
	for i := 0; i < keepImages; i++ {
		writeImage(t, media, "demo-"+string(rune('a'+i))+"-1.jpg")
	}

	if _, err := Keep(image, media, filepath.Join(t.TempDir(), "demo")); err != nil {
		t.Fatalf("Keep: %v", err)
	}
	entries, err := os.ReadDir(media)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != keepImages {
		t.Errorf("cache holds %d files, want %d (keep renders, drop the oldest)", len(entries), keepImages)
	}
}

func TestPruneKeepsTheNewest(t *testing.T) {
	dir := t.TempDir()
	writeImage(t, dir, "demo-100.jpg")
	writeImage(t, dir, "demo-200.jpg")
	writeImage(t, dir, "demo-300.jpg")
	// A leading dash does not exempt the name: the sequence is read from the LAST one, so "-5.jpg" is an old render and goes.
	writeImage(t, dir, "-5.jpg")
	if err := os.MkdirAll(filepath.Join(dir, subtree), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "git-sim.jpg"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	Prune(dir, 1)

	if _, err := os.Stat(filepath.Join(dir, "demo-300.jpg")); err != nil {
		t.Errorf("the newest render was deleted: %v", err)
	}
	for _, gone := range []string{"demo-100.jpg", "demo-200.jpg", "-5.jpg"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s survived the prune: err=%v", gone, err)
		}
	}
	for _, kept := range []string{"notes.txt", "git-sim.jpg", subtree} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Errorf("prune touched %q (no sequence in the name): %v", kept, err)
		}
	}
}

func TestPruneAllWithKeepZero(t *testing.T) {
	dir := t.TempDir()
	writeImage(t, dir, "demo-1.jpg")

	Prune(dir, 0)

	if _, err := os.Stat(filepath.Join(dir, "demo-1.jpg")); !os.IsNotExist(err) {
		t.Errorf("keep=0 left the render: err=%v", err)
	}
}

func TestPruneWithoutDirIsSilent(t *testing.T) {
	Prune(filepath.Join(t.TempDir(), "nope"), 5)
}

func TestSequenceReadsTheNanosBack(t *testing.T) {
	if got, ok := sequence("demo-1760000000123456789.jpg"); !ok || got != 1760000000123456789 {
		t.Errorf("sequence = %d/%v, want the nanos", got, ok)
	}
	if _, ok := sequence("notes.txt"); ok {
		t.Error("a name without a sequence claimed one")
	}
	if _, ok := sequence("git-sim.jpg"); ok {
		t.Error("git-sim's suffix-less name claimed a sequence")
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"demo":       "demo",
		"alpha repo": "alpha-repo",
		"repo0 zap":  "repo0-zap",
		"zoo9":       "zoo9",
		"a.b_c-d":    "a.b_c-d",
		"-edge-":     "edge",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
