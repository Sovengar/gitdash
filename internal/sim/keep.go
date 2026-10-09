package sim

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// keepImages bounds the cache: a simulation is a few hundred KB and nothing expires them but us.
const keepImages = 20

// subtree is where git-sim lays its media out under the media dir: <media>/git-sim_media/<repo basename>/{images,texts,videos}.
const subtree = "git-sim_media"

// Keep copies the rendered image out of git-sim's per-repo subtree and deletes that subtree: a render also leaves an animated mp4 and a dozen svg texts nobody opens, and without the deletion the cache would grow one video per simulation. The subtree is keyed by the repo's basename and the TUI allows only one visual per repo (m.running), which is what makes deleting it safe while another repo renders. The copy's name ends in the render's sequence, so Prune can bound the cache without ever stat'ing a file.
func Keep(image, mediaDir, workdir string) (string, error) {
	data, err := os.ReadFile(image)
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(image)
	if ext == "" {
		ext = ".jpg"
	}
	dst := filepath.Join(mediaDir, fmt.Sprintf("%s-%d%s", slug(filepath.Base(workdir)), time.Now().UnixNano(), ext))
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	_ = os.RemoveAll(filepath.Join(mediaDir, subtree, filepath.Base(workdir)))
	Prune(mediaDir, keepImages)
	return dst, nil
}

// Prune leaves the keep newest renders and deletes the rest: the sequence is IN the name (Keep writes it), so pruning never stats a file that a concurrent render could be replacing, and a name without a trailing sequence (git-sim's own leftovers) is left alone.
func Prune(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	renders := make([]render, 0, len(entries))
	for _, e := range entries {
		if seq, ok := sequence(e.Name()); ok {
			renders = append(renders, render{path: filepath.Join(dir, e.Name()), seq: seq})
		}
	}
	sort.Slice(renders, func(i, j int) bool { return renders[i].seq > renders[j].seq })
	for _, r := range renders[min(keep, len(renders)):] {
		_ = os.Remove(r.path)
	}
}

type render struct {
	path string
	seq  int64
}

// sequence reads the render's UnixNano back out of the name.
func sequence(name string) (int64, bool) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	i := strings.LastIndexByte(base, '-')
	if i < 0 {
		return 0, false
	}
	seq, err := strconv.ParseInt(base[i+1:], 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}

// slug keeps the repo's basename readable in the cache without letting spaces or other punctuation ride along into the file name.
func slug(s string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, s)
	return strings.Trim(out, "-")
}
