package tui

import "strings"

func (m Model) commandOverlay() []string {
	if m.nameOpen {
		content := "  " + m.nameInput.View() + "\n" + styleHint.Render("  enter save · esc cancel")
		return strings.Split(m.section("name · "+m.nameOf(m.namePath), content, pickerBoxWidth(m.width)), "\n")
	}
	if m.cmdOpen {
		content := "  " + m.cmdInput.View() + "\n" + styleHint.Render("  enter run · esc cancel")
		return strings.Split(m.section("command", content, pickerBoxWidth(m.width)), "\n")
	}
	if m.cmdResultOpen == nil {
		return nil
	}
	r := m.cmdResultOpen
	content := styleHint.Render("  $ "+truncate(sanitizeLogText(r.command), pickerBoxWidth(m.width)-6)) + "\n"
	if r.output == "" {
		content += styleHint.Render("  (no output)")
	} else {
		content += indent(clipTo(sanitizeLogText(r.output), max(1, m.height-8)), "  ")
	}
	content += "\n" + styleHint.Render("  enter/esc close")
	return strings.Split(m.section("command output · exit "+r.exit, content, pickerBoxWidth(m.width)), "\n")
}
