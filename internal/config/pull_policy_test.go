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
			t.Errorf("commands[%s] ausente", action)
			continue
		}
		fields := strings.Fields(raw)
		if fields[0] != "pull" {
			t.Errorf("commands[%s] = %q, want subcomando pull", action, raw)
		}
		if len(fields)-1 != len(flags) {
			t.Errorf("commands[%s] = %q, want flags %v", action, raw, flags)
			continue
		}
		for i, f := range flags {
			if fields[1+i] != f {
				t.Errorf("commands[%s] = %q, want flag %q en %d", action, raw, f, i)
			}
		}
	}
}

func TestActionsRemoved(t *testing.T) {
	kb := DefaultKeybindings()
	for _, gone := range []string{"sync", "update"} {
		if _, ok := kb[gone]; ok {
			t.Errorf("keybindings[%s] still present", gone)
		}
	}
	if _, ok := DefaultCommands()["sync"]; ok {
		t.Error("commands.sync still present")
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
