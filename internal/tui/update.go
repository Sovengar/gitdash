// Update y View del modelo gitdash.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cache"
	"gitdash/internal/cmdlog"
)

// Update procesa mensajes: eventos de fondo, teclas, tick y resize.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tickMsg:
		m.toasts.update()
		return m, tickCmd()

	case scanProjectsMsg:
		m.projects = msg.projects
		m.clampCursor()
		if msg.note != "" {
			m.toasts.showError("roots: " + msg.note)
		}
		return m.withPump(nil)

	case statusMsg:
		m.states[msg.path] = msg.snap
		delete(m.running, msg.path)
		// La sub-fila de un worktree borrado desaparece: el cursor debe
		// quedar en rango.
		m.clampCursor()
		return m.withPump(nil)

	case collectDoneMsg:
		m.scanning = false
		cmds := []tea.Cmd{}
		// cache best-effort al final de cada rescan
		if path, err := cache.Path(); err == nil {
			projects := m.projects
			go func() { _ = cache.Save(path, projects) }()
		}
		// fetch automático en batches
		if m.cfg.FetchAuto {
			if c := m.fetchBatchCmd(m.fetchTargets(), cmdlog.ClassAuto); c != nil {
				cmds = append(cmds, c)
			}
		}
		return m.withPump(tea.Batch(cmds...))

	case fetchStateMsg:
		m.fetchStates[msg.path] = msg.state
		if msg.state == "failed" {
			m.toasts.showError(fmt.Sprintf("fetch failed %s: %s", m.nameOf(msg.path), msg.err))
		}
		return m.withPump(nil)

	case fetchDoneMsg:
		switch {
		case msg.failed > 0:
			m.toasts.showWarning(fmt.Sprintf("fetch: %d ok, %d failed", msg.ok, msg.failed))
		case msg.ok == 1:
			m.toasts.showSuccess("fetch ok")
		default:
			m.toasts.showSuccess(fmt.Sprintf("fetch ok (%d repos)", msg.ok))
		}
		return m.withPump(nil)

	case actionMsg:
		delete(m.running, msg.path)
		m.lastAction[msg.path] = actionResult{kind: msg.kind, cmd: msg.cmd, output: msg.output, err: msg.err}
		note := actionNote(msg.kind, m.nameOf(msg.path), msg.cmd, msg.output, msg.err, msg.rebaseInProgress)
		if msg.err != "" {
			m.toasts.showError(note)
		} else {
			m.toasts.showSuccess(note)
		}
		return m.withPump(nil)

	case worktreeRemovedMsg:
		tok, inflight := m.removeTokens[msg.parent]
		switch {
		case inflight && msg.gen == tok:
			// Intento vigente de este padre: se consume el token y se libera el
			// running si sigue siendo el del borrado.
			delete(m.removeTokens, msg.parent)
			if m.running[msg.parent] == "worktree_remove" {
				delete(m.running, msg.parent)
			}
		case !inflight:
			// Sin intento registrado (cancelado con esc): se libera el running
			// residual y se descarta el resultado sin tocar banner ni toast.
			if m.running[msg.parent] == "worktree_remove" {
				delete(m.running, msg.parent)
			}
			return m.withPump(nil)
		default:
			// Token distinto: el intento fue sustituido, así que el resultado
			// es obsoleto. Ocurre cuando un statusMsg de fondo (scan/fetch)
			// libera running[parent] con un borrado aún en vuelo: el usuario
			// relanza (t2) y sobrescribe removeTokens[parent]; cuando llega el
			// resultado de t1 hay que ignorarlo. NO se libera running[parent]
			// porque ahora pertenece al intento nuevo (t2), ni se muta el
			// banner.
			return m.withPump(nil)
		}
		m.lastAction[msg.parent] = actionResult{kind: "worktree_remove", cmd: msg.cmd, output: msg.output, err: msg.err}
		// La mutación del estado armado solo aplica si este sigue apuntando al
		// mismo worktree (o está vacío): un armado posterior sobre otro
		// worktree no se pisa con el resultado tardío.
		targetsArmed := m.armed == nil || m.armed.matches(msg.parent, msg.wtPath)
		if msg.err == "" {
			if targetsArmed {
				m.armed = nil
			}
			m.toasts.showSuccess("worktree removed " + msg.name)
			return m.withPump(nil)
		}
		m.toasts.showError(fmt.Sprintf("worktree remove failed %s: %s", msg.name, msg.err))
		if !targetsArmed {
			return m.withPump(nil)
		}
		if msg.force {
			// El forzado también falló: se desarma para no entrar en bucle.
			m.armed = nil
		} else {
			// Primer intento fallido (worktree sucio): se arma el forzado para
			// la siguiente pulsación.
			m.armed = &armedRemoval{
				wtPath: msg.wtPath, parent: msg.parent,
				name: msg.name, force: true,
			}
		}
		return m.withPump(nil)

	case execDoneMsg:
		delete(m.running, msg.path)
		// El handoff presta la terminal al hijo, así que no hay salida que
		// registrar: en el command log quedan el argv y cómo terminó.
		// Dur = 0 (medirlo exigiría guardar el arranque en el modelo).
		cmdlog.RecordExec(cmdlog.Entry{
			Repo:   m.nameOf(msg.path),
			Dir:    msg.path,
			Class:  cmdlog.ClassAction,
			Action: msg.action,
			Argv:   msg.argv,
			Exit:   execExit(msg.err),
		})
		// el handoff pudo cambiar el estado del repo: siempre re-colecta
		cmd := m.recollectCmd(msg.path)
		if msg.err != nil {
			m.toasts.showError(fmt.Sprintf("command: %v", msg.err))
		}
		return m, cmd

	case cmdResultMsg:
		delete(m.running, msg.path)
		m.lastCmd[msg.path] = cmdResult{command: msg.command, output: msg.output, exit: msg.exit}
		if msg.exit != "0" {
			m.toasts.showError(fmt.Sprintf("! %s — exit %s", msg.command, msg.exit))
		} else {
			m.toasts.showInfo(fmt.Sprintf("! %s — ok", msg.command))
		}
		return m.withPump(nil)

	case notifyMsg:
		m.toasts.show(msg.text, msg.level)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// withPump rearma la bomba de eventos después de procesar uno del canal
// (los Cmds leen UN evento cada vez: sin esto los estados nunca llegan).
func (m Model) withPump(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	return m, tea.Batch(cmd, waitForEvent(m.events))
}

// actionNote compone la notificación de una acción terminada (pull/push).
// En el fallo incluye el motivo real de git (errStr, ya resumido) y, cuando se
// reconoce, un hint accionable; la salida completa queda en el detalle.
//
// rebaseInProgress tiene prioridad sobre los demás hints: un pull --rebase que
// choca no dejó el repo como estaba, lo dejó con la historia reescrita a medias
// y el índice en conflicto. Decir solo "falló" invita a reintentar, y reintentar
// sobre un rebase a medias es peor que no hacer nada.
func actionNote(kind, name, cmd, output, errStr string, rebaseInProgress bool) string {
	if errStr == "" {
		if cmd == "" {
			return fmt.Sprintf("%s ok %s", kind, name)
		}
		return fmt.Sprintf("%s ok %s (%s)", kind, name, cmd)
	}
	note := fmt.Sprintf("%s failed %s: %s", kind, name, errStr)
	if cmd != "" {
		note += " — " + cmd
	}
	switch {
	case rebaseInProgress && IsPullKind(kind):
		note += " — rebase a medias: resolvé los conflictos y `git rebase --continue` (o `--abort`)"
	case IsPullKind(kind):
		switch {
		case strings.Contains(output, "Not possible to fast-forward"),
			strings.Contains(output, "divergent"):
			note += " — divergió: probá el rebase del selector (p luego r)"
		case strings.Contains(errStr, "no tracking information"),
			strings.Contains(errStr, "no upstream"):
			note += " — sin upstream: P la publica y configura el tracking"
		}
	}
	return note
}

// fetchingAll reporta si hay algún fetch en curso (para el spinner).
func (m *Model) fetchingAll() bool {
	for _, st := range m.fetchStates {
		if st == "fetching" {
			return true
		}
	}
	return false
}

// clampCursor mantiene el cursor dentro de los límites visibles.
func (m *Model) clampCursor() {
	n := len(m.entries())
	if m.cursor >= n {
		m.cursor = max(0, n-1)
	}
}

// handleKey enruta las teclas: input de búsqueda primero, luego el input
// del modo comando (`!` en el detalle) y por último la tabla. Las teclas
// se resuelven contra el mapa de keybindings configurado (config.toml).
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// esc cancela de forma definitiva TODOS los borrados en vuelo, no solo uno:
	// limpia el mapa completo de tokens para que cualquier resultado tardío se
	// descarte sin re-armar el forzado ni tocar el banner (su running residual
	// se libera al llegar el resultado). El esc sigue su curso normal (cerrar
	// detalle, etc.).
	if key == "esc" && len(m.removeTokens) > 0 {
		clear(m.removeTokens)
	}

	// Confirmación armada de borrado de worktree: tiene prioridad sobre el
	// resto (incluido el esc que cierra el detalle y los inputs de
	// búsqueda/comando). Cualquier tecla distinta de la acción de borrado y de
	// esc desarma y sigue su curso normal.
	if m.armed != nil {
		switch {
		case m.actionForKey(key) == "worktree_remove":
			return m.handleWorktreeRemove()
		case key == "esc":
			m.armed = nil
			return m, nil
		default:
			m.armed = nil
		}
	}

	// Selector de variante de pull: la tecla de pull solo arma, la segunda
	// tecla elige. Las cuatro opciones (p/r/f/m) están-en-concurrencia con
	// acciones reales de la tabla (pull/rescan/fetch), así que el estado
	// armado tiene que consumir la tecla antes de que llegue al resto del
	// enrutado. Cualquier otra tecla cancela y sigue su curso normal: es lo
	// que evita que la app quede pegada esperando una segunda pulsación.
	if m.pullArmed != nil {
		armed := *m.pullArmed
		m.pullArmed = nil
		if kind, ok := PullKinds[key]; ok {
			// La intención lleva la variante elegida, no "pull": es lo que
			// explica el argv que se ve una línea más abajo en el log.
			cmdlog.RecordIntent(cmdlog.Entry{
				Class:  cmdlog.ClassAction,
				Repo:   m.nameOf(armed.path),
				Dir:    armed.path,
				Key:    key,
				Action: kind,
			})
			return m, m.startActionCmd(armed.path, kind)
		}
	}

	// Panel del command log: sus teclas se consultan antes del enrutado
	// normal (como los estados armados) porque j/k chocan con la navegación
	// de la tabla. Las teclas que no son suyas siguen su curso normal: el
	// panel es un view mode, no una modal, y así la app nunca queda
	// encerrada aquí dentro.
	if m.logOpen {
		if m.handleLogKey(key, m.layout().bodyLines) {
			return m, nil
		}
	}

	if m.cmdOpen { // modo comando del detalle: prioridad sobre todo
		switch key {
		case "enter":
			cmdStr := strings.TrimSpace(m.cmdInput.Value())
			m.cmdOpen = false
			m.cmdInput.Blur()
			m.cmdInput.SetValue("")
			if r, ok := m.selected(); ok {
				if !r.project.HasRepo {
					return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
				}
				// El comando tecleado viaja en la exec entry (con su argv
				// `sh -c …`); la intención deja la tecla que lo lanzó.
				m.logIntent(key, "cmd")
				if cmdStr == "" {
					return m, m.openShellCmd(r.project.Path) // shell interactiva
				}
				return m, m.openCmdCmd(r.project.Path, cmdStr)
			}
			return m, nil
		case "esc":
			m.cmdOpen = false
			m.cmdInput.Blur()
			return m, nil
		default:
			in, cmd := m.cmdInput.Update(msg)
			m.cmdInput = in
			return m, cmd
		}
	}

	if m.searchActive {
		switch key {
		case "enter":
			m.searchActive = false
			m.searchInput.Blur()
			m.search = strings.TrimSpace(m.searchInput.Value()) // confirmar
			m.clampCursor()
			return m, nil
		case "esc":
			m.searchActive = false
			m.searchInput.Blur()
			if strings.TrimSpace(m.searchInput.Value()) == "" {
				m.search = "" // esc en input vacío limpia el filtro
			}
			m.clampCursor()
			return m, nil
		default:
			in, cmd := m.searchInput.Update(msg)
			m.searchInput = in
			m.search = strings.TrimSpace(m.searchInput.Value()) // en vivo
			m.clampCursor()
			return m, cmd
		}
	}

	// Teclas fijas (universales, no configurables).
	switch key {
	case "q", "ctrl+c":
		m.cancel()
		return m, tea.Quit
	case "esc":
		// El dashboard no tiene vista que cerrar: esc cancela el aviso armado o
		// el filtro, y el resto de estados se resuelven en sus propios handlers.
		return m, nil
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
		return m, nil
	case "down", "j":
		m.cursor = min(m.cursor+1, max(0, len(m.entries())-1))
		return m, nil
	case "home":
		m.cursor = 0
		return m, nil
	case "end":
		m.cursor = max(0, len(m.entries())-1)
		return m, nil
	}

	// Resolver acción desde keybindings configurados.
	action := m.actionForKey(key)

	// Un punto único de intención para las acciones que lanzan algo: la tecla
	// y qué acción resolvió, sobre el repo del cursor. Las de navegación
	// (filtro, plegado, detalle, el propio panel del log) no se registran:
	// esto es un log de comandos, no de teclas. Las que necesitan más
	// detalle (la variante de pull, el comando `!`, la ejecución del borrado)
	// registran la suya donde lo saben.
	if launchesCommand(action) {
		m.logIntent(key, action)
	}

	switch action {
	case "dirty":
		m.onlyDirty = !m.onlyDirty
		m.clampCursor()
	case "search":
		m.searchActive = true
		m.searchInput.SetValue(m.search)
		return m, m.searchInput.Focus()
	case "fetch":
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
			}
			return m, m.fetchBatchCmd([]string{r.project.Path}, cmdlog.ClassAction)
		}
	case "fetch_all":
		paths := m.fetchTargets()
		if len(paths) == 0 {
			return m, m.toastCmd(toastInfo, "no repositories with upstream to fetch")
		}
		return m, m.fetchBatchCmd(paths, cmdlog.ClassAction)
	case "pull":
		// La tecla de pull no ejecuta: arma el selector de variante. El
		// guard de "no repo" se resuelve al armar, no al elegir, para no
		// dejar un selector vivo sobre una fila donde no hay nada que hacer.
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		} else if ok {
			m.pullArmed = &armedPull{path: r.project.Path}
			return m, nil
		}
	case "push":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "push")
		}
	case "editor":
		if r, ok := m.selected(); ok {
			if r.project.MarkerErr != "" {
				return m, m.toastCmd(toastWarning, "marker error — fix .gitdash.toml first")
			}
			return m, m.openEditorCmd(r.project.Path)
		}
	case "lazygit":
		// G abre lazygit en el repo bajo el cursor.
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
			}
			return m, m.openLazygitCmd(r.project.Path)
		}
	case "rescan":
		if m.scanning {
			return m, m.toastCmd(toastInfo, "scan already running")
		}
		return m, m.startScanCmd()
	case "recollect":
		if r, ok := m.selected(); ok {
			return m, m.recollectCmd(r.project.Path)
		}
	case "fold":
		return m.toggleFold()
	case "worktree_remove":
		return m.handleWorktreeRemove()
	case "command":
		// `!` abre el input de comandos en el panel de la fila del cursor
		// ($SHELL -c capturado; enter vacío = shell interactiva).
		if r, ok := m.selected(); !ok || !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		}
		m.cmdOpen = true
		return m, m.cmdInput.Focus()
	case "log":
		// `l` abre/cierra el panel del command log. La tecla es de la
		// sección log, así que el enrutado normal también la cierra: el
		// panel se comprueba antes de llegar aquí.
		m.toggleLog()
	}
	return m, nil
}

// toggleFold pliega o despliega lo que haya bajo el cursor. Es la única tecla de
// plegado (`enter`) y cubre los tres niveles, cada uno con lo que le toca:
//
//   - header primario o secundario → su bloque de repos.
//   - fila de repo → sus sub-filas de worktree.
//   - sub-fila de worktree, o repo sin worktrees → no-op: no hay nada que
//     plegar, y plegar el grupo del padre desde la sub-fila sería una sorpresa.
//
// Los dos estados se persisten en el mismo `collapsed.json` (el de worktrees bajo
// su propio prefijo), así que el plegado sobrevive entre sesiones igual que
// antes.
func (m Model) toggleFold() (tea.Model, tea.Cmd) {
	e, ok := entryAt(m.entries(), m.cursor)
	if !ok {
		return m, nil
	}
	switch e.kind {
	case kindPrimary, kindSecondary:
		m.collapsed[e.group] = !m.collapsed[e.group]
	case kindRepo:
		if !m.expandable(e.r) {
			return m, nil
		}
		m.expanded[e.r.project.Path] = !m.expanded[e.r.project.Path]
	default: // sub-fila de worktree
		return m, nil
	}
	m.clampCursor()
	m.saveCollapsed()
	return m, nil
}

// commandActions son las acciones que acaban en un proceso. El resto (filtro,
// búsqueda, plegado, el panel del log, salir) solo mueven la vista, así que no
// dejan entrada en el command log: registrar "pulsé enter para plegar" no aporta
// nada sobre qué comandos se ejecutan.
var commandActions = map[string]bool{
	"fetch": true, "fetch_all": true, "pull": true, "push": true,
	"lazygit": true, "editor": true, "rescan": true, "recollect": true,
	"command": true, "worktree_remove": true,
}

// launchesCommand reporta si la acción acaba en un proceso.
func launchesCommand(action string) bool { return commandActions[action] }

// rowActions son las que además necesitan una fila: sin fila bajo el cursor no
// se despachan, así que tampoco dejan intención (pulsar `p` sobre un header de
// grupo no es un comando que alguien quisiera auditar).
var rowActions = map[string]bool{
	"fetch": true, "pull": true, "push": true, "lazygit": true,
	"editor": true, "recollect": true, "command": true, "worktree_remove": true,
}

// actionNeedsRow reporta si la acción requiere una fila seleccionada.
func actionNeedsRow(action string) bool { return rowActions[action] }

// actionForKey resuelve la acción para una tecla dada usando el mapa
// de keybindings configurado. Si no hay match, devuelve "".
func (m Model) actionForKey(key string) string {
	for action, k := range m.cfg.Keybindings {
		if k == key {
			return action
		}
	}
	return ""
}

// handleWorktreeRemove gestiona la acción de borrado de worktree. Solo actúa
// sobre una sub-fila de worktree con padre válido; si el cursor no está en una,
// avisa y no arma nada. Sobre una sub-fila válida: si ya hay una confirmación
// armada sobre esa misma sub-fila, ejecuta el borrado con el nivel de forzado
// armado; si no, arma la confirmación normal. La rama del worktree nunca se
// toca.
func (m Model) handleWorktreeRemove() (tea.Model, tea.Cmd) {
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree || e.parent == "" || e.wt.Path == "" {
		m.armed = nil
		return m, m.toastCmd(toastInfo, "select a worktree")
	}
	if m.armed != nil && m.armed.matches(e.parent, e.wt.Path) {
		// Segunda pulsación sobre la misma sub-fila: si el padre está libre, se
		// lanza el borrado con el nivel de forzado armado; el banner se limpia
		// mientras la acción está en vuelo y se marca el token del intento.
		if cmd := m.busyActionCmd(m.armed.parent); cmd != nil {
			return m, cmd // el armado no se rompe: no se toca
		}
		armed := *m.armed
		m.removeGen++
		if m.removeTokens == nil {
			m.removeTokens = map[string]int{}
		}
		m.removeTokens[armed.parent] = m.removeGen
		m.armed = nil
		// La intención del borrado: la pulsación que lo confirma, no la que
		// lo arma (esa ya quedó registrada por el enrutado de la acción).
		cmdlog.RecordIntent(cmdlog.Entry{
			Class:  cmdlog.ClassAction,
			Repo:   filepath.Base(armed.wtPath),
			Dir:    armed.wtPath,
			Key:    m.cfg.KeyFor("worktree_remove"),
			Action: "worktree_remove",
		})
		return m, m.removeWorktreeCmd(armed.parent, armed.wtPath, armed.name, armed.force, m.removeGen)
	}
	// Primera pulsación o re-armado sobre la sub-fila actual: nunca se borra
	// un worktree distinto al que se armó.
	m.armed = &armedRemoval{
		wtPath: e.wt.Path,
		parent: e.parent,
		name:   filepath.Base(e.wt.Path),
	}
	return m, nil
}

// pullPrompt compone el aviso persistente del selector de variante de pull.
// Se resolución es por tecla (p/r/f/m), no por flechas: el set es corto y
// fijo, y una lista navegable obligaría a dos teclas extra para la variante que
// se usa el 90% de las veces. El aviso sobrevive a los toasts porque es el
// único sitio donde se anuncia qué hace cada tecla, y se pinta en keybinds
// (sustituyendo las hints) porque comparte función con ellas.
func (m Model) pullPrompt() string {
	if m.pullArmed == nil {
		return ""
	}
	// Las etiquetas salen de pullVariantLabel, no de hardcodearlas: la tecla
	// y el nombre de la variante tienen que ser la misma fuente que la del
	// hint bar, o el prompt miente cuando algo se reescribe.
	variants := make([]string, 0, len(PullKinds))
	for _, k := range []string{"p", "r", "f", "m"} {
		kind := PullKinds[k]
		variants = append(variants, fmt.Sprintf("%s %s", k, pullVariantLabel(kind)))
	}
	return fmt.Sprintf("pull %s: %s · esc cancel", m.nameOf(m.pullArmed.path), strings.Join(variants, " · "))
}

// pullVariantLabel nombra una variante de pull para el prompt y los hints.
func pullVariantLabel(kind string) string {
	switch kind {
	case "pull":
		return "default"
	case "pull_rebase":
		return "rebase"
	case "pull_ff":
		return "ff-only"
	case "pull_merge":
		return "merge"
	}
	return kind
}

// removePrompt compone el aviso persistente de la confirmación armada. La
// tecla mostrada es la configurada para la acción.
func (m Model) removePrompt() string {
	if m.armed == nil {
		return ""
	}
	key := m.cfg.KeyFor("worktree_remove")
	if m.armed.force {
		return fmt.Sprintf("remove worktree %s? has changes — %s to force, esc to cancel", m.armed.name, key)
	}
	return fmt.Sprintf("remove worktree %s? %s to confirm, esc to cancel", m.armed.name, key)
}

// View compone la pantalla del dashboard con el overlay de toasts en la esquina
// inferior derecha. La ficha del repo bajo el cursor va en su propia sección, así
// que aquí no hay una vista alternativa que componer; el command log sí lo es
// (toma el cuerpo entero) y lo resuelve renderDashboard.
func (m Model) View() tea.View {
	content := m.renderDashboard()
	if toasts := m.toasts.blocksFor(m.width); len(toasts) > 0 {
		content = overlayToasts(content, toasts, m.width, m.height, m.toastReserve())
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderDashboard apila las secciones bordadas del dashboard: stats, filtro,
// tabla, panel con la ficha del repo bajo el cursor y keybinds. Las entradas se
// calculan una vez y se comparten entre la tabla y el panel.
//
// Con el command log abierto el cuerpo NO es la tabla: es el log, y la ficha no
// se dibuja (layout ya le devolvió su alto). El log es una vista a la que se va
// a mirar, no información ambiente como la ficha, así que no se reparte el
// espacio con la tabla: se sustituye.
func (m Model) renderDashboard() string {
	lay := m.layout()
	if m.logOpen {
		return m.compose(lay, m.logSection(lay.bodyLines), "")
	}
	entries := m.entries()
	return m.compose(lay, m.tableSection(lay.bodyLines, entries), m.previewSection(lay, entries))
}

// toastReserve es el alto de la sección de keybinds visible, para que el
// overlay de toasts no la tape.
func (m Model) toastReserve() int {
	lay := m.layout()
	if !lay.showKeybinds {
		return 0
	}
	return keybindsChrome + lay.hintLines
}

// syncOffset ajusta el scroll para que el cursor siga visible.
func (m *Model) syncOffset(total, window int) {
	if total <= window {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+window {
		m.offset = m.cursor - window + 1
	}
}
