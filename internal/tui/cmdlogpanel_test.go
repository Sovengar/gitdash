package tui

import (
	"fmt"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func logModel(t *testing.T) (Model, *cmdlog.Recorder) {
	t.Helper()
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	return m, cmdlog.Active()
}

func sembrar(rec *cmdlog.Recorder, entries ...cmdlog.Entry) {
	for _, e := range entries {
		cmdlog.RecordExec(e)
	}
}

// The intent goes in through RecordIntent and not RecordExec, which would leave Intent=false and label the line "exec" with the Key inside the argv, so any assertion about the key would be satisfied by the wrong entry.
func sembrarIntent(e cmdlog.Entry) {
	e.Intent = true
	e.Exit = 0
	e.Dur = 0
	e.Argv = nil
	e.Outcome = ""
	cmdlog.RecordIntent(e)
}

func execEntry(repo, action, cmdLine, outcome string, exit int) cmdlog.Entry {
	return cmdlog.Entry{
		Repo: repo, Action: action, Class: cmdlog.ClassAction,
		Argv: strings.Fields(cmdLine), Outcome: outcome, Exit: exit,
		Dur: 146 * time.Millisecond,
	}
}

// Looked up by action instead of by position: the model has the background scan running, so a `git status` entry can be appended AFTER the one the test just synchronised on, and "the last one" then depends on the goroutine race (it failed one run in fifteen).
func entryFor(t *testing.T, rec *cmdlog.Recorder, action string) cmdlog.Entry {
	t.Helper()
	entries := rec.Entries()
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Action == action {
			return entries[i]
		}
	}
	t.Fatalf("the command log has no entry with action %q", action)
	return cmdlog.Entry{}
}

func TestPanelTeachesTheCommandRealAndItsResult(t *testing.T) {
	m, rec := logModel(t)
	m = cursorOn(t, m, "/tmp/dirty-api")

	m, _ = press(m, "p")
	m, _ = press(m, "p")

	sembrarIntent(cmdlog.Entry{Repo: "dirty-api", Key: "p", Action: "pull", Class: cmdlog.ClassAction})
	sembrar(rec, execEntry("dirty-api", "pull", "git pull", "rebase+autostash", 0))

	m, _ = press(m, "l")
	if !m.logOpen {
		t.Fatal("l did not open the panel")
	}
	plano := stripANSI(m.View().Content)
	log := sectionContent(t, plano, "log")
	if !strings.Contains(log, "git pull") {
		t.Errorf("the panel does not show the argv:\n%s", log)
	}
	if !strings.Contains(log, "rebase+autostash") {
		t.Errorf("the panel does not show the pull's real result:\n%s", log)
	}
	if !strings.Contains(log, "key p") {
		t.Errorf("the panel does not show the key that fired it:\n%s", log)
	}
}

func TestPanelDistinguishesTheVariantOfPull(t *testing.T) {
	m, rec := logModel(t)
	m = cursorOn(t, m, "/tmp/dirty-api")
	m, _ = press(m, "p")
	m, _ = press(m, "r") // explicit rebase variant

	sembrarIntent(cmdlog.Entry{Repo: "dirty-api", Key: "r", Action: "pull_rebase", Class: cmdlog.ClassAction})
	sembrar(rec, execEntry("dirty-api", "pull", "git pull --rebase --autostash", "rebase+autostash", 0))
	m, _ = press(m, "l")

	log := sectionContent(t, stripANSI(m.View().Content), "log")
	if !strings.Contains(log, "pull_rebase") {
		t.Errorf("the chosen variant is not visible:\n%s", log)
	}
	if !strings.Contains(log, "--rebase") {
		t.Errorf("the argv with the flags is not visible:\n%s", log)
	}
}

func TestPanelHidesTheReadsForBug(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec,
		cmdlog.Entry{Repo: "dirty-api", Action: "status", Class: cmdlog.ClassRead,
			Argv: []string{"git", "status", "--porcelain=v2", "--branch"}, Exit: 0},
		execEntry("dirty-api", "pull", "git pull", "up-to-date", 0),
	)
	m, _ = press(m, "l")

	log := sectionContent(t, stripANSI(m.View().Content), "log")
	if strings.Contains(log, "porcelain") {
		t.Errorf("the scan read shows without being asked:\n%s", log)
	}
	if !strings.Contains(log, "git pull") {
		t.Errorf("the action is not visible:\n%s", log)
	}
}

func TestPanelAlternatesTheFilterWithA(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec,
		cmdlog.Entry{Repo: "dirty-api", Action: "status", Class: cmdlog.ClassRead,
			Argv: []string{"git", "status", "--porcelain=v2", "--branch"}, Exit: 0},
		cmdlog.Entry{Repo: "dirty-api", Action: "fetch", Class: cmdlog.ClassAuto,
			Argv: []string{"git", "fetch", "--prune"}, Exit: 0},
	)
	m, _ = press(m, "l")
	if m.logShowAll {
		t.Fatal("logShowAll = true on exit")
	}
	m, _ = press(m, "a")
	if !m.logShowAll {
		t.Fatal("a did not enable logShowAll")
	}
	plano := stripANSI(m.View().Content)
	if !strings.Contains(plano, "log · all") {
		t.Errorf("the title does not reflect the filter:\n%s", plano)
	}
	log := sectionContent(t, plano, "log")
	if !strings.Contains(log, "porcelain") || !strings.Contains(log, "fetch --prune") {
		t.Errorf("with the wide filter reads and the automatic fetch must be visible:\n%s", log)
	}
	m, _ = press(m, "a")
	if strings.Contains(stripANSI(m.View().Content), "porcelain") {
		t.Error("after filtering by actions again, the read is still there")
	}
}

func TestPanelScrollNotMovesTheCursor(t *testing.T) {
	m, rec := logModel(t)
	for range 40 {
		sembrar(rec, execEntry("dirty-api", "pull", "git pull", "up-to-date", 0))
	}
	m, _ = press(m, "l")
	if m.logOffset != 0 {
		t.Fatalf("logOffset = %d on open, want 0 (anchored to the tail)", m.logOffset)
	}
	cursor := m.cursor
	m, _ = press(m, "k")
	if m.cursor != cursor {
		t.Errorf("k moved the table cursor: %d → %d", cursor, m.cursor)
	}
	if m.logOffset == 0 {
		t.Error("k did not scroll the panel")
	}
	m, _ = press(m, "j")
	if m.logOffset != 0 {
		t.Errorf("logOffset = %d after coming back down, want 0", m.logOffset)
	}
}

// The minimum of ONE visible line is what keeps the panel showing something in a 1 or 2 line terminal, where bodyLines-1 gives 0 or negative; with the minimum at 0 the box would be painted EMPTY and the user would lose the log in a narrow panel with no error at all, which is why this assertion looks at the content and not only at the height.
func TestPanelShowsOnTheLessALine(t *testing.T) {
	for _, height := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("a %d-line terminal", height), func(t *testing.T) {
			m, rec := logModel(t)
			for i := range 5 {
				sembrar(rec, execEntry(fmt.Sprintf("repo-%d", i), "pull", "git pull", "up-to-date", 0))
			}
			m, _ = press(m, "l")
			m.height = height
			m.width = 60

			out := stripANSI(m.logSection(height))
			if !strings.Contains(out, "git pull") {
				t.Errorf("with %d body lines the panel painted no entry:\n%q", height, out)
			}
		})
	}
}

func TestPanelOffsetIsClips(t *testing.T) {
	m, rec := logModel(t)
	for range 40 {
		sembrar(rec, execEntry("dirty-api", "pull", "git pull", "up-to-date", 0))
	}
	m, _ = press(m, "l")
	visible := max(1, m.layout().bodyLines-1)
	want := max(0, len(rec.Entries())-visible)
	for range 50 {
		m, _ = press(m, "k")
	}
	if m.logOffset != want {
		t.Errorf("logOffset = %d after 50 scrolls, want %d (entries %d - visible %d)",
			m.logOffset, want, len(rec.Entries()), visible)
	}
	if first := m.logSection(visible + 1); !strings.Contains(first, "git pull") {
		t.Errorf("with the scroll cap no entry is visible:\n%s", first)
	}
}

func TestPanelLegendInKeybinds(t *testing.T) {
	m, _ := logModel(t)
	m, _ = press(m, "l")

	if m.keybindsLines() != 1 {
		t.Errorf("keybindsLines = %d with the panel open, want 1", m.keybindsLines())
	}
	kb := sectionContent(t, stripANSI(m.View().Content), "keybinds")
	if !strings.Contains(kb, "command log") || !strings.Contains(kb, "scroll") {
		t.Errorf("the panel's legend is not in keybinds:\n%s", kb)
	}
	if lines := strings.Split(stripANSI(m.View().Content), "\n"); len(lines) != m.height {
		t.Errorf("lines = %d, want %d", len(lines), m.height)
	}
	for _, hint := range []string{"j/k move", "f fetch"} {
		if strings.Contains(kb, hint) {
			t.Errorf("the hint %q is still next to the legend:\n%s", hint, kb)
		}
	}
}

func TestPanelIsCloses(t *testing.T) {
	m, _ := logModel(t)
	m, _ = press(m, "l")
	m, _ = press(m, "esc")
	if m.logOpen {
		t.Error("esc did not close the panel")
	}
	m, _ = press(m, "l")
	m, _ = press(m, "l")
	if m.logOpen {
		t.Error("the second l did not close the panel")
	}
}

func TestCloseThePanelDropsTheSelectorArmed(t *testing.T) {
	m, _ := logModel(t)
	m = cursorOn(t, m, "/tmp/dirty-api")
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("p did not arm the selector")
	}
	m, _ = press(m, "l")
	m, _ = press(m, "esc")
	if m.pullArmed != nil {
		t.Error("closing the panel left the pull selector armed")
	}
}

func TestOpenThePanelDropsTheSelectorArmed(t *testing.T) {
	m, _ := logModel(t)
	m = cursorOn(t, m, "/tmp/dirty-api")
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("p did not arm the selector")
	}
	m, _ = press(m, "l") // opens the panel
	if m.pullArmed != nil {
		t.Error("opening the panel left the pull selector armed")
	}
	m, _ = press(m, "a")
	if !m.logShowAll {
		t.Error("a did not toggle the panel's filter")
	}
	if len(m.running) != 0 {
		t.Errorf("a launched an action: %v", m.running)
	}
}

func TestArmPullInsideOfThePanelNotArms(t *testing.T) {
	m, _ := logModel(t)
	m, _ = press(m, "l")
	m, _ = press(m, "p")
	if m.pullArmed != nil {
		t.Error("p armed the selector inside the log panel")
	}
	m, _ = press(m, "a")
	if !m.logShowAll {
		t.Error("a did not toggle the filter: the selector shadowed the key")
	}
	if len(m.running) != 0 {
		t.Errorf("a launched an action: %v", m.running)
	}
}

func TestPanelNotIsEatsThePOfTheInputs(t *testing.T) {
	t.Run("filtro", func(t *testing.T) {
		m, _ := logModel(t)
		m, _ = press(m, "l") // opens the panel
		m, _ = press(m, "/") // activates the filter
		m, _ = press(m, "x")
		m, _ = press(m, "p")
		if got := m.searchInput.Value(); got != "xp" {
			t.Errorf("filter = %q, want \"xp\" (the p got lost)", got)
		}
	})
	t.Run("command", func(t *testing.T) {
		m, _ := logModel(t)
		m, _ = press(m, "l")
		m, _ = press(m, "!") // opens the command input
		m, _ = press(m, "p")
		if got := m.cmdInput.Value(); got != "p" {
			t.Errorf("cmdInput = %q, want \"p\" (the p got lost)", got)
		}
	})
}

func TestOpenThePanelNotRecordsIntent(t *testing.T) {
	m, rec := logModel(t)
	before := len(rec.Entries())
	_, _ = press(m, "l")
	if got := len(rec.Entries()); got != before {
		t.Errorf("opening the panel recorded %d new entries, want 0", got-before)
	}
}

func TestNavigationNotArrivesOnTheLog(t *testing.T) {
	m, rec := logModel(t)
	before := len(rec.Entries())
	for _, k := range []string{"d", "/", "tab", "space", "enter"} {
		_, _ = press(m, k)
	}
	if got := len(rec.Entries()); got != before {
		t.Errorf("the view keys recorded %d entries, want 0", got-before)
	}
}

func TestIntentWithoutRowNotIsRecords(t *testing.T) {
	m, rec := logModel(t)
	header := -1
	for i, e := range m.entries() {
		if e.kind != kindRepo {
			header = i
			break
		}
	}
	if header < 0 {
		t.Skip("the fixture has no group headers")
	}
	m.cursor = header
	if _, ok := m.selected(); ok {
		t.Fatalf("row %d should be a header, not a repo", header)
	}
	before := len(rec.Entries())
	m, _ = press(m, "p")
	if got := len(rec.Entries()); got != before {
		t.Errorf("an action with no row recorded %d entries, want 0", got-before)
	}
}

func TestPanelRespectsTheWidth(t *testing.T) {
	for _, width := range []int{200, 120, 80, 60, 40} {
		m, rec := logModel(t)
		sembrar(rec, execEntry("dirty-api", "pull", "git pull --rebase --autostash", "rebase+autostash", 0))
		sembrarIntent(cmdlog.Entry{Repo: "dirty-api", Key: "r", Action: "pull_rebase", Class: cmdlog.ClassAction})
		m.width, m.height = width, 24
		m, _ = press(m, "l")
		for i, l := range strings.Split(stripANSI(m.View().Content), "\n") {
			if got := ansi.StringWidth(l); got != width {
				t.Errorf("width=%d line %d: width = %d, want %d: %q", width, i, got, width, ansi.Strip(l))
			}
		}
	}
}

func TestPanelPrioritisesTheCommandInWidthNarrow(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec, execEntry("dirty-api", "pull", "git pull --rebase --autostash", "rebase", 0))
	m.width, m.height = 60, 24
	m, _ = press(m, "l")

	log := sectionContent(t, stripANSI(m.View().Content), "log")
	if !strings.Contains(log, "git pull --rebase --autostash") {
		t.Errorf("at 60 columns the full command should fit:\n%s", log)
	}
}

// The panel replaces both the table and the card, so the total height has to keep adding up to the terminal's at any height: it is the invariant easiest to break when the split between sections changes.
func TestLogTakesTheTerminalInAllTheHeights(t *testing.T) {
	for _, height := range []int{12, 16, 20, 24, 30, 45, 60} {
		for _, withEntries := range []bool{false, true} {
			m, rec := logModel(t)
			if withEntries {
				for range 50 {
					sembrar(rec, execEntry("dirty-api", "pull", "git pull", "rebase", 0))
				}
			}
			m.width, m.height = 100, height
			m, _ = press(m, "l")
			lines := strings.Split(stripANSI(m.View().Content), "\n")
			if len(lines) != height {
				t.Errorf("height=%d entries=%v: lines = %d, want %d", height, withEntries, len(lines), height)
			}
			for i, l := range lines {
				if got := ansi.StringWidth(l); got != 100 {
					t.Errorf("height=%d line %d: width = %d, want 100: %q", height, i, got, ansi.Strip(l))
				}
			}
		}
	}
}

func TestLogColumnsNotBreakWithWidthsImpossible(t *testing.T) {
	for _, inner := range []int{-5, 0, 1, 10, 40, 78, 200} {
		c := computeLogColumns(inner)
		if c.argv < 0 || c.kind < 0 || c.repo < 0 || c.outcome < 0 || c.verdict < 0 {
			t.Errorf("inner=%d: negative column: %+v", inner, c)
		}
		if c.argv == 0 && inner > logColTime {
			t.Errorf("inner=%d: no room for the argv: %+v", inner, c)
		}
	}
}

// The edges matter: at the width where the fixed columns plus the minimum argv fit EXACTLY nothing is degraded, and a `> ` in the wrong place would start dropping columns with no need.
func TestLogColumnsInTheFitExact(t *testing.T) {
	holgura := logColTime + 4*logColSep + logColKind + logColRepo + logColOutcome + logColVerdict
	if holgura != 65 {
		t.Fatalf("precondition: the slack width is %d, the test measures 65", holgura)
	}

	justo := holgura + logMinArgv
	t.Run("exact fit, nothing degrades", func(t *testing.T) {
		c := computeLogColumns(justo)
		if c.kind != logColKind || c.repo != logColRepo || c.outcome != logColOutcome || c.verdict != logColVerdict {
			t.Errorf("inner=%d: a column that fit got degraded: %+v", justo, c)
		}
		if c.argv != logMinArgv {
			t.Errorf("inner=%d: argv = %d, want %d", justo, c.argv, logMinArgv)
		}
	})

	t.Run("one cell less and the verdict drops", func(t *testing.T) {
		c := computeLogColumns(justo - 1)
		if c.verdict != 0 {
			t.Errorf("inner=%d: verdict = %d, want 0 (it is worth the least)", justo-1, c.verdict)
		}
		for _, c2 := range []struct {
			name string
			got  int
			want int
		}{{"kind", c.kind, logColKind}, {"repo", c.repo, logColRepo}, {"outcome", c.outcome, logColOutcome}} {
			if c2.got != c2.want {
				t.Errorf("inner=%d: %s = %d, want %d (it should not drop yet)", justo-1, c2.name, c2.got, c2.want)
			}
		}
	})

	t.Run("argv goes to the floor, never below", func(t *testing.T) {
		for inner := justo - 1; inner >= logColTime+logColSep+logMinArgv; inner-- {
			c := computeLogColumns(inner)
			if c.argv < logMinArgv {
				t.Errorf("inner=%d: argv = %d, want >= %d", inner, c.argv, logMinArgv)
			}
		}
		for _, inner := range []int{logColTime + logColSep + logMinArgv - 1, 10, 5, 1, 0, -5} {
			c := computeLogColumns(inner)
			if c.argv < 0 {
				t.Errorf("inner=%d: argv = %d, want >= 0", inner, c.argv)
			}
			if c.argv > max(0, inner-logColTime-logColSep) {
				t.Errorf("inner=%d: argv = %d wants more width than there is (%d)",
					inner, c.argv, max(0, inner-logColTime-logColSep))
			}
		}
	})
}

// If the guard were `>=` instead of `>`, a narrow terminal would show headers for columns that no longer exist (and the split would show up broken in the header itself).
func TestPanelTheColumnsDegradesNotIsPaint(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec, execEntry("api", "pull", "git pull", "fast-forward", 0))
	m.logOpen = true

	t.Run("wide width, all the columns", func(t *testing.T) {
		m.width = 200
		header := m.logHeader(computeLogColumns(max(0, m.width-2)))
		for _, want := range []string{"TIME", "KIND", "REPO", "COMMAND", "RESULT", "VERDICT"} {
			if !strings.Contains(header, want) {
				t.Errorf("with a wide width the column %q is missing: %q", want, header)
			}
		}
	})

	t.Run("narrow width, only those that remain", func(t *testing.T) {
		m.width = 40
		c := computeLogColumns(max(0, m.width-2))
		if c.verdict != 0 || c.outcome != 0 || c.repo != 0 {
			t.Fatalf("precondition: at width=40 repo/result/verdict should drop: %+v", c)
		}
		header := m.logHeader(c)
		for _, noDebe := range []string{"REPO", "RESULT", "VERDICT"} {
			if strings.Contains(header, noDebe) {
				t.Errorf("degraded column %q still in the header: %q", noDebe, header)
			}
		}
		for _, want := range []string{"TIME", "KIND", "COMMAND"} {
			if !strings.Contains(header, want) {
				t.Errorf("with a narrow width %q is missing: %q", want, header)
			}
		}
		want := pad("TIME", logColTime) + pad("KIND", c.kind) + strings.Repeat(" ", logColSep) +
			pad("COMMAND", c.argv) + strings.Repeat(" ", logColSep)
		if header != want {
			t.Errorf("header = %q, want %q", header, want)
		}
		lay := m.layout()
		out := stripANSI(m.logSection(max(3, lay.bodyLines)))
		if !strings.Contains(out, "git pull") {
			t.Errorf("the command is not visible with a narrow width:\n%s", out)
		}
		if strings.Contains(out, "api") {
			t.Errorf("a repo with no column was painted anyway:\n%s", out)
		}
	})

	t.Run("no room for the argv means no command column", func(t *testing.T) {
		m.width = logColTime + logColSep
		c := computeLogColumns(max(0, m.width-2))
		if c.argv != 0 {
			t.Fatalf("precondition: at width=%d the argv should be 0: %+v", m.width, c)
		}
		header := m.logHeader(c)
		if strings.Contains(header, "COMMAND") {
			t.Errorf("command column with no width in the header: %q", header)
		}
		if !strings.Contains(header, "TIME") {
			t.Errorf("the time is always there: %q", header)
		}
		lay := m.layout()
		out := stripANSI(m.logSection(max(3, lay.bodyLines)))
		if strings.Contains(out, "git pull") {
			t.Errorf("the command was painted with no column:\n%s", out)
		}
	})
}

func TestPanelOffsetInTheEdges(t *testing.T) {
	m, rec := logModel(t)
	for i := 0; i < 6; i++ {
		sembrar(rec, execEntry("repo"+string(rune('a'+i)), "pull", "git pull", "fast-forward", 0))
	}
	m.logOpen = true

	t.Run("offset 0 sees the tail", func(t *testing.T) {
		m.logOffset = 0
		out := stripANSI(m.logSection(4))
		if !strings.Contains(out, "repoe") {
			t.Errorf("with offset 0 the most recent entry is not visible:\n%s", out)
		}
	})
	t.Run("scrolling up and back down clips", func(t *testing.T) {
		m.logOffset = 0
		visible := 3
		for range 20 {
			m.logScroll(1, visible) // k: backwards, towards the oldest
		}
		if m.logOffset == 0 {
			t.Error("20 scrolls up did not move the offset")
		}
		maxOffset := max(0, len(m.logEntries())-visible)
		if m.logOffset > maxOffset {
			t.Errorf("offset = %d, want <= %d (it would leave blank lines)", m.logOffset, maxOffset)
		}
		if m.logOffset != maxOffset {
			t.Errorf("offset = %d, want %d (the cap with 6 entries and 3 visible)", m.logOffset, maxOffset)
		}
		for range 40 {
			m.logScroll(-1, visible) // j: towards the tail
		}
		if m.logOffset != 0 {
			t.Errorf("offset = %d on reaching the bottom, want 0 (the tail is always visible)", m.logOffset)
		}
		m.logScroll(1, 100)
		if m.logOffset != 0 {
			t.Errorf("offset = %d with every entry visible, want 0", m.logOffset)
		}
	})
	t.Run("the section fills with no gaps and no panic at any height", func(t *testing.T) {
		if got := strings.Count(stripANSI(m.logSection(1)), "\n") + 1; got < 1 {
			t.Errorf("bodyLines=1: box of %d lines", got)
		}
		for bodyLines := 2; bodyLines <= 12; bodyLines++ {
			for off := 0; off <= 8; off++ {
				m.logOffset = off
				out := m.logSection(bodyLines)
				plano := stripANSI(out)
				want := bodyLines + 2 // 1 header + (bodyLines-1) rows + 2 borders
				if got := strings.Count(plano, "\n") + 1; got != want {
					t.Fatalf("bodyLines=%d offset=%d: box of %d lines, want %d", bodyLines, off, got, want)
				}
				for i, l := range strings.Split(plano, "\n") {
					if w := ansi.StringWidth(l); w > m.width {
						t.Errorf("bodyLines=%d offset=%d line %d width = %d > %d", bodyLines, off, i, w, m.width)
					}
				}
			}
		}
	})
}

// The sanitizing also has to let valid UTF-8 through and not only attack what looks like a sequence: the earlier tests used one-byte payloads (RuneError), the easy case, while an emoji or a non-BMP accent cannot be lost because the argv of the marker prompt is text the user wrote.
func TestSanitizeLogTextKeepsUTF8Valid(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"emoji de 4 bytes", "a👍b", "a👍b"},
		{"acentos", "Ω action", "Ω action"},
		{"emoji at the start", "🚀 go", "🚀 go"},
		{"emoji at the end", "go 🚀", "go 🚀"},
		{"dos emojis", "🚀🎯 fin", "🚀🎯 fin"},
		{"emoji with control around it", "a\x1b[31m👍\x1b[0mb", "a👍b"},
		{"multibyte y RuneError juntos", "👍\xffn", "👍n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLogText(tc.in); got != tc.want {
				t.Errorf("sanitizeLogText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeLogTextNotResellsWithSequencesWithoutClose(t *testing.T) {
	casos := []struct {
		name, in string
		want     string
	}{
		{"unterminated OSC", "\x1b]0;title", ""},
		{"OSC ending in ESC", "\x1b]0;abc\x1b", ""},
		{"OSC with BEL", "\x1b]0;abc\x07tail", "tail"},
		// The payload is ONE byte and the scan has to start at it, not after: skipping further would lose "tail" and the argv of the marker prompt would swallow legitimate text.
		{"one-byte OSC with BEL", "x\x1b]\x07tail", "xtail"},
		{"one-byte OSC with ST", "x\x1b]\x1b\\tail", "xtail"},
		{"OSC de dos bytes", "x\x1b]a\x07tail", "xtail"},
		{"OSC with a full ST", "\x1b]0;abc\x1b\\tail", "tail"},
		{"unterminated CSI", "\x1b[38;5", ""},
		{"CSI closed at @", "\x1b[@tail", "tail"},
		{"CSI closed at ~", "\x1b[1~tail", "tail"},
		{"stray ESC", "\x1btail", "ail"},
		{"ESC at the end", "tail\x1b", "tail"},
		{"empty ESC", "\x1b", ""},
		{"varios seguidos", "\x1b[\x1b]\x1b\\tail", "tail"},
	}
	for _, c := range casos {
		if got := sanitizeLogText(c.in); got != c.want {
			t.Errorf("%s: sanitizeLogText(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestPanelVerdictColoredForTheExit(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec,
		execEntry("ok", "pull", "git pull", "fast-forward", 0),
		execEntry("ko", "pull", "git pull", "diverged", 1),
	)
	for _, c := range []struct {
		name string
		e    cmdlog.Entry
		want lipglossStyle
	}{
		{"exit 0 is the quiet style", cmdlog.Entry{Exit: 0}, styleHint},
		{"an exit different from 0 is an error", cmdlog.Entry{Exit: 1}, styleError},
		{"exit 128 too", cmdlog.Entry{Exit: 128}, styleError},
		{"no exit (-1) is an error", cmdlog.Entry{Exit: -1}, styleError},
	} {
		if got, want := m.logVerdictStyle(c.e).Render("x"), c.want.Render("x"); got != want {
			t.Errorf("%s: estilo = %q, want %q", c.name, got, want)
		}
	}
}

func TestLogVerdict(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    cmdlog.Entry
		want string
	}{
		{"ok with a duration", cmdlog.Entry{Exit: 0, Dur: 146 * time.Millisecond}, "146ms"},
		{"fallido", cmdlog.Entry{Exit: 128, Dur: 20 * time.Millisecond}, "exit 128"},
		{"lento", cmdlog.Entry{Exit: 0, Dur: 2500 * time.Millisecond}, "2.5s"},
		// With a `>` instead of a `>=`, a command of exactly 1.000 s would fall into the end case and print "1000ms" next to a "2.5s".
		{"exactly one second", cmdlog.Entry{Exit: 0, Dur: time.Second}, "1.0s"},
		{"milliseconds below the second, in ms", cmdlog.Entry{Exit: 0, Dur: 999 * time.Millisecond}, "999ms"},
		{"handoff with no code nor duration", cmdlog.Entry{Exit: -1}, "no exit"},
		{"nothing to measure", cmdlog.Entry{Exit: 0}, ""},
		{"intent", cmdlog.Entry{Intent: true, Exit: -1}, ""},
	} {
		if got := logVerdict(tc.e); got != tc.want {
			t.Errorf("%s: logVerdict = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestLogOutcomeStyle(t *testing.T) {
	m, _ := logModel(t)
	for _, tc := range []struct {
		name    string
		outcome string
		want    string
	}{
		{"rebase with autostash", "rebase+autostash", "38;5;46"}, // green
		{"conflict", "conflict", "38;5;196"},                     // red
		{"nothing to integrate", "up-to-date", "38;5;241"},       // faint
		{"unclassified", "", "38;5;240"},                         // faint
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := m.logOutcomeStyle(cmdlog.Entry{Outcome: tc.outcome}).Render("X")
			if !strings.Contains(got, tc.want) {
				t.Errorf("outcome %q painted as %q, want colour %s", tc.outcome, got, tc.want)
			}
		})
	}
}

func TestNewInstallsTheRecorder(t *testing.T) {
	cmdlog.SetRecorder(nil)
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	_ = New(config.Defaults())
	if cmdlog.Active() == nil {
		t.Error("New did not install the command log's recorder")
	}
}

func TestHandoffRecordsArgvAndOutput(t *testing.T) {
	m, rec := logModel(t)

	out, _ := m.Update(execDoneMsg{
		path: "/tmp/dirty-api", action: "lazygit",
		argv: []string{"lazygit"}, err: nil,
	})
	if _, ok := out.(Model); !ok {
		t.Fatal("Update returned a model of another type")
	}

	e := entryFor(t, rec, "lazygit")
	if e.Action != "lazygit" {
		t.Errorf("Action = %q, want %q", e.Action, "lazygit")
	}
	if e.Command() != "lazygit" {
		t.Errorf("Command() = %q, want %q", e.Command(), "lazygit")
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
	if e.Dur != 0 {
		t.Errorf("Dur = %v in a handoff, want 0 (it is not measured)", e.Dur)
	}
}

func TestExecExit(t *testing.T) {
	if got := execExit(nil); got != 0 {
		t.Errorf("execExit(nil) = %d, want 0", got)
	}
	if got := execExit(errNotExec{}); got != -1 {
		t.Errorf("execExit(no-exec-error) = %d, want -1", got)
	}
}

type errNotExec struct{}

func (errNotExec) Error() string { return "it did not start" }

// With the pull policy delegated to the gitconfig an invisible argv is a UI that lies, which is why worktreeRemovedMsg has to carry it.
func TestDeletedWorktreeRecordsTheArgvResolved(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-real")
	testutil.MakeWorktree(t, dir, wtDir, "wt-real")
	snap := gitstatus.Collect(t.Context(), dir, "main", false)

	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap})
	rec := cmdlog.Active()
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })

	m, _ = press(m, "enter") // enter is the fold/expand key
	m, _ = press(m, "down")
	m, _ = press(m, "D")
	m, _ = press(m, "D")

	want := "git " + strings.Join(gitstatus.RemoveWorktreeArgv(wtDir, false), " ")
	waitEvent(t, &m, func(ev event) bool {
		_, ok := ev.(worktreeRemovedMsg)
		return ok
	})

	act, ok := m.lastAction[dir]
	if !ok {
		t.Fatal("no actionResult was left by the deletion")
	}
	if act.cmd != want {
		t.Errorf("actionResult.cmd = %q, want %q", act.cmd, want)
	}
	var found bool
	for _, e := range rec.Entries() {
		if e.Action == "worktree" && strings.Contains(e.Command(), "worktree remove") {
			found = true
		}
	}
	if !found {
		t.Errorf("the command log did not record the deletion: %v", rec.Entries())
	}
}

func TestSanitizeLogTextRemovesControlAndCollapsesLines(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"osc52", "x\x1b]52;c;cGF3bmVk\x07y", "xy"},
		{"csi", "a\x1b[31mrojo\x1b[0mb", "arojob"},
		{"multilinea", "line1\nline2\r\nline3", "line1 line2 line3"},
		{"tabulador", "a\tb", "a b"},
		{"c0", "a\x01b\x7fc", "abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLogText(tc.in); got != tc.want {
				t.Errorf("sanitizeLogText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeLogTextRemovesFormatAndZeroWidth(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"rtl-override", "a\u202eb", "ab"},
		{"zwsp", "a\u200bb", "ab"},
		{"line-separator", "a\u2028b", "ab"},
		{"for-separator", "a\u2029b", "ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLogText(tc.in); got != tc.want {
				t.Errorf("sanitizeLogText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// "The index AFTER the escape sequence" is the whole contract, and every +1/+2 is a mutant that eats a byte of text; the cases put a PRINTABLE character after the terminator so the extra or missing byte is visible.
func TestSkipEscapeConsumesTheSequenceAndNothingMore(t *testing.T) {
	casos := []struct {
		name    string
		in      string
		want    string
		wantIdx int
	}{
		{"BEL seguido de letra", "\x1b]0;t\x07tail", "tail", 6},
		{"ST seguido de letra", "\x1b]0;t\x1b\\tail", "tail", 7},
		{"CSI closed at a letter", "\x1b[31mX", "X", 5},
		{"stray ESC", "\x1btail", "ail", 2},
	}
	for _, c := range casos {
		t.Run(c.name, func(t *testing.T) {
			if got := skipEscape(c.in, 0); got != c.wantIdx {
				t.Errorf("skipEscape(%q, 0) = %d, want %d", c.in, got, c.wantIdx)
			}
			if got := sanitizeLogText(c.in); got != c.want {
				t.Errorf("sanitizeLogText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestPanelSanitizesTheArgvOfAInput(t *testing.T) {
	for _, prompt := range []string{
		"x\x1b]52;c;cGF3bmVk\x07y",
		"line1\nline2\nline3",
		"x\u202ey\u200bz",
	} {
		m, rec := logModel(t)
		sembrar(rec, cmdlog.Entry{
			Repo: "dirty-api", Class: cmdlog.ClassAction, Action: "pull_ai",
			Argv: []string{"jcode", "-run", prompt}, Exit: 0,
		})
		m.width, m.height = 200, 30
		m, _ = press(m, "l")

		raw := m.View().Content
		if strings.Contains(raw, "]52;c;") || strings.Contains(raw, "cGF3bmVk") {
			t.Errorf("prompt %q: the escape payload reached the panel", prompt)
		}
		if strings.ContainsRune(raw, '\u202e') || strings.ContainsRune(raw, '\u200b') {
			t.Errorf("prompt %q: format/zero-width reached the panel", prompt)
		}
		plano := stripANSI(raw)
		if lines := strings.Split(plano, "\n"); len(lines) != 30 {
			t.Errorf("prompt %q: lines = %d, want 30 (a multiline prompt must not break the height)", prompt, len(lines))
		}
		if log := sectionContent(t, plano, "log"); !strings.Contains(log, "jcode") {
			t.Errorf("prompt %q: the sanitised argv was not painted:\n%s", prompt, log)
		}
	}
}

// That branch was exercised by no test, and `size <= 1` is exactly what tells a badly decoded rune from a legitimate RuneError (a real U+FFFD comes with size == 3 and MUST survive).
func TestSanitizeLogTextDiscardsBytesThatAreNotUTF8(t *testing.T) {
	casos := []struct {
		name, in, want string
	}{
		{"byte suelto", "go\xfftest", "gotest"},
		{"stray byte between letters", "a\xffb", "ab"},
		{"secuencia truncada", "\xe4\xb8", ""},
		{"byte valido conako", "ca\xfe", "ca"},
		{"U+FFFD legitimo", "a\uFFFDb", "a\uFFFDb"},
		{"mixto", "x\xffy\uFFFDz", "xy\uFFFDz"},
	}
	for _, c := range casos {
		if got := sanitizeLogText(c.in); got != c.want {
			t.Errorf("%s: sanitizeLogText(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// A git exiting 1 must show "exit 1" and not the -1 that means "never started", which is what tells a conflict from a broken repo; the error comes from a real command because *exec.ExitError cannot be built by hand.
func TestExecExitWithErrorReal(t *testing.T) {
	cmd := osexec.Command("sh", "-c", "exit 3")
	err := cmd.Run()
	if err == nil {
		t.Fatal("the test command did not fail")
	}
	if got := execExit(err); got != 3 {
		t.Errorf("execExit(exit 3) = %d, want 3", got)
	}
}

// The command log is opt-in (--print does not install it and tests may not), so without that return a TUI with no log would blow up on any keystroke, which is exactly what would happen in --print if it shared the model.
func TestLogIntentWithoutRecorderNotPanics(t *testing.T) {
	// The recorder is disabled AFTER building the model because New always installs one; what is tested is that not recording from then on does not blow up, and switching it off earlier would let newTestModel install it again.
	m := newTestModel(t, nil, nil)
	prev := cmdlog.Active()
	cmdlog.SetRecorder(nil)
	t.Cleanup(func() { cmdlog.SetRecorder(prev) })

	if cmdlog.Active() != nil {
		t.Fatal("the recorder could not be disabled")
	}
	m.logIntent("p", "pull")
	cmdlog.RecordExec(cmdlog.Entry{Class: cmdlog.ClassAction, Key: "p", Action: "pull"})
	if got := cmdlog.Entries(); len(got) != 0 {
		t.Errorf("cmdlog.Entries() = %d with the log off, want 0", len(got))
	}
}

// An execution with no argv paints a dash instead of a blank column, because an empty column reads as "the command got lost" while the dash reads as "there is no command to show".
func TestLogLineWithoutArgvSetsDash(t *testing.T) {
	m, _ := logModel(t)
	cols := logColumns{kind: logColKind, repo: logColRepo, outcome: logColOutcome, verdict: logColVerdict, argv: 24}
	line := stripANSI(m.logLine(cmdlog.Entry{Class: cmdlog.ClassAction, Repo: "api"}, cols))
	if !strings.Contains(line, "-") {
		t.Errorf("line without argv = %q, want the dash in the command column", line)
	}
}
