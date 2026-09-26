package gitstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/cmdlog"
	"gitdash/internal/testutil"
)

// installRecorder instala un recorder limpio para el test y lo desactiva al
// terminar. El recorder es global porque los exec de git están en este paquete
// y no en la TUI: enhebrarlo por Collect/StreamPool/Fetch/Run no aportaría nada.
func installRecorder(t *testing.T) *cmdlog.Recorder {
	t.Helper()
	rec := cmdlog.New(cmdlog.DefaultCap)
	cmdlog.SetRecorder(rec)
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	return rec
}

// lastEntry devuelve la última entrada del log.
func lastEntry(t *testing.T, rec *cmdlog.Recorder) cmdlog.Entry {
	t.Helper()
	entries := rec.Entries()
	if len(entries) == 0 {
		t.Fatal("el command log está vacío: el exec no se registró")
	}
	return entries[len(entries)-1]
}

// Un pull con la política del gitconfig del usuario: el argv no dice nada del
// resultado, pero la clasificación sí. Aquí el repo está detrás del upstream y
// sin config de rebase alguna, así que integra con fast-forward.
func TestRunRegistraArgvYResultado(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)

	if _, err := Run(context.Background(), dir, "pull", "--ff-only"); err != nil {
		t.Fatalf("pull: %v", err)
	}

	e := lastEntry(t, rec)
	if got, want := e.Command(), "git pull --ff-only"; got != want {
		t.Errorf("Command() = %q, want %q", got, want)
	}
	if e.Repo != filepath.Base(dir) {
		t.Errorf("Repo = %q, want %q (basename del dir)", e.Repo, filepath.Base(dir))
	}
	if e.Dir != dir {
		t.Errorf("Dir = %q, want %q", e.Dir, dir)
	}
	if e.Action != "pull" {
		t.Errorf("Action = %q, want %q", e.Action, "pull")
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
	if e.Outcome != "fast-forward" {
		t.Errorf("Outcome = %q, want %q", e.Outcome, "fast-forward")
	}
	if e.Dur <= 0 {
		t.Error("Dur sin medir: la duración es parte del registro")
	}
	if e.Intent {
		t.Error("Intent = true en una ejecución")
	}
}

// Un pull que choca registra el código de salida real (no siempre 1: git
// devuelve 128 en los fallos fatales) y el motivo clasificado.
func TestRunRegistraFallo(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	// Commit local además del remoto: ahora sí hay divergencia, y --ff-only
	// no puede reconciliarla.
	testutil.CommitFiles(t, dir, map[string]string{"local.txt": "local"}, "local")
	testutil.FetchLocal(t, dir)

	if _, err := Run(context.Background(), dir, "pull", "--ff-only"); err == nil {
		t.Fatal("pull --ff-only sobre un repo divergido debería fallar")
	}
	e := lastEntry(t, rec)
	if e.Exit == 0 {
		t.Error("Exit = 0 en un pull fallido")
	}
	if e.Outcome != "diverged" {
		t.Errorf("Outcome = %q, want %q", e.Outcome, "diverged")
	}
}

// Fetch es el único exec cuyo origen no se deduce del argv: la misma llamada
// la hace el scan automático y la tecla f. Por eso la clase la trae quien llama.
func TestFetchRegistraLaClaseQueLePasan(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 1, "up")

	if err := Fetch(context.Background(), dir, cmdlog.ClassAuto, "fetch", "--prune"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	e := lastEntry(t, rec)
	if e.Class != cmdlog.ClassAuto {
		t.Errorf("Class = %v, want %v", e.Class, cmdlog.ClassAuto)
	}
	if e.Action != "fetch" {
		t.Errorf("Action = %q, want %q", e.Action, "fetch")
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
}

// Collect lanza cuatro lecturas por repo. Se registran (para poder auditar por
// qué un behind tarda) pero como ClassRead, que el panel oculta por defecto:
// con 60 repos serían 240 líneas por scan.
func TestCollectRegistraLasLecturasComoRead(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)

	Collect(context.Background(), dir, "main")

	entries := rec.Entries()
	if len(entries) < 3 {
		t.Fatalf("entradas = %d, want >= 3 (status, log, worktree list…)", len(entries))
	}
	for _, e := range entries {
		if e.Class != cmdlog.ClassRead {
			t.Errorf("Class = %v, want %v para %q", e.Class, cmdlog.ClassRead, e.Command())
		}
	}
	// Las lecturas no se clasifican: no hay "resultado" que deducir de un
	// status. El panel enseña su código de salida.
	for _, e := range entries {
		if e.Outcome != "" {
			t.Errorf("Outcome = %q en la lectura %q, want \"\"", e.Outcome, e.Command())
		}
	}
	var sawStatus bool
	for _, e := range entries {
		if strings.HasPrefix(e.Command(), "git status") {
			sawStatus = true
		}
	}
	if !sawStatus {
		t.Errorf("no se registró el status; hubo: %v", entries)
	}
}

// RemoveWorktreeArgv es la fuente única del argv: el log y RemoveWorktree no
// pueden discrepar, que es lo único que hace fiable el log.
func TestRemoveWorktreeArgv(t *testing.T) {
	got := strings.Join(append([]string{"git"}, RemoveWorktreeArgv("/tmp/wt", false)...), " ")
	if want := "git worktree remove /tmp/wt"; got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
	got = strings.Join(append([]string{"git"}, RemoveWorktreeArgv("/tmp/wt", true)...), " ")
	if want := "git worktree remove --force /tmp/wt"; got != want {
		t.Errorf("argv forzado = %q, want %q", got, want)
	}
}

// El log no debe romper la app si no hay recorder (--print, o un test que no
// lo instaló): Collect tiene que seguir funcionando igual.
func TestExecSinRecorderNoRompe(t *testing.T) {
	cmdlog.SetRecorder(nil)
	dir, _ := testutil.NewRepo(t, true)
	snap := Collect(context.Background(), dir, "main")
	if snap.Err != "" {
		t.Fatalf("Collect sin recorder falló: %s", snap.Err)
	}
	if _, err := Run(context.Background(), dir, "status"); err != nil {
		t.Fatalf("Run sin recorder falló: %v", err)
	}
}
