// Package cache persists discovery on disk to paint the table instantly while the rescan runs in background.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gitdash/internal/discovery"
)

const FileName = "repos.json"

const DirName = "gitdash"

// Format version; a file from another version is ignored (v3 replaced group with primary_group/secondary_group).
const version = 3

type Entry struct {
	Path           string `json:"path"`
	Name           string `json:"name"`
	PrimaryGroup   string `json:"primary_group,omitempty"`
	SecondaryGroup string `json:"secondary_group,omitempty"`
	SyncBranch     string `json:"sync_branch,omitempty"`
	HasRepo        bool   `json:"has_repo"`
	IsWorktree     bool   `json:"is_worktree"`
	MainRepo       string `json:"main_repo,omitempty"`
}

type File struct {
	Version int     `json:"version"`
	Repos   []Entry `json:"repos"`
}

func Path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, FileName), nil
}

// A corrupt or unknown-version cache yields an empty list without error.
func Load(path, marker string) []discovery.Project {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != version {
		return nil
	}
	var projects []discovery.Project
	for _, e := range f.Repos {
		if e.Path == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.Path, marker)); err != nil {
			continue
		}
		projects = append(projects, discovery.Project{
			Path:           e.Path,
			Name:           e.Name,
			PrimaryGroup:   e.PrimaryGroup,
			SecondaryGroup: e.SecondaryGroup,
			SyncBranch:     e.SyncBranch,
			HasRepo:        e.HasRepo,
			IsWorktree:     e.IsWorktree,
			MainRepo:       e.MainRepo,
		})
	}
	return projects
}

// Best effort: a write error is not fatal.
func Save(path string, projects []discovery.Project) error {
	f := File{Version: version, Repos: make([]Entry, 0, len(projects))}
	for _, p := range projects {
		f.Repos = append(f.Repos, Entry{
			Path:           p.Path,
			Name:           p.Name,
			PrimaryGroup:   p.PrimaryGroup,
			SecondaryGroup: p.SecondaryGroup,
			SyncBranch:     p.SyncBranch,
			HasRepo:        p.HasRepo,
			IsWorktree:     p.IsWorktree,
			MainRepo:       p.MainRepo,
		})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// No error branch on Marshal: File is an int plus a slice of string/bool structs, which encoding/json cannot fail on.
	raw, _ := json.MarshalIndent(f, "", "  ")
	return os.WriteFile(path, raw, 0o644)
}
