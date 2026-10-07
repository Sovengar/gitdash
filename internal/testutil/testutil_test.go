package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The marker is the contract between the fixture and the code under test, so the helper has to be faithful: a key with value "" is OMITTED and not written as `name = ""`. Writing it would make discovery read a project with no name (or no group) and the test using it would be testing something else without anyone knowing: the failure would show up in production code, not in the fixture.
func TestMarkerSkipsTheKeysEmpty(t *testing.T) {
	for _, c := range []struct {
		title                    string
		name, primary, secondary string
		want                     []string
		noWant                   []string
	}{
		{
			title: "all three", name: "api", primary: "vsocial", secondary: "backend",
			want:   []string{`name = "api"`, `primary_group = "vsocial"`, `secondary_group = "backend"`},
			noWant: nil,
		},
		{
			title: "no name", name: "", primary: "vsocial", secondary: "backend",
			want:   []string{`primary_group = "vsocial"`, `secondary_group = "backend"`},
			noWant: []string{"name"},
		},
		{
			title: "no groups", name: "api", primary: "", secondary: "",
			want:   []string{`name = "api"`},
			noWant: []string{"group"},
		},
		{
			title: "only the primary", name: "api", primary: "vsocial", secondary: "",
			want:   []string{`name = "api"`, `primary_group = "vsocial"`},
			noWant: []string{"secondary_group"},
		},
		{
			// Only the secondary, no primary: it is written anyway. A one-level group is not a caller error and the fixture must not invent a rule the product does not have.
			title: "only the secondary", name: "", primary: "", secondary: "backend",
			want:   []string{`secondary_group = "backend"`},
			noWant: []string{"primary_group", "name"},
		},
		{
			title: "nothing at all", name: "", primary: "", secondary: "",
			want:   nil,
			noWant: []string{"name", "group"},
		},
	} {
		t.Run(c.title, func(t *testing.T) {
			dir := t.TempDir()
			Marker(t, dir, c.name, c.primary, c.secondary, false)

			raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
			if err != nil {
				t.Fatalf("the marker was not written: %v", err)
			}
			got := string(raw)
			for _, key := range c.want {
				if !strings.Contains(got, key) {
					t.Errorf("%s missing from the marker:\n%s", key, got)
				}
			}
			for _, key := range c.noWant {
				if strings.Contains(got, key) {
					t.Errorf("%s shows up in the marker and should have been omitted:\n%s", key, got)
				}
			}
		})
	}
}

func TestMarkerMalformedIsTOMLInvalid(t *testing.T) {
	dir := t.TempDir()
	Marker(t, dir, "api", "vsocial", "backend", true)
	raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
	if err != nil {
		t.Fatalf("the marker was not written: %v", err)
	}
	if !strings.Contains(string(raw), "[roto") {
		t.Errorf("the malformed marker does not look malformed:\n%s", raw)
	}
}
