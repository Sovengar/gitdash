package config

import (
	"reflect"
	"testing"
)

func TestAICommandOverride(t *testing.T) {
	path := write(t, `[ai.pull]
command = "jcode -run {prompt}"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("unexpected warning: %q", warn)
	}
	if got := cfg.AICommand("pull"); got != "jcode -run {prompt}" {
		t.Errorf("AICommand(pull) = %q, want the template", got)
	}
	if got := cfg.AICommand("commit"); got != "" {
		t.Errorf("AICommand(commit) = %q, want empty (not configured)", got)
	}
}

func TestAICommandExtensible(t *testing.T) {
	path := write(t, `[ai.commit]
command = "ai commit {prompt}"
`)
	cfg, _ := LoadFrom(path)
	if got := cfg.AICommand("commit"); got != "ai commit {prompt}" {
		t.Errorf("AICommand(commit) = %q", got)
	}
	if got := cfg.AICommand("pull"); got != "" {
		t.Errorf("AICommand(pull) = %q, want empty", got)
	}
}

// The security boundary: the marker prompt enters as ONE argv element and is never interpolated into an `sh -c`, so spaces, `;`, quotes and `$` do not change the element count.
func TestBuildAIArgvPromptIsAOnlyElement(t *testing.T) {
	prompt := `he said "hi"; rm -rf / && echo $HOME`
	argv := BuildAIArgv("jcode -run {prompt}", prompt, nil)
	want := []string{"jcode", "-run", prompt}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %#v, want %#v", argv, want)
	}
}

func TestBuildAIArgvContextPlaceholders(t *testing.T) {
	argv := BuildAIArgv("ai --branch {branch} --behind {behind} {prompt}",
		"fix", map[string]string{"branch": "main", "behind": "3"})
	want := []string{"ai", "--branch", "main", "--behind", "3", "fix"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %#v, want %#v", argv, want)
	}
}

func TestBuildAIArgvNotRescansThePrompt(t *testing.T) {
	argv := BuildAIArgv("ai {prompt}", "uses {branch}", map[string]string{"branch": "main"})
	if len(argv) != 2 || argv[1] != "uses {branch}" {
		t.Errorf("argv = %#v, want the literal prompt", argv)
	}
}

func TestBuildAIArgvTemplateEmpty(t *testing.T) {
	if argv := BuildAIArgv("", "something", nil); len(argv) != 0 {
		t.Errorf("argv = %#v, want empty", argv)
	}
}

// The prompt is one value and never rescanned: the placeholders inside it must
// survive as text, or a prompt that mentions {branch} would be rewritten.
func TestExpandAIVars(t *testing.T) {
	vars := map[string]string{"branch": "main", "behind": "2"}
	got := ExpandAIVars("fix {branch}, {behind} behind: {prompt}", "do {branch} now", vars)
	want := "fix main, 2 behind: do {branch} now"
	if got != want {
		t.Errorf("ExpandAIVars = %q, want %q", got, want)
	}
}

func TestExpandAIVarsWithoutVars(t *testing.T) {
	if got := ExpandAIVars("say {prompt}", "hi", nil); got != "say hi" {
		t.Errorf("ExpandAIVars = %q, want %q", got, "say hi")
	}
}

// An empty template yields no argv at all, so the caller can tell "nothing
// configured" from a command whose first field happens to be empty.
func TestBuildAIArgvEmptyTemplate(t *testing.T) {
	for _, template := range []string{"", "   ", "\t\n"} {
		if argv := BuildAIArgv(template, "prompt", nil); argv != nil {
			t.Errorf("BuildAIArgv(%q) = %#v, want nil", template, argv)
		}
	}
}
