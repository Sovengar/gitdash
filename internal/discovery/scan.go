// Package discovery finds projects by marker file; the repo is resolved in the marker's own folder, never by walking up to .git.
package discovery

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"gitdash/internal/config"
)

type Project struct {
	Path           string
	Name           string
	PrimaryGroup   string
	SecondaryGroup string
	SyncBranch     string
	HasRepo        bool
	IsWorktree     bool
	MainRepo       string
	MarkerErr      string
}

// Unreadable roots are reported as one aggregated error instead of aborting the scan.
func Scan(cfg config.Config) ([]Project, error) {
	var projects []Project
	var errs []string

	for _, root := range cfg.Roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", root, err))
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			errs = append(errs, fmt.Sprintf("unreadable root: %s", root))
			continue
		}
		projects = append(projects, scanRoot(abs, cfg)...)
	}

	// slices.SortFunc instead of sort.Slice: the three-way comparator is idiomatic and hides no path comparison inside a closure.
	slices.SortFunc(projects, func(a, b Project) int { return strings.Compare(a.Path, b.Path) })

	var err error
	if len(errs) > 0 {
		err = fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return projects, err
}

// No error return on purpose: WalkDir only propagates what its callback returns, and this one swallows everything, so an error would be an unkillable branch.
func scanRoot(root string, cfg config.Config) []Project {
	exclude := make(map[string]bool, len(cfg.Exclude))
	for _, name := range cfg.Exclude {
		exclude[name] = true
	}

	var projects []Project
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && (isHidden(d.Name()) || exclude[d.Name()]) {
			return fs.SkipDir
		}
		if hasMarker(path, cfg.Marker) {
			projects = append(projects, inspect(path, cfg.Marker))
		}
		return nil
	})
	return projects
}

func inspect(dir, marker string) Project {
	p := Project{
		Path: dir,
		Name: filepath.Base(dir),
	}

	metadata, err := parseMarker(filepath.Join(dir, marker))
	if err != nil {
		p.MarkerErr = err.Error()
	} else {
		if metadata.Name != "" {
			p.Name = metadata.Name
		}
		p.PrimaryGroup = metadata.PrimaryGroup
		if metadata.PrimaryGroup != "" {
			p.SecondaryGroup = metadata.SecondaryGroup
		}
		p.SyncBranch = metadata.SyncBranch
	}

	switch k, main := classifyGit(filepath.Join(dir, ".git")); k {
	case gitDir:
		p.HasRepo = true
	case gitFile:
		p.HasRepo = true
		p.IsWorktree = true
		p.MainRepo = main
	default:
	}
	return p
}

// The old `group` key is gone with no fallback: hard rename on purpose.
type markerMeta struct {
	Name           string `toml:"name"`
	PrimaryGroup   string `toml:"primary_group"`
	SecondaryGroup string `toml:"secondary_group"`
	SyncBranch     string `toml:"sync_branch"`
	// [ai.<action>] carries the prompt only; the executable lives in the global config, never in the committed marker.
	AI map[string]markerAIAction `toml:"ai"`
}

type markerAIAction struct {
	Prompt string `toml:"prompt"`
}

// A missing marker is not an error (empty prompt) but an unreadable or malformed one is, so the TUI warns instead of launching blind.
func MarkerPrompt(dir, marker, action string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, marker))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	var meta markerMeta
	if err := toml.Unmarshal(raw, &meta); err != nil {
		return "", fmt.Errorf("marker: %v", err)
	}
	return meta.AI[action].Prompt, nil
}

func parseMarker(path string) (markerMeta, error) {
	var meta markerMeta
	raw, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	if err := toml.Unmarshal(raw, &meta); err != nil {
		return meta, fmt.Errorf("marker: %v", err)
	}
	return meta, nil
}

func hasMarker(dir, marker string) bool {
	info, err := os.Stat(filepath.Join(dir, marker))
	return err == nil && !info.IsDir()
}

type gitKind int

const (
	gitNone gitKind = iota
	gitDir
	gitFile
)

func classifyGit(gitPath string) (gitKind, string) {
	info, err := os.Lstat(gitPath)
	if err != nil {
		return gitNone, ""
	}
	if info.IsDir() {
		return gitDir, ""
	}
	raw, err := os.ReadFile(gitPath)
	if err != nil {
		return gitNone, ""
	}
	if rest, ok := strings.CutPrefix(string(raw), "gitdir:"); ok {
		main := strings.TrimSpace(rest)
		main = filepath.Dir(filepath.Dir(main))
		return gitFile, filepath.Dir(main)
	}
	return gitNone, ""
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}
