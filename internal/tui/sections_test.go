package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

func layoutTest(height int, hasFilter bool, keybinds int, keep bool) layout {
	return computeLayout(height, hasFilter, keybinds, keep, 0)
}

func TestDashboardSectionsBordered(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	out := m.renderDashboard()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Errorf("lines = %d, want %d (the exact height of the terminal)", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d width = %d, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}
	flat := stripANSI(out)
	for _, title := range []string{"╭ gitdash ", "╭ repos ", "╭ keybinds "} {
		if !strings.Contains(flat, title) {
			t.Errorf("the section %q is missing:\n%s", title, flat)
		}
	}
	for _, glyph := range []string{"╭", "╮", "╰", "╯"} {
		if !strings.Contains(flat, glyph) {
			t.Errorf("the rounded glyph %q is missing", glyph)
		}
	}
}

func TestTableScrollsInItsSection(t *testing.T) {
	var projects []discovery.Project
	states := map[string]gitstatus.Snapshot{}
	for i := 0; i < 20; i++ {
		p := proj(fmt.Sprintf("repo%02d", i), fmt.Sprintf("/tmp/repo%02d", i), true)
		projects = append(projects, p)
		states[p.Path] = snapClean()
	}
	m := newTestModel(t, projects, states)
	m.height = 20
	m.cursor = len(m.entries()) - 1

	out := stripANSI(m.renderDashboard())
	if !strings.Contains(out, "repo19") {
		t.Errorf("the row under the cursor is not visible:\n%s", out)
	}
	if !strings.Contains(out, "╭ gitdash ") || !strings.Contains(out, "╭ keybinds ") {
		t.Errorf("the later sections are no longer visible:\n%s", out)
	}
}

func TestLayoutDegradesInTerminalDrops(t *testing.T) {
	wide := layoutTest(40, false, defaultHintLines, false)
	if !wide.showStats || !wide.showKeybinds || wide.hintLines != defaultHintLines {
		t.Errorf("tall layout: %+v, want everything visible", wide)
	}
	if wide.previewLines < minPreviewLines {
		t.Errorf("previewLines = %d, want >= %d", wide.previewLines, minPreviewLines)
	}
	if want := 40 - (statsSectionLines + tableChrome + keybindsChrome +
		defaultHintLines + previewChrome + wide.previewLines); wide.bodyLines != want {
		t.Errorf("bodyLines = %d, want %d", wide.bodyLines, want)
	}

	narrow := layoutTest(40, false, 1, false)
	if narrow.hintLines != 1 {
		t.Errorf("keybindsLines=1: hintLines = %d, want 1", narrow.hintLines)
	}
	if narrow.previewLines <= wide.previewLines {
		t.Errorf("with 1 hint the panel = %d, want > %d (it takes the free room)",
			narrow.previewLines, wide.previewLines)
	}

	mid := layoutTest(10, false, defaultHintLines, false)
	if mid.hintLines != 1 || !mid.showKeybinds {
		t.Errorf("h=10: %+v, want keybinds with 1 hint", mid)
	}

	short := layoutTest(9, false, defaultHintLines, false)
	if short.showKeybinds || !short.showStats {
		t.Errorf("h=9: %+v, want keybinds hidden and stats visible", short)
	}

	for h := 0; h <= 8; h++ {
		l := layoutTest(h, false, defaultHintLines, false)
		if l.bodyLines < 1 {
			t.Errorf("h=%d: bodyLines = %d, want >= 1", h, l.bodyLines)
		}
		if h <= 6 && l.showStats {
			t.Errorf("h=%d: stats visible on a terminal that is too short: %+v", h, l)
		}
	}

	for h := 0; h <= 8; h++ {
		l := layoutTest(h, false, 1, true)
		if !l.showKeybinds || l.hintLines != 1 {
			t.Errorf("h=%d with keepKeybinds: keybinds degraded: %+v", h, l)
		}
		if l.bodyLines < 1 {
			t.Errorf("h=%d with keepKeybinds: bodyLines = %d, want >= 1", h, l.bodyLines)
		}
	}
	for h := 0; h <= 8; h++ {
		if l := layoutTest(h, false, 1, false); l.hintLines != 0 || l.showKeybinds {
			t.Errorf("h=%d without keepKeybinds: keybinds should drop: %+v", h, l)
		}
	}
}

func TestPreviewPanelIsItLastInFall(t *testing.T) {
	if l := layoutTest(30, false, defaultHintLines, false); l.previewLines == 0 {
		t.Errorf("h=30: no panel: %+v", l)
	}
	// The smallest panel that fits is the card's header, and only if minBodyLines rows are left for the table; the header is detailHeadLines lines, so the minimum derives from it instead of being hardcoded.
	first := minPanelHeight(t)
	if l := layoutTest(first, false, defaultHintLines, false); l.previewLines != detailHeadLines {
		t.Errorf("h=%d: previewLines = %d, want %d", first, l.previewLines, detailHeadLines)
	}
	for h := 14; h < first; h++ {
		l := layoutTest(h, false, defaultHintLines, false)
		if l.previewLines != 0 {
			t.Errorf("h=%d: previewLines = %d, want 0 (the panel cannot ask for more)", h, l.previewLines)
		}
		if !l.showStats || !l.showKeybinds || l.hintLines != defaultHintLines {
			t.Errorf("h=%d: the panel took something that already existed: %+v", h, l)
		}
		want := h - (statsSectionLines + tableChrome + keybindsChrome + defaultHintLines)
		if l.bodyLines != want {
			t.Errorf("h=%d: bodyLines = %d, want %d", h, l.bodyLines, want)
		}
	}
	for h := first; h <= 80; h++ {
		l := layoutTest(h, false, defaultHintLines, false)
		if l.previewLines == 0 {
			continue
		}
		if !l.showStats || !l.showKeybinds || l.hintLines != defaultHintLines {
			t.Errorf("h=%d: panel with previewLines=%d but the rest degraded: %+v", h, l.previewLines, l)
		}
		if l.bodyLines < minBodyLines {
			t.Errorf("h=%d: panel with previewLines=%d and bodyLines=%d: %+v", h, l.previewLines, l.bodyLines, l)
		}
	}
}

func TestLayoutHeightExactInAllTheHeights(t *testing.T) {
	for h := 0; h <= 60; h++ {
		for _, tc := range []struct {
			name      string
			hasFilter bool
			keybinds  int
			keep      bool
		}{
			{"dashboard", false, defaultHintLines, false},
			{"filter", true, defaultHintLines, false},
			{"armed", false, 1, true},
			{"armed+filter", true, 1, true},
			{"no hints", false, 0, false},
		} {
			l := layoutTest(h, tc.hasFilter, tc.keybinds, tc.keep)
			total := l.totalHeight(tc.hasFilter)
			if l.bodyLines < 1 {
				t.Errorf("%s h=%d: bodyLines = %d, want >= 1", tc.name, h, l.bodyLines)
			}
			if total < h {
				t.Errorf("%s h=%d: total height = %d < %d (a gap in the view): %+v",
					tc.name, h, total, h, l)
			}
			if l.bodyLines > 1 && total != h {
				t.Errorf("%s h=%d: total height = %d, want %d: %+v", tc.name, h, total, h, l)
			}
		}
	}
}

// Searched instead of hardcoded because it depends on detailHeadLines: if the card's header grows or shrinks, the floor moves on its own.
func minPanelHeight(t *testing.T) int {
	t.Helper()
	for h := 0; h <= 120; h++ {
		if layoutTest(h, false, defaultHintLines, false).previewLines > 0 {
			return h
		}
	}
	t.Fatal("the panel fits at no height")
	return 0
}

func (l layout) totalHeight(hasFilter bool) int {
	n := tableChrome
	if hasFilter {
		n += filterSectionLines
	}
	if l.showStats {
		n += statsSectionLines
	}
	if l.showKeybinds {
		n += keybindsChrome + l.hintLines
	}
	if l.previewLines > 0 {
		n += previewChrome + l.previewLines
	}
	return n + l.bodyLines
}

// The cases that matter are the EXACT fits, where the section just fits, because that is where a `> ` in the wrong place hides a section that does fit.
func TestLayoutInTheFitExact(t *testing.T) {
	if l := layoutTest(7, false, defaultHintLines, false); !l.showStats {
		t.Errorf("h=7: stats hidden even though they just fit: %+v", l)
	}
	if l := layoutTest(6, false, defaultHintLines, false); l.showStats {
		t.Errorf("h=6: stats visible with no room: %+v", l)
	}

	// The 2/5 is the contract, not a "nearly half" that any calculation could produce.
	const h = 41
	free := h - (tableChrome + statsSectionLines + keybindsChrome + defaultHintLines + previewChrome)
	if want := free * previewShare / 5; want <= minPreviewLines {
		t.Fatalf("h=%d: free=%d gives a share of %d, which does not exercise the 2/5 (it would fall to the minimum)", h, free, want)
	}
	l := layoutTest(h, false, defaultHintLines, false)
	if l.previewLines != free*previewShare/5 {
		t.Errorf("h=%d: previewLines = %d, want %d (2/5 of %d free)", h, l.previewLines, free*previewShare/5, free)
	}
	if want := h - (tableChrome + statsSectionLines + keybindsChrome + defaultHintLines + previewChrome + l.previewLines); l.bodyLines != want {
		t.Errorf("h=%d: bodyLines = %d, want %d", h, l.bodyLines, want)
	}

	// A bigger or smaller table would also "fall fine" in any test only checking that the panel fits, so the edge is looked at closely.
	for h := 14; h <= 40; h++ {
		got := layoutTest(h, false, defaultHintLines, false)
		if got.previewLines == 0 {
			continue
		}
		top := panelHeight(h, tableChrome, 0, defaultHintLines)
		if got.previewLines > top {
			t.Errorf("h=%d: previewLines = %d, want <= %d (the share rules)", h, got.previewLines, top)
		}
		if more := fitLayout(h, tableChrome, 0, got.previewLines+1, defaultHintLines, false); got.previewLines < top &&
			more.bodyLines >= minBodyLines && more.mismaChromeQue(got) {
			t.Errorf("h=%d: the panel stayed at %d when it could reach %d (bodyLines=%d, top=%d)",
				h, got.previewLines, got.previewLines+1, more.bodyLines, top)
		}
		if got.previewLines < top && got.bodyLines != minBodyLines {
			t.Errorf("h=%d: panel at %d below its share %d with bodyLines=%d, want exactly %d",
				h, got.previewLines, top, got.bodyLines, minBodyLines)
		}
	}
}

func TestTerminalNarrowNotBreaks(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width = 24

	out := m.renderDashboard()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Errorf("lines = %d, want %d (no wrap)", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d width = %d, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}
}

func TestSectionFilterConditional(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	if strings.Contains(stripANSI(m.renderDashboard()), "╭ filter ") {
		t.Error("the filter section shows with no active filter")
	}

	m, _ = press(m, "/")
	out := stripANSI(m.renderDashboard())
	if !strings.Contains(out, "╭ filter ") {
		t.Errorf("the filter section did not appear:\n%s", out)
	}
	if !strings.Contains(out, "[/") || !strings.Contains(out, "name/group") {
		t.Errorf("the live input is not shown:\n%s", out)
	}

	m, _ = press(m, "a")
	m, _ = press(m, "enter")
	if out := stripANSI(m.renderDashboard()); !strings.Contains(out, "[/a]") {
		t.Errorf("the confirmed filter is not shown:\n%s", out)
	}

	m, _ = press(m, "/")
	m, _ = press(m, "backspace")
	m, _ = press(m, "esc")
	if strings.Contains(stripANSI(m.renderDashboard()), "╭ filter ") {
		t.Error("the filter section did not disappear when cleared")
	}
}

func TestIndicatorActivityWidthNarrow(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width = 40
	m.running["/tmp/old-clean"] = "pull"

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "pull old-clean") {
		t.Errorf("the running-action indicator does not survive width=40:\n%s", out)
	}
	// "Without duplicating" refers to the indicator: the hint bar legitimately mentions the pull key, so the indicator's label is counted, not the bare word.
	if n := strings.Count(out, "pull old-clean"); n != 1 {
		t.Errorf("the indicator appears %d times, want 1 (no duplication):\n%s", n, out)
	}
	for i, l := range strings.Split(m.View().Content, "\n") {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d width = %d, want %d", i, w, m.width)
		}
	}
}

func TestHeaderSkipsColumnsNarrow(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width = 60
	content := m.renderDashboard()
	out := stripANSI(content)
	for _, gone := range []string{"FETCH", "ACTIVITY"} {
		if strings.Contains(out, gone) {
			t.Errorf("%q is a removed column and must not fit at width=60:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "BRANCH") {
		t.Errorf("basic columns are missing:\n%s", out)
	}
	for i, l := range strings.Split(content, "\n") {
		if w := ansi.StringWidth(l); w > m.width {
			t.Errorf("line %d width = %d > %d", i, w, m.width)
		}
	}
}

func TestToastOfFailureWithHintVisible(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	updated, _ := m.Update(actionMsg{
		path: "/tmp/old-clean", kind: "pull",
		output: "fatal: Not possible to fast-forward, aborting.",
		err:    "exit 1",
	})
	m = updated.(Model)

	rendered := collapse(strings.Join(m.toasts.lines(), "\n"))
	if !strings.Contains(rendered, "diverged") {
		t.Errorf("the hint is not visible in the rendered toast:\n%s", rendered)
	}
}

func TestWithoutLinePermanentOfNotification(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	out := stripANSI(m.renderDashboard())
	if strings.Contains(out, strings.Repeat("─", m.width)) {
		t.Errorf("the bottom bar's separator is still drawn:\n%s", out)
	}

	updated, _ := m.Update(notifyMsg{text: "boom", level: toastError})
	m = updated.(Model)
	if len(m.toasts.toasts) != 1 {
		t.Fatalf("the notification did not become a toast")
	}
	m.toasts.toasts[0].created = time.Now().Add(-toastDuration() - time.Second)
	m.toasts.update()
	if strings.Contains(stripANSI(m.View().Content), "boom") {
		t.Error("the notification stayed on screen permanently")
	}
}

func TestActionGeneratesToast(t *testing.T) {
	projects, states := fixtureProjects()

	ok, _ := newTestModel(t, projects, states).Update(actionMsg{
		path: "/tmp/old-clean", kind: "pull", output: "",
	})
	m := ok.(Model)
	if len(m.toasts.toasts) != 1 || m.toasts.toasts[0].level != toastSuccess {
		t.Fatalf("action ok: %+v, want a success toast", m.toasts.toasts)
	}

	ko, _ := newTestModel(t, projects, states).Update(actionMsg{
		path: "/tmp/old-clean", kind: "pull", output: "error: divergent branches", err: "exit 1",
	})
	m = ko.(Model)
	if len(m.toasts.toasts) != 1 || m.toasts.toasts[0].level != toastError {
		t.Fatalf("action ko: %+v, want an error toast", m.toasts.toasts)
	}
	if txt := m.toasts.toasts[0].text; !strings.Contains(txt, "failed") || !strings.Contains(txt, "diverged") {
		t.Errorf("the error toast carries no reason/hint: %q", txt)
	}
}

func TestActionInCourseVisibleInStats(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.running["/tmp/old-clean"] = "pull"

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "pull old-clean") {
		t.Errorf("the running action is not shown in stats:\n%s", out)
	}
}

func TestErrorsAsToast(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m2, _ := m.Update(fetchStateMsg{path: "/tmp/old-clean", state: "failed", err: "no route"})
	m = m2.(Model)
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.level != toastError || !strings.Contains(last.text, "no route") {
		t.Errorf("fetch failed generated no error toast: %+v", last)
	}

	m3, _ := m.Update(scanProjectsMsg{projects: projects, note: "roots ilegibles"})
	m = m3.(Model)
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastError || !strings.Contains(last.text, "roots ilegibles") {
		t.Errorf("the roots error generated no error toast: %+v", last)
	}
	if strings.Contains(stripANSI(m.renderDashboard()), "roots ilegibles") {
		t.Error("the roots error stayed as a permanent line")
	}
}

func TestToastsNotCoverKeybinds(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	for i := 0; i < 3; i++ {
		m.toasts.showInfo(fmt.Sprintf("event %d", i))
	}

	out := stripANSI(m.View().Content)
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Fatalf("lines = %d, want %d", len(lines), m.height)
	}
	if !strings.Contains(out, "╭ keybinds ") || !strings.Contains(out, "╰") {
		t.Errorf("the keybinds section degraded:\n%s", out)
	}
	for _, hint := range []string{"j/k move", "f fetch", "q quit"} {
		if !strings.Contains(out, hint) {
			t.Errorf("hint %q covered by the toasts:\n%s", hint, out)
		}
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(out, fmt.Sprintf("event %d", i)) {
			t.Errorf("the stacked toast %d is missing:\n%s", i, out)
		}
	}
}

func TestToastsExpireWithTheTick(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.toasts.showInfo("hello")

	updated, cmd := m.Update(tickMsg{})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("the tick was not rearmed")
	}
	m.toasts.toasts[0].created = time.Now().Add(-toastDuration() - time.Second)
	updated, _ = m.Update(tickMsg{})
	m = updated.(Model)
	if len(m.toasts.toasts) != 0 {
		t.Errorf("the toast did not expire with the tick: %d alive", len(m.toasts.toasts))
	}
}

func TestRemoveWorktreeRunningVisibleInStats(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.running[p.Path] = "worktree_remove"

	if out := stripANSI(m.View().Content); !strings.Contains(out, "worktree_remove") {
		t.Errorf("the running deletion is not shown in stats:\n%s", out)
	}
}

func TestRemoveWorktreePromptVisibleInShortTerminal(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.height = 6 // without the armed warning, keybinds hides at this height

	if strings.Contains(stripANSI(m.View().Content), "remove worktree") {
		t.Fatal("precondition: with nothing armed there should be no prompt")
	}

	m, _ = press(m, "D")
	out := stripANSI(m.View().Content)
	if !strings.Contains(sectionContent(t, out, "keybinds"), "remove worktree wt-a? D to confirm") {
		t.Errorf("the prompt is not visible on a short terminal:\n%s", out)
	}
}

func TestPromptArmedReplacesTheHints(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	if m.keybindsLines() != defaultHintLines {
		t.Fatalf("with nothing armed, keybindsLines = %d, want %d", m.keybindsLines(), defaultHintLines)
	}
	if m.promptLine() != "" {
		t.Errorf("with nothing armed there is a prompt: %q", m.promptLine())
	}

	m, _ = press(m, "p")
	if m.keybindsLines() != 1 {
		t.Errorf("armed, keybindsLines = %d, want 1", m.keybindsLines())
	}

	out := m.View().Content
	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) != m.height {
		t.Errorf("lines = %d, want %d (the exact height of the terminal)", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d width = %d, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}

	flat := stripANSI(out)
	kb := sectionContent(t, flat, "keybinds")
	if !strings.Contains(kb, "p default") {
		t.Errorf("the prompt is not in keybinds:\n%s", kb)
	}
	for _, hint := range []string{"j/k move", "f fetch", "q quit"} {
		if strings.Contains(kb, hint) {
			t.Errorf("the hint %q is still there after arming the prompt:\n%s", hint, kb)
		}
	}

	m, _ = press(m, "esc")
	if m.promptLine() != "" {
		t.Errorf("esc did not clear the prompt: %q", m.promptLine())
	}
	if lines := strings.Split(stripANSI(m.View().Content), "\n"); len(lines) != m.height {
		t.Errorf("after cancelling, lines = %d, want %d", len(lines), m.height)
	}
}

func TestIndicatorActivityCountsTheActions(t *testing.T) {
	projects, states := fixtureProjects()

	t.Run("none", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		m.scanning = false
		if got := m.activityIndicator(); got != "" {
			t.Errorf("indicator = %q, want empty with no actions", got)
		}
	})

	// The scan has its own line and its own precedence over the actions: while the scan is running what is shown is the scan and not the counter, and the cases below only exercised the actions, so the scanning branch never ran and its ARITHMETIC_BASE mutant on the "+ " stayed NOT COVERED.
	t.Run("scanning takes precedence", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		m.scanning = true
		m.running = map[string]string{"/tmp/dirty-api": "pull"}
		got := m.activityIndicator()
		if !strings.Contains(got, "scanning") {
			t.Errorf("indicator = %q, want the scan above the action", got)
		}
		if strings.Contains(got, "pull") {
			t.Errorf("indicator = %q, want the action hidden while scanning", got)
		}
	})

	t.Run("one alone with no suffix", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		m.running = map[string]string{"/tmp/dirty-api": "pull"}
		got := m.activityIndicator()
		if !strings.Contains(got, "pull dirty-api…") {
			t.Errorf("indicator = %q, want the running action", got)
		}
		if strings.Contains(got, "+") {
			t.Errorf("indicator = %q, want no suffix with a single action", got)
		}
	})

	t.Run("three with +2", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		m.running = map[string]string{
			"/tmp/dirty-api":     "pull",
			"/tmp/old-clean":     "push",
			"/tmp/worktree-host": "worktree_remove",
		}
		got := m.activityIndicator()
		if !strings.Contains(got, "+2") {
			t.Errorf("indicator = %q, want +2 with three actions", got)
		}
	})

	t.Run("each kind counts", func(t *testing.T) {
		// push and worktree_remove are not pulls but are still actions that launch git: if they slipped out of the indicator the user would see a quiet row while the push is in flight.
		for _, kind := range []string{"pull", "pull_rebase", "pull_ff", "pull_merge", "push", "worktree_remove"} {
			m := newTestModel(t, projects, states)
			m.running = map[string]string{"/tmp/dirty-api": kind}
			if got := m.runningActions(); len(got) != 1 {
				t.Errorf("kind %q: runningActions = %v, want 1", kind, got)
			}
		}
	})

	t.Run("what does not launch git does not count", func(t *testing.T) {
		for _, kind := range []string{"fetch", "fetch_all", "pull_ai", "scan", "rescan"} {
			m := newTestModel(t, projects, states)
			m.running = map[string]string{"/tmp/dirty-api": kind}
			if got := m.runningActions(); len(got) != 0 {
				t.Errorf("kind %q: runningActions = %v, want empty", kind, got)
			}
			if got := m.activityIndicator(); got != "" {
				t.Errorf("kind %q: indicator = %q, want empty", kind, got)
			}
		}
	})

	t.Run("ordered by path", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		m.running = map[string]string{
			"/tmp/worktree-host": "push",
			"/tmp/dirty-api":     "pull",
			"/tmp/old-clean":     "push",
		}
		got := m.runningActions()
		want := []string{"pull dirty-api…", "push old-clean…", "push worktree-host…"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("runningActions = %v, want %v (ordered by path)", got, want)
		}
	})
}

func TestSummaryOfGroupOnlyPaintsWhatIsThere(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "backend", HasRepo: true},
	}
	clean := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}

	t.Run("clean-only repos", func(t *testing.T) {
		m := newTestModel(t, projects, clean)
		out := stripANSI(m.renderGroupSummary(groupHeader("backend"), 20))
		if !strings.Contains(out, "repos    2") {
			t.Errorf("the repo total is missing:\n%s", out)
		}
		for _, zero := range []string{"errors", "dirty", "ahead", "behind", "wt "} {
			if strings.Contains(out, zero) {
				t.Errorf("paints %q at zero:\n%s", zero, out)
			}
		}
	})

	t.Run("with each state", func(t *testing.T) {
		states := map[string]gitstatus.Snapshot{
			"/a": func() gitstatus.Snapshot {
				s := snapDirty(1, 0)
				s.Status.Ahead = 2
				return s
			}(),
			"/b": func() gitstatus.Snapshot {
				s := snapClean()
				s.Status.Behind = 3
				return s
			}(),
		}
		m := newTestModel(t, projects, states)
		out := stripANSI(m.renderGroupSummary(groupHeader("backend"), 20))
		for _, want := range []string{"dirty    1", "ahead    1", "behind   1"} {
			if !strings.Contains(out, want) {
				t.Errorf("%q is missing from the summary:\n%s", want, out)
			}
		}
		if strings.Contains(out, "errors") {
			t.Errorf("errors at zero in a group with no errors:\n%s", out)
		}
	})

	// The previous case loads the four states a repo can have without failing but misses the two the summary also paints from a different source (`errors` from StateError, `wt` from Worktrees), so those lines never ran and their ARITHMETIC_BASE mutants were NOT COVERED with no real gap behind: the branch existed and simply nobody looked at it.
	t.Run("with errors and worktrees", func(t *testing.T) {
		states := map[string]gitstatus.Snapshot{
			"/a": func() gitstatus.Snapshot {
				s := snapClean()
				s.Err = "boom"
				return s
			}(),
			"/b": func() gitstatus.Snapshot {
				s := snapClean()
				s.Worktrees = []gitstatus.Worktree{{Path: "/a-wt", Branch: "wt"}}
				return s
			}(),
		}
		m := newTestModel(t, projects, states)
		out := stripANSI(m.renderGroupSummary(groupHeader("backend"), 20))
		for _, want := range []string{"repos    2", "errors   1", "wt       1"} {
			if !strings.Contains(out, want) {
				t.Errorf("%q is missing from the summary:\n%s", want, out)
			}
		}
	})
}

func groupHeader(key string) tableEntry {
	return tableEntry{kind: kindPrimary, group: key}
}

func TestPreviewWithoutHeightNotDrawsBox(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.height = 14 // below the panel's minimum height
	m.width = 80  // and below the commits panel's width, so the count isolates the card
	lay := layoutTest(m.height, false, defaultHintLines, false)
	if lay.previewLines != 0 {
		t.Fatalf("h=%d: the layout gave a panel of %d; the test needs a height with no panel", m.height, lay.previewLines)
	}
	if got := m.previewSection(lay, m.entries()); got != "" {
		t.Errorf("previewSection with 0 lines = %q, want empty", got)
	}
	flat := stripANSI(m.renderDashboard())
	if strings.Contains(flat, "╭ ") && strings.Count(flat, "╭ ") != 3 {
		t.Errorf("a \"more\" section was drawn with no panel:\n%s", flat)
	}
}

func TestKeybindsRespectsTheBudgetOfHints(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	if len(m.cfg.HintBarLines()) < 2 {
		t.Fatalf("the test needs several hints: %v", m.cfg.HintBarLines())
	}
	for _, n := range []int{0, 1, 2} {
		out := stripANSI(m.keybindsSection(n))
		got := len(nonEmptyLines(strings.Split(sectionContent(t, out, "keybinds"), "\n")))
		if got != n {
			t.Errorf("n=%d: %d hint lines, want %d:\n%s", n, got, n, out)
		}
	}
}

func TestCursorMarksAOnlyRow(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	entries := m.entries()
	if len(entries) < 3 {
		t.Fatalf("the test needs 3 entries, there are %d", len(entries))
	}
	m.cursor = 1
	out := stripANSI(m.tableSection(len(entries), entries, m.width))
	lines := strings.Split(out, "\n")
	var marked []string
	for i, l := range lines {
		if strings.Contains(l, "▸") {
			marked = append(marked, strings.TrimSpace(l))
		}
		_ = i
	}
	if len(marked) != 1 {
		t.Fatalf("marked rows = %d, want 1:\n%s", len(marked), out)
	}
	if want := stripANSI(m.renderEntry(entries[1], true, m.width)); !strings.Contains(marked[0], strings.TrimSpace(want)) {
		t.Errorf("the marked row = %q, want the cursor's %q", marked[0], strings.TrimSpace(want))
	}
}

func nonEmptyLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.Trim(strings.TrimSpace(l), "│| ") != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestSyncOffsetBorders(t *testing.T) {
	cases := []struct {
		name               string
		total, window      int
		cursor, offsetPrev int
		want               int
	}{
		{"everything fits", 3, 5, 2, 4, 0},
		{"everything just fits", 5, 5, 4, 3, 0},
		{"cursor on the first visible row", 10, 5, 3, 3, 3},
		{"cursor on the last visible row", 10, 5, 7, 3, 3},
		{"one below the last", 10, 5, 8, 3, 4},
		{"cursor above the window", 10, 5, 1, 4, 1},
		{"scrolled to the bottom, cursor at the start", 10, 5, 0, 5, 0},
		{"window of 1", 10, 1, 4, 3, 4},
		{"no rows", 0, 5, 0, 2, 0},
	}
	for _, c := range cases {
		m := newTestModel(t, nil, nil)
		m.cursor, m.offset = c.cursor, c.offsetPrev
		m.syncOffset(c.total, c.window)
		if m.offset != c.want {
			t.Errorf("%s: offset = %d, want %d (cursor %d, total %d, window %d)",
				c.name, m.offset, c.want, c.cursor, c.total, c.window)
		}
	}
}

// The `len > 0` check is only a shortcut to avoid an empty clear(): since `&&` short-circuits it protects nothing else.
func TestEscCancelsTheDeletedAndOtherKeysNot(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter") // expands the worktrees

	m, _ = press(m, "down") // sub-row
	m, _ = press(m, "D")
	if m.armed == nil {
		t.Fatal("D did not arm the deletion")
	}
	m, _ = press(m, "D")
	if len(m.removeTokens) == 0 {
		t.Fatal("the confirmation left no token in flight")
	}
	token := m.removeTokens["/tmp/multi"]

	m, _ = press(m, "j")
	if m.removeTokens["/tmp/multi"] != token {
		t.Errorf("a normal key cancelled the deletion in flight: %v", m.removeTokens)
	}

	m, _ = press(m, "esc")
	if len(m.removeTokens) != 0 {
		t.Errorf("esc did not cancel the deletions in flight: %v", m.removeTokens)
	}
}

// The token comes from a MONOTONIC counter and IS the counter's value, which is what discards a late result from a previous attempt; if the counter did not advance (or went backwards), two attempts could share a token and the second would accept the first's result.
func TestTokenOfDeletedExitsOfACounterMonotonic(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down") // worktree sub-row

	before := m.removeGen
	m, _ = press(m, "D")
	if m.armed == nil {
		t.Fatal("D did not arm the deletion")
	}
	m, _ = press(m, "D")
	if m.removeGen <= before {
		t.Errorf("removeGen = %d, want > %d (monotonic counter)", m.removeGen, before)
	}
	if tok := m.removeTokens["/tmp/multi"]; tok != m.removeGen {
		t.Errorf("token = %d, want the counter %d", tok, m.removeGen)
	}
}

func TestEscCleansTheFilterOnlyWithTheInputEmpty(t *testing.T) {
	newModel := func() (Model, discovery.Project, map[string]gitstatus.Snapshot) {
		projects, states := fixtureProjects()
		m := newTestModel(t, projects, states)
		m.search = "old"
		return m, projects[0], states
	}

	t.Run("empty input, esc clears the filter", func(t *testing.T) {
		m, _, _ := newModel()
		m, _ = press(m, "/")
		for range len("old") {
			m, _ = press(m, "backspace")
		}
		m, _ = press(m, "esc")
		if m.search != "" {
			t.Errorf("esc with an empty input left the filter %q", m.search)
		}
	})

	t.Run("input with text, esc keeps the filter", func(t *testing.T) {
		m, _, _ := newModel()
		m, _ = press(m, "/")
		if m.search != "old" {
			t.Fatalf("the input was not seeded with the current filter: %q", m.search)
		}
		m, _ = press(m, "esc")
		if m.search != "old" {
			t.Errorf("esc with text in the input threw the filter away: %q", m.search)
		}
		if m.searchActive {
			t.Error("esc with text in the input left the editing active")
		}
	})
}

// Opening the editor does not fix an invalid TOML and the user would lose what they were about to change.
func TestEditorWithMarkerBrokenWarns(t *testing.T) {
	good := proj("ok", "/tmp/ok", true)
	broken := proj("broken", "/tmp/broken", true)
	broken.MarkerErr = "line 3: invalid value"
	states := map[string]gitstatus.Snapshot{"/tmp/ok": snapClean(), "/tmp/broken": snapClean()}
	m := newTestModel(t, []discovery.Project{good, broken}, states)

	m = cursorOn(t, m, "/tmp/ok")
	if _, cmd := press(m, "e"); cmd == nil {
		t.Error("e on a healthy repo did not launch the editor")
	}

	m = cursorOn(t, m, "/tmp/broken")
	_, cmd := press(m, "e")
	if cmd == nil {
		t.Fatal("e with a broken marker returned no command (it should be the toast)")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "marker error") {
		t.Errorf("e with a broken marker did not warn: %#v", cmd())
	}
}

// With the guard inverted the spinner would stay stuck with an already finished fetch.
func TestFetchStateForItsValue(t *testing.T) {
	projects, states := fixtureProjects()
	path := "/tmp/old-clean"

	for _, c := range []struct {
		state        string
		wantSpin     bool
		wantFail     bool
		quiereQuieto bool
	}{
		{"fetching", true, false, false},
		{"failed", false, true, false},
		{"ok", false, false, true},
	} {
		m := newTestModel(t, projects, states)
		out, _ := m.Update(fetchStateMsg{path: path, state: c.state, err: "no network"})
		flat := stripANSI(out.(Model).renderDashboard())
		if got := strings.Contains(flat, "fetching"); got != c.wantSpin {
			t.Errorf("%s: 'fetching' presente = %v, want %v", c.state, got, c.wantSpin)
		}
		if got := strings.Contains(flat, "✗ "); got != c.wantFail {
			t.Errorf("%s: failure present = %v, want %v", c.state, got, c.wantFail)
		}
		if got := strings.Contains(flat, "⟳ "); got != c.wantSpin {
			t.Errorf("%s: spin present = %v, want %v", c.state, got, c.wantSpin)
		}
		_ = c.quiereQuieto
	}
}

// Its size is the SHARE of that gap and not "the biggest that fits": the search starts at the share and only goes down when the minimum of table rows leaves no other spot.
func TestPreviewWithFilterRespectsTheShareOfTheGap(t *testing.T) {
	for _, h := range []int{24, 30, 40, 50, 60, 80, 100, 140} {
		lay := computeLayout(h, true, defaultHintLines, false, 0)
		free := h - (tableChrome + filterSectionLines + statsSectionLines +
			keybindsChrome + defaultHintLines + previewChrome)
		if free < 1 {
			t.Fatalf("h=%d: the gap with a filter is negative", h)
		}
		share := min(max(minPreviewLines, free*previewShare/5), free)

		if lay.previewLines > share {
			t.Errorf("h=%d with a filter: the panel measures %d and the gap's share is %d",
				h, lay.previewLines, share)
		}
		if h-share-reservedConFiltro >= minBodyLines {
			if lay.previewLines != share {
				t.Errorf("h=%d with a filter: the panel measures %d, want the share %d "+
					"(with %d free lines for the table there was no need to go lower)",
					h, lay.previewLines, share, h-share-reservedConFiltro)
			}
		}
		used := reservedConFiltro + lay.previewLines
		if lay.previewLines > 0 {
			used += previewChrome
		}
		if !lay.showStats {
			used -= statsSectionLines
		}
		if total := used + lay.bodyLines; total != h {
			t.Errorf("h=%d with a filter: the sections add up to %d, want %d", h, total, h)
		}
	}
}

const reservedConFiltro = tableChrome + filterSectionLines + statsSectionLines +
	keybindsChrome + defaultHintLines

// The three deadlines are asserted in UNITS and not against their own source (comparing actionTimeout() with itself cannot fail), and they live in functions because Go does not instrument constant expressions, leaving their ARITHMETIC_BASE mutant NOT COVERED forever.
func TestDeadlinesInUnits(t *testing.T) {
	if d := actionTimeout(); d != 120*time.Second {
		t.Errorf("actionTimeout() = %v, want 2m (a %v makes every git fail instantly)", d, d)
	}
	if d := commandTimeout(); d != 5*time.Minute {
		t.Errorf("commandTimeout() = %v, want 5m (a %v really kills a slow command)", d, d)
	}
	if d := toastDuration(); d != 3*time.Second {
		t.Errorf("toastDuration() = %v, want 3s (a %v would be read too fast)", d, d)
	}
}

func TestStatsAnnouncesTheFilterOnlyDirty(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.onlyDirty = true

	if sec := m.statsSection(); !strings.Contains(sec, "[dirty]") {
		t.Errorf("stats with onlyDirty = %q, want it to advertise it", sec)
	}
}

func TestDetailTitleMarksTheWorktree(t *testing.T) {
	r := row{project: discovery.Project{Name: "feature", IsWorktree: true}}
	if got := detailTitle(r); !strings.Contains(got, "feature") || !strings.Contains(got, "[worktree]") {
		t.Errorf("detailTitle = %q, want the name and the worktree mark", got)
	}
}

func TestDetailTitleJoinsGroupAndWorktree(t *testing.T) {
	r := row{project: discovery.Project{Name: "feature", PrimaryGroup: "vroom", IsWorktree: true}}
	got := detailTitle(r)
	for _, want := range []string{"feature", "vroom", "[worktree]"} {
		if !strings.Contains(got, want) {
			t.Errorf("detailTitle = %q, want it to contain %q", got, want)
		}
	}
}
