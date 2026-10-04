// Package group arranges rows in two levels, extending vroom's pattern: always grouped when primarys exist, each block contiguous from its first member, plus a foldable ungrouped section at the end.
package group

import (
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

const Ungrouped = "(ungrouped)"

type Entry struct {
	Primary   string
	Secondary string
	Proj      discovery.Project
	Snap      gitstatus.Snapshot
	State     gitstatus.State
}

func Arrange(rows []Entry) []Entry {
	hasReal := false
	for _, r := range rows {
		if r.Primary != "" {
			hasReal = true
			break
		}
	}
	if !hasReal {
		return rows
	}

	members := make(map[string][]Entry)
	secMembers := make(map[string]map[string][]Entry)
	var order []string
	for _, r := range rows {
		p, s := r.Primary, r.Secondary
		if p == "" {
			p, s = Ungrouped, ""
		}
		if _, seen := members[p]; !seen {
			order = append(order, p)
			secMembers[p] = make(map[string][]Entry)
		}
		e := Entry{Primary: p, Secondary: s, Proj: r.Proj, Snap: r.Snap, State: r.State}
		members[p] = append(members[p], e)
		if s != "" {
			secMembers[p][s] = append(secMembers[p][s], e)
		}
	}

	var out, ungrouped []Entry
	for _, p := range order {
		block := assemble(members[p], secMembers[p])
		if p == Ungrouped {
			ungrouped = block
			continue
		}
		out = append(out, block...)
	}
	return append(out, ungrouped...)
}

func assemble(members []Entry, secs map[string][]Entry) []Entry {
	emitted := make(map[string]bool)
	var block []Entry
	for _, e := range members {
		if e.Secondary == "" {
			block = append(block, e)
			continue
		}
		if !emitted[e.Secondary] {
			emitted[e.Secondary] = true
			block = append(block, secs[e.Secondary]...)
		}
	}
	return block
}

func IsPrimaryHeader(entries []Entry, i int) bool {
	return entries[i].Primary != "" && (i == 0 || entries[i-1].Primary != entries[i].Primary)
}

func IsSecondaryHeader(entries []Entry, i int) bool {
	if entries[i].Primary == "" || entries[i].Secondary == "" {
		return false
	}
	if i == 0 {
		return true
	}
	prev := entries[i-1]
	return prev.Primary != entries[i].Primary || prev.Secondary != entries[i].Secondary
}
