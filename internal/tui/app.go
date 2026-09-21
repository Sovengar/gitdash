// Package tui implementa el dashboard gitdash con Bubbletea v2 (spec 0001
// R6-R10): tabla de repos con estado git vivo, filtros, fetch automático en
// batches y acciones pull/push/editor.
package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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

// fetchStateMsg cambia el estado de fetch de una fila (S6.3, S8.2).
type fetchStateMsg struct {
	path  string
	state string // fetching | ok | failed
	err   string
}

// fetchDoneMsg cierra un batch de fetch (R8).
type fetchDoneMsg struct{ ok, failed int }

// actionMsg entrega el resultado de pull/push (R9).
type actionMsg struct {
	path, kind, output string
	err                string
}

// execDoneMsg marca la vuelta de un proceso con handoff de terminal:
// editor (S9.4), lazygit (tecla g) o shell interactiva (tecla !, vacío).
type execDoneMsg struct {
	path string
	err  error
}

// cmdResultMsg entrega la salida capturada de un comando `!`.
type cmdResultMsg struct {
	path, command, output, exit string
}

// notifyMsg fija una notificación transitoria en la barra.
type notifyMsg struct{ text string }

// tickMsg expira notificaciones y anima el spinner.
type tickMsg struct{}

// actionResult guarda la salida de la última acción por repo (S10.2).
type actionResult struct {
	kind, output, err string
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
	collapsed    map[string]bool // 0002 R16: grupos plegados

	scanning  bool
	scanNote  string
	fetchNote string

	events  chan event
	ctx     context.Context
	cancel  context.CancelFunc
	spinner spinner.Model

	fetchStates map[string]string
	running     map[string]string // path → kind en curso (pull/push/collect)
	lastAction  map[string]actionResult

	notify      string
	notifyUntil time.Time

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
// (S11.1: pintura instantánea; el rescan corre vía Init).
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
	}
	m.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	in := textinput.New()
	in.Placeholder = searchPlaceholder
	in.Prompt = "/" // el prompt pinta [/aquí][cursor], no "filter: " (S7.2)

	ci := textinput.New()
	// El primer rune del placeholder queda bajo el cursor (bubbles v2
	// placeholderView): espacio inicial para que el cursor no tape una letra.
	ci.Placeholder = " npm test · git status… (enter vacío = shell interactiva)"
	ci.Prompt = "! "
	m.cmdInput = ci
	m.searchInput = in
	m.scanning = true // el scan arranca en Init (R6: spinner visible desde ya)

	if path, err := cache.Path(); err == nil {
		m.projects = cache.Load(path, cfg.Marker) // S11.1
	}
	// Restaurar el estado de plegado persistido (S20.5).
	if store != nil {
		if persisted := store.LoadCollapsed(); persisted != nil {
			for k, v := range persisted {
				m.collapsed[k] = v
			}
		}
	}
	return m
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

// startScanCmd lanza discovery + recolección streaming (R2, R5) y, si
// procede, el fetch automático (R8) al terminar. El guard de "un scan a la
// vez" vive en el handler de la tecla r (New ya marca scanning=true).
func (m *Model) startScanCmd() tea.Cmd {
	m.scanning = true
	m.scanNote = ""

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
// excluyendo los que ya están en curso (R8).
func (m *Model) fetchTargets() []string {
	var paths []string
	for _, p := range m.projects {
		if !p.HasRepo {
			continue // S9.6: sin repo no hay fetch
		}
		snap := m.states[p.Path]
		if !snap.Status.HasUpstream || snap.Err != "" {
			continue // S8.4: sin upstream se saltan
		}
		if m.fetchStates[p.Path] == "fetching" {
			continue
		}
		paths = append(paths, p.Path)
	}
	return paths
}

// fetchBatchCmd lanza `git fetch --prune` en batches de fetch.concurrency
// con timeout por fetch (R8); tras cada fetch re-colecciona el estado.
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

// startActionCmd lanza pull/push capturado sobre un repo (R9). Devuelve
// además el texto de guard si la acción está bloqueada.
func (m *Model) startActionCmd(path, kind string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.notifyCmd(fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = kind
	appCtx := m.ctx
	events := m.events
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, 120*time.Second)
		defer cancel()
		var out string
		var err error
		if kind == "pull" {
			out, err = gitstatus.Pull(ctx, path, m.cfg.CmdArgs("pull")...)
		} else if kind == "sync" {
			out, err = gitstatus.Sync(ctx, path, m.cfg.CmdArgs("sync")...)
		} else {
			out, err = gitstatus.Push(ctx, path, m.cfg.CmdArgs("push")...)
		}
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		sendEvent(appCtx, events, actionMsg{path: path, kind: kind, output: out, err: errStr})
		if err == nil {
			sendEvent(appCtx, events, statusMsg{path: path, snap: gitstatus.Collect(appCtx, path, m.syncOf(path))})
		}
	}()
	return nil
}

// recollectCmd re-colecciona un solo repo (R7, tecla R).
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

// notifyCmd fija una notificación transitoria (3s).
func (m *Model) notifyCmd(text string) tea.Cmd {
	return func() tea.Msg { return notifyMsg{text: text} }
}

// openEditorCmd abre $EDITOR en el repo con handoff de terminal (S9.4).
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
		return m.notifyCmd("lazygit not installed")
	}
	if prev, busy := m.running[path]; busy {
		return m.notifyCmd(fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "lazygit"
	cmd := exec.Command("lazygit")
	cmd.Dir = path
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{path: path, err: err}
	})
}

// openUpdateCmd abre el binario configurado en commands.update en el
// repo con handoff de terminal (tecla u). Al salir re-colecciona el
// estado.
func (m *Model) openUpdateCmd(path string) tea.Cmd {
	bin := m.cfg.Commands["update"]
	if bin == "" {
		return m.notifyCmd("commands.update not configured")
	}
	if _, err := exec.LookPath(bin); err != nil {
		return m.notifyCmd(fmt.Sprintf("%s not installed", bin))
	}
	if prev, busy := m.running[path]; busy {
		return m.notifyCmd(fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "update"
	cmd := exec.Command(bin)
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
	cmd := exec.Command(shell, "-c", command)
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
		return m.notifyCmd(fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
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
		return m.notifyCmd("shell not found")
	}
	if prev, busy := m.running[path]; busy {
		return m.notifyCmd(fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "shell"
	cmd := exec.Command(shell)
	cmd.Dir = path
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{path: path, err: err}
	})
}

// nameOf devuelve el nombre visible de un path.
func (m *Model) nameOf(path string) string {
	for _, p := range m.projects {
		if p.Path == path {
			return p.Name
		}
	}
	return path
}

// syncOf resuelve la sync branch efectiva de un path (R14): override del
// marcador > global. Proyectos no descubiertos → global.
func (m *Model) syncOf(path string) string {
	for _, p := range m.projects {
		if p.Path == path {
			return gitstatus.SyncFor(p, m.cfg.SyncBranch)
		}
	}
	return m.cfg.SyncBranch
}

// saveCollapsed persiste el estado de plegado a disco (best-effort:
// no bloquear la UI). Se llama tras cada toggle de plegado.
func (m *Model) saveCollapsed() {
	if m.store == nil {
		return
	}
	_ = m.store.SaveCollapsed(m.collapsed)
}

// selected devuelve la fila de repo bajo el cursor, si la hay. Los
// headers de grupo (0002 R16) no seleccionan repo: ok=false.
func (m *Model) selected() (row, bool) {
	entries := m.entries()
	if len(entries) == 0 || m.cursor >= len(entries) {
		return row{}, false
	}
	e := entries[m.cursor]
	if e.kind != kindRepo {
		return row{}, false
	}
	return e.r, true
}
