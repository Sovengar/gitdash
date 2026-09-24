// Update y View del modelo gitdash.
package tui

import (
	"fmt"
	"strings"
	"time"

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
		cmds := []tea.Cmd{tickCmd()}
		if time.Now().After(m.notifyUntil) && m.notify != "" {
			m.notify = ""
		}
		return m, tea.Batch(cmds...)

	case scanProjectsMsg:
		m.projects = msg.projects
		m.scanNote = msg.note
		m.clampCursor()
		return m.withPump(nil)

	case statusMsg:
		m.states[msg.path] = msg.snap
		delete(m.running, msg.path)
		return m.withPump(nil)

	case collectDoneMsg:
		m.scanning = false
		cmds := []tea.Cmd{}
		// cache best-effort al final de cada rescan (R11)
		if path, err := cache.Path(); err == nil {
			projects := m.projects
			go cache.Save(path, projects)
		}
		// fetch automático en batches (S8.1)
		if m.cfg.FetchAuto {
			if c := m.fetchBatchCmd(m.fetchTargets()); c != nil {
				cmds = append(cmds, c)
			}
		}
		return m.withPump(tea.Batch(cmds...))

	case fetchStateMsg:
		m.fetchStates[msg.path] = msg.state
		if msg.state == "failed" {
			return m.withPump(m.notifyCmd(fmt.Sprintf("fetch failed %s: %s", m.nameOf(msg.path), msg.err)))
		}
		return m.withPump(nil)

	case fetchDoneMsg:
		var note string
		switch {
		case msg.failed > 0:
			note = fmt.Sprintf("fetch: %d ok, %d failed", msg.ok, msg.failed)
		case msg.ok == 1:
			note = "fetch ok"
		default:
			note = fmt.Sprintf("fetch ok (%d repos)", msg.ok)
		}
		return m.withPump(m.notifyCmd(note))

	case actionMsg:
		delete(m.running, msg.path)
		m.lastAction[msg.path] = actionResult{kind: msg.kind, output: msg.output, err: msg.err}
		return m.withPump(m.notifyCmd(actionNote(msg.kind, m.nameOf(msg.path), msg.output, msg.err)))

	case execDoneMsg:
		delete(m.running, msg.path)
		// el handoff pudo cambiar el estado del repo: siempre re-colecta
		cmd := m.recollectCmd(msg.path)
		if msg.err != nil {
			return m, tea.Batch(cmd, m.notifyCmd(fmt.Sprintf("command: %v", msg.err)))
		}
		return m, cmd

	case cmdResultMsg:
		delete(m.running, msg.path)
		verdict := "ok"
		if msg.exit != "0" {
			verdict = "exit " + msg.exit
		}
		m.lastCmd[msg.path] = cmdResult{command: msg.command, output: msg.output, exit: msg.exit}
		return m.withPump(m.notifyCmd(fmt.Sprintf("! %s — %s", msg.command, verdict)))

	case notifyMsg:
		m.notify = msg.text
		m.notifyUntil = time.Now().Add(3 * time.Second)
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

	if m.cmdOpen { // modo comando del detalle: prioridad sobre todo
		switch key {
		case "enter":
			cmdStr := strings.TrimSpace(m.cmdInput.Value())
			m.cmdOpen = false
			m.cmdInput.Blur()
			m.cmdInput.SetValue("")
			if r, ok := m.selected(); ok {
				if !r.project.HasRepo {
					return m, m.notifyCmd("no git repo — nothing to do")
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
			m.search = strings.TrimSpace(m.searchInput.Value()) // S7.2 confirmar
			m.clampCursor()
			return m, nil
		case "esc":
			m.searchActive = false
			m.searchInput.Blur()
			if strings.TrimSpace(m.searchInput.Value()) == "" {
				m.search = "" // S7.2: esc en input vacío limpia el filtro
			}
			m.clampCursor()
			return m, nil
		default:
			in, cmd := m.searchInput.Update(msg)
			m.searchInput = in
			m.search = strings.TrimSpace(m.searchInput.Value()) // S7.2 en vivo
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
			m.detailOpen = false // S10.1
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
		m.onlyDirty = !m.onlyDirty // S7.1
		m.clampCursor()
	case "search":
		m.searchActive = true
		m.searchInput.SetValue(m.search)
		return m, m.searchInput.Focus()
	case "fetch":
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.notifyCmd("no git repo — nothing to do") // S9.6
			}
			return m, m.fetchBatchCmd([]string{r.project.Path}) // S8.5
		}
	case "fetch_all":
		paths := m.fetchTargets()
		if len(paths) == 0 {
			return m, m.notifyCmd("no repositories with upstream to fetch")
		}
		return m, m.fetchBatchCmd(paths) // S8.5
	case "pull":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.notifyCmd("no git repo — nothing to do") // S9.6
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "pull") // S9.1
		}
	case "sync":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.notifyCmd("no git repo — nothing to do")
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "sync")
		}
	case "push":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.notifyCmd("no git repo — nothing to do")
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "push") // S9.3
		}
	case "editor":
		if r, ok := m.selected(); ok {
			if r.project.MarkerErr != "" {
				return m, m.notifyCmd("marker error — fix .gitdash.toml first")
			}
			return m, m.openEditorCmd(r.project.Path) // S9.4
		}
	case "lazygit":
		// 0005: g abre lazygit en el repo bajo el cursor.
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.notifyCmd("no git repo — nothing to do")
			}
			return m, m.openLazygitCmd(r.project.Path)
		}
	case "update":
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.notifyCmd("no git repo — nothing to do")
			}
			return m, m.openUpdateCmd(r.project.Path)
		}
	case "rescan":
		if m.scanning {
			return m, m.notifyCmd("scan already running")
		}
		return m, m.startScanCmd() // S7.3
	case "recollect":
		if r, ok := m.selected(); ok {
			return m, m.recollectCmd(r.project.Path) // S7.4
		}
	case "fold":
		// 0002 R16/S16.2, 0003 S20.3: plegar/desplegar el contenedor bajo
		// el cursor (secundario interno para repos, header si es header).
		// 0006 R39: sobre una sub-fila de worktree es no-op (no debe plegar
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
		// 0006 R30: `space` alterna las sub-filas de worktree del repo bajo
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
	case "detail":
		entries := m.entries()
		if len(entries) > 0 && m.cursor < len(entries) {
			switch entries[m.cursor].kind {
			case kindPrimary, kindSecondary: // R16/R20: enter en header pliega
				g := entries[m.cursor].group
				m.collapsed[g] = !m.collapsed[g]
				m.clampCursor()
				m.saveCollapsed()
				return m, nil
			}
		}
		if _, ok := m.selected(); ok {
			m.detailOpen = true // S10.1
		}
	case "command":
		// `!` abre el input de comandos del detalle ($SHELL -c capturado;
		// enter vacío = shell interactiva).
		if m.detailOpen {
			if r, ok := m.selected(); !ok || !r.project.HasRepo {
				return m, m.notifyCmd("no git repo — nothing to do")
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

// View compone la pantalla: tabla o detalle (R6, R10). El detalle usa la
// fila viva bajo el cursor; si el filtro la hizo desaparecer, cae a la tabla.
func (m Model) View() tea.View {
	content := m.renderDashboard()
	if m.detailOpen {
		if e, ok := m.selectedEntry(); ok && e.kind == kindWorktree {
			// 0006 R33.6: detalle dedicado del worktree (sin inventar estado).
			content = m.renderWorktreeDetail(e)
		} else if r, ok := m.selected(); ok {
			content = m.renderDetail(r)
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderDashboard compone título, cabecera, filas y barras.
func (m Model) renderDashboard() string {
	var b strings.Builder

	// título + resumen + estado
	total, dirty, ahead, behind := m.summary()
	title := styleTitle.Render("gitdash") +
		styleBar.Render(fmt.Sprintf("  %d repos · %d dirty · %d ahead · %d behind", total, dirty, ahead, behind))
	flags := ""
	if m.onlyDirty {
		flags += styleWarn.Render(" [dirty]")
	}
	if m.searchActive {
		// S7.2: feedback inmediato al pulsar / — [/|] con cursor sólido y
		// placeholder estático (el typewriter animado de bubbles se queda
		// en el primer carácter sin ticks)
		in := m.searchInput
		in.Placeholder = ""
		flags += styleWarn.Render(" [") + in.View()
		if in.Value() == "" {
			flags += styleDim.Render(searchPlaceholder)
		}
		flags += styleWarn.Render("]")
	} else if m.search != "" {
		flags += styleWarn.Render(" [/" + m.search + "]")
	}
	status := ""
	if m.scanning {
		status = " " + m.spinner.View() + " scanning"
	} else if m.fetchingAll() {
		status = " " + m.spinner.View() + " fetching"
	}
	b.WriteString(title + flags + status + "\n")
	if m.scanNote != "" {
		b.WriteString(styleError.Render("roots: "+m.scanNote) + "\n")
	}

	// cabecera (0004 R25: sin GROUP — los headers plegables ya lo dicen —,
	// Work Tree en vez de STATE y ↑↓up explícito; anchos > headers →
	// siempre hay separador, nunca "ACTIVITYFETCH")
	header := "  " + pad("NAME", colName) + pad("BRANCH", colBranch) +
		pad("Work Tree", colWT) + pad("↑↓up", colUpDown) + pad("SYNC", colSync) +
		pad("ACTIVITY", colActivity) + pad("FETCH", colFetch)
	b.WriteString(styleHint.Render(header) + "\n")

	// filas con scroll (headers de grupo incluidos, 0002 R16)
	entries := m.entries()
	// altura del bar: 1 separador + 1 notify/status + 3 hints lines
	barLines := 4 // separator + 3 hint rows
	if m.notify != "" || m.scanning || m.fetchingAll() {
		barLines++
	}
	for _, kind := range m.running {
		if kind == "pull" || kind == "push" || kind == "sync" {
			barLines++
			break
		}
	}
	bodyLines := max(1, m.height-1-barLines)
	if m.scanNote != "" {
		bodyLines--
	}
	m.syncOffset(len(entries), bodyLines)
	for i := m.offset; i < min(len(entries), m.offset+bodyLines); i++ {
		b.WriteString(m.renderEntry(entries[i], i == m.cursor) + "\n")
	}
	if len(entries) == 0 && !m.scanning {
		hint := "no repositories — create a .gitdash.toml in your projects"
		if m.search != "" || m.onlyDirty {
			hint = "no repositories match the current filter"
		}
		b.WriteString(styleHint.Render("  "+hint) + "\n")
	}

	// barra inferior
	b.WriteString(m.renderBar())
	return b.String()
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

// renderBar pinta notificación o run-states y las líneas de hints.
func (m Model) renderBar() string {
	var b strings.Builder

	// separador visual entre contenido y barra
	b.WriteString(styleSeparator.Render(strings.Repeat("─", m.width)) + "\n")

	left := ""
	if m.notify != "" {
		// truncate ANTES del estilo: el ANSI rompe el cálculo de ancho.
		left = styleWarn.Render(truncate(m.notify, max(40, m.width)))
	} else {
		if m.scanning || m.fetchingAll() {
			left = m.spinner.View() + " working  "
		}
		for path, kind := range m.running {
			if kind == "pull" || kind == "push" || kind == "sync" {
				left += styleFetchRun.Render(fmt.Sprintf("%s %s…  ", kind, m.nameOf(path)))
			}
		}
	}
	if left != "" {
		b.WriteString(left + "\n")
	}

	for _, line := range m.cfg.HintBarLines() {
		b.WriteString(styleHint.Render(truncate(line, max(40, m.width))) + "\n")
	}
	return b.String()
}
