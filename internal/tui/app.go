// Package tui implementa el dashboard gitdash con Bubbletea v2:
// tabla de repos con estado git vivo, filtros, fetch automático en
// batches y acciones pull/push/editor.
package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cache"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/state"
)

// event es el mensaje unificado del canal de trabajo en background.
type event interface{}

// scanProjectsMsg anuncia los proyectos descubiertos (antes de recolectar).
type scanProjectsMsg struct {
	projects []discovery.Project
	note     string // error agregado de roots ilegibles
}

// statusMsg entrega el snapshot vivo de un repo (streaming por repo).
type statusMsg struct {
	path string
	snap gitstatus.Snapshot
}

// collectDoneMsg marca el fin de la recolección del scan.
type collectDoneMsg struct{}

// fetchStateMsg cambia el estado de fetch de una fila.
type fetchStateMsg struct {
	path  string
	state string // fetching | ok | failed
	err   string
}

// fetchDoneMsg cierra un batch de fetch.
type fetchDoneMsg struct{ ok, failed int }

// actionMsg entrega el resultado de pull/push. cmd es el argv resuelto que se
// ejecutó (la política de pull puede venir del gitconfig, así que la UI no
// puede asumir los flags) y rebaseInProgress marca que el pull --rebase
// dejó el repo con un rebase a medias en vez de fallar limpio.
type actionMsg struct {
	path, kind, cmd  string
	output           string
	err              string
	rebaseInProgress bool
}

// worktreeRemovedMsg entrega el resultado de borrar un worktree: el nombre
// visible, la salida combinada y el motivo real de git si falló. gen es el
// token del intento: los resultados cuyo token ya no es el vigente se
// descartan (se canceló con esc o fueron sustituidos).
type worktreeRemovedMsg struct {
	parent, wtPath, name string
	output, err          string
	force                bool
	gen                  int
}

// execDoneMsg marca la vuelta de un proceso con handoff de terminal:
// editor, lazygit (tecla g) o shell interactiva (tecla !, vacío).
type execDoneMsg struct {
	path string
	err  error
}

// cmdResultMsg entrega la salida capturada de un comando `!`.
type cmdResultMsg struct {
	path, command, output, exit string
}

// notifyMsg alimenta un toast (el nivel clasifica color e icono).
type notifyMsg struct {
	text  string
	level toastLevel
}

// tickMsg expira notificaciones y anima el spinner.
type tickMsg struct{}

// actionResult guarda la salida de la última acción por repo. cmd es el argv
// resuelto: con la política de pull delegada en el gitconfig es la única forma
// de que el usuario vea qué se ejecutó de verdad.
type actionResult struct {
	kind, cmd, output, err string
}

// PullKinds son las variantes de pull del selector de la tecla `p`. Cada una
// existe porque la política por defecto vive en el gitconfig y hay que poder
// pisarla sin editar la config de gitdash.
var PullKinds = map[string]string{
	"p": "pull",        // sin flags: decide el gitconfig
	"r": "pull_rebase", // rebase + autostash
	"f": "pull_ff",     // ff-only
	"m": "pull_merge",  // merge clásico
}

// IsPullKind reporta si un kind es una variante de pull.
func IsPullKind(kind string) bool {
	for _, k := range PullKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// armedPull es el selector de variante de pull pendiente (nil = ninguno).
// Captura el path al armar: la segunda tecla resuelve sobre esa fila, no sobre
// la que esté bajo el cursor cuando llegue.
type armedPull struct {
	path string
}

// armedRemoval es la confirmación pendiente de borrado de un worktree (nil =
// sin confirmación). Un único nivel de estado cubre los dos escalones: normal
// y forzado (force=true).
type armedRemoval struct {
	wtPath string
	parent string
	name   string // basename visible del worktree
	force  bool
}

// matches reporta si la confirmación apunta al mismo worktree (parent + path),
// comparando paths normalizados.
func (a armedRemoval) matches(parent, wtPath string) bool {
	return filepath.Clean(a.parent) == filepath.Clean(parent) &&
		filepath.Clean(a.wtPath) == filepath.Clean(wtPath)
}

// Model es el modelo raíz de la TUI.
type Model struct {
	cfg      config.Config
	projects []discovery.Project
	states   map[string]gitstatus.Snapshot

	store     *state.Store
	cursor    int
	offset    int // scroll de la tabla
	onlyDirty bool
	search    string

	searchActive bool
	searchInput  textinput.Model
	collapsed    map[string]bool // grupos plegados

	// Expansión de worktrees por path canónico del repo principal.
	// Ausente = plegado. Persiste en collapsed.json bajo namespace
	// propio (polaridad inversa a las claves de grupo).
	expanded map[string]bool

	scanning bool

	events  chan event
	ctx     context.Context
	cancel  context.CancelFunc
	spinner spinner.Model

	fetchStates map[string]string
	running     map[string]string // path → kind en curso (pull/push/collect)
	lastAction  map[string]actionResult

	toasts toastManager

	// armed es la confirmación armada de borrado de worktree (nil = ninguna).
	// Es estado efímero de sesión: no se persiste.
	armed *armedRemoval
	// pullArmed es el selector de variante de pull pendiente (nil = ninguno).
	// Mismo carácter efímero que armed: se resuelve o se cancela con la
	// siguiente tecla.
	pullArmed *armedPull
	// removeGen/removeTokens correlacionan cada borrado en vuelo con su
	// resultado. removeTokens mapea path del repo padre → token del intento
	// vigente (el guard de "acción en curso" es por padre, así que el token
	// también). removeGen es el contador monótono que los genera; un resultado
	// cuyo token ya no es el vigente se descarta (cancelado con esc o
	// sustituido).
	removeGen    int
	removeTokens map[string]int

	detailOpen    bool
	width, height int

	// modo comando del detalle (tecla !): input de shell ejecutada en el
	// repo con $SHELL -c; Enter con input vacío abre una shell interactiva.
	cmdOpen  bool
	cmdInput textinput.Model

	// lastCmd guarda la salida del último comando `!` por repo (S-! :
	// capturado, queda visible hasta el próximo comando o cierre).
	lastCmd map[string]cmdResult
}

// cmdResult es el resultado capturado de un comando `!`.
type cmdResult struct {
	command, output, exit string
}

// searchPlaceholder es el hint del filter input (/).
const searchPlaceholder = "name/group…"

// New construye el modelo con la config dada y pinta el cache si existe
// (pintura instantánea; el rescan corre vía Init).
func New(cfg config.Config) Model {
	ctx, cancel := context.WithCancel(context.Background())
	store, _ := state.NewStore()
	m := Model{
		cfg:         cfg,
		store:       store,
		states:      map[string]gitstatus.Snapshot{},
		events:      make(chan event, 256),
		ctx:         ctx,
		cancel:      cancel,
		fetchStates: map[string]string{},
		running:     map[string]string{},
		lastAction:  map[string]actionResult{},
		lastCmd:     map[string]cmdResult{},
		collapsed:   map[string]bool{},
		expanded:    map[string]bool{},

		removeTokens: map[string]int{},
	}
	m.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	in := textinput.New()
	in.Placeholder = searchPlaceholder
	in.Prompt = "/" // el prompt pinta [/aquí][cursor], no "filter: "

	ci := textinput.New()
	// El primer rune del placeholder queda bajo el cursor (bubbles v2
	// placeholderView): espacio inicial para que el cursor no tape una letra.
	ci.Placeholder = " npm test · git status… (enter vacío = shell interactiva)"
	ci.Prompt = "! "
	m.cmdInput = ci
	m.searchInput = in
	m.scanning = true // el scan arranca en Init (spinner visible desde ya)

	if path, err := cache.Path(); err == nil {
		m.projects = cache.Load(path, cfg.Marker)
	}
	// Restaurar el estado de plegado persistido y la expansión de
	// worktrees. La carga separa ambos espacios por prefijo: las
	// claves con WorktreePrefix van a `expanded` (true = expandido), el
	// resto a `collapsed` (true = plegado).
	if store != nil {
		if persisted := store.LoadCollapsed(); persisted != nil {
			m.loadPersisted(persisted)
		}
	}
	return m
}

// loadPersisted vuelca el mapa plano de collapsed.json en los dos espacios
// de nombres del modelo: expansión de worktrees vs. plegado de
// grupos. Fichero corrupto o ausente ya llega como nil.
func (m *Model) loadPersisted(persisted map[string]bool) {
	for k, v := range persisted {
		if path, ok := strings.CutPrefix(k, state.WorktreePrefix); ok {
			m.expanded[path] = v
			continue
		}
		m.collapsed[k] = v
	}
}

// Init lanza el primer scan, la bomba de eventos y el tick.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.startScanCmd(),
		waitForEvent(m.events),
		m.spinner.Tick,
		tickCmd(),
	)
}

// waitForEvent rearma la lectura del canal: un evento por Cmd (patrón tea).
func waitForEvent(ch <-chan event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

// sendEvent publica en el canal respetando la cancelación.
func sendEvent(ctx context.Context, ch chan<- event, ev event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

// ---- pipelines de fondo ----

// startScanCmd lanza discovery + recolección streaming y, si
// procede, el fetch automático al terminar. El guard de "un scan a la
// vez" vive en el handler de la tecla r (New ya marca scanning=true).
func (m *Model) startScanCmd() tea.Cmd {
	m.scanning = true

	ctx, cancel := context.WithCancel(m.ctx)
	events := m.events
	cfg := m.cfg
	go func() {
		defer cancel()
		projects, err := discovery.Scan(cfg)
		note := ""
		if err != nil {
			note = err.Error()
		}
		sendEvent(ctx, events, scanProjectsMsg{projects: projects, note: note})
		if ctx.Err() != nil {
			return
		}
		gitstatus.StreamPool(ctx, projects, cfg.SyncBranch, 8, func(path string, snap gitstatus.Snapshot) {
			sendEvent(ctx, events, statusMsg{path: path, snap: snap})
		})
		if ctx.Err() != nil {
			return
		}
		sendEvent(ctx, events, collectDoneMsg{})
	}()
	return nil
}

// fetchTargets devuelve los paths con upstream pendientes de fetch,
// excluyendo los que ya están en curso.
func (m *Model) fetchTargets() []string {
	var paths []string
	for _, p := range m.projects {
		if !p.HasRepo {
			continue // sin repo no hay fetch
		}
		snap := m.states[p.Path]
		if !snap.Status.HasUpstream || snap.Err != "" {
			continue // sin upstream se saltan
		}
		if m.fetchStates[p.Path] == "fetching" {
			continue
		}
		paths = append(paths, p.Path)
	}
	return paths
}

// fetchBatchCmd lanza `git fetch --prune` en batches de fetch.concurrency
// con timeout por fetch; tras cada fetch re-colecciona el estado.
func (m *Model) fetchBatchCmd(paths []string) tea.Cmd {
	if len(paths) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	events := m.events
	timeout := m.cfg.FetchTimeout
	concurrency := m.cfg.FetchConcurrency

	go func() {
		defer cancel()
		var wg sync.WaitGroup
		sem := make(chan struct{}, max(1, concurrency))
		var mu sync.Mutex
		ok, failed := 0, 0

		for _, path := range paths {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				sendEvent(ctx, events, fetchStateMsg{path: p, state: "fetching"})
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}
				fctx, fcancel := context.WithTimeout(ctx, timeout)
				defer fcancel()
				if err := gitstatus.Fetch(fctx, p, m.cfg.CmdArgs("fetch")...); err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					sendEvent(ctx, events, fetchStateMsg{path: p, state: "failed", err: err.Error()})
					return
				}
				mu.Lock()
				ok++
				mu.Unlock()
				sendEvent(ctx, events, fetchStateMsg{path: p, state: "ok"})
				sendEvent(ctx, events, statusMsg{path: p, snap: gitstatus.Collect(ctx, p, m.syncOf(p))})
			}(path)
		}
		wg.Wait()
		sendEvent(ctx, events, fetchDoneMsg{ok: ok, failed: failed})
	}()
	return nil
}

// startActionCmd lanza pull/push capturado sobre un repo. Devuelve
// además el texto de guard si la acción está bloqueada.
//
// El argv se resuelve aquí y viaja en el mensaje: con la política de pull en el
// gitconfig, la UI no puede deducir qué se ejecutó, y un pull --rebase que
// choca deja el repo a medias en vez de fallar limpio (rebaseInProgress).
func (m *Model) startActionCmd(path, kind string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = kind
	appCtx := m.ctx
	events := m.events
	args := m.cfg.CmdArgs(kind)
	// El argv resuelto se compone en el hilo principal (lectura de cfg) para
	// que el mensaje sea determinista respecto a la tecla que lo disparó.
	resolved := "git " + strings.Join(args, " ")
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, 120*time.Second)
		defer cancel()
		out, err := gitstatus.Run(ctx, path, args...)
		errStr := ""
		if err != nil {
			// El error del proceso es siempre "exit status 1"; el motivo real
			// está en la salida combinada de git.
			errStr = gitstatus.FailureReason(out, err)
		}
		msg := actionMsg{path: path, kind: kind, cmd: resolved, output: out, err: errStr}
		if errStr != "" && IsPullKind(kind) {
			msg.rebaseInProgress = gitstatus.RebaseInProgress(ctx, path)
		}
		sendEvent(appCtx, events, msg)
		if err == nil {
			sendEvent(appCtx, events, statusMsg{path: path, snap: gitstatus.Collect(appCtx, path, m.syncOf(path))})
		}
	}()
	return nil
}

// runAction ya no existe: el argv llega resuelto desde config y lo ejecuta
// gitstatus.Run.

// busyActionCmd devuelve el toast de "ya hay una acción en curso" en el repo,
// o nil si está libre.
func (m *Model) busyActionCmd(path string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	return nil
}

// removeWorktreeCmd lanza el borrado de un worktree desde el repo padre.
// Replica el patrón de startActionCmd: guard de acción en curso por path del
// padre, goroutine con timeout, y tras el éxito publica además el snapshot del
// padre para que la sub-fila desaparezca. No usa recollectCmd porque su guard
// chocaría con el flag running de esta propia acción. token correlaciona el
// resultado con el intento que lo lanzó.
func (m *Model) removeWorktreeCmd(parent, wtPath, name string, withForce bool, token int) tea.Cmd {
	if cmd := m.busyActionCmd(parent); cmd != nil {
		return cmd
	}
	m.running[parent] = "worktree_remove"
	appCtx := m.ctx
	events := m.events
	syncBranch := m.syncOf(parent)
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, 120*time.Second)
		defer cancel()
		out, err := gitstatus.RemoveWorktree(ctx, parent, wtPath, withForce)
		errStr := ""
		if err != nil {
			// El error del proceso es "exit status 1"; el motivo real está en
			// la salida combinada de git.
			errStr = gitstatus.FailureReason(out, err)
		}
		sendEvent(appCtx, events, worktreeRemovedMsg{
			parent: parent, wtPath: wtPath, name: name,
			output: out, err: errStr, force: withForce, gen: token,
		})
		if err == nil {
			sendEvent(appCtx, events, statusMsg{path: parent, snap: gitstatus.Collect(appCtx, parent, syncBranch)})
		}
	}()
	return nil
}

// recollectCmd re-colecciona un solo repo (tecla R).
func (m *Model) recollectCmd(path string) tea.Cmd {
	if _, busy := m.running[path]; busy {
		return nil
	}
	m.running[path] = "collect"
	appCtx := m.ctx
	events := m.events
	go func() {
		sendEvent(appCtx, events, statusMsg{path: path, snap: gitstatus.Collect(appCtx, path, m.syncOf(path))})
	}()
	return nil
}

// toastCmd emite un toast efímero con el nivel dado (llega como notifyMsg).
func (m *Model) toastCmd(level toastLevel, text string) tea.Cmd {
	return func() tea.Msg { return notifyMsg{text: text, level: level} }
}

// openEditorCmd abre $EDITOR en el repo con handoff de terminal.
func (m *Model) openEditorCmd(path string) tea.Cmd {
	cmd := exec.Command(m.cfg.Editor)
	cmd.Dir = path
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{path: path, err: err}
	})
}

// openLazygitCmd abre lazygit en el repo con handoff de terminal (tecla g).
// Requiere intérprete instalado; al salir re-colecciona el estado (lazygit
// puede hacer pull/commit/push).
func (m *Model) openLazygitCmd(path string) tea.Cmd {
	if _, err := exec.LookPath("lazygit"); err != nil {
		return m.toastCmd(toastWarning, "lazygit not installed")
	}
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "lazygit"
	cmd := exec.Command("lazygit")
	cmd.Dir = path
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{path: path, err: err}
	})
}

// commandTimeout es el límite de un comando `!` capturado.
const commandTimeout = 5 * time.Minute

// runShellCmd ejecuta el comando en el repo con el shell dado, capturando
// stdout+stderr juntos; devuelve la salida y el código de salida.
func runShellCmd(ctx context.Context, dir, shell, command string) (string, int) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		code := 1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return out.String(), code
	}
	return out.String(), 0
}

// openCmdCmd lanza el comando tipeado con `!` en el directorio del repo con
// $SHELL -c y captura la salida: queda visible en el detail hasta el
// próximo comando (S-!: nvim-style, no handoff de terminal, así no se
// pierde de vista). Aliases y config del shell quedan cargados; las
// abreviaciones de fish no aplican porque no hay sesión de edición.
func (m *Model) openCmdCmd(path, command string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "cmd"
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	appCtx := m.ctx
	events := m.events
	go func() {
		out, code := runShellCmd(appCtx, path, shell, command)
		sendEvent(appCtx, events, cmdResultMsg{
			path: path, command: command, output: out,
			exit: fmt.Sprintf("%d", code),
		})
	}()
	return nil
}

// openShellCmd abre una shell interactiva ($SHELL) en el repo con handoff
// de terminal: la sesión carga config.fish/.bashrc, así que abreviaciones,
// aliases y aliases de fish funcionan (tecla ! con input vacío).
func (m *Model) openShellCmd(path string) tea.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if _, err := exec.LookPath(shell); err != nil {
		return m.toastCmd(toastWarning, "shell not found")
	}
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "shell"
	cmd := exec.Command(shell)
	cmd.Dir = path
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{path: path, err: err}
	})
}

// nameOf devuelve el nombre visible de un path: el nombre del
// proyecto descubierto, el basename para un worktree (aunque tenga marcador
// propio, la notificación usa el directorio) y el path absoluto en último
// término si no es resoluble.
func (m *Model) nameOf(path string) string {
	for _, p := range m.projects {
		if p.Path == path {
			if p.IsWorktree {
				return filepath.Base(path)
			}
			return p.Name
		}
	}
	if path == "" {
		return path
	}
	return filepath.Base(path)
}

// syncOf resuelve la sync branch efectiva de un path: override del
// marcador > global. Proyectos no descubiertos → global.
func (m *Model) syncOf(path string) string {
	for _, p := range m.projects {
		if p.Path == path {
			return gitstatus.SyncFor(p, m.cfg.SyncBranch)
		}
	}
	return m.cfg.SyncBranch
}

// saveCollapsed persiste el estado de plegado y de expansión a disco
// (best-effort: no bloquear la UI). Se llama tras cada toggle. Compone los
// dos espacios de nombres: claves de grupo tal cual y la
// expansión de worktrees bajo WorktreePrefix.
func (m *Model) saveCollapsed() {
	if m.store == nil {
		return
	}
	combined := make(map[string]bool, len(m.collapsed)+len(m.expanded))
	for k, v := range m.collapsed {
		combined[k] = v
	}
	for path, v := range m.expanded {
		combined[state.WorktreePrefix+path] = v
	}
	_ = m.store.SaveCollapsed(combined)
}

// selectedEntry devuelve la entrada navegable bajo el cursor, si la hay.
func (m *Model) selectedEntry() (tableEntry, bool) {
	return entryAt(m.entries(), m.cursor)
}

// selected devuelve la fila (con path resoluble) bajo el cursor, si la hay.
// Los headers de grupo no seleccionan repo: ok=false. Una
// sub-fila de worktree se resuelve a una fila sintética con
// el path del worktree y HasRepo=true, de forma que TODAS las operaciones
// (que leen r.project.Path/HasRepo/MarkerErr) operan sobre el worktree.
func (m *Model) selected() (row, bool) {
	e, ok := m.selectedEntry()
	if !ok {
		return row{}, false
	}
	switch e.kind {
	case kindRepo:
		return e.r, true
	case kindWorktree:
		return m.worktreeRow(e.wt), true
	default:
		return row{}, false
	}
}

// worktreeRow sintetiza la fila operable de un worktree. Si el
// worktree fue descubierto con marcador (dedupe), reutiliza su
// proyecto y su snapshot vivo; si no, un proyecto mínimo con HasRepo=true y
// MarkerErr vacío para satisfacer los guards de las operaciones.
func (m *Model) worktreeRow(wt gitstatus.Worktree) row {
	if p, ok := m.discoveredByPath(wt.Path); ok {
		snap := m.states[p.Path]
		return row{project: p, snap: snap, state: snap.State(p.HasRepo)}
	}
	return row{project: discovery.Project{
		Path:       wt.Path,
		Name:       filepath.Base(wt.Path),
		HasRepo:    true,
		IsWorktree: true,
	}}
}

// discoveredByPath busca el proyecto descubierto que corresponde a un path
// de worktree (dedupe), comparando paths normalizados para tolerar
// symlinks o barras finales.
func (m *Model) discoveredByPath(path string) (discovery.Project, bool) {
	clean := filepath.Clean(path)
	for _, p := range m.projects {
		if filepath.Clean(p.Path) == clean {
			return p, true
		}
	}
	return discovery.Project{}, false
}
