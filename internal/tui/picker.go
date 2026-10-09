package tui

import (
	"strings"

	"gitdash/internal/gitstatus"
)

// filterRefs keeps the refs whose name contains filter (case-insensitive), preserving git's refname
// order. localOnly drops remote-tracking names: a remote-tracking name is not a valid PR head for
// gh/glab, so the head picker must not offer one.
func filterRefs(refs []gitstatus.Ref, filter string, localOnly bool) []gitstatus.Ref {
	needle := strings.ToLower(filter)
	out := make([]gitstatus.Ref, 0, len(refs))
	for _, r := range refs {
		if localOnly && r.Remote {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(r.Name), needle) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// indexOfRef is the highlight of a value that may not be in the list (a ref typed by hand): absent,
// the pane still needs a valid highlight and 0 is the first entry.
func indexOfRef(refs []gitstatus.Ref, name string) int {
	for i, r := range refs {
		if r.Name == name {
			return i
		}
	}
	return 0
}

// clampHighlight never leaves the list: 0 on an empty list, otherwise within [0, n-1].
func clampHighlight(n, cursor int) int {
	if n <= 0 || cursor < 0 {
		return 0
	}
	return min(cursor, n-1)
}

// pickerWindow returns the visible [start,end) range keeping cursor inside, and whether a "… N more"
// tail is painted. When the list does not fit whole, the last row is reserved for the tail so the
// window height never changes while filtering.
func pickerWindow(n, cursor, rows int) (start, end int, more bool) {
	if n == 0 || rows <= 0 {
		return 0, 0, false
	}
	show := min(n, rows)
	if n > rows {
		show = rows - 1
	}
	cursor = clampHighlight(n, cursor)
	start = 0
	if cursor >= show {
		start = cursor - show + 1
	}
	start = min(max(start, 0), n-show)
	end = start + show
	return start, end, end < n
}
