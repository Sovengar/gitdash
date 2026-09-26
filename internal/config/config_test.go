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

func TestDefaults(t *testing.T) {
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
	found := false
	for _, ex := range cfg.Exclude {
		if ex == "testdata" {
			found = true
		}
	}
	if !found {
		t.Errorf("exclude default sin testdata: %v", cfg.Exclude)
	}
}

func TestPartialOverride(t *testing.T) {
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

func TestMalformed(t *testing.T) {
	path := write(t, `roots = [`)
	cfg, warn := LoadFrom(path)
	if warn == "" {
		t.Fatal("se esperaba warning de parseo")
	}
	if cfg.Marker != ".gitdash.toml" || !cfg.FetchAuto {
		t.Errorf("no se usaron defaults: %+v", cfg)
	}
}

func TestMissingFile(t *testing.T) {
	cfg, warn := LoadFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.Marker != ".gitdash.toml" || len(cfg.Roots) != 1 {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestExpansion(t *testing.T) {
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

// El plegado tiene default `enter` (cubre worktrees y grupos) y es configurable
// como el resto de keybindings. Su hint lleva la tecla configurada y solo una vez.
func TestFoldKeybinding(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("fold") != "enter" {
		t.Errorf("default fold = %q, want enter", cfg.KeyFor("fold"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "enter fold") {
		t.Errorf("hint de plegado ausente: %v", cfg.HintBarLines())
	}

	// Rebind via config.toml.
	path := write(t, `
[keybindings]
fold = "w"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("fold") != "w" {
		t.Errorf("fold = %q, want w", cfg.KeyFor("fold"))
	}
	hints := strings.Join(cfg.HintBarLines(), "\n")
	if !strings.Contains(hints, "w fold") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
	if strings.Contains(hints, "enter fold") {
		t.Errorf("el hint sigue con la tecla anterior: %v", cfg.HintBarLines())
	}
}

// El borrado de worktree tiene default `D` y es reconfigurable.
func TestDefaultKeybindingsWorktreeRemove(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("worktree_remove") != "D" {
		t.Errorf("default worktree_remove = %q, want D", cfg.KeyFor("worktree_remove"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "D remove wt") {
		t.Errorf("hint de borrado ausente: %v", cfg.HintBarLines())
	}

	// Rebind via config.toml.
	path := write(t, `
[keybindings]
worktree_remove = "W"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("worktree_remove") != "W" {
		t.Errorf("worktree_remove = %q, want W", cfg.KeyFor("worktree_remove"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "W remove wt") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
}

// Una config vieja con acciones que ya no existen (detail, expand) avisa en vez
// de dejar la tecla muerta: sin aviso, `detail = "enter"` hace que enter no haga
// nada y parece un bug de la TUI.
func TestKeybindingsObsoletosAvisan(t *testing.T) {
	path := write(t, `
[keybindings]
detail = "enter"
expand = "space"
fold   = "w"
`)
	cfg, warn := LoadFrom(path)
	if !strings.Contains(warn, "detail") || !strings.Contains(warn, "expand") {
		t.Errorf("aviso sin las acciones obsoletas: %q", warn)
	}
	if strings.Contains(warn, "fold") {
		t.Errorf("avisa de una acción válida: %q", warn)
	}
	// Las obsoletas no se cuelan en el mapa (su tecla queda libre) y las
	// válidas sí se aplican.
	if _, ok := cfg.Keybindings["detail"]; ok {
		t.Error("la acción obsoleta quedó en el mapa de teclas")
	}
	if cfg.KeyFor("fold") != "w" {
		t.Errorf("fold = %q, want w", cfg.KeyFor("fold"))
	}
}
