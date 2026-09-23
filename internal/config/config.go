// Package config carga la configuración XDG de gitdash (spec 0001 R1).
//
// El fichero ~/.config/gitdash/config.toml es opcional: cualquier clave que
// falte conserva su default. Una config malformada degrada a defaults con
// warning, sin abortar el arranque (S1.3).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// FileName es el nombre del fichero de config dentro del directorio XDG.
const FileName = "config.toml"

// DirName es el subdirectorio de gitdash bajo $XDG_CONFIG_HOME.
const DirName = "gitdash"

// DefaultMarker es el marcador por defecto que identifica un proyecto.
const DefaultMarker = ".gitdash.toml"

// DefaultExclude son los nombres de directorio podados durante el walk
// cuando la config no define exclusiones propias.
var DefaultExclude = []string{
	"node_modules", "target", "vendor", "dist", "build", "out", "coverage",
	".venv", "__pycache__", ".gradle", ".terraform",
}

// Keybindings mapea nombre de acción → tecla (una sola rune o nombre
// especial como "enter", "esc", "tab", "home", "end", "ctrl+c").
type Keybindings map[string]string

// Commands mapea nombre de acción → comando git tal cual se pasa a
// exec.Command (p. ej. "pull --rebase --autostash").
type Commands map[string]string

// Config es la configuración resuelta de gitdash.
type Config struct {
	Marker  string
	Roots   []string
	Exclude []string
	Editor  string
	// SyncBranch es la rama de referencia global para la columna SYNC
	// (spec 0002 R14): los marcadores pueden overridden por repo.
	SyncBranch       string
	FetchAuto        bool
	FetchConcurrency int
	FetchTimeout     time.Duration
	Keybindings      Keybindings
	Commands         Commands
}

// fetchConfig refleja la sección [fetch] del TOML, con punteros para
// distinguir "ausente" (conservar default) de "valor cero" (p. ej. auto=false).
type fetchConfig struct {
	Auto        *bool   `toml:"auto"`
	Concurrency *int    `toml:"concurrency"`
	Timeout     *string `toml:"timeout"`
}

// fileConfig refleja el TOML crudo del disco.
type fileConfig struct {
	Marker     *string             `toml:"marker"`
	Roots      []string            `toml:"roots"`
	Exclude    []string            `toml:"exclude"`
	Editor     *string             `toml:"editor"`
	SyncBranch *string             `toml:"sync_branch"`
	Fetch      *fetchConfig        `toml:"fetch"`
	Keybindings map[string]string  `toml:"keybindings"`
	Commands    map[string]string   `toml:"commands"`
}

// Load lee la config del path estándar XDG. Devuelve la config resuelta y
// un warning (posiblemente nil) para notificar en la UI. Nunca falla:
// ante cualquier error usa defaults (S1.3).
func Load() (Config, string) {
	path, err := Path()
	if err != nil {
		return Defaults(), ""
	}
	return LoadFrom(path)
}

// LoadFrom resuelve la config desde un fichero concreto (testeable).
func LoadFrom(path string) (Config, string) {
	cfg := Defaults()

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, "" // S1.1: sin fichero, defaults silenciosos
		}
		return cfg, fmt.Sprintf("config: %v", err)
	}

	var fc fileConfig
	if _, err := toml.Decode(string(raw), &fc); err != nil {
		return cfg, fmt.Sprintf("config: %v", err) // S1.3
	}

	if fc.Marker != nil && *fc.Marker != "" {
		cfg.Marker = *fc.Marker
	}
	if fc.Roots != nil {
		cfg.Roots = expandAll(fc.Roots) // S1.4: sustituye, no añade
	}
	if fc.Exclude != nil {
		cfg.Exclude = fc.Exclude
	}
	if fc.Editor != nil && *fc.Editor != "" {
		cfg.Editor = *fc.Editor
	}
	if fc.SyncBranch != nil && *fc.SyncBranch != "" {
		cfg.SyncBranch = *fc.SyncBranch // R14: override global
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
	// Keybindings: merge sobre defaults (el usuario solo sobreescribe lo que cambia).
	for k, v := range fc.Keybindings {
		if v != "" {
			cfg.Keybindings[k] = v
		}
	}
	// Commands: merge sobre defaults.
	for k, v := range fc.Commands {
		if v != "" {
			cfg.Commands[k] = v
		}
	}
	return cfg, ""
}

// Path devuelve la ruta del fichero de config respetando $XDG_CONFIG_HOME.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, FileName), nil
}

// DefaultKeybindings devuelve el mapa de teclas por defecto.
// Las teclas de navegación (arrows, home, end) y especiales (enter, esc,
// tab, ctrl+c) son universales y no se configuran aquí.
func DefaultKeybindings() Keybindings {
	return Keybindings{
		"quit":      "q",
		"dirty":     "d",
		"search":    "/",
		"fetch":     "f",
		"fetch_all": "F",
		"sync":      "s",
		"pull":      "p",
		"push":      "P",
		"editor":    "e",
		"lazygit":   "g",
		"rescan":    "r",
		"recollect": "R",
		"fold":      "tab",
		"detail":    "enter",
		"command":   "!",
		"update":    "u",
		"expand":    "space", // 0006 R37.1: toggle de expansión de worktrees
	}
}

// DefaultCommands devuelve los comandos git por defecto.
func DefaultCommands() Commands {
	return Commands{
		"pull":  "pull --ff-only",
		"push":  "push",
		"sync":  "pull --rebase --autostash",
		"fetch": "fetch --prune",
	}
}

// Defaults construye la config por defecto (R1).
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
		SyncBranch:       "main", // R14: default global de la sync branch
		FetchAuto:        true,
		FetchConcurrency: 4,
		FetchTimeout:     30 * time.Second,
		Keybindings:      DefaultKeybindings(),
		Commands:         DefaultCommands(),
	}
}

// expandAll expande `~/` a home en cada entrada.
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

// KeyFor devuelve la tecla configurada para una acción, o el default si
// no está en el mapa.
func (c Config) KeyFor(action string) string {
	if k, ok := c.Keybindings[action]; ok {
		return k
	}
	return DefaultKeybindings()[action]
}

// CmdArgs devuelve los argumentos del comando git para una acción,
// separados por espacios. Ej: "pull --rebase --autostash" →
// ["pull", "--rebase", "--autostash"].
func (c Config) CmdArgs(action string) []string {
	raw, ok := c.Commands[action]
	if !ok {
		raw = DefaultCommands()[action]
	}
	return strings.Fields(raw)
}

// KeyByAction devuelve un mapa invertido tecla → acción para el
// procesamiento de input en la TUI.
func (c Config) KeyByAction() map[string]string {
	inv := make(map[string]string, len(c.Keybindings))
	for action, key := range c.Keybindings {
		inv[key] = action
	}
	return inv
}

// HintLabel devuelve la etiqueta corta para la barra de hints.
// Acciones internas como "quit" no aparecen (ya están hardcodeadas
// en la UI o son universales).
var hintLabels = map[string]string{
	"up":        "↑/k",
	"down":      "↓/j",
	"dirty":     "d dirty",
	"search":    "/ filter",
	"fetch":     "f fetch",
	"fetch_all": "F fetch all",
	"sync":      "s sync",
	"pull":      "p pull",
	"push":      "P push",
	"lazygit":   "g lazygit",
	"editor":    "e edit",
	"rescan":    "r rescan",
	"recollect": "R recollect",
	"fold":      "tab fold",
	"detail":    "enter detail",
	"command":   "! cmd",
	"update":    "u update",
	"expand":    "expand",
	"quit":      "q quit",
}

// HintBarLines devuelve las líneas de hints agrupadas por categoría,
// derivada de los keybindings configurados. Cada línea es un string
// con los hints separados por " · ".
func (c Config) HintBarLines() []string {
	row1 := []string{"j/k move"} // navegación + vista
	row2 := []string{}           // acciones git
	row3 := []string{}           // tools

	for _, action := range []string{
		"dirty", "search", "fetch", "fetch_all", "sync",
		"pull", "push", "lazygit", "update", "editor", "rescan", "recollect",
		"fold", "expand", "detail", "command", "quit",
	} {
		key, ok := c.Keybindings[action]
		if !ok {
			continue
		}
		label, ok := hintLabels[action]
		if !ok {
			label = key
		}
		hint := key + " " + strings.TrimPrefix(label, key+" ")

		switch action {
		case "dirty", "search", "fold", "expand", "detail", "command":
			row1 = append(row1, hint)
		case "fetch", "fetch_all", "sync", "pull", "push":
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
