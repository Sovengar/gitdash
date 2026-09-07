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
	"time"

	"github.com/BurntSushi/toml"
)

// FileName es el nombre del fichero de config dentro del directorio XDG.
const FileName = "config.toml"

// DirName es el subdirectorio de gitdash bajo $XDG_CONFIG_HOME.
const DirName = "gitdash"

// DefaultMarker es el marcador por defecto que identifica un proyecto.
const DefaultMarker = ".repo.toml"

// DefaultExclude son los nombres de directorio podados durante el walk
// cuando la config no define exclusiones propias.
var DefaultExclude = []string{
	"node_modules", "target", "vendor", "dist", "build", "out", "coverage",
	".venv", "__pycache__", ".gradle", ".terraform",
}

// Config es la configuración resuelta de gitdash.
type Config struct {
	Marker           string
	Roots            []string
	Exclude          []string
	Editor           string
	FetchAuto        bool
	FetchConcurrency int
	FetchTimeout     time.Duration
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
	Marker  *string      `toml:"marker"`
	Roots   []string     `toml:"roots"`
	Exclude []string     `toml:"exclude"`
	Editor  *string      `toml:"editor"`
	Fetch   *fetchConfig `toml:"fetch"`
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
		FetchAuto:        true,
		FetchConcurrency: 4,
		FetchTimeout:     30 * time.Second,
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
