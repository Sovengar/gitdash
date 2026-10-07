package forge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitdash/internal/forge/tool"
)

// The NUL separator is the only byte exec does not let pass inside an argument, which is what allows asserting that a body with newlines arrives whole, something a line separator could not prove.
const echoArgs = `#!/bin/sh
printf 'argc=%s\n' "$#"
printf '%s\0' "$@"
`

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// An argv that "almost" matches is the one that breaks silently: a -b where a -B belonged creates the PR against another branch with nothing failing visibly.
func diffArgv(got, want []string) string {
	for i := 0; i < len(got) || i < len(want); i++ {
		switch {
		case i >= len(got):
			return fmt.Sprintf("argv[%d] missing: I want %q (len %d < %d)", i, want[i], len(got), len(want))
		case i >= len(want):
			return fmt.Sprintf("argv[%d] = %q sobra (len %d > %d)", i, got[i], len(got), len(want))
		case got[i] != want[i]:
			return fmt.Sprintf("argv[%d] = %q, quiero %q", i, got[i], want[i])
		}
	}
	return ""
}

func TestBuildCreateArgv(t *testing.T) {
	full := Params{
		Title:  "Fixes the overlay's pull",
		Body:   "PR body",
		Base:   "main",
		Head:   "feat/pr-opening",
		Draft:  true,
		Labels: []string{"bug", "tui"},
	}
	cases := []struct {
		name string
		ref  RepoRef
		p    Params
		want []string
	}{
		{
			name: "github minimal: title, body and base",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-B", "main", "-R", "acme/widget"},
		},
		{
			name: "github completo: draft y labels repetibles",
			ref:  refGH("acme/widget"),
			p:    full,
			want: []string{
				"gh", "pr", "create",
				"-t", "Fixes the overlay's pull",
				"-b", "PR body",
				"-B", "main",
				"-H", "feat/pr-opening",
				"-d",
				"-l", "bug", "-l", "tui",
				"-R", "acme/widget",
			},
		},
		{
			name: "github without base nor head: they do not appear",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-R", "acme/widget"},
		},
		{
			name: "github without host: -R is the bare owner/repo",
			ref:  RepoRef{Forge: ForgeGitHub, Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-B", "main", "-R", "acme/widget"},
		},
		{
			name: "github enterprise: -R carries the host first",
			ref:  RepoRef{Forge: ForgeGitHub, Host: "github.example.com", Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-B", "main", "-R", "github.example.com/acme/widget"},
		},
		{
			name: "gitlab minimal: -d is the body and -b the base",
			ref:  refGL("grupo/sub/proy"),
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"glab", "mr", "create", "-t", "T", "-d", "C", "-b", "main", "-y", "-R", "grupo/sub/proy"},
		},
		{
			name: "gitlab full: long draft and repeatable labels",
			ref:  refGL("grupo/sub/proy"),
			p:    full,
			want: []string{
				"glab", "mr", "create",
				"-t", "Fixes the overlay's pull",
				"-d", "PR body",
				"-b", "main",
				"-s", "feat/pr-opening",
				"--draft",
				"-l", "bug", "-l", "tui",
				"-y",
				"-R", "grupo/sub/proy",
			},
		},
		{
			name: "gitlab without base nor head: they do not appear",
			ref:  refGL("grupo/sub/proy"),
			p:    Params{Title: "T", Body: "C"},
			want: []string{"glab", "mr", "create", "-t", "T", "-d", "C", "-y", "-R", "grupo/sub/proy"},
		},
		{
			name: "gitlab in a subfolder: -R carries no prefix",
			ref:  RepoRef{Forge: ForgeGitLab, Host: "gitlab.example.com", Project: "grupo/proy"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"glab", "mr", "create", "-t", "T", "-d", "C", "-b", "main", "-y", "-R", "grupo/proy"},
		},
		{
			name: "forge desconocido",
			ref:  RepoRef{Forge: "bitbucket", Host: "bitbucket.org", Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: nil,
		},
		{
			name: "empty forge",
			ref:  RepoRef{Host: "github.com", Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: nil,
		},
		{
			name: "ref cero",
			ref:  RepoRef{},
			p:    Params{Title: "T", Body: "C"},
			want: nil,
		},
		{
			name: "forge with uppercase letters and spaces",
			ref:  RepoRef{Forge: " GitHub ", Host: "github.com", Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-B", "main", "-R", "acme/widget"},
		},
		{
			name: "without labels: no -l",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C", Labels: []string{}},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-R", "acme/widget"},
		},
		{
			name: "dirty labels: they are trimmed and the empty ones dropped",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C", Labels: []string{" bug ", "", "   ", "tui"}},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-l", "bug", "-l", "tui", "-R", "acme/widget"},
		},
		{
			name: "repeated labels repeat, they are not merged",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C", Labels: []string{"a", "a"}},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-l", "a", "-l", "a", "-R", "acme/widget"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if d := diffArgv(BuildCreateArgv(tc.ref, tc.p), tc.want); d != "" {
				t.Fatalf("%s", d)
			}
		})
	}
}

func TestBuildCreateArgvEmitsTheBodyEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		ref  RepoRef
		want []string
	}{
		{"github", refGH("acme/widget"), []string{"gh", "pr", "create", "-t", "", "-b", "", "-R", "acme/widget"}},
		{"gitlab", refGL("grupo/proy"), []string{"glab", "mr", "create", "-t", "", "-d", "", "-y", "-R", "grupo/proy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if d := diffArgv(BuildCreateArgv(tc.ref, Params{}), tc.want); d != "" {
				t.Fatalf("%s", d)
			}
		})
	}
}

func TestBuildCreateArgvNotCrossesBodyAndBase(t *testing.T) {
	p := Params{Title: "TITLE", Body: "BODY", Base: "BASE", Head: "HEAD"}
	cases := []struct {
		name     string
		ref      RepoRef
		wantBody string
		wantBase string
		wantHead string
	}{
		{"gh", refGH("acme/widget"), "-b", "-B", "-H"},
		{"glab", refGL("grupo/proy"), "-d", "-b", "-s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argv := BuildCreateArgv(tc.ref, p)
			for _, c := range []struct{ value, want string }{
				{"BODY", tc.wantBody},
				{"BASE", tc.wantBase},
				{"HEAD", tc.wantHead},
				{"TITLE", "-t"},
			} {
				if got := flagBefore(argv, c.value); got != c.want {
					t.Errorf("the flag before %q is %q, want %q (argv: %q)", c.value, got, c.want, argv)
				}
			}
		})
	}
}

func flagBefore(argv []string, value string) string {
	for i := 1; i < len(argv); i++ {
		if argv[i] == value && strings.HasPrefix(argv[i-1], "-") {
			return argv[i-1]
		}
	}
	return ""
}

func TestBuildCreateArgvNotPassesHostnameATheCli(t *testing.T) {
	for _, tc := range []struct {
		name string
		ref  RepoRef
	}{
		{"github", refGH("acme/widget")},
		{"gitlab", RepoRef{Forge: ForgeGitLab, Host: "umane.emeal.nttdata.com", Project: "grupo/proy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argv := BuildCreateArgv(tc.ref, Params{Title: "T", Body: "C", Base: "main", Head: "h"})
			for _, bad := range []string{"--hostname", "--api-host", "--yes-and-no"} {
				if slices.Contains(argv, bad) {
					t.Errorf("argv %q contiene %q", argv, bad)
				}
			}
		})
	}
}

func TestBuildCreateArgvGlabJumpsTheConfirmation(t *testing.T) {
	argv := BuildCreateArgv(refGL("grupo/proy"), Params{Title: "T", Body: "C", Base: "main"})
	if !slices.Contains(argv, "-y") {
		t.Errorf("glab argv %q carries no -y: glab would ask for send confirmation", argv)
	}
}

// The test is end-to-end on purpose: it passes the argv through a real process and reads what the kernel delivered, instead of checking that a string looks right in a diff; a title with quotes or $(...) that got split would be an injection into the CLI.
func TestBuildCreateArgvAValueIsAOnlyElement(t *testing.T) {
	nastyTitle := "fix the \"pull\" & $(whoami) `id` ; rm -rf / | tee $(pwd) && echo \"end\""
	nastyBody := "line 1\nline 2\twith a tab, \"quotes\" and $HOME"

	for _, tc := range []struct {
		name string
		ref  RepoRef
		base string
		head string
	}{
		{"gh", refGH("acme/widget"), "-B", "-H"},
		{"glab", refGL("grupo/proy"), "-b", "-s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argv := BuildCreateArgv(tc.ref, Params{
				Title:  nastyTitle,
				Body:   nastyBody,
				Base:   "main",
				Head:   "feat/x",
				Labels: []string{"with a space and a \"quote\""},
			})
			r := &tool.Runner{Bin: writeScript(t, echoArgs)}
			out, err := r.Run(context.Background(), argv...)
			if err != nil {
				t.Fatalf("the stub failed: %v", err)
			}
			got := parseEchoedArgs(t, out)
			if len(got) != len(argv) {
				t.Fatalf("the process received %d elements, the argv has %d:\n%s", len(got), len(argv), out)
			}
			for i := range argv {
				if got[i] != argv[i] {
					t.Fatalf("argv[%d]: the process received %q, %q was passed", i, got[i], argv[i])
				}
			}
			if n := countEqual(got, nastyTitle); n != 1 {
				t.Errorf("the title arrived %d times as a whole element, want 1", n)
			}
			if flagBefore(argv, nastyTitle) != "-t" {
				t.Errorf("the title does not arrive after -t: %q", argv)
			}
		})
	}
}

func parseEchoedArgs(t *testing.T, out string) []string {
	t.Helper()
	_, payload, ok := strings.Cut(out, "\n")
	if !ok {
		t.Fatalf("the stub printed no argc: %q", out)
	}
	payload = strings.TrimSuffix(payload, "\x00")
	return strings.Split(payload, "\x00")
}

func countEqual(got []string, want string) int {
	n := 0
	for _, g := range got {
		if g == want {
			n++
		}
	}
	return n
}

func TestCreateBin(t *testing.T) {
	cases := []struct {
		ref  RepoRef
		want string
	}{
		{refGH("acme/widget"), "gh"},
		{refGL("grupo/proy"), "glab"},
		{RepoRef{Forge: " GitLab "}, "glab"},
		{RepoRef{Forge: "bitbucket"}, ""},
		{RepoRef{}, ""},
	}
	for _, tc := range cases {
		if got := CreateBin(tc.ref); got != tc.want {
			t.Errorf("CreateBin(%q) = %q, quiero %q", tc.ref.Forge, got, tc.want)
		}
	}
}

func TestPromptEnv(t *testing.T) {
	cases := []struct {
		name string
		ref  RepoRef
		want []string
	}{
		{"github", refGH("acme/widget"), []string{"GH_PROMPT_DISABLED=1"}},
		{
			name: "gitlab self-managed",
			ref:  RepoRef{Forge: ForgeGitLab, Host: "umane.emeal.nttdata.com", Project: "grupo/proy"},
			want: []string{"GITLAB_HOST=umane.emeal.nttdata.com"},
		},
		{"gitlab without host: without a host there is nothing to point at", RepoRef{Forge: ForgeGitLab, Project: "grupo/proy"}, nil},
		{"unsupported forge", RepoRef{Forge: "bitbucket"}, nil},
		{"empty ref", RepoRef{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PromptEnv(tc.ref); !slices.Equal(got, tc.want) {
				t.Errorf("PromptEnv = %q, quiero %q", got, tc.want)
			}
		})
	}
}

// The case does not come out of ParseRemoteURL (an empty path does not parse there) but out of a RepoRef built by hand, which is what the TUI does when the remote could not be read.
func TestBuildCreateArgvGlabWithoutProjectRaisesAndOnly(t *testing.T) {
	// Built by hand and not with refGL: that helper splits the project into segments, so with "" it would hit parts[-2], a panic of the test rather than of the code.
	empty := RepoRef{Forge: ForgeGitLab, Host: "gitlab.example.com"}
	argv := BuildCreateArgv(empty, Params{Title: "T", Body: "C", Base: "main"})
	if !slices.Contains(argv, "-y") {
		t.Errorf("argv %q carries no -y: glab would ask for send confirmation", argv)
	}
	for i, a := range argv {
		if a == "-R" {
			t.Errorf("argv %q carries -R with an empty project: -R \"\" means nothing", argv)
		}
		if a == "" {
			t.Errorf("argv %q has an empty argument at position %d: %q", argv, i, argv)
		}
	}
}
