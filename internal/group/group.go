// Package group agrupa proyectos por primary_group/secondary_group del
// marcador (0003 R19), extendiendo el patrón validado de vroom R24 a dos
// niveles: siempre agrupado si hay primarios, bloque de cada grupo contiguo
// desde la posición de su primer miembro (también dentro del primario) y
// sección (ungrouped) plegable al final.
package group

import (
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// Ungrouped es el nombre de la sección pseudo-grupo para los proyectos sin
// primary_group (0002 R16).
const Ungrouped = "(ungrouped)"

// Entry es una fila de la vista agrupada. Primary no vacío delimita el
// bloque al que pertenece la entrada ("" solo en la vista plana);
// Secondary solo es válido si Primary != "" (0003 R18: el secundario
// únicamente existe dentro de un primario).
type Entry struct {
	Primary   string
	Secondary string
	Proj      discovery.Project
	Snap      gitstatus.Snapshot
	State     gitstatus.State
}

// Arrange compone la vista agrupada a partir de filas ya ordenadas y
// filtradas (0003 R19):
//   - Sin ningún primario real → vista plana tal cual (Primary queda "").
//   - El bloque de un primario se emite completo en la posición de su
//     primer miembro tras el sort.
//   - Dentro de un primario, el bloque de cada secundario se emite contiguo
//     en la posición de su primer miembro; los miembros sin secundario
//     conservan su posición de sort (S19.3).
//   - Los proyectos sin primario forman el bloque final Ungrouped (el
//     secondary se ignora: S18.3).
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

	// normaliza: sin primario → sección Ungrouped sin secundario (S18.3)
	members := make(map[string][]Entry)               // primario → miembros en orden de sort
	secMembers := make(map[string]map[string][]Entry) // primario → secundario → miembros
	var order []string                                // orden de primera aparición tras el sort
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

// assemble emite los miembros de un primario: los de cada secundario como
// bloque contiguo en la posición de su primer miembro; los sin secundario
// en su propia posición (S19.3).
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

// IsPrimaryHeader reporta si la entrada en i abre un primario nuevo y debe
// renderizarse como header plegable de nivel 1 (los bloques son contiguos).
func IsPrimaryHeader(entries []Entry, i int) bool {
	return entries[i].Primary != "" && (i == 0 || entries[i-1].Primary != entries[i].Primary)
}

// IsSecondaryHeader reporta si la entrada en i abre un secundario nuevo
// dentro de su primario (header plegable de nivel 2).
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
