// Tests de las acciones con handoff de terminal: tecla g (lazygit) y
// modo comando `!` del detalle via worktrunk.
package tui

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

func TestRunShellCmdCapturaSalidaYExit(t *testing.T) {
	out, code := runShellCmd(t.Context(), t.TempDir(), "/bin/sh", "echo hola && exit 0")
	if code != 0 {
		t.Fatalf("code = %d, quiero 0", code)
	}
	if !strings.Contains(out, "hola") {
		t.Fatalf("output = %q, quiero contener \"hola\"", out)
	}

	out2, code2 := runShellCmd(t.Context(), t.TempDir(), "/bin/sh", "stderr-va-junto 1>&2; exit 3")
	if code2 != 3 {
		t.Fatalf("code = %d, quiero 3", code2)
	}
	if !strings.Contains(out2, "stderr-va-junto") {
		t.Fatalf("stderr no fue capturado: %q", out2)
	}
}

// flujo completo del modo comando: enter abre detalle, ! abre el input,
// teclear, enter lanza el comando en la worktree y marca running "cmd".
func TestBangModoComandoFlujo(t *testing.T) {
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, _ = press(m, "enter") // abre el detalle
	if !m.detailOpen {
		t.Fatal("enter no abrió el detalle")
	}

	m, _ = press(m, "!") // abre el input
	if !m.cmdOpen {
		t.Fatal("! no abrió el modo comando en el detalle")
	}

	m, _ = press(m, "g")
	m, _ = press(m, "i")
	m, _ = press(m, "t")
	if got := m.cmdInput.Value(); got != "git" {
		t.Fatalf("input = %q, quiero \"git\"", got)
	}

	m, _ = press(m, "enter")
	if m.cmdOpen {
		t.Fatal("enter no cerró el modo comando")
	}
	if m.running[p.Path] != "cmd" {
		t.Fatalf("running = %q, quiero \"cmd\"", m.running[p.Path])
	}
}

// esc cancela el input sin lanzar nada; enter con input vacío tampoco.
func TestBangModoComandoCancelar(t *testing.T) {
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, _ = press(m, "enter")
	m, _ = press(m, "!")
	m, _ = press(m, "esc")
	if m.cmdOpen {
		t.Fatal("esc no cerró el modo comando")
	}
	if _, busy := m.running[p.Path]; busy {
		t.Fatal("esc lanzó un comando")
	}

	m, _ = press(m, "!")
	m, cmd3 := press(m, "enter")
	if m.cmdOpen {
		t.Fatal("enter con input vacío no cerró el modo comando")
	}
	// enter vacío abre shell interactiva ($SHELL), si existe
	if _, err := shellPath(); err == nil && m.running[p.Path] != "shell" {
		t.Fatalf("enter vacío debería abrir shell interactiva, running = %q", m.running[p.Path])
	} else if err != nil && cmd3 == nil {
		t.Fatal("sin $SHELL debería notificar")
	}
}

// shellPath replica el fallback de openShellCmd.
func shellPath() (string, error) {
	s := os.Getenv("SHELL")
	if s == "" {
		s = "/bin/sh"
	}
	return exec.LookPath(s)
}

// ! en un repo sin git notifica en vez de abrir el input.
func TestBangSinRepo(t *testing.T) {
	p := proj("bare", "/tmp/gitdash-test/bare", false)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, _ = press(m, "enter")
	m, cmd := press(m, "!")
	if m.cmdOpen {
		t.Fatal("! abrió el input en un repo sin git")
	}
	if cmd == nil {
		t.Fatal("sin repo debería notificar")
	}
}

// tecla g: marca lazygit como running en el repo; sin repo solo notifica.
func TestGLazygit(t *testing.T) {
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	if !hasLazygit() {
		t.Skip("lazygit no instalado")
	}
	m, cmd := press(m, "g")
	if m.running[p.Path] != "lazygit" {
		t.Fatalf("running = %q, quiero \"lazygit\"", m.running[p.Path])
	}
	if cmd == nil {
		t.Fatal("g no devolvió tea.Cmd")
	}

	sin := proj("bare", "/tmp/gitdash-test/bare", false)
	m2 := newTestModel(t, []discovery.Project{sin},
		map[string]gitstatus.Snapshot{sin.Path: snapClean()})
	m2, cmd2 := press(m2, "g")
	if _, busy := m2.running[sin.Path]; busy {
		t.Fatal("g lanzó lazygit en un repo sin git")
	}
	if cmd2 == nil {
		t.Fatal("sin repo debería notificar")
	}
}

func hasLazygit() bool {
	_, err := exec.LookPath("lazygit")
	return err == nil
}

// El cursor del input ! cae sobre el primer rune del placeholder (bubbles
// v2 placeholderView): debe ser un espacio, no una letra que parezca
// tecleada (bug del "! c" fantasma).
func TestBangPlaceholderCursorLimpio(t *testing.T) {
	m := New(config.Defaults())
	m.detailOpen = true
	m.cmdOpen = true
	v := stripANSI(m.cmdInput.View())
	if !strings.HasPrefix(v, "!  ") {
		t.Fatalf("view = %q, quizo empezar por \"!  \" (prompt + espacio del placeholder)", v)
	}
}
