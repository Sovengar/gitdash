// Tests del contrato de config para la familia de pull: la política por
// defecto NO lleva flags (vive en el gitconfig) y las variantes del selector
// sí los llevan.
package config

import (
	"strings"
	"testing"
)

// El default de pull va sin flags a propósito. Con `--ff-only` hardcodeado, el
// `pull.rebase` del usuario quedaba pisado: los flags en la línea de comandos
// ganan sobre la config, así que la tecla hacía ff-only siempre.
func TestDefaultPullSinFlags(t *testing.T) {
	got := DefaultCommands()["pull"]
	if got != "pull" {
		t.Errorf("commands.pull = %q, want %q (la política la decide el gitconfig)", got, "pull")
	}
	for _, forbidden := range []string{"--ff-only", "--rebase", "--no-rebase", "--autostash"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("commands.pull = %q, no debe llevar %s", got, forbidden)
		}
	}
}

// Cada variante del selector pisa la política con un flag explícito y
// distinto: sin eso, "rebase" y "default" serían la misma tecla.
func TestVariantesPullConFlagsPropios(t *testing.T) {
	want := map[string][]string{
		"pull":        {},
		"pull_rebase": {"--rebase", "--autostash"},
		"pull_ff":     {"--ff-only"},
		"pull_merge":  {"--no-rebase"},
	}
	cmds := DefaultCommands()
	for action, flags := range want {
		raw, ok := cmds[action]
		if !ok {
			t.Errorf("commands[%s] ausente", action)
			continue
		}
		fields := strings.Fields(raw)
		if fields[0] != "pull" {
			t.Errorf("commands[%s] = %q, want subcomando pull", action, raw)
		}
		if len(fields)-1 != len(flags) {
			t.Errorf("commands[%s] = %q, want flags %v", action, raw, flags)
			continue
		}
		for i, f := range flags {
			if fields[1+i] != f {
				t.Errorf("commands[%s] = %q, want flag %q en %d", action, raw, f, i)
			}
		}
	}
}

// `sync` y `update` desaparecieron: sync era un pull --rebase sin motivo
// propio, y update era un exec de binario sin argumentos que los scripts
// git-update/git-sync necesitan.
func TestAccionesRetiradas(t *testing.T) {
	kb := DefaultKeybindings()
	for _, gone := range []string{"sync", "update"} {
		if _, ok := kb[gone]; ok {
			t.Errorf("keybindings[%s] sigue presente", gone)
		}
	}
	if _, ok := DefaultCommands()["sync"]; ok {
		t.Error("commands.sync sigue presente")
	}
}

// La tecla de pull se mantiene: lo que cambia es que arma el selector.
func TestPullTeclaPreservada(t *testing.T) {
	if got := DefaultKeybindings()["pull"]; got != "p" {
		t.Errorf("keybindings.pull = %q, want %q", got, "p")
	}
}

// El hint bar tiene que insinuar que la tecla abre un selector; si no, `p` es
// una tecla muerta desde la Discoverability.
func TestHintBarAnunciaElSelector(t *testing.T) {
	lines := strings.Join(Defaults().HintBarLines(), "\n")
	if !strings.Contains(lines, "p pull") {
		t.Errorf("el hint no menciona la tecla de pull:\n%s", lines)
	}
	if !strings.Contains(lines, "▸") {
		t.Errorf("el hint no insinúa el selector de variante:\n%s", lines)
	}
}

// CmdArgs resuelve el override del usuario por encima del default, y una
// variante sin default explícito cae en lo que haya en config.
func TestCmdArgsResuelveVariante(t *testing.T) {
	cfg := Defaults()
	if got := cfg.CmdArgs("pull_ff"); strings.Join(got, " ") != "pull --ff-only" {
		t.Errorf("CmdArgs(pull_ff) = %v", got)
	}
	cfg.Commands["pull_rebase"] = "pull --rebase"
	if got := cfg.CmdArgs("pull_rebase"); strings.Join(got, " ") != "pull --rebase" {
		t.Errorf("CmdArgs con override = %v, want el override", got)
	}
}
