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
		t.Fatalf("warn inesperado: %q", warn)
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
	if argv := BuildAIArgv("", "algo", nil); len(argv) != 0 {
		t.Errorf("argv = %#v, want empty", argv)
	}
}
