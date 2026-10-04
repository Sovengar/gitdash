// Package config loads gitdash's XDG config: the file is optional and a malformed one degrades to defaults with a warning.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const FileName = "config.toml"

const DirName = "gitdash"

const DefaultMarker = ".gitdash.toml"

var DefaultExclude = []string{
	"node_modules", "target", "vendor", "dist", "build", "out", "coverage",
	".venv", "__pycache__", ".gradle", ".terraform", "testdata",
}

type Keybindings map[string]string

type Commands map[string]string

type Config struct {
	Marker     string
	Roots      []string
	Exclude    []string
	Editor     string
	SyncBranch string
	// SyncBranchExplicit tells a hand-written sync_branch from an inherited default: a default is a guess the repo can disprove, a declared branch is not.
	SyncBranchExplicit bool
	FetchAuto          bool
	FetchConcurrency   int
	FetchTimeout       time.Duration
	Keybindings        Keybindings
	Commands           Commands
	// The executable comes from the global config only, never from the committed marker (untrusted input), and without a template the AI action cannot launch.
	AICommands map[string]string
	// Public hosts come from DefaultForges, so what needs declaring are the self-managed instances (or disabling unused ones with enabled = false).
	Forges map[string]ForgeConfig
}

type aiActionConfig struct {
	Command string `toml:"command"`
}

// Pointers tell "absent" (keep what is declared) from "zero": enabled = false has to be able to switch a provider off.
type forgeConfig struct {
	Enabled   *bool   `toml:"enabled"`
	Host      *string `toml:"host"`
	APIBase   *string `toml:"api_base"`
	CloneBase *string `toml:"clone_base"`
}

type fetchConfig struct {
	Auto        *bool   `toml:"auto"`
	Concurrency *int    `toml:"concurrency"`
	Timeout     *string `toml:"timeout"`
}

type fileConfig struct {
	Marker      *string                   `toml:"marker"`
	Roots       []string                  `toml:"roots"`
	Exclude     []string                  `toml:"exclude"`
	Editor      *string                   `toml:"editor"`
	SyncBranch  *string                   `toml:"sync_branch"`
	Fetch       *fetchConfig              `toml:"fetch"`
	Keybindings map[string]string         `toml:"keybindings"`
	Commands    map[string]string         `toml:"commands"`
	AI          map[string]aiActionConfig `toml:"ai"`
	Forge       map[string]forgeConfig    `toml:"forge"`
}

// Never fails: any error falls back to defaults and travels as a warning for the UI.
func Load() (Config, string) {
	path, err := Path()
	if err != nil {
		return Defaults(), ""
	}
	return LoadFrom(path)
}

func LoadFrom(path string) (Config, string) {
	cfg := Defaults()

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, ""
		}
		return cfg, fmt.Sprintf("config: %v", err)
	}

	var fc fileConfig
	if _, err := toml.Decode(string(raw), &fc); err != nil {
		return cfg, fmt.Sprintf("config: %v", err)
	}
	// Warnings accumulate because a config can be fine almost everywhere and still have one dead key.
	var warns []string

	if fc.Marker != nil && *fc.Marker != "" {
		cfg.Marker = *fc.Marker
	}
	if fc.Roots != nil {
		cfg.Roots = expandAll(fc.Roots)
	}
	if fc.Exclude != nil {
		cfg.Exclude = fc.Exclude
	}
	if fc.Editor != nil && *fc.Editor != "" {
		cfg.Editor = *fc.Editor
	}
	if fc.SyncBranch != nil && *fc.SyncBranch != "" {
		cfg.SyncBranch = *fc.SyncBranch
		cfg.SyncBranchExplicit = true
	}
	if fc.Fetch != nil {
		if fc.Fetch.Auto != nil {
			cfg.FetchAuto = *fc.Fetch.Auto
		}
		if fc.Fetch.Concurrency != nil {
			if c := *fc.Fetch.Concurrency; c >= 1 {
				cfg.FetchConcurrency = c
			}
		}
		if fc.Fetch.Timeout != nil {
			if d, err := time.ParseDuration(*fc.Fetch.Timeout); err == nil && d > 0 {
				cfg.FetchTimeout = d
			}
		}
	}
	// Unknown actions are ignored but warned about: without the warning a stale `detail = "enter"` looks like a dead TUI key.
	var stale []string
	for k, v := range fc.Keybindings {
		if _, known := DefaultKeybindings()[k]; !known {
			stale = append(stale, k)
			continue
		}
		if v != "" {
			cfg.Keybindings[k] = v
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		warns = append(warns, "config: ignored keybindings, unknown action: "+strings.Join(stale, ", "))
	}
	for k, v := range fc.Commands {
		if v != "" {
			cfg.Commands[k] = v
		}
	}
	// Merged key by key and with no list of valid actions on purpose: the namespace is open, so an unknown one is simply never used.
	for k, v := range fc.AI {
		if v.Command != "" {
			cfg.AICommands[k] = v.Command
		}
	}
	// An unsupported provider warns instead of being accepted silently, otherwise its hosts resolve to a forge with no door and every PR fails with a cause that does not point at the config.
	var unknownForges []string
	for name, f := range fc.Forge {
		if !supportedForge(name) {
			unknownForges = append(unknownForges, name)
			continue
		}
		cfg.addForge(name, f)
	}
	if len(unknownForges) > 0 {
		sort.Strings(unknownForges)
		warns = append(warns, "config: unsupported forge, ignored: "+strings.Join(unknownForges, ", "))
	}
	return cfg, strings.Join(warns, "; ")
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, FileName), nil
}

// Navigation and universal keys (arrows, home, end, enter, esc, tab, ctrl+c) are not configurable.
func DefaultKeybindings() Keybindings {
	return Keybindings{
		"quit":      "q",
		"dirty":     "d",
		"search":    "/",
		"fetch":     "f",
		"fetch_all": "F",
		"pull":      "p",
		"push":      "P",
		"editor":    "e",
		"lazygit":   "g",
		"rescan":    "r",
		"recollect": "R",
		// enter is the only fold key: it folds the worktrees under the cursor and the group blocks, since the repo card has its own section and nothing needs a detail view.
		"fold":            "enter",
		"command":         "!",
		"log":             "l",
		"worktree_remove": "D",
		"visual":          "v",
		"pr":              "O",
	}
}

// `pull` carries no flags on purpose: flags on the command line override the gitconfig, and the user's reconciliation policy must win.
func DefaultCommands() Commands {
	return Commands{
		"pull":        "pull",
		"pull_rebase": "pull --rebase --autostash",
		"pull_ff":     "pull --ff-only",
		"pull_merge":  "pull --no-rebase",
		"push":        "push",
		"fetch":       "fetch --prune",
	}
}

func Defaults() Config {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return Config{
		Marker:           DefaultMarker,
		Roots:            expandAll([]string{"~/dev"}),
		Exclude:          append([]string(nil), DefaultExclude...),
		Editor:           editor,
		SyncBranch:       "main",
		FetchAuto:        true,
		FetchConcurrency: 4,
		FetchTimeout:     30 * time.Second,
		Keybindings:      DefaultKeybindings(),
		Commands:         DefaultCommands(),
		// No default AI binary: the feature is opt-in, so without `command` the AI action can only warn.
		AICommands: map[string]string{},
		Forges:     DefaultForges(),
	}
}

func expandAll(in []string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return in
	}
	out := make([]string, 0, len(in))
	for _, p := range in {
		if len(p) >= 2 && p[0] == '~' && (p[1] == '/' || p[1] == filepath.Separator) {
			p = filepath.Join(home, p[2:])
		}
		out = append(out, p)
	}
	return out
}

func (c Config) KeyFor(action string) string {
	if k, ok := c.Keybindings[action]; ok {
		return k
	}
	return DefaultKeybindings()[action]
}

func (c Config) CmdArgs(action string) []string {
	raw, ok := c.Commands[action]
	if !ok {
		raw = DefaultCommands()[action]
	}
	return strings.Fields(raw)
}

func (c Config) AICommand(action string) string {
	return c.AICommands[action]
}

func (c Config) KeyByAction() map[string]string {
	inv := make(map[string]string, len(c.Keybindings))
	for action, key := range c.Keybindings {
		inv[key] = action
	}
	return inv
}

// Labels carry the action only, never the key (HintBarLines prepends it), or a rebind would render hints like "w enter fold".
var hintLabels = map[string]string{
	"up":              "↑/k",
	"down":            "↓/j",
	"dirty":           "dirty",
	"search":          "filter",
	"fetch":           "fetch",
	"fetch_all":       "fetch all",
	"pull":            "pull ▸",
	"push":            "push",
	"lazygit":         "lazygit",
	"editor":          "edit",
	"rescan":          "rescan",
	"recollect":       "recollect",
	"fold":            "fold",
	"command":         "cmd",
	"log":             "log",
	"quit":            "quit",
	"worktree_remove": "remove wt",
	"visual":          "visual",
	"pr":              "open PR",
}

// Lives outside HintBarLines so a test can require a label per action: an unlabelled one would render as a bare key ("p "), indistinguishable from a render bug.
var hintActions = []string{
	"dirty", "search", "fetch", "fetch_all", "pull",
	"push", "lazygit", "editor", "rescan", "recollect",
	"fold", "command", "log", "quit", "worktree_remove", "visual",
	"pr",
}

func (c Config) HintBarLines() []string {
	row1 := []string{"j/k move"}
	row2 := []string{}
	row3 := []string{}

	for _, action := range hintActions {
		key, ok := c.Keybindings[action]
		if !ok {
			continue
		}
		hint := key + " " + hintLabels[action]

		switch action {
		case "dirty", "search", "fold", "command":
			row1 = append(row1, hint)
		case "fetch", "fetch_all", "pull", "push":
			row2 = append(row2, hint)
		default:
			row3 = append(row3, hint)
		}
	}

	return []string{
		strings.Join(row1, " · "),
		strings.Join(row2, " · "),
		strings.Join(row3, " · "),
	}
}
