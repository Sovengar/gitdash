// Height budget of the dashboard: single source shared by the render, the detail and the tests.
package tui

const (
	statsSectionLines  = 3
	filterSectionLines = 3
	keybindsChrome     = 2
	tableChrome        = 3
	defaultHintLines   = 3

	previewChrome = 2
	// Share of the FREE height the panel takes (2/5 = 40%), the same split prdash uses for its item card.
	previewShare = 2
	// Below the status header (detailHeadLines) the panel says nothing, so it does not enter.
	minPreviewLines = 10
	// Below this there is no window left to scroll.
	minBodyLines = 3

	// Width budget: the table's row prefix, the right column (a share of the terminal, capped by
	// the table's minimum) and the card's columns.
	rowPrefixWidth = 4
	fetchSlotWidth = 2

	commitsPanelWidth = 30
	// The 1-cell gap between the two boxes of a band: table|commits on top, detail|files below.
	commitsPanelGap = 1

	cardLeftWidth  = 36
	cardSepWidth   = 1
	cardRightWidth = 24
	// The lists column takes this share of the inner width past its floor; the fields keep the rest
	// because path/upstream are what clip first.
	cardRightShare = 2
)

type layout struct {
	showStats    bool
	showFilter   bool
	showKeybinds bool
	hintLines    int
	bodyLines    int
	previewLines int

	// Width split, assembled only here: every pane renderer receives its width from this value.
	showPanel  bool
	tableWidth int
	panelWidth int
	// detailWidth is the detail box (bottom-left); zero files or a collapsed band make it the full width.
	detailWidth int
	// filesWidth is the files box (bottom-right) when the row has files; it mirrors panelWidth.
	filesWidth int
	cardSplit  bool
}

// The preview panel is ADDITIVE: it only stays at a height that leaves minBodyLines rows and costs no hint or section.
func computeLayout(height int, hasFilter bool, keybindsLines int, keepKeybinds bool) layout {
	filterH := 0
	if hasFilter {
		filterH = filterSectionLines
	}
	chrome := tableChrome

	sinPanel := fitLayout(height, chrome, filterH, 0, keybindsLines, keepKeybinds)
	lay := sinPanel

	for _, preview := range panelCandidates(height, chrome, filterH, keybindsLines) {
		l := fitLayout(height, chrome, filterH, preview, keybindsLines, keepKeybinds)
		if l.bodyLines >= minBodyLines && l.mismaChromeQue(sinPanel) {
			l.previewLines = preview
			lay = l
			break
		}
	}
	// The filter is the only section whose visibility is not degraded: it is painted while being typed or confirmed and its height is already counted.
	lay.showFilter = hasFilter
	return lay
}

// This is the starting point of the search and not a final decision: computeLayout keeps lowering it until the rest of the dashboard allows it.
func panelHeight(height, chrome, filterH, keybinds int) int {
	free := max(0, height-(chrome+filterH+statsSectionLines+keybindsChrome+max(0, keybinds)+previewChrome))
	return min(max(minPreviewLines, free*previewShare/5), free)
}

// The original `for preview := ...; preview >= detailHeadLines; preview--` cannot be killed: an inverted `preview++` keeps the condition true from the first turn (an infinite loop, which no test kills), so the range counts DOWN with a TURN LIMIT instead.
func panelCandidates(height, chrome, filterH, keybinds int) []int {
	top := panelHeight(height, chrome, filterH, keybinds)
	if top < detailHeadLines {
		return nil
	}
	turns := top - detailHeadLines + 1
	// `range` over the number of turns, not a counter in the for header: that is the difference between an INCREMENT_DECREMENT mutant that reverses the list and one that hangs, since range has no post expression to invert.
	c := make([]int, 0, turns)
	for i := range turns {
		c = append(c, top-i)
	}
	return c
}

// The body never goes below one line, and the minimum of rows the panel must respect (minBodyLines) is up to whoever chose the panel height, not to this split.
func fitLayout(height, chrome, filterH, preview, keybindsLines int, keepKeybinds bool) layout {
	showStats := true
	showKeybinds := true
	hint := max(0, keybindsLines)

	reserved := func() int {
		n := chrome + filterH
		if showStats {
			n += statsSectionLines
		}
		if showKeybinds {
			n += keybindsChrome + hint
		}
		if preview > 0 {
			n += previewChrome + preview
		}
		return n
	}
	if !keepKeybinds {
		// Degraded one at a time while they do not fit: with `range` there is no post expression to invert, so the inverted `hint--` is no longer a reversed walk that trims one hint too many and yields a wrong height.
		for range hint {
			if reserved()+1 <= height {
				break
			}
			hint--
		}
	}
	if hint == 0 {
		showKeybinds = false
	}
	if reserved()+1 > height {
		showStats = false
	}

	return layout{
		showStats:    showStats,
		showKeybinds: showKeybinds,
		hintLines:    hint,
		bodyLines:    max(1, height-reserved()),
	}
}

// This is the additivity condition: if the panel forces trimming a hint or hiding stats/keybinds, it does not enter.
func (l layout) mismaChromeQue(o layout) bool {
	return l.showStats == o.showStats &&
		l.showKeybinds == o.showKeybinds &&
		l.hintLines == o.hintLines
}

// The card's columns: the lists take cardRightShare/5 of the inner width (floor cardRightWidth),
// the fields the rest; both floors hold whenever cardSplit is on.
func cardColumns(inner int) (left, right int) {
	right = max(cardRightWidth, (inner-cardSepWidth)*cardRightShare/5)
	return inner - cardSepWidth - right, right
}
