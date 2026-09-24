// Tests del arrangement de grupos.
package group

import (
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

func entry(path, primary, secondary string) Entry {
	return Entry{
		Primary:   primary,
		Secondary: secondary,
		Proj:      discovery.Project{Path: path, Name: path, PrimaryGroup: primary, SecondaryGroup: secondary, HasRepo: true},
		Snap:      gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main"}},
		State:     gitstatus.StateClean,
	}
}

// bloques anidados contiguos; ungrouped al final.
func TestArrangeNestedBlocks(t *testing.T) {
	in := []Entry{
		entry("/a", "vsocial", "backend"),
		entry("/b", "", ""),
		entry("/c", "vsocial", "infra"),
		entry("/d", "vsocial", "backend"),
	}
	got := Arrange(in)
	want := []struct {
		path, prim, sec string
	}{
		{"/a", "vsocial", "backend"},
		{"/d", "vsocial", "backend"},
		{"/c", "vsocial", "infra"},
		{"/b", Ungrouped, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Proj.Path != w.path || got[i].Primary != w.prim || got[i].Secondary != w.sec {
			t.Errorf("pos %d = %q/%q/%q, want %q/%q/%q",
				i, got[i].Proj.Path, got[i].Primary, got[i].Secondary, w.path, w.prim, w.sec)
		}
	}
}

// primario en posición del primer miembro; orden de primera aparición.
func TestArrangePrimaryPosition(t *testing.T) {
	in := []Entry{
		entry("/x", "otros", ""),
		entry("/y", "vsocial", "backend"),
		entry("/z", "otros", ""),
	}
	got := Arrange(in)
	want := []string{"/x", "/z", "/y"}
	for i, w := range want {
		if got[i].Proj.Path != w {
			t.Fatalf("posición %d = %q, want %q", i, got[i].Proj.Path, w)
		}
	}
}

// repo con primario y sin secundario conserva su posición de sort
// dentro del primario, sin bloque propio que lo reordene.
func TestArrangeMixedSecondary(t *testing.T) {
	in := []Entry{
		entry("/a", "p", "backend"),
		entry("/b", "p", ""), // sin secundario, aparece entre bloques
		entry("/c", "p", "backend"),
	}
	got := Arrange(in)
	want := []string{"/a", "/c", "/b"}
	for i, w := range want {
		if got[i].Proj.Path != w {
			t.Fatalf("posición %d = %q, want %q", i, got[i].Proj.Path, w)
		}
		if i < 2 && (got[i].Secondary != "backend") {
			t.Errorf("bloque backend roto en %d", i)
		}
	}
}

// aplicado a dos niveles: grupo de un solo miembro mantiene header.
func TestArrangeSingleMember(t *testing.T) {
	in := []Entry{entry("/a", "backend", ""), entry("/b", "solo", "")}
	got := Arrange(in)
	if len(got) != 2 || got[1].Primary != "solo" {
		t.Errorf("miembro único: %+v", got)
	}
	if !IsPrimaryHeader(got, 1) {
		t.Errorf("'solo' debe tener header")
	}
}

// Sin primarios reales: vista plana sin headers, Primary queda "".
func TestArrangeFlat(t *testing.T) {
	in := []Entry{entry("/a", "", ""), entry("/b", "", "")}
	got := Arrange(in)
	if len(got) != 2 || got[0].Primary != "" || got[1].Primary != "" {
		t.Errorf("flat: %+v", got)
	}
	for i := range got {
		if IsPrimaryHeader(got, i) || IsSecondaryHeader(got, i) {
			t.Errorf("flat: header en %d", i)
		}
	}
}

// a través del arrangement: secondary sin primary ya llega vacío de
// discovery, pero Arrange también lo defiende.
func TestArrangeSecondaryIgnoredWithoutPrimary(t *testing.T) {
	in := []Entry{
		entry("/a", "p", ""),
		entry("/b", "", "infra"), // normalizado a Ungrouped sin secundario
	}
	got := Arrange(in)
	if got[1].Primary != Ungrouped || got[1].Secondary != "" {
		t.Errorf("secondary sin primary: %+v", got[1])
	}
}

func TestIsPrimaryHeader(t *testing.T) {
	in := Arrange([]Entry{
		entry("/a", "vsocial", "backend"),
		entry("/c", "vsocial", "backend"),
		entry("/d", "vsocial", "frontend"),
		entry("/e", "otros", ""),
	})
	want := []bool{true, false, false, true}
	for i, w := range want {
		if got := IsPrimaryHeader(in, i); got != w {
			t.Errorf("IsPrimaryHeader(%d) = %v, want %v", i, got, w)
		}
	}
}

func TestIsSecondaryHeader(t *testing.T) {
	in := Arrange([]Entry{
		entry("/a", "vsocial", "backend"),
		entry("/c", "vsocial", "backend"),
		entry("/d", "vsocial", "frontend"),
		entry("/e", "otros", ""), // primario sin secundario: nunca header 2
	})
	want := []bool{true, false, true, false}
	for i, w := range want {
		if got := IsSecondaryHeader(in, i); got != w {
			t.Errorf("IsSecondaryHeader(%d) = %v, want %v", i, got, w)
		}
	}
}
