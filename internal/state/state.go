// Package state persists UI state (collapsed groups) across sessions; keys are the primary name or "primary/secondary", so identically named groups under different primaries cannot collide.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const DirName = "gitdash"

const FileName = "collapsed.json"

// Worktree expansion keys carry the opposite polarity to group keys (true means expanded, not collapsed), which is why both spaces share one file behind a prefix.
const WorktreePrefix = "wt/"

type Store struct {
	base string
}

func DefaultBaseDir() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, DirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", DirName), nil
}

func NewStore() (*Store, error) {
	base, err := DefaultBaseDir()
	if err != nil {
		return nil, err
	}
	return &Store{base: base}, nil
}

func NewStoreAt(base string) *Store {
	return &Store{base: base}
}

func (s *Store) Base() string { return s.base }

func (s *Store) CollapsedFile() string {
	return filepath.Join(s.base, FileName)
}

// Best effort: a write error must not block the UI.
func (s *Store) SaveCollapsed(groups map[string]bool) error {
	// No error branch on Marshal: a map[string]bool always serializes.
	data, _ := json.MarshalIndent(groups, "", "  ")
	if err := os.MkdirAll(s.base, 0o755); err != nil {
		return fmt.Errorf("could not create state directory %s: %w", s.base, err)
	}
	target := s.CollapsedFile()
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("could not write collapsed.json: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("could not replace collapsed.json: %w", err)
	}
	return nil
}

// A missing or corrupt file yields no state without error: the user loses the state, not the session.
func (s *Store) LoadCollapsed() map[string]bool {
	data, err := os.ReadFile(s.CollapsedFile())
	if err != nil {
		return nil
	}
	groups := make(map[string]bool)
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil
	}
	return groups
}
