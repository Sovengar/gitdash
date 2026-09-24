// Tests de la vista agrupada y plegado.
package tui

import (
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/group"
)

func TestGroupedView(t *testing.T) {
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
		t.Fatalf("entries = %d, want 5", len(entries))
	}
	if entries[0].kind != kindPrimary || entries[0].group != "backend" {
		t.Errorf("entrada 0 = %+v, want header backend", entries[0])
	}
	if entries[3].kind != kindPrimary || entries[3].group != group.Ungrouped {
		t.Errorf("entrada 3 = %+v, want header (ungrouped)", entries[3])
	}

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▾ backend (2)") || !strings.Contains(out, "▾ (ungrouped) (1)") {
		t.Errorf("vista sin headers:\n%s", out)
	}
}

func TestFoldToggle(t *testing.T) {
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
		t.Errorf("plegado entries = %d, want 1 (solo header)", len(m.entries()))
	}

	m, _ = press(m, "tab")
	if len(m.entries()) != 3 {
		t.Errorf("desplegado entries = %d, want 3", len(m.entries()))
	}

	// enter sobre el header también pliega
	m, _ = press(m, "enter")
	if len(m.entries()) != 1 || !m.collapsed["backend"] {
		t.Errorf("enter no plegó el header (entries=%d)", len(m.entries()))
	}
}

// Los filtros aplican antes de agrupar; grupos vacíos desaparecen.
func TestGroupsWithFilter(t *testing.T) {
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
		t.Errorf("entries = %v, want solo backend", rowsOf(entries))
	}
}

// Vista anidada con headers de dos niveles.
func TestNestedView(t *testing.T) {
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
		t.Fatalf("entries = %d\n%s", len(entries), want)
	}
	if entries[0].kind != kindPrimary || entries[0].group != "vsocial" {
		t.Errorf("entrada 0 = %+v, want header primario vsocial", entries[0])
	}
	if entries[1].kind != kindSecondary || entries[1].group != "vsocial/backend" {
		t.Errorf("entrada 1 = %+v, want header secundario vsocial/backend", entries[1])
	}
	if entries[4].kind != kindSecondary || entries[4].group != "vsocial/frontend" {
		t.Errorf("entrada 4 = %+v, want header secundario vsocial/frontend", entries[4])
	}
	if entries[5].r.project.Name != "b" {
		t.Errorf("entrada 5 = %+v, want repo b", entries[5])
	}

	out := stripANSI(m.View().Content)
	for _, want := range []string{"▾ vsocial (3)", "▾ backend (2)", "▾ frontend (1)", "▾ (ungrouped) (1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("vista sin %q:\n%s", want, out)
		}
	}
}

// Plegar un secundario oculta solo sus repos.
func TestFoldSecondary(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "down") // cursor sobre header secundario backend
	if m.entries()[m.cursor].kind != kindSecondary {
		t.Fatalf("cursor en %+v, want header secundario", m.entries()[m.cursor])
	}
	m, _ = press(m, "tab")
	entries := m.entries()
	// primario + sec backend + sec frontend + repo b = 4
	if len(entries) != 4 {
		t.Errorf("entries = %s, want sin los repos de backend", rowsOf(entries))
	}
	if !m.collapsed["vsocial/backend"] {
		t.Errorf("clave vsocial/backend no plegada")
	}
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▸ backend (1)") || !strings.Contains(out, "▾ frontend (1)") {
		t.Errorf("glyphs incorrectos:\n%s", out)
	}
}

// Plegar el primario oculta también sus headers secundarios.
func TestFoldPrimaryHidesSecondaryHeaders(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "tab") // cursor sobre header primario vsocial: pliega
	if n := len(m.entries()); n != 1 {
		t.Errorf("entries = %s, want solo header primario", rowsOf(m.entries()))
	}
}

// Tab sobre un repo pliega el contenedor más interno.
func TestTabOnRepoFoldsInnermost(t *testing.T) {
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
		t.Errorf("repo con secundario pliega %v, want vsocial/backend", m.collapsed)
	}

	// reset y tab sobre repo b (sin secundario) → pliega el primario
	m2 := newTestModel(t, projects, states)
	m2, _ = press(m2, "down") // header secundario
	m2, _ = press(m2, "down") // repo a
	m2, _ = press(m2, "down") // repo b
	m2, _ = press(m2, "tab")
	if !m2.collapsed["vsocial"] || m2.collapsed["vsocial/"] {
		t.Errorf("repo sin secundario pliega %v, want vsocial", m2.collapsed)
	}
}

// Claves de plegado sin colisión entre primarios distintos.
func TestFoldKeysNoCollision(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "alfa", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "beta", SecondaryGroup: "backend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "down") // header secundario alfa/backend
	m, _ = press(m, "tab")
	if !m.collapsed["alfa/backend"] {
		t.Fatalf("alfa/backend no plegado: %v", m.collapsed)
	}
	entries := m.entries()
	for _, e := range entries {
		if e.kind == kindRepo && e.r.project.Name == "b" {
			return // repo b sigue visible: beta/backend intacto
		}
	}
	t.Errorf("repo b desapareció al plegar alfa/backend: %s", rowsOf(entries))
}

// El conteo del primario suma todos sus secundarios.
func TestPrimaryCountIncludesSecondary(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "vsocial", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▾ vsocial (3)") {
		t.Errorf("conteo primario incorrecto:\n%s", out)
	}
}

// Un secundario plegado no debe filtrar el bloque del primario siguiente
// cuando este no tiene secundarios (regresión: skipSec se filtraba entre
// primarios y ocultaba proyectos bajo un header expandido).
func TestFoldSecondaryDoesNotLeakToNextPrimary(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "projects/mine", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m.collapsed["vsocial/backend"] = true

	entries := m.entries()
	// primario vsocial + sec backend (plegado) + primario projects/mine + repo b = 4
	if len(entries) != 4 {
		t.Fatalf("entries = %s, want el repo de projects/mine visible", rowsOf(entries))
	}
	if entries[2].kind != kindPrimary || entries[2].group != "projects/mine" {
		t.Errorf("entrada 2 = %+v, want header projects/mine", entries[2])
	}
	if entries[3].kind != kindRepo || entries[3].r.project.Name != "b" {
		t.Errorf("entrada 3 = %+v, want repo b visible", entries[3])
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
