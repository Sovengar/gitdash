// Package tui implementa el dashboard gitdash con Bubbletea v2 (spec 0001
// R6-R10): tabla de repos con estado git vivo, filtros, fetch automático en
// batches y acciones pull/push/editor.
package tui

import (
	"context"
	"fmt"
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

// editorDoneMsg marca la vuelta del editor (S9.4).
type editorDoneMsg struct {
	path string
	err  error
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

	cursor int
	offset int // scroll de la tabla

	onlyDirty    bool
	search       string
	searchActive bool
	searchInput  textinput.Model

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
}

// New construye el modelo con la config dada y pinta el cache si existe
// (S11.1: pintura instantánea; el rescan corre vía Init).
func New(cfg config.Config) Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := Model{
		cfg:         cfg,
		states:      map[string]gitstatus.Snapshot{},
		events:      make(chan event, 256),
		ctx:         ctx,
		cancel:      cancel,
		fetchStates: map[string]string{},
		running:     map[string]string{},
		lastAction:  map[string]actionResult{},
	}
	m.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	in := textinput.New()
	in.Placeholder = "name/group…"
	in.Prompt = "filter: "
	m.searchInput = in
	m.scanning = true // el scan arranca en Init (R6: spinner visible desde ya)

	if path, err := cache.Path(); err == nil {
		m.projects = cache.Load(path, cfg.Marker) // S11.1
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
		gitstatus.StreamPool(ctx, projects, 8, func(path string, snap gitstatus.Snapshot) {
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
				if err := gitstatus.Fetch(fctx, p); err != nil {
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
				sendEvent(ctx, events, statusMsg{path: p, snap: gitstatus.Collect(ctx, p)})
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
			out, err = gitstatus.Pull(ctx, path)
		} else {
			out, err = gitstatus.Push(ctx, path)
		}
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		sendEvent(appCtx, events, actionMsg{path: path, kind: kind, output: out, err: errStr})
		if err == nil {
			sendEvent(appCtx, events, statusMsg{path: path, snap: gitstatus.Collect(appCtx, path)})
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
		sendEvent(appCtx, events, statusMsg{path: path, snap: gitstatus.Collect(appCtx, path)})
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
		return editorDoneMsg{path: path, err: err}
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

// selected devuelve la fila bajo el cursor, si la hay.
func (m *Model) selected() (row, bool) {
	rows := m.rows()
	if len(rows) == 0 || m.cursor >= len(rows) {
		return row{}, false
	}
	return rows[m.cursor], true
}
