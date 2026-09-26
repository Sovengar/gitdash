package config

import (
	"reflect"
	"testing"
)

// El namespace [ai.*] es abierto: cada acción AI declara su plantilla de
// comando; el marcador (input no confiable) no aporta ejecutable.
func TestAICommandOverride(t *testing.T) {
	path := write(t, `[ai.pull]
command = "jcode -run {prompt}"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if got := cfg.AICommand("pull"); got != "jcode -run {prompt}" {
		t.Errorf("AICommand(pull) = %q, want la plantilla", got)
	}
	if got := cfg.AICommand("commit"); got != "" {
		t.Errorf("AICommand(commit) = %q, want vacío (no configurada)", got)
	}
}

// Una acción ausente no existe: la extensibilidad del namespace no inventa
// plantillas.
func TestAICommandExtensible(t *testing.T) {
	path := write(t, `[ai.commit]
command = "ai commit {prompt}"
`)
	cfg, _ := LoadFrom(path)
	if got := cfg.AICommand("commit"); got != "ai commit {prompt}" {
		t.Errorf("AICommand(commit) = %q", got)
	}
	if got := cfg.AICommand("pull"); got != "" {
		t.Errorf("AICommand(pull) = %q, want vacío", got)
	}
}

// El límite de seguridad: el prompt del marcador entra como UN elemento de
// argv, jamás interpolado en un `sh -c`. Espacios, `;`, comillas y `$` no
// cambian el número de elementos.
func TestBuildAIArgvPromptEsUnSoloElemento(t *testing.T) {
	prompt := `he said "hi"; rm -rf / && echo $HOME`
	argv := BuildAIArgv("jcode -run {prompt}", prompt, nil)
	want := []string{"jcode", "-run", prompt}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %#v, want %#v", argv, want)
	}
}

// Los placeholders de contexto se sustituyen por campo.
func TestBuildAIArgvContextPlaceholders(t *testing.T) {
	argv := BuildAIArgv("ai --branch {branch} --behind {behind} {prompt}",
		"arregla", map[string]string{"branch": "main", "behind": "3"})
	want := []string{"ai", "--branch", "main", "--behind", "3", "arregla"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %#v, want %#v", argv, want)
	}
}

// El reemplazo es de una sola pasada: un prompt que contiene `{branch}` queda
// literal (no se expande en cascada).
func TestBuildAIArgvNoReescaneaElPrompt(t *testing.T) {
	argv := BuildAIArgv("ai {prompt}", "usa {branch}", map[string]string{"branch": "main"})
	if len(argv) != 2 || argv[1] != "usa {branch}" {
		t.Errorf("argv = %#v, want el prompt literal", argv)
	}
}

// Plantilla vacía = sin argv; la TUI lo rechaza antes de lanzar nada.
func TestBuildAIArgvPlantillaVacia(t *testing.T) {
	if argv := BuildAIArgv("", "algo", nil); len(argv) != 0 {
		t.Errorf("argv = %#v, want vacío", argv)
	}
}
