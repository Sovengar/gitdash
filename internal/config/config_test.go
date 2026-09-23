package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultsS1_1(t *testing.T) {
	cfg := Defaults()
	if cfg.Marker != ".gitdash.toml" {
		t.Errorf("marker = %q", cfg.Marker)
	}
	if len(cfg.Roots) != 1 || filepath.Base(cfg.Roots[0]) != "dev" {
		t.Errorf("roots = %v", cfg.Roots)
	}
	if !cfg.FetchAuto || cfg.FetchConcurrency != 4 || cfg.FetchTimeout != 30*time.Second {
		t.Errorf("fetch defaults = %+v", cfg)
	}
	if cfg.Editor == "" {
		t.Error("editor default vacío")
	}
}

func TestPartialOverrideS1_2(t *testing.T) {
	path := write(t, `roots = ["~/code", "~/work"]`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if len(cfg.Roots) != 2 || filepath.Base(cfg.Roots[0]) != "code" {
		t.Errorf("roots = %v", cfg.Roots)
	}
	if cfg.Marker != ".gitdash.toml" || !cfg.FetchAuto || cfg.FetchConcurrency != 4 {
		t.Errorf("defaults no conservados: %+v", cfg)
	}
}

func TestMalformedS1_3(t *testing.T) {
	path := write(t, `roots = [`)
	cfg, warn := LoadFrom(path)
	if warn == "" {
		t.Fatal("se esperaba warning de parseo")
	}
	if cfg.Marker != ".gitdash.toml" || !cfg.FetchAuto {
		t.Errorf("no se usaron defaults: %+v", cfg)
	}
}

func TestMissingFileS1_1(t *testing.T) {
	cfg, warn := LoadFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.Marker != ".gitdash.toml" || len(cfg.Roots) != 1 {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestExpansionS1_4(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := write(t, "roots = [\"~/dev\"]\n")
	cfg, _ := LoadFrom(path)
	if cfg.Roots[0] != filepath.Join(home, "dev") {
		t.Errorf("root = %q, want %q", cfg.Roots[0], filepath.Join(home, "dev"))
	}
}

func TestFetchOverride(t *testing.T) {
	path := write(t, `
[fetch]
auto = false
concurrency = 8
timeout = "10s"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.FetchAuto {
		t.Error("auto debería ser false")
	}
	if cfg.FetchConcurrency != 8 || cfg.FetchTimeout != 10*time.Second {
		t.Errorf("fetch = %+v", cfg)
	}
}

func TestInvalidValuesIgnored(t *testing.T) {
	path := write(t, `
[fetch]
concurrency = 0
timeout = "nope"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.FetchConcurrency != 4 || cfg.FetchTimeout != 30*time.Second {
		t.Errorf("valores inválidos no ignorados: %+v", cfg)
	}
}

// 0006 R37.1: la expansión de worktrees tiene default `space` y es
// configurable como el resto de keybindings.
func TestExpandKeybindingR37(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("expand") != "space" {
		t.Errorf("R37.1: default expand = %q, want space", cfg.KeyFor("expand"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "space expand") {
		t.Errorf("R37.3: hint de expansión ausente: %v", cfg.HintBarLines())
	}

	// R37.2: rebind via config.toml.
	path := write(t, `
[keybindings]
expand = "w"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("expand") != "w" {
		t.Errorf("R37.2: expand = %q, want w", cfg.KeyFor("expand"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "w expand") {
		t.Errorf("R37.3: hint rebindeado ausente: %v", cfg.HintBarLines())
	}
}
