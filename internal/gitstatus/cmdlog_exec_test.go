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

	Collect(context.Background(), dir, "main", false)

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

// El remote se lee ON DEMAND (al abrir un PR), no en el scan: por eso es el
// único verbo de lectura que no sale de Collect. Y sale por runGit, así que
// deja entrada en el command log como ClassRead (es una lectura, aunque la
// pulse una persona).
func TestRemoteURLSalePorRunGitYQuedaEnElLog(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)

	got, err := RemoteURL(context.Background(), dir)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if got != origin {
		t.Errorf("RemoteURL = %q, want %q (el origin del repo)", got, origin)
	}
	e := lastEntry(t, rec)
	if want := "git remote get-url origin"; e.Command() != want {
		t.Errorf("Command() = %q, want %q", e.Command(), want)
	}
	if e.Class != cmdlog.ClassRead {
		t.Errorf("Class = %v, want %v", e.Class, cmdlog.ClassRead)
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
}

// La URL viene recortada: el remoto de git trae el salto de línea, y sin
// TrimSpace el host que se busca en el mapa de forges no casaría con ninguno.
func TestRemoteURLRecortaLaSalida(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)

	got, err := RemoteURL(context.Background(), dir)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if got != strings.TrimSpace(got) || strings.ContainsAny(got, "\n\r") {
		t.Errorf("RemoteURL = %q, want la URL sin saltos", got)
	}
}

// Un repo sin `origin` falla con el motivo de git: el toast lo necesita para
// decir qué falta en vez de un "no se pudo" sin más.
func TestRemoteURLSinRemoteFallaConElMotivo(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false) // sin upstream ni remote

	if _, err := RemoteURL(context.Background(), dir); err == nil {
		t.Fatal("RemoteURL en un repo sin origin debería fallar")
	} else if !strings.Contains(err.Error(), "remote") {
		t.Errorf("err = %q, want el motivo de git sobre el remote", err)
	}
}

// Y no aparece en Collect: el scan no gana un `git remote get-url` por repo y
// por ciclo solo para un dato que casi nadie mira.
func TestCollectNoLeeElRemote(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)

	Collect(context.Background(), dir, "main", false)

	for _, e := range rec.Entries() {
		if strings.Contains(e.Command(), "remote") {
			t.Errorf("Collect lanzó una lectura de remote: %q", e.Command())
		}
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
	snap := Collect(context.Background(), dir, "main", false)
	if snap.Err != "" {
		t.Fatalf("Collect sin recorder falló: %s", snap.Err)
	}
	if _, err := Run(context.Background(), dir, "status"); err != nil {
		t.Fatalf("Run sin recorder falló: %v", err)
	}
}

// Fetch sin args usa el default `fetch --prune`. No se deduce del resultado
// (un `git` a secas sale con 0 sin hacer nada), así que lo que lo ata es el argv
// registrado: sin default, el log mentiría sobre lo que se ejecutó.
func TestFetchSinArgsUsaElDefault(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)

	if err := Fetch(context.Background(), dir, cmdlog.ClassAction); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	e := lastEntry(t, rec)
	if got, want := e.Command(), "git fetch --prune"; got != want {
		t.Errorf("Command() = %q, want %q", got, want)
	}
	if e.Action != "fetch" {
		t.Errorf("Action = %q, want fetch", e.Action)
	}
}

// Un exec que falla sin escribir nada en stderr (el caso real: el contexto se
// cancela a mitad del scan) tiene que conservar el motivo del error de proceso.
// Si no, el Snapshot llega a la UI con un motivo vacío y el usuario no ve por
// qué desapareció el repo.
func TestExecSinStderrConservaElMotivo(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // nadie va a escribir en stderr

	_, err := runGit(ctx, dir, cmdlog.ClassRead, "status")
	if err == nil {
		t.Fatal("runGit con contexto cancelado debería fallar")
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("err = %q, want el motivo del proceso (context canceled)", err)
	}
	// El mismo camino desde Collect: el error viaja en el Snapshot.
	snap := Collect(ctx, dir, "", false)
	if snap.Err == "" {
		t.Fatal("Collect con contexto cancelado sin Err")
	}
	if !strings.Contains(snap.Err, context.Canceled.Error()) {
		t.Errorf("snap.Err = %q, want el motivo del proceso", snap.Err)
	}
}

// Un exec que no llegó a salir (contexto cancelado) no tiene código de salida de
// git: se registra como -1, que es "no salió", no como 1, que en el panel se
// leería como un fallo real de git.
func TestExecSinCodigoDeSalidaSeRegistraComoMenosUno(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := runGit(ctx, dir, cmdlog.ClassRead, "status"); err == nil {
		t.Fatal("runGit con contexto cancelado debería fallar")
	}
	if e := lastEntry(t, rec); e.Exit != -1 {
		t.Errorf("Exit = %d, want -1 (no llegó a salir)", e.Exit)
	}
}

// firstLine recorta a la primera línea sin partirse en los bordes: sin salto de
// línea devuelve la cadena entera, y un "\n" inicial es una primera línea vacía
// (no "sin recorte").
func TestFirstLine(t *testing.T) {
	casos := []struct{ in, want string }{
		{"sin salto", "sin salto"},
		{"", ""},
		{"una\ndos", "una"},
		{"\nprimera", ""},
		{"\n", ""},
		{"con\r\n", "con\r"},
		{"tres\nlíneas\ny más", "tres"},
	}
	for _, c := range casos {
		if got := firstLine(c.in); got != c.want {
			t.Errorf("firstLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Un argv vacío es alcanzable desde la config del usuario (`[commands] pull = ""`
// → CmdArgs → strings.Fields → []) y la TUI SIEMPRE tiene recorder instalado:
// sin esta guarda, `Action: args[0]` revienta la goroutine de la acción y con
// ella la app entera. Se registra con verbo vacío, no con un panic.
func TestExecSinArgsNoRevienta(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)

	if _, err := Run(context.Background(), dir); err == nil {
		t.Fatal("git sin argumentos debería fallar")
	}
	e := lastEntry(t, rec)
	if e.Action != "" {
		t.Errorf("Action = %q, want vacío (no hay verbo que nombrar)", e.Action)
	}
	if len(e.Argv) != 1 || e.Argv[0] != "git" {
		t.Errorf("Argv = %v, want [git]", e.Argv)
	}
}
