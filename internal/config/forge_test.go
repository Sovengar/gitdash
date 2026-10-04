package config

import (
	"strings"
	"testing"
)

const forgeSelfManaged = `
[forge.github]
enabled = true
host = "github.com"

[forge.gitlab]
enabled = true
host = "umane.emeal.nttdata.com"
api_base = "/git/api/v4/"
`

// Public hosts resolve with nothing declared: without them even a github.com remote would return "unknown forge" and the PR action would fail silently.
func TestForgeDefaults(t *testing.T) {
	cfg := Defaults()
	hosts := cfg.ForgeHosts()
	if len(hosts) != 2 {
		t.Errorf("ForgeHosts() = %v, want only the two public ones", hosts)
	}
	for host, want := range map[string]string{
		"github.com": "github",
		"gitlab.com": "gitlab",
	} {
		if got := hosts[host]; got != want {
			t.Errorf("ForgeHosts()[%q] = %q, want %q", host, got, want)
		}
	}
	for name, want := range map[string]ForgeConfig{
		"github": {Enabled: true, Host: "github.com"},
		"gitlab": {Enabled: true, Host: "gitlab.com", APIBase: DefaultGitLabAPIBase},
	} {
		if got := cfg.Forges[name]; got != want {
			t.Errorf("default %q = %+v, want %+v", name, got, want)
		}
	}
	// gitlab.com lives at the root of its host: an invented prefix would send all its projects to /git/group/proj, which does not exist; and being empty, the prefix does not even enter the map.
	if _, ok := cfg.ForgePrefixes()["gitlab.com"]; ok {
		t.Error("gitlab.com entered ForgePrefixes with the host root")
	}
	if got := cfg.Forges["gitlab"].ClonePrefix(); got != "" {
		t.Errorf("ClonePrefix() de gitlab.com = %q, want \"\"", got)
	}
}

// Declaring the host replaces the provider default's, because there is one instance per provider: it is not a list the self-managed is added to.
func TestForgeSelfManagedInSubfolder(t *testing.T) {
	cfg, warn := LoadFrom(write(t, forgeSelfManaged))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	hosts := cfg.ForgeHosts()
	if len(hosts) != 2 {
		t.Errorf("ForgeHosts() = %v, want the declared host and github's", hosts)
	}
	if got := hosts["umane.emeal.nttdata.com"]; got != "gitlab" {
		t.Errorf("ForgeHosts()[umane.emeal.nttdata.com] = %q, want gitlab", got)
	}
	if _, ok := hosts["gitlab.com"]; ok {
		t.Error("the declared host did not replace the provider's default")
	}
	prefixes := cfg.ForgePrefixes()
	if len(prefixes) != 1 {
		t.Errorf("ForgePrefixes() = %v, want only the host with a prefix", prefixes)
	}
	if got := prefixes["umane.emeal.nttdata.com"]; got != "git" {
		t.Errorf("prefix = %q, want \"git\" (derivado de /git/api/v4/)", got)
	}
}

func TestForgeEnabledFalseDisablesTheProvider(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[forge.gitlab]
enabled = false
host = "umane.emeal.nttdata.com"
api_base = "/git/api/v4/"
`))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if _, ok := cfg.ForgeHosts()["umane.emeal.nttdata.com"]; ok {
		t.Error("the host of a disabled provider resolved to forge")
	}
	if _, ok := cfg.ForgePrefixes()["umane.emeal.nttdata.com"]; ok {
		t.Error("the prefix of a disabled provider entered the map")
	}
	// The flag is PER PROVIDER and not per host: gitlab.com stops resolving too, while github keeps working (this is not a global blackout).
	if _, ok := cfg.ForgeHosts()["gitlab.com"]; ok {
		t.Error("gitlab.com resolved with its provider disabled")
	}
	if got := cfg.ForgeHosts()["github.com"]; got != "github" {
		t.Errorf("ForgeHosts()[github.com] = %q, want github", got)
	}
}

func TestForgeEnabledMissingNotDisables(t *testing.T) {
	cfg, _ := LoadFrom(write(t, `
[forge.gitlab]
host = "git.example.com"
`))
	if got := cfg.ForgeHosts()["git.example.com"]; got != "gitlab" {
		t.Errorf("ForgeHosts()[git.example.com] = %q, want gitlab", got)
	}
}

func TestForgeCloneBaseIsOverrideOfThePrefix(t *testing.T) {
	cases := []struct {
		name string
		toml string
		want string
	}{
		{
			name: "without clone_base it comes from the api_base",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\napi_base = \"/git/api/v4/\"\n",
			want: "git",
		},
		{
			name: "clone_base wins over an api_base with no subfolder",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\napi_base = \"/api/v4/\"\nclone_base = \"git\"\n",
			want: "git",
		},
		{
			name: "clone_base wins over an api_base with another subfolder",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\napi_base = \"/git/api/v4/\"\nclone_base = \"otro\"\n",
			want: "otro",
		},
		{
			name: "clone_base with slashes is normalised",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\nclone_base = \"/git/\"\n",
			want: "git",
		},
		{
			name: "an unexpected api_base invents no prefix",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\napi_base = \"/git/api/v3/\"\n",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, warn := LoadFrom(write(t, tc.toml))
			if warn != "" {
				t.Fatalf("warn inesperado: %q", warn)
			}
			if got := cfg.ForgePrefixes()["a.example.com"]; got != tc.want {
				t.Errorf("prefix = %q, want %q", got, tc.want)
			}
		})
	}
}

// An empty prefix does NOT go into the map: with the host root, an "" entry and its absence produce the same Project, so it would only add noise to the map the parser reads.
func TestForgePrefixEmptyNotEntersInTheMap(t *testing.T) {
	cfg, _ := LoadFrom(write(t, forgeSelfManaged))
	if _, ok := cfg.ForgePrefixes()["github.com"]; ok {
		t.Error("github (without api_base) entered ForgePrefixes")
	}
}

func TestForgeHostDuplicateNotIsRepeats(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[forge.github]
host = "github.com"

[forge.gitlab]
host = "gitlab.com"
`))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	hosts := cfg.ForgeHosts()
	if len(hosts) != 2 {
		t.Errorf("ForgeHosts() = %v, want two hosts, one per provider", hosts)
	}
	for host, want := range map[string]string{
		"github.com": "github",
		"gitlab.com": "gitlab",
	} {
		if got := hosts[host]; got != want {
			t.Errorf("ForgeHosts()[%q] = %q, want %q", host, got, want)
		}
	}
	if got := hosts["github.com"]; got != "github" {
		t.Errorf("the re-declared host lost its provider: %q", got)
	}
}

// An empty or blank host does not invent an entry: without a host the provider has nowhere to live, and an "" in the map would make a hostless remote pass for GitHub (ForgeForHost normalizes to "" too).
func TestForgeHostEmptyNotEntersInTheMap(t *testing.T) {
	cfg, _ := LoadFrom(write(t, `
[forge.gitlab]
host = "   "
`))
	for h := range cfg.ForgeHosts() {
		if h == "" {
			t.Error("ForgeHosts() has the empty key")
		}
	}
	if got := cfg.Forges["gitlab"].Host; got != "gitlab.com" {
		t.Errorf("host = %q, want the default's (the empty one does not replace)", got)
	}
}

func TestForgeNameNormalized(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[forge.GitLab]
host = "a.example.com"
`))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if got := cfg.ForgeHosts()["a.example.com"]; got != "gitlab" {
		t.Errorf("ForgeHosts()[a.example.com] = %q, want gitlab", got)
	}
}

func TestForgeNotSupportedWarns(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[forge.bitbucket]
host = "bitbucket.org"
`))
	if !strings.Contains(warn, "bitbucket") {
		t.Errorf("warn without the unsupported provider: %q", warn)
	}
	if _, ok := cfg.ForgeHosts()["bitbucket.org"]; ok {
		t.Error("the unsupported provider stayed in the hosts map")
	}
}

func TestForgeAPIBaseEmptyKeepsTheDefault(t *testing.T) {
	cfg, _ := LoadFrom(write(t, `
[forge.gitlab]
api_base = ""
`))
	if got := cfg.Forges["gitlab"].APIBase; got != DefaultGitLabAPIBase {
		t.Errorf("api_base = %q, want the default %q", got, DefaultGitLabAPIBase)
	}
}

func TestDefaultKeybindingsPR(t *testing.T) {
	cfg := Defaults()
	if got := cfg.KeyFor("pr"); got != "O" {
		t.Errorf("default pr = %q, want O", got)
	}
	for action, key := range DefaultKeybindings() {
		if action != "pr" && key == "O" {
			t.Errorf("the action %q also uses the O key", action)
		}
	}
	// The label carries NO key inside (HintBarLines prepends it), or a rebind would produce hints like "W O open PR".
	if label, ok := hintLabels["pr"]; !ok || label != "open PR" {
		t.Errorf("hintLabels[pr] = %q (%v), want \"open PR\"", label, ok)
	}
	hints := strings.Join(cfg.HintBarLines(), "\n")
	if !strings.Contains(hints, "O open PR") {
		t.Errorf("hint de PR ausente: %v", cfg.HintBarLines())
	}
	if strings.Contains(hints, "new PR") {
		t.Errorf("the hints should not say \"new PR\": %v", cfg.HintBarLines())
	}
	for _, fixed := range []string{"q", "k", "j", "up", "down", "home", "end", "enter", "esc", "tab"} {
		if DefaultKeybindings()["pr"] == fixed {
			t.Errorf("pr overrides the fixed key %q", fixed)
		}
	}
}

func TestDefaultKeybindingsPRRebind(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[keybindings]
pr = "W"
`))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if got := cfg.KeyFor("pr"); got != "W" {
		t.Errorf("pr = %q, want W", got)
	}
	hints := strings.Join(cfg.HintBarLines(), "\n")
	if !strings.Contains(hints, "W open PR") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
}

// A provider with an EMPTY name is ignored instead of entering the map with the "" key: a "" host cannot be attributed to anyone and `ForgeForHost("")` would return that phantom provider; the case comes from a `[forge.""]` table in the TOML, which is rare but writable.
func TestForgeWithNameEmptyIsIgnores(t *testing.T) {
	cfg := Defaults()
	cfg.addForge("   ", forgeConfig{})
	for name := range cfg.Forges {
		if name == "" {
			t.Errorf("Forges = %v, want no entry with an empty name", cfg.Forges)
		}
	}
	if got := len(cfg.Forges); got != len(Defaults().Forges) {
		t.Errorf("Forges has %d entries, want %d (the default ones)", got, len(Defaults().Forges))
	}
}

// An enabled provider with an EMPTY Host contributes nothing to either map: it is a configuration error warned about on load, but an empty host must not slip into the map, since `ForgeForHost("")` has to keep answering nothing and an "" in ForgeHosts would attribute every hostless remote to this provider.
func TestForgeWithoutHostNotEntersInTheMaps(t *testing.T) {
	cfg := Defaults()
	enabled := true
	empty := "   "
	cfg.addForge("bitbucket", forgeConfig{Enabled: &enabled, Host: &empty})

	if got, ok := cfg.ForgeHosts()["bitbucket"]; ok && got != "" {
		t.Errorf("ForgeHosts()[%s] = %q, want no entry (the host is empty)", "bitbucket", got)
	}
	if got, ok := cfg.ForgePrefixes()["bitbucket"]; ok {
		t.Errorf("ForgePrefixes()[%s] = %q, want no entry", "bitbucket", got)
	}
	if f, ok := cfg.ForgeHosts()[""]; ok {
		t.Errorf("ForgeHosts()[\"\"] = %q, want no entry (an empty host is no forge)", f)
	}
}
