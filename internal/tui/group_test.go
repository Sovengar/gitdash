// Tests de la vista agrupada y plegado (0002 R16, 0003 R19/R20).
package tui

import (
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/group"
)

func TestGroupedViewS16_1(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "backend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)

	entries := m.entries()
	// header backend + 2 miembros + header ungrouped + 1 miembro = 5 filas
	if len(entries) != 5 {
		t.Fatalf("S16.1: entries = %d, want 5", len(entries))
	}
	if entries[0].kind != kindPrimary || entries[0].group != "backend" {
		t.Errorf("S16.1: entrada 0 = %+v, want header backend", entries[0])
	}
	if entries[3].kind != kindPrimary || entries[3].group != group.Ungrouped {
		t.Errorf("S16.1: entrada 3 = %+v, want header (ungrouped)", entries[3])
	}

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▾ backend (2)") || !strings.Contains(out, "▾ (ungrouped) (1)") {
		t.Errorf("S16.1: vista sin headers:\n%s", out)
	}
}

func TestFoldToggleS16_2(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "backend", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "backend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)
	if len(m.entries()) != 3 {
		t.Fatalf("60: entries = %d, want header+2", len(m.entries()))
	}

	m, _ = press(m, "tab") // cursor en header backend
	if len(m.entries()) != 1 {
		t.Errorf("S16.2: plegado entries = %d, want 1 (solo header)", len(m.entries()))
	}

	m, _ = press(m, "tab")
	if len(m.entries()) != 3 {
		t.Errorf("S16.2: desplegado entries = %d, want 3", len(m.entries()))
	}

	// enter sobre el header también pliega
	m, _ = press(m, "enter")
	if len(m.entries()) != 1 || !m.collapsed["backend"] {
		t.Errorf("S16.2: enter no plegó el header (entries=%d)", len(m.entries()))
	}
}

// S16.5: los filtros aplican antes de agrupar; grupos vacíos desaparecen.
func TestGroupsWithFilterS16_5(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "api", PrimaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "web", PrimaryGroup: "frontend", HasRepo: true},
		{Path: "/c", Name: "cli", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)
	m.search = "api"

	entries := m.entries()
	groups := map[string]int{}
	for _, e := range entries {
		if e.kind == kindPrimary {
			groups[e.group]++
		}
	}
	if len(entries) != 2 || len(groups) != 1 || groups["backend"] != 1 {
		t.Errorf("S16.5: entries = %v, want solo backend", rowsOf(entries))
	}
}

// S19.1: vista anidada con headers de dos niveles.
func TestNestedViewS19_1(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/d", Name: "d", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{
		"/a": snapClean(), "/b": snapClean(), "/c": snapClean(), "/d": snapClean(),
	}
	m := newTestModel(t, projects, states)

	entries := m.entries()
	// primario + sec backend + 2 repos + sec frontend + 1 repo + ungrouped + 1 repo = 8
	want := rowsOf(entries)
	if len(entries) != 8 {
		t.Fatalf("S19.1: entries = %d\n%s", len(entries), want)
	}
	if entries[0].kind != kindPrimary || entries[0].group != "vsocial" {
		t.Errorf("S19.1: entrada 0 = %+v, want header primario vsocial", entries[0])
	}
	if entries[1].kind != kindSecondary || entries[1].group != "vsocial/backend" {
		t.Errorf("S19.1: entrada 1 = %+v, want header secundario vsocial/backend", entries[1])
	}
	if entries[4].kind != kindSecondary || entries[4].group != "vsocial/frontend" {
		t.Errorf("S19.1: entrada 4 = %+v, want header secundario vsocial/frontend", entries[4])
	}
	if entries[5].r.project.Name != "b" {
		t.Errorf("S19.1: entrada 5 = %+v, want repo b", entries[5])
	}

	out := stripANSI(m.View().Content)
	for _, want := range []string{"▾ vsocial (3)", "▾ backend (2)", "▾ frontend (1)", "▾ (ungrouped) (1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("S19.1: vista sin %q:\n%s", want, out)
		}
	}
}

// S20.1: plegar un secundario oculta solo sus repos.
func TestFoldSecondaryS20_1(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "down") // cursor sobre header secundario backend
	if m.entries()[m.cursor].kind != kindSecondary {
		t.Fatalf("S20.1: cursor en %+v, want header secundario", m.entries()[m.cursor])
	}
	m, _ = press(m, "tab")
	entries := m.entries()
	// primario + sec backend + sec frontend + repo b = 4
	if len(entries) != 4 {
		t.Errorf("S20.1: entries = %s, want sin los repos de backend", rowsOf(entries))
	}
	if !m.collapsed["vsocial/backend"] {
		t.Errorf("S20.1: clave vsocial/backend no plegada")
	}
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▸ backend (1)") || !strings.Contains(out, "▾ frontend (1)") {
		t.Errorf("S20.1: glyphs incorrectos:\n%s", out)
	}
}

// S20.2: plegar el primario oculta también sus headers secundarios.
func TestFoldPrimaryHidesSecondaryHeadersS20_2(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "tab") // cursor sobre header primario vsocial: pliega
	if n := len(m.entries()); n != 1 {
		t.Errorf("S20.2: entries = %s, want solo header primario", rowsOf(m.entries()))
	}
}

// S20.3: tab sobre un repo pliega el contenedor más interno.
func TestTabOnRepoFoldsInnermostS20_3(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", HasRepo: true}, // sin secundario
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	// cursor al repo a (tras primario y secundario)
	m, _ = press(m, "down")
	m, _ = press(m, "down")
	m, _ = press(m, "tab")
	if !m.collapsed["vsocial/backend"] || m.collapsed["vsocial"] {
		t.Errorf("S20.3: repo con secundario pliega %v, want vsocial/backend", m.collapsed)
	}

	// reset y tab sobre repo b (sin secundario) → pliega el primario
	m2 := newTestModel(t, projects, states)
	m2, _ = press(m2, "down") // header secundario
	m2, _ = press(m2, "down") // repo a
	m2, _ = press(m2, "down") // repo b
	m2, _ = press(m2, "tab")
	if !m2.collapsed["vsocial"] || m2.collapsed["vsocial/"] {
		t.Errorf("S20.3: repo sin secundario pliega %v, want vsocial", m2.collapsed)
	}
}

// S20.5: claves de plegado sin colisión entre primarios distintos.
func TestFoldKeysNoCollisionS20_5(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "alfa", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "beta", SecondaryGroup: "backend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "down") // header secundario alfa/backend
	m, _ = press(m, "tab")
	if !m.collapsed["alfa/backend"] {
		t.Fatalf("S20.5: alfa/backend no plegado: %v", m.collapsed)
	}
	entries := m.entries()
	for _, e := range entries {
		if e.kind == kindRepo && e.r.project.Name == "b" {
			return // repo b sigue visible: beta/backend intacto
		}
	}
	t.Errorf("S20.5: repo b desapareció al plegar alfa/backend: %s", rowsOf(entries))
}

// S20.4: el conteo del primario suma todos sus secundarios.
func TestPrimaryCountIncludesSecondaryS20_4(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "vsocial", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▾ vsocial (3)") {
		t.Errorf("S20.4: conteo primario incorrecto:\n%s", out)
	}
}

func rowsOf(entries []tableEntry) string {
	out := ""
	for _, e := range entries {
		switch e.kind {
		case kindPrimary:
			out += "|P:" + e.group
		case kindSecondary:
			out += "|S:" + e.group
		default:
			out += "|repo:" + e.r.project.Name
		}
	}
	return out
}
