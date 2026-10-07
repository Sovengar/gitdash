// Another view mode, not an overlay: it needs scroll and many lines, and covering the table would force reserving its height the way toasts do.
package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gitdash/internal/cmdlog"
)

// Column widths, where the argv gets what is left: in a log the command is what has to be read whole.
const (
	logColTime    = 12
	logColKind    = 7
	logColRepo    = 14
	logColOutcome = 16
	logColVerdict = 8
	logColSep     = 2
	// 20 fits "git pull --ff-only": below that the command is read half-way, which is exactly what this panel exists to prevent.
	logMinArgv = 20
)

// It degrades by value (verdict first, then result and repo, and only then the argv); bordered already clips every line to the inner width, so this is about not wasting terminal, not about avoiding overflows.
type logColumns struct {
	kind, repo, outcome, verdict, argv int
}

func computeLogColumns(inner int) logColumns {
	c := logColumns{kind: logColKind, repo: logColRepo, outcome: logColOutcome, verdict: logColVerdict}
	fixed := func() int {
		return logColTime + logColSep + c.kind + logColSep + c.repo + logColSep + c.outcome + logColSep + c.verdict
	}
	for fixed()+logMinArgv > inner {
		switch {
		case c.verdict > 0:
			c.verdict = 0
		case c.outcome > 0:
			c.outcome = 0
		case c.repo > 0:
			c.repo = 0
		case c.kind > 4:
			// KIND is clipped but never disappears: it is the column that explains the argv ("key p" vs "exec"), and without it the line does not read, which is why the render paints it unguarded (kind is never 0).
			c.kind = 4
		default:
			c.argv = max(0, inner-logColTime-logColSep)
			return c
		}
	}
	c.argv = max(logMinArgv, inner-fixed())
	return c
}

// Without the cached copy every frame would copy 500 entries to paint the last ~30.
func (m *Model) logEntries() []cmdlog.Entry {
	if seq := cmdlog.LastSeq(); seq != m.logCacheSeq {
		m.logCache = cmdlog.Entries()
		m.logCacheSeq = seq
	}
	if m.logShowAll {
		return m.logCache
	}
	// By default only the user's actions: the scan's reads are ~4 per repo and would repeat the same question.
	out := make([]cmdlog.Entry, 0, len(m.logCache))
	for _, e := range m.logCache {
		if e.Class == cmdlog.ClassAction {
			out = append(out, e)
		}
	}
	return out
}

// The offset counts from the tail (0 = most recent at the bottom), which is where a log is read, and new entries do not move you if you were scrolled up.
func (m *Model) logSection(bodyLines int) string {
	entries := m.logEntries()
	cols := computeLogColumns(max(0, m.width-2))

	visible := max(1, bodyLines-1)
	// No offset clamp here: logScroll already leaves it in [0, len(entries)-visible] on every keystroke and `start` bounds it again, so a second `min` was arithmetic mutation could not tell apart (mutating the 0 of the max does not move a single row).
	start := max(0, len(entries)-visible-m.logOffset)

	rows := make([]string, 0, visible)
	for _, e := range entries[start : start+min(visible, len(entries)-start)] {
		rows = append(rows, m.logLine(e, cols))
	}
	// rellenaHasta instead of `for len(rows) < visible`, because comparing and appending turns the `<`->`>=` mutant into a hang (see rellenaHasta in sections.go).
	rows = rellenaHasta(rows, visible)

	title := "log · actions"
	if m.logShowAll {
		title = "log · all"
	}
	header := "  " + m.logHeader(cols)
	body := styleHint.Render(header) + "\n" + strings.Join(rows, "\n")
	return m.section(title, body, m.width)
}

func (m Model) logHeader(c logColumns) string {
	var b strings.Builder
	b.WriteString(pad("TIME", logColTime))
	b.WriteString(pad("KIND", c.kind) + strings.Repeat(" ", logColSep))
	if c.repo > 0 {
		b.WriteString(pad("REPO", c.repo) + strings.Repeat(" ", logColSep))
	}
	if c.argv > 0 {
		b.WriteString(pad("COMMAND", c.argv) + strings.Repeat(" ", logColSep))
	}
	if c.outcome > 0 {
		b.WriteString(pad("RESULT", c.outcome) + strings.Repeat(" ", logColSep))
	}
	if c.verdict > 0 {
		b.WriteString("VERDICT")
	}
	return b.String()
}

// The intent is painted faint and with no verdict (context for "what you asked", not a result), and everything from outside goes through sanitizeLogText because an entry must take exactly ONE line free of injected control characters.
func (m Model) logLine(e cmdlog.Entry, c logColumns) string {
	style := styleLogExec
	if e.Intent {
		style = styleLogIntent
	}
	var b strings.Builder
	b.WriteString(style.Render(pad(e.At.Format("15:04:05.000"), logColTime)))
	kind := "key " + e.Key
	if !e.Intent {
		kind = "exec"
	}
	b.WriteString("  " + style.Render(pad(truncate(sanitizeLogText(kind), c.kind), c.kind)))
	if c.repo > 0 {
		b.WriteString("  " + style.Render(pad(truncate(sanitizeLogText(e.Repo), c.repo), c.repo)))
	}
	if c.argv > 0 {
		cmdline := e.Action
		if !e.Intent {
			cmdline = e.Command()
		}
		cmdline = sanitizeLogText(cmdline)
		if cmdline == "" {
			cmdline = "-"
		}
		b.WriteString("  " + style.Render(pad(truncate(cmdline, c.argv), c.argv)))
	}
	if c.outcome > 0 {
		outcome := e.Outcome
		if outcome == "" && !e.Intent {
			outcome = "-"
		}
		b.WriteString("  " + m.logOutcomeStyle(e).Render(pad(truncate(sanitizeLogText(outcome), c.outcome), c.outcome)))
	}
	if c.verdict > 0 {
		b.WriteString("  " + m.logVerdictStyle(e).Render(pad(truncate(logVerdict(e), c.verdict), c.verdict)))
	}
	return b.String()
}

// Control and format characters, the U+2028/U+2029 separators and escape sequences are removed because they reorder or split the line visually; the argv carries the untrusted marker prompt, and this only affects painting, not the record.
func sanitizeLogText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = skipEscape(s, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		// else-if and not switch: the order IS the rule (a whitespace collapses, a control is dropped, everything else is painted), and this way each condition sits in a block the instrument can measure.
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		case r == utf8.RuneError && size <= 1:
		case unicode.IsControl(r):
		case unicode.Is(unicode.Cf, r):
		case r == '\u2028' || r == '\u2029':
		default:
			b.WriteRune(r)
			lastSpace = r == ' '
		}
	}
	return b.String()
}

// An unterminated sequence eats the rest: losing the tail beats leaving an escape fragment.
func skipEscape(s string, i int) int {
	j := i + 1
	if j >= len(s) {
		return j
	}
	switch s[j] {
	case '[':
		j++
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		// min() and not a "j++": an unterminated sequence leaves j == len(s), so the returned index can never pass the end of the string.
		return min(j+1, len(s))
	case ']':
		// The scan starts at the FIRST payload byte: neither ESC nor ']' terminates, but the payload is part of the sequence, so skipping further would lose legitimate argv text.
		j = i + 2
		for j < len(s) {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return j
	default:
		return j + 1
	}
}

func (m Model) logOutcomeStyle(e cmdlog.Entry) lipglossStyle {
	if e.Outcome == "" {
		return styleLogIntent
	}
	switch e.Outcome {
	case "rebase", "rebase+autostash", "merge", "fast-forward", "pushed":
		return styleLogOK
	case "up-to-date":
		return styleHint
	default:
		return styleError
	}
}

func (m Model) logVerdictStyle(e cmdlog.Entry) lipglossStyle {
	if e.Exit == 0 {
		return styleHint
	}
	return styleError
}

func logVerdict(e cmdlog.Entry) string {
	if e.Intent {
		return ""
	}
	if e.Exit > 0 {
		return fmt.Sprintf("exit %d", e.Exit)
	}
	if e.Exit < 0 && e.Dur == 0 {
		return "no exit"
	}
	if e.Dur >= time.Second {
		return fmt.Sprintf("%.1fs", e.Dur.Seconds())
	}
	if e.Dur == 0 {
		return ""
	}
	return fmt.Sprintf("%dms", e.Dur.Milliseconds())
}

// It shares the keybinds section because it shares its function with the hints ("what do I do now"), and its visibility is forced: without it the panel would be plain text with no explanation of how to navigate.
func (m Model) logLegend() string {
	toggle := "show all (reads + auto fetch)"
	if m.logShowAll {
		toggle = "show actions only"
	}
	keys := m.cfg.KeyFor("log")
	return fmt.Sprintf("command log: j/k scroll · a %s · %s or esc back", toggle, keys)
}

// The offset is clipped against the visible line count, which depends on the section's real height: a bigger offset would just leave blank lines at the bottom.
func (m *Model) logScroll(n, visible int) {
	maxOffset := max(0, len(m.logEntries())-visible)
	m.logOffset = min(max(0, m.logOffset+n), maxOffset)
}

func (m *Model) handleLogKey(key string, bodyLines int) bool {
	visible := max(1, bodyLines-1)
	switch key {
	case "j", "down":
		m.logScroll(-1, visible)
		return true
	case "k", "up":
		m.logScroll(1, visible)
		return true
	case "a":
		m.logShowAll = !m.logShowAll
		m.logOffset = 0
		return true
	case "esc":
		m.logOpen = false
		return true
	default:
		action := m.actionForKey(key)
		if action == "log" {
			m.logOpen = false
			return true
		}
	}
	return false
}

// Opening and closing both drop the armed states: navigating away makes their warning meaningless, and inside the panel `a` is "show all", so an armed pull selector would hijack the key.
func (m *Model) toggleLog() {
	m.logOpen = !m.logOpen
	m.armed = nil
	m.pullArmed = nil
	m.visualArmed = nil
	if m.logOpen {
		m.logOffset = 0
	}
}
