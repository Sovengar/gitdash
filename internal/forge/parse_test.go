package forge

import (
	"strings"
	"testing"
)

func testHosts() map[string]string {
	return map[string]string{
		"github.com":         ForgeGitHub,
		"gitlab.example.com": ForgeGitLab,
	}
}

func refGL(project string) RepoRef {
	parts := strings.Split(project, "/")
	return RepoRef{
		Forge:   ForgeGitLab,
		Host:    "gitlab.example.com",
		Project: project,
		Owner:   parts[len(parts)-2],
		Name:    parts[len(parts)-1],
	}
}

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		hosts    map[string]string
		prefixes map[string]string
		want     RepoRef
		wantOK   bool
	}{
		{
			name:   "scp",
			raw:    "git@github.com:acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "scp without .git",
			raw:    "git@github.com:acme/widget",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			// An @ at position 0: an empty user is legal in scp (ssh takes the current user) and it is exactly the edge of `at >= 0`; with `at > 0` this remote falls into the "no user@host" case and a repo that does have a forge is lost.
			name:   "scp with an empty user",
			raw:    "@github.com:acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "https",
			raw:    "https://github.com/acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "ssh without .git",
			raw:    "ssh://git@github.com/acme/widget",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "barra final",
			raw:    "https://github.com/acme/widget/",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "spaces around",
			raw:    "  https://github.com/acme/widget.git\n",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "host lowercased",
			raw:    "git@GitHub.com:acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "the port in the url is dropped",
			raw:    "ssh://git@github.com:2222/acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "owner and name with dots and dashes",
			raw:    "https://github.com/acme-corp/widget.js.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme-corp/widget.js", Owner: "acme-corp", Name: "widget.js"},
			wantOK: true,
		},
		{
			name:   "gitlab with a subgroup",
			raw:    "https://gitlab.example.com/grupo/sub/proy.git",
			want:   RepoRef{Forge: ForgeGitLab, Host: "gitlab.example.com", Project: "grupo/sub/proy", Owner: "sub", Name: "proy"},
			wantOK: true,
		},
		{
			name:     "gitlab in an instance subfolder",
			raw:      "https://gitlab.example.com/git/grupo/sub/proy.git",
			prefixes: map[string]string{"gitlab.example.com": "git"},
			want:     refGL("grupo/sub/proy"),
			wantOK:   true,
		},
		{name: "host desconocido", raw: "https://bitbucket.org/acme/widget.git", wantOK: false},
		{name: "local path", raw: "/home/u/dev/widget", wantOK: false},
		{name: "local path with the file scheme", raw: "file:///home/u/dev/widget", wantOK: false},
		{name: "empty", raw: "", wantOK: false},
		{name: "without owner nor repo", raw: "https://github.com/acme.git", wantOK: false},
		{name: "solo owner", raw: "https://github.com/", wantOK: false},
		{name: "scheme without host", raw: "https:///acme/widget.git", wantOK: false},
		{name: "scp without path", raw: "git@github.com:", wantOK: false},
		{name: "user without host", raw: "git@", wantOK: false},
		{name: "hosts empty", raw: "https://github.com/acme/widget.git", hosts: map[string]string{}, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hosts := testHosts()
			if tc.hosts != nil {
				hosts = tc.hosts
			}
			got, ok := ParseRemoteURL(tc.raw, hosts, tc.prefixes)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, quiero %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Fatalf("ref = %+v, quiero %+v", got, tc.want)
			}
		})
	}
}

func TestParseRemoteURLStripsClonePrefix(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": ForgeGitLab}
	prefixes := map[string]string{"gitlab.example.com": "git"}
	want := refGL("grupo/sub/proy")

	cases := []struct {
		name string
		raw  string
	}{
		{"without prefix", "https://gitlab.example.com/grupo/sub/proy.git"},
		{"with prefix", "https://gitlab.example.com/git/grupo/sub/proy.git"},
		{"with prefix y barra final", "https://gitlab.example.com/git/grupo/sub/proy/"},
		{"scp with prefix", "git@gitlab.example.com:git/grupo/sub/proy.git"},
		{"scp with prefix and no .git", "git@gitlab.example.com:git/grupo/sub/proy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRemoteURL(tc.raw, hosts, prefixes)
			if !ok {
				t.Fatalf("did not parse %q", tc.raw)
			}
			if got != want {
				t.Fatalf("ref = %+v, quiero %+v", got, want)
			}
		})
	}
}

func TestParseRemoteURLKeepsConfiguredPrefix(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": ForgeGitLab}
	prefixes := map[string]string{"gitlab.example.com": "/git/"}

	got, ok := ParseRemoteURL("https://gitlab.example.com/grupo/proy.git", hosts, prefixes)
	if !ok {
		t.Fatal("did not parse")
	}
	if got.Project != "grupo/proy" {
		t.Fatalf("Project = %q, want %q (a path without the prefix is not touched)", got.Project, "grupo/proy")
	}
	if got.Owner != "grupo" || got.Name != "proy" {
		t.Fatalf("Owner/Name = %q/%q, quiero grupo/proy", got.Owner, got.Name)
	}
}

func TestPrefixFromAPIBase(t *testing.T) {
	cases := []struct {
		name    string
		apiBase string
		want    string
	}{
		{"instance in a subfolder", "/git/api/v4/", "git"},
		{"instance at the root", "/api/v4/", ""},
		{"instance in a subfolder without slashes", "git/api/v4", "git"},
		{"subfolder with more levels", "/custom/api/v4/", "custom"},
		{"empty", "", ""},
		{"api_base inesperado: v3", "/git/api/v3/", ""},
		{"api_base inesperado: subruta", "/api/v4/projects", ""},
		{"unexpected api_base: only the api prefix", "/git/api/", ""},
		{"api_base absoluto", "https://gitlab.example.com/api/v4/", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrefixFromAPIBase(tc.apiBase); got != tc.want {
				t.Fatalf("PrefixFromAPIBase(%q) = %q, quiero %q", tc.apiBase, got, tc.want)
			}
		})
	}
}

func TestParseRemoteWithoutPathNotIsRepo(t *testing.T) {
	hosts := map[string]string{"github.com": ForgeGitHub}
	for _, raw := range []string{
		"https://github.com/",
		"https://github.com",
		"git@github.com:",
	} {
		if ref, ok := ParseRemoteURL(raw, hosts, nil); ok {
			t.Errorf("ParseRemoteURL(%q) = %+v, want unparseable (there is no Owner nor Name)", raw, ref)
		}
	}
}

// A DOUBLE slash inside the path is not an empty segment that can be normalized: accepting it would make Owner and Name say one thing and the `gh pr create` argv go to another, so the remote is rejected whole instead of leaving a half-built RepoRef. It differs from the no-path case (this one has segments but one is empty), which is why it has its own test: the `len(parts) < 2` filter would not catch it.
func TestParseRemoteWithSegmentEmptyNotIsRepo(t *testing.T) {
	hosts := testHosts()
	for _, raw := range []string{
		"git@github.com:acme//widget.git",
		"https://github.com/acme//widget.git",
		"https://gitlab.example.com//widget.git",
	} {
		if ref, ok := ParseRemoteURL(raw, hosts, nil); ok {
			t.Errorf("ParseRemoteURL(%q) = %+v, want unparseable (empty segment)", raw, ref)
		}
	}
}

// `colon < 0` and `colon <= 0` are only told apart by a remote starting with ":" (index 0); without this case a boundary mutant survives, since an empty host is not in the map anyway and the result is the same by another path.
func TestParseRemoteWithColonInThePositionZero(t *testing.T) {
	hosts := testHosts()
	conVacio := map[string]string{"": ForgeGitHub, "github.com": ForgeGitHub}
	if ref, ok := ParseRemoteURL("git@:owner/repo", conVacio, nil); ok {
		t.Errorf("ParseRemoteURL(empty host) = %+v, want unparseable: a remote with no host is not a repo", ref)
	}
	if ref, ok := ParseRemoteURL("git@:owner/repo", hosts, nil); ok {
		t.Errorf("ParseRemoteURL(empty host) = %+v, want unparseable", ref)
	}
	if ref, ok := ParseRemoteURL("git@github.com:acme/widget.git", hosts, nil); !ok {
		t.Errorf("ParseRemoteURL(scp bien formado) = %+v, want parseable", ref)
	}
}
