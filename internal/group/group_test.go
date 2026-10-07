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

func TestArrangePrimaryPosition(t *testing.T) {
	in := []Entry{
		entry("/x", "others", ""),
		entry("/y", "vsocial", "backend"),
		entry("/z", "others", ""),
	}
	got := Arrange(in)
	want := []string{"/x", "/z", "/y"}
	for i, w := range want {
		if got[i].Proj.Path != w {
			t.Fatalf("position %d = %q, want %q", i, got[i].Proj.Path, w)
		}
	}
}

func TestArrangeMixedSecondary(t *testing.T) {
	in := []Entry{
		entry("/a", "p", "backend"),
		entry("/b", "p", ""), // no secondary, it appears between blocks
		entry("/c", "p", "backend"),
	}
	got := Arrange(in)
	want := []string{"/a", "/c", "/b"}
	for i, w := range want {
		if got[i].Proj.Path != w {
			t.Fatalf("position %d = %q, want %q", i, got[i].Proj.Path, w)
		}
		if i < 2 && (got[i].Secondary != "backend") {
			t.Errorf("broken backend block at %d", i)
		}
	}
}

func TestArrangeSingleMember(t *testing.T) {
	in := []Entry{entry("/a", "backend", ""), entry("/b", "only", "")}
	got := Arrange(in)
	if len(got) != 2 || got[1].Primary != "only" {
		t.Errorf("only member: %+v", got)
	}
	if !IsPrimaryHeader(got, 1) {
		t.Errorf("'only' must have a header")
	}
}

func TestArrangeFlat(t *testing.T) {
	in := []Entry{entry("/a", "", ""), entry("/b", "", "")}
	got := Arrange(in)
	if len(got) != 2 || got[0].Primary != "" || got[1].Primary != "" {
		t.Errorf("flat: %+v", got)
	}
	for i := range got {
		if IsPrimaryHeader(got, i) || IsSecondaryHeader(got, i) {
			t.Errorf("flat: header at %d", i)
		}
	}
}

func TestArrangeSecondaryIgnoredWithoutPrimary(t *testing.T) {
	in := []Entry{
		entry("/a", "p", ""),
		entry("/b", "", "infra"), // normalized to Ungrouped with no secondary
	}
	got := Arrange(in)
	if got[1].Primary != Ungrouped || got[1].Secondary != "" {
		t.Errorf("secondary without primary: %+v", got[1])
	}
}

func TestIsPrimaryHeader(t *testing.T) {
	in := Arrange([]Entry{
		entry("/a", "vsocial", "backend"),
		entry("/c", "vsocial", "backend"),
		entry("/d", "vsocial", "frontend"),
		entry("/e", "others", ""),
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
		entry("/e", "others", ""), // primary with no secondary: never a level-2 header
	})
	want := []bool{true, false, true, false}
	for i, w := range want {
		if got := IsSecondaryHeader(in, i); got != w {
			t.Errorf("IsSecondaryHeader(%d) = %v, want %v", i, got, w)
		}
	}
}
