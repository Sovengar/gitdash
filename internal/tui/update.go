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
		name := m.nameOf(msg.path)
		note := fmt.Sprintf("%s ok %s", msg.kind, name)
		if msg.err != "" {
			hint := ""
			if msg.kind == "pull" &&
				(strings.Contains(msg.err, "divergent") || strings.Contains(msg.err, "Not possible to fast-forward")) {
				hint = " — diverged? pull --rebase manual" // S9.2
			}
			note = fmt.Sprintf("%s failed %s%s", msg.kind, name, hint)
		}
		return m.withPump(m.notifyCmd(note))

	case editorDoneMsg:
		delete(m.running, msg.path)
		if msg.err != nil {
			return m, m.notifyCmd(fmt.Sprintf("editor: %v", msg.err))
		}
		return m, m.recollectCmd(msg.path) // S9.4: re-colecciona al salir

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
	n := len(m.rows())
	if m.cursor >= n {
		m.cursor = max(0, n-1)
	}
}

// handleKey enruta las teclas: input de búsqueda primero, luego la tabla.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

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

	switch key {
	case "q", "ctrl+c":
		m.cancel()
		return m, tea.Quit
	case "down", "j":
		m.cursor = min(m.cursor+1, max(0, len(m.rows())-1))
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, len(m.rows())-1)
	case "n":
		m.onlyDirty = !m.onlyDirty // S7.1
		m.clampCursor()
	case "/":
		m.searchActive = true
		m.searchInput.SetValue(m.search)
		m.searchInput.Focus()
	case "r":
		if m.scanning {
			return m, m.notifyCmd("scan already running")
		}
		return m, m.startScanCmd() // S7.3
	case "R":
		if r, ok := m.selected(); ok {
			return m, m.recollectCmd(r.project.Path) // S7.4
		}
	case "f":
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.notifyCmd("no git repo — nothing to do") // S9.6
			}
			return m, m.fetchBatchCmd([]string{r.project.Path}) // S8.5
		}
	case "F":
		paths := m.fetchTargets()
		if len(paths) == 0 {
			return m, m.notifyCmd("no repositories with upstream to fetch")
		}
		return m, m.fetchBatchCmd(paths) // S8.5
	case "p":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.notifyCmd("no git repo — nothing to do") // S9.6
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "pull") // S9.1
		}
	case "P":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.notifyCmd("no git repo — nothing to do")
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "push") // S9.3
		}
	case "e":
		if r, ok := m.selected(); ok {
			if r.project.MarkerErr != "" {
				return m, m.notifyCmd("marker error — fix .repo.toml first")
			}
			return m, m.openEditorCmd(r.project.Path) // S9.4
		}
	case "enter":
		if _, ok := m.selected(); ok {
			m.detailOpen = true // S10.1
		}
	case "esc":
		if m.detailOpen {
			m.detailOpen = false // S10.1
		}
	}
	return m, nil
}

// View compone la pantalla: tabla o detalle (R6, R10). El detalle usa la
// fila viva bajo el cursor; si el filtro la hizo desaparecer, cae a la tabla.
func (m Model) View() tea.View {
	content := m.renderDashboard()
	if m.detailOpen {
		if r, ok := m.selected(); ok {
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
	if m.search != "" {
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

	// cabecera
	header := "  " + pad("NAME", colName) + pad("GROUP", colGroup) + pad("BRANCH", colBranch) +
		pad("STATE", colState) + pad("↑↓", colUpDown) + pad("ACTIVITY", colActivity) + pad("FETCH", colFetch)
	b.WriteString(styleHint.Render(header) + "\n")

	// filas con scroll
	rows := m.rows()
	bodyLines := max(1, m.height-5)
	if m.scanNote != "" {
		bodyLines--
	}
	m.syncOffset(len(rows), bodyLines)
	for i := m.offset; i < min(len(rows), m.offset+bodyLines); i++ {
		line := m.renderRow(rows[i], i == m.cursor)
		b.WriteString(line + "\n")
	}
	if len(rows) == 0 && !m.scanning {
		hint := "no repositories — create a .repo.toml in your projects"
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

// renderBar pinta notificación o run-states y la línea de hints.
func (m Model) renderBar() string {
	var b strings.Builder

	left := ""
	if m.notify != "" {
		left = styleWarn.Render(m.notify)
	} else {
		if m.scanning || m.fetchingAll() {
			left = m.spinner.View() + " working  "
		}
		for path, kind := range m.running {
			if kind == "pull" || kind == "push" {
				left += styleFetchRun.Render(fmt.Sprintf("%s %s…  ", kind, m.nameOf(path)))
			}
		}
	}
	if left != "" {
		b.WriteString(left + "\n")
	}

	hints := "j/k move · n dirty · / filter · f/F fetch · p pull · P push · e edit · r rescan · enter detail · q quit"
	b.WriteString(styleHint.Render(truncate(hints, max(40, m.width))) + "\n")
	return b.String()
}
