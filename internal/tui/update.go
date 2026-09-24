// Update y View del modelo gitdash.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cache"
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
			if c := m.fetchBatchCmd(m.fetchTargets()); c != nil {
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
		m.lastAction[msg.path] = actionResult{kind: msg.kind, output: msg.output, err: msg.err}
		note := actionNote(msg.kind, m.nameOf(msg.path), msg.output, msg.err)
		if msg.err != "" {
			m.toasts.showError(note)
		} else {
			m.toasts.showSuccess(note)
		}
		return m.withPump(nil)

	case worktreeRemovedMsg:
		// Liberar el running del padre solo si corresponde a este intento: si
		// ya lo reemplazó otro (cancelado y reintentado, o una acción distinta)
		// no se pisa.
		if m.running[msg.parent] == "worktree_remove" && (m.removeToken == 0 || msg.gen == m.removeToken) {
			delete(m.running, msg.parent)
		}
		if msg.gen == 0 || msg.gen != m.removeToken {
			// Resultado obsoleto (se canceló con esc o lo sustituyó otro
			// intento): no toca el banner ni re-arma el forzado.
			return m.withPump(nil)
		}
		m.removeToken = 0
		m.lastAction[msg.parent] = actionResult{kind: "worktree_remove", output: msg.output, err: msg.err}
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

// actionNote compone la notificación de una acción terminada (pull/push/sync).
// En el fallo incluye el motivo real de git (errStr, ya resumido) y, cuando se
// reconoce, un hint accionable; la salida completa queda en el detalle.
func actionNote(kind, name, output, errStr string) string {
	if errStr == "" {
		return fmt.Sprintf("%s ok %s", kind, name)
	}
	hint := ""
	if kind == "pull" || kind == "sync" {
		switch {
		case strings.Contains(output, "Not possible to fast-forward"),
			strings.Contains(output, "divergent"):
			hint = " — diverged? pull --rebase manual"
		case strings.Contains(errStr, "no tracking information"),
			strings.Contains(errStr, "no upstream"):
			hint = " — no upstream; set it with git branch --set-upstream-to"
		}
	}
	return fmt.Sprintf("%s failed %s: %s%s", kind, name, errStr, hint)
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

	// Un borrado en vuelo se cancela de forma definitiva con esc: se invalida
	// su token para que un resultado tardío no re-arme el forzado ni toque el
	// banner. El esc sigue su curso normal (cerrar detalle, etc.).
	if key == "esc" && m.removeToken != 0 {
		m.removeToken = 0
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
		if m.detailOpen {
			m.detailOpen = false
		}
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
			return m, m.fetchBatchCmd([]string{r.project.Path})
		}
	case "fetch_all":
		paths := m.fetchTargets()
		if len(paths) == 0 {
			return m, m.toastCmd(toastInfo, "no repositories with upstream to fetch")
		}
		return m, m.fetchBatchCmd(paths)
	case "pull":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "pull")
		}
	case "sync":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "sync")
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
	case "update":
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
			}
			return m, m.openUpdateCmd(r.project.Path)
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
		// Plegar/desplegar el contenedor bajo
		// el cursor (secundario interno para repos, header si es header).
		// sobre una sub-fila de worktree es no-op (no debe plegar
		// la sección (ungrouped) por accidente).
		entries := m.entries()
		if len(entries) == 0 || m.cursor >= len(entries) {
			return m, nil
		}
		if entries[m.cursor].kind == kindWorktree {
			return m, nil
		}
		g := groupOfEntry(entries[m.cursor])
		m.collapsed[g] = !m.collapsed[g]
		m.clampCursor()
		m.saveCollapsed()
		return m, nil
	case "expand":
		// `space` alterna las sub-filas de worktree del repo bajo
		// el cursor. No-op sobre headers de grupo (selected() false), sobre
		// sub-filas de worktree y sobre repos sin worktrees / no-repo.
		r, ok := m.selected()
		if !ok || !m.expandable(r) {
			return m, nil
		}
		m.expanded[r.project.Path] = !m.expanded[r.project.Path]
		m.clampCursor()
		m.saveCollapsed()
		return m, nil
	case "worktree_remove":
		return m.handleWorktreeRemove()
	case "detail":
		entries := m.entries()
		if len(entries) > 0 && m.cursor < len(entries) {
			switch entries[m.cursor].kind {
			case kindPrimary, kindSecondary: // enter en header pliega
				g := entries[m.cursor].group
				m.collapsed[g] = !m.collapsed[g]
				m.clampCursor()
				m.saveCollapsed()
				return m, nil
			}
		}
		if _, ok := m.selected(); ok {
			m.detailOpen = true
		}
	case "command":
		// `!` abre el input de comandos del detalle ($SHELL -c capturado;
		// enter vacío = shell interactiva).
		if m.detailOpen {
			if r, ok := m.selected(); !ok || !r.project.HasRepo {
				return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
			}
			m.cmdOpen = true
			return m, m.cmdInput.Focus()
		}
	}
	return m, nil
}

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
		m.removeToken = m.removeGen
		m.armed = nil
		return m, m.removeWorktreeCmd(armed.parent, armed.wtPath, armed.name, armed.force, m.removeToken)
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

// View compone la pantalla: dashboard o detalle, con el overlay de toasts en
// la esquina inferior derecha. El detalle usa la fila viva bajo el cursor; si
// el filtro la hizo desaparecer, cae al dashboard.
func (m Model) View() tea.View {
	var content string
	if m.detailOpen {
		lay := m.layout()
		if e, ok := m.selectedEntry(); ok && e.kind == kindWorktree {
			// Detalle dedicado del worktree (sin inventar estado).
			content = m.compose(lay, m.detailSection(worktreeTitle(e), m.renderWorktreeDetail(e), lay.bodyLines))
		} else if r, ok := m.selected(); ok {
			content = m.compose(lay, m.detailSection(detailTitle(r), m.renderDetail(r), lay.bodyLines))
		}
	}
	if content == "" {
		content = m.renderDashboard()
	}
	if toasts := m.toasts.blocksFor(m.width); len(toasts) > 0 {
		content = overlayToasts(content, toasts, m.width, m.height, m.toastReserve())
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderDashboard apila las secciones bordadas del dashboard.
func (m Model) renderDashboard() string {
	lay := m.layout()
	return m.compose(lay, m.tableSection(lay.bodyLines))
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
