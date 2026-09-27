// Tests del selector de preview visual (git-sim): la tecla `v` arma, la segunda
// tecla elige pull/merge/rebase, y cualquier otra cancela. Estilo del repo:
// Model directo + Update, sin teatest.
package tui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

// withVisualEnv prepara un entorno determinista para los tests que llegan a
// lanzar el handoff: un `git-sim` falso en PATH (nunca se ejecuta: solo se
// comprueba el Cmd devuelto) y una caché XDG aislada donde crear el media-dir.
func withVisualEnv(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	exe := filepath.Join(bin, "git-sim")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// El fake va primero para que gane al git-sim real de la máquina; el PATH
	// original se conserva porque los fixtures necesitan `git`.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

// La tecla `v` solo arma: captura path y upstream de la fila y no lanza nada.
func TestVisualArmaSinEjecutar(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	before := len(m.running)

	m, _ = press(m, "v")

	if m.visualArmed == nil {
		t.Fatal("v no armó el selector visual")
	}
	if m.visualArmed.path != "/tmp/old-clean" {
		t.Errorf("path armado = %q, want /tmp/old-clean", m.visualArmed.path)
	}
	if m.visualArmed.upstream != "origin/main" {
		t.Errorf("upstream armado = %q, want origin/main", m.visualArmed.upstream)
	}
	if len(m.running) != before {
		t.Errorf("v lanzó algo: running = %v, want sin cambios", m.running)
	}
}

// Sin fila bajo el cursor no hay nada que previsualizar: toast y sin armado.
func TestVisualNoArmaSinFila(t *testing.T) {
	m := newTestModel(t, nil, map[string]gitstatus.Snapshot{})
	_, cmd := press(m, "v")
	if m.visualArmed != nil {
		t.Errorf("se armó el selector sin fila: %+v", m.visualArmed)
	}
	if cmd == nil {
		t.Fatal("sin fila debería avisar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
		t.Errorf("notificación = %v", cmd())
	}
}

// Sobre una fila sin repo git tampoco se arma.
func TestVisualNoArmaSinRepo(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/no-repo-docs")
	_, cmd := press(m, "v")
	if m.visualArmed != nil {
		t.Errorf("se armó el selector sobre una fila sin repo: %+v", m.visualArmed)
	}
	if cmd == nil {
		t.Fatal("sin repo debería avisar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
		t.Errorf("notificación = %v", cmd())
	}
}

// Una tecla que no es variante desarma y sigue su curso normal: si se comiera,
// la app quedaría pegada esperando una segunda pulsación que nunca llega.
func TestVisualTeclaNoVarianteCancelaYSigue(t *testing.T) {
	m := newPullModel(t)
	start := m.cursor
	m, _ = press(m, "v")
	if m.visualArmed == nil {
		t.Fatal("precondición: no se armó")
	}
	m, _ = press(m, "j")
	if m.visualArmed != nil {
		t.Error("el selector sigue armado tras una tecla no-variante")
	}
	if m.cursor == start {
		t.Error("la tecla no-variante no ejecutó su acción (cursor quieto)")
	}
	if len(m.running) != 0 {
		t.Errorf("la cancelación lanzó algo: %v", m.running)
	}
}

// esc desarma sin lanzar nada.
func TestVisualEscCancela(t *testing.T) {
	m := newPullModel(t)
	m, _ = press(m, "v")
	m, _ = press(m, "esc")
	if m.visualArmed != nil {
		t.Error("esc no canceló el selector")
	}
	if len(m.running) != 0 {
		t.Errorf("esc lanzó algo: %v", m.running)
	}
}

// El armado fija el repo objetivo: aunque el cursor cambie, la variante resuelve
// sobre el path y upstream capturados al armar.
func TestVisualResuelveSobreElArmado(t *testing.T) {
	withVisualEnv(t)
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")
	// Mover el cursor por detrás (p. ej. un rescan que reordena filas) no debe
	// redirigir la variante.
	m = cursorOn(t, m, "/tmp/dirty-api")

	m, cmd := press(m, "p")
	if cmd == nil {
		t.Fatal("la variante no lanzó nada")
	}
	if m.running["/tmp/old-clean"] != "visual" {
		t.Errorf("running = %q, want visual en /tmp/old-clean", m.running["/tmp/old-clean"])
	}
	if _, ok := m.running["/tmp/dirty-api"]; ok {
		t.Error("la variante resolvió sobre la fila bajo el cursor, no sobre la armada")
	}
}

// Cada variante lanza el handoff, marca la acción en curso y desarma el
// selector.
func TestVisualDespachaCadaVariante(t *testing.T) {
	for _, key := range []string{"p", "m", "r"} {
		withVisualEnv(t)
		m := newPullModel(t)
		m = cursorOn(t, m, "/tmp/old-clean")
		m, _ = press(m, "v")

		m, cmd := press(m, key)
		if m.visualArmed != nil {
			t.Errorf("v%s dejó el selector armado", key)
		}
		if m.running["/tmp/old-clean"] != "visual" {
			t.Errorf("v%s → running = %q, want visual", key, m.running["/tmp/old-clean"])
		}
		if cmd == nil {
			t.Errorf("v%s no devolvió tea.Cmd", key)
		}
	}
}

// TestVisualArgvVariantes fija el argv exacto por variante: `pull` sin arg
// posicional; `merge`/`rebase` con el ref del upstream.
func TestVisualArgvVariantes(t *testing.T) {
	dir := filepath.Join("/cache", "gitdash", "git-sim")
	cases := []struct {
		sub      string
		upstream string
		want     []string
	}{
		{"pull", "origin/main", []string{"git-sim", "--media-dir", dir, "pull"}},
		{"merge", "origin/main", []string{"git-sim", "--media-dir", dir, "merge", "origin/main"}},
		{"rebase", "origin/main", []string{"git-sim", "--media-dir", dir, "rebase", "origin/main"}},
	}
	for _, c := range cases {
		got := visualArgv(c.sub, c.upstream, dir)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("visualArgv(%q) = %#v, want %#v", c.sub, got, c.want)
		}
	}
}

// El auto-open de git-sim queda activo: ni `-d` ni `--animate` en ninguna
// variante.
func TestVisualArgvSinFlagsDePreview(r *testing.T) {
	dir := "/cache/gitdash/git-sim"
	for _, o := range visualOptions {
		for _, arg := range visualArgv(o.sub, "origin/main", dir) {
			if arg == "-d" || arg == "--animate" || arg == "--output-only-path" {
				r.Errorf("la variante %q incluye %q en el argv", o.sub, arg)
			}
		}
	}
}

// El media-dir sale de la caché XDG de gitdash (nunca del repo) y se crea si
// falta.
func TestVisualMediaDirBajoCacheYSeCrea(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	dir, err := visualMediaDir()
	if err != nil {
		t.Fatalf("visualMediaDir: %v", err)
	}
	want := filepath.Join(cacheRoot, "gitdash", "git-sim")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("el media-dir no se creó: err=%v", err)
	}
}

// Sin upstream, merge y rebase avisan y no lanzan nada; pull sí se permite.
func TestVisualSinUpstream(t *testing.T) {
	for _, key := range []string{"m", "r"} {
		withVisualEnv(t)
		m := newPullModel(t)
		m = cursorOn(t, m, "/tmp/no-up-cli")
		m, _ = press(m, "v")
		if m.visualArmed == nil || m.visualArmed.upstream != "" {
			t.Fatalf("precondición: upstream armado = %+v", m.visualArmed)
		}
		m, cmd := press(m, key)
		if m.visualArmed != nil {
			t.Errorf("%s dejó el selector armado", key)
		}
		if _, busy := m.running["/tmp/no-up-cli"]; busy {
			t.Errorf("%s lanzó sin upstream: running=%v", key, m.running)
		}
		if cmd == nil {
			t.Fatalf("%s sin upstream debería avisar", key)
		}
		if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no upstream") {
			t.Errorf("%s notificación = %v", key, cmd())
		}
	}

	// pull no exige ref explícito: se lanza igual sin upstream.
	withVisualEnv(t)
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/no-up-cli")
	m, _ = press(m, "v")
	m, cmd := press(m, "p")
	if cmd == nil || m.running["/tmp/no-up-cli"] != "visual" {
		t.Errorf("p sin upstream no lanzó: running=%v", m.running)
	}
}

// Sin el binario en PATH solo hay toast, sin handoff.
func TestVisualSinBinario(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // sin git-sim
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")

	m, cmd := press(m, "p")
	if _, busy := m.running["/tmp/old-clean"]; busy {
		t.Errorf("lanzó el handoff sin binario: running=%v", m.running)
	}
	if cmd == nil {
		t.Fatal("sin binario debería avisar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "git-sim not installed") {
		t.Errorf("notificación = %v", cmd())
	}
}

// Si el media-dir no se puede crear, se aborta con toast: lanzar igualmente
// ensuciaría el repo con `git-sim_media/`.
func TestVisualMediaDirNoCreable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", filepath.Join(blocker, "sub"))

	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")

	m, cmd := press(m, "p")
	if _, busy := m.running["/tmp/old-clean"]; busy {
		t.Errorf("lanzó el handoff con media-dir no creable: running=%v", m.running)
	}
	if cmd == nil {
		t.Fatal("media-dir no creable debería avisar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "media dir") {
		t.Errorf("notificación = %v", cmd())
	}
}

// Con una acción ya en curso en el repo, no se relanza.
func TestVisualBloqueadoSiYaCorre(t *testing.T) {
	withVisualEnv(t)
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m.running["/tmp/old-clean"] = "lazygit"
	m, _ = press(m, "v")

	m, cmd := press(m, "p")
	if m.running["/tmp/old-clean"] != "lazygit" {
		t.Errorf("running = %q, want el original intacto", m.running["/tmp/old-clean"])
	}
	if cmd == nil {
		t.Fatal("con acción en curso debería avisar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already running") {
		t.Errorf("notificación = %v", cmd())
	}
}

// El aviso del selector sale por el punto único de keybinds y sustituye las
// hints (una sola línea).
func TestVisualPromptYKeybinds(t *testing.T) {
	m := newPullModel(t)
	if got := m.promptLine(); got != "" {
		t.Errorf("promptLine sin armado = %q, want vacío", got)
	}
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")

	got := m.promptLine()
	for _, want := range []string{"p pull", "m merge", "r rebase", "esc cancel", "old-clean"} {
		if !strings.Contains(got, want) {
			t.Errorf("el aviso no menciona %q:\n%s", want, got)
		}
	}
	if m.keybindsLines() != 1 {
		t.Errorf("keybindsLines = %d, want 1 (el aviso sustituye las hints)", m.keybindsLines())
	}
	out := stripANSI(m.View().Content)
	if kb := sectionContent(t, out, "keybinds"); !strings.Contains(kb, "merge") {
		t.Errorf("el aviso no está en keybinds:\n%s", kb)
	}
}

// El panel del log y el selector visual no conviven: abrirlo suelta el armado, y
// con el panel abierto `v` no rearma (su aviso taparía la leyenda del panel).
func TestVisualYPanelDelLog(t *testing.T) {
	m := newPullModel(t)
	m.visualArmed = &armedVisual{path: "/tmp/old-clean"}
	m.toggleLog()
	if m.visualArmed != nil {
		t.Error("abrir el panel no soltó el selector visual")
	}

	m2 := newPullModel(t)
	m2.logOpen = true
	m2, _ = press(m2, "v")
	if m2.visualArmed != nil {
		t.Error("v armó el selector con el panel del log abierto")
	}
}

// La vuelta del handoff registra el exec con el argv real (media-dir + ref) y
// Dur=0, como los demás handoffs.
func TestVisualExecDoneRegistra(t *testing.T) {
	m, rec := logModel(t)
	path := "/tmp/old-clean"
	argv := []string{"git-sim", "--media-dir", "/cache/gitdash/git-sim", "merge", "origin/main"}

	updated, _ := m.Update(execDoneMsg{path: path, action: "visual", argv: argv, err: nil})
	_ = updated.(Model)

	var got *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent || e.Action != "visual" {
			continue
		}
		cp := e
		got = &cp
	}
	if got == nil {
		t.Fatal("no se registró el exec de visual")
	}
	if !reflect.DeepEqual(got.Argv, argv) {
		t.Errorf("argv = %#v, want %#v", got.Argv, argv)
	}
	if got.Dur != 0 {
		t.Errorf("Dur = %v, want 0 (handoff)", got.Dur)
	}
	if got.Exit != 0 {
		t.Errorf("Exit = %d, want 0", got.Exit)
	}
	if got.Class != cmdlog.ClassAction || got.Dir != path {
		t.Errorf("clase/dir = %v/%q", got.Class, got.Dir)
	}
}

// Elegir la variante deja la intención (tecla + acción + repo) al margen del
// exec, que solo llega al volver del handoff.
func TestVisualIntencionRegistrada(t *testing.T) {
	withVisualEnv(t)
	m, rec := logModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")
	_, _ = press(m, "m")

	var variantIntent *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent && e.Key == "m" {
			cp := e
			variantIntent = &cp
		}
	}
	if variantIntent == nil {
		t.Fatal("no se registró la intención de la variante")
	}
	if variantIntent.Action != "visual" || variantIntent.Repo != "old-clean" {
		t.Errorf("intención = %+v", variantIntent)
	}
}

// El selector visual no consulta ni bloquea por un rebase a medias: git-sim no
// muta el repo real, así que previsualizar un rebase a medio resolver es útil.
func TestVisualNoBloqueaPorRebaseEnCurso(t *testing.T) {
	withVisualEnv(t)
	dir, _ := testutil.NewRepo(t, true) // con upstream origin/main
	if err := os.MkdirAll(filepath.Join(dir, ".git", "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !gitstatus.RebaseInProgress(context.Background(), dir) {
		t.Fatal("precondición: el repo no reporta rebase en curso")
	}

	p := proj("demo", dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapClean()})
	m = cursorOn(t, m, dir)
	m, _ = press(m, "v")
	m, cmd := press(m, "m")
	if cmd == nil || m.running[dir] != "visual" {
		t.Errorf("el rebase en curso bloqueó el preview: running=%v cmd=%v", m.running, cmd != nil)
	}
}
