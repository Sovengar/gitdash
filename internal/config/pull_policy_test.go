package config

import (
	"strings"
	"testing"
)

func TestDefaultPullWithoutFlags(t *testing.T) {
	got := DefaultCommands()["pull"]
	if got != "pull" {
		t.Errorf("commands.pull = %q, want %q (the policy is the gitconfig's decision)", got, "pull")
	}
	for _, forbidden := range []string{"--ff-only", "--rebase", "--no-rebase", "--autostash"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("commands.pull = %q, it must not carry %s", got, forbidden)
		}
	}
}

func TestVariantsPullWithFlagsOwn(t *testing.T) {
	want := map[string][]string{
		"pull":        {},
		"pull_rebase": {"--rebase", "--autostash"},
		"pull_ff":     {"--ff-only"},
		"pull_merge":  {"--no-rebase"},
	}
	cmds := DefaultCommands()
	for action, flags := range want {
		raw, ok := cmds[action]
		if !ok {
			t.Errorf("commands[%s] missing", action)
			continue
		}
		fields := strings.Fields(raw)
		if fields[0] != "pull" {
			t.Errorf("commands[%s] = %q, want subcommand pull", action, raw)
		}
		if len(fields)-1 != len(flags) {
			t.Errorf("commands[%s] = %q, want flags %v", action, raw, flags)
			continue
		}
		for i, f := range flags {
			if fields[1+i] != f {
				t.Errorf("commands[%s] = %q, want flag %q at %d", action, raw, f, i)
			}
		}
	}
}

// `update` (u) stays removed; `sync` (s) came back with sync-branch semantics. Its `commands` base is only the policy — the builder appends `origin <resolved sync>`, so the entry is configurable without carrying the ref.
func TestActionsRemoved(t *testing.T) {
	if _, ok := DefaultKeybindings()["update"]; ok {
		t.Error("keybindings[update] still present")
	}
	if _, ok := DefaultKeybindings()["sync"]; !ok {
		t.Error("keybindings[sync] is missing: sync came back")
	}
	if got := DefaultCommands()["sync"]; got != "pull --rebase --autostash" {
		t.Errorf("commands.sync = %q, want the default %q", got, "pull --rebase --autostash")
	}
}

func TestCmdArgsSyncIsConfigurable(t *testing.T) {
	if got := strings.Join(Defaults().CmdArgs("sync"), " "); got != "pull --rebase --autostash" {
		t.Errorf("CmdArgs(sync) = %q, want the default", got)
	}
	cfg := Defaults()
	cfg.Commands["sync"] = "pull --rebase"
	if got := strings.Join(cfg.CmdArgs("sync"), " "); got != "pull --rebase" {
		t.Errorf("CmdArgs(sync) with an override = %q, want the override", got)
	}
}

func TestPullKeyPreserved(t *testing.T) {
	if got := DefaultKeybindings()["pull"]; got != "p" {
		t.Errorf("keybindings.pull = %q, want %q", got, "p")
	}
}

func TestHintBarAnnouncesTheSelector(t *testing.T) {
	lines := strings.Join(Defaults().HintBarLines(), "\n")
	if !strings.Contains(lines, "p pull") {
		t.Errorf("the hint does not mention the pull key:\n%s", lines)
	}
	if !strings.Contains(lines, "▸") {
		t.Errorf("the hint does not hint at the variant selector:\n%s", lines)
	}
}

func TestCmdArgsResolvesVariant(t *testing.T) {
	cfg := Defaults()
	if got := cfg.CmdArgs("pull_ff"); strings.Join(got, " ") != "pull --ff-only" {
		t.Errorf("CmdArgs(pull_ff) = %v", got)
	}
	cfg.Commands["pull_rebase"] = "pull --rebase"
	if got := cfg.CmdArgs("pull_rebase"); strings.Join(got, " ") != "pull --rebase" {
		t.Errorf("CmdArgs with an override = %v, want the override", got)
	}
}
