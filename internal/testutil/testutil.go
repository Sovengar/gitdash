// Package testutil construye repos git de prueba deterministicos para los
// tests de discovery, gitstatus y tui (fixtures de la spec 0001).
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// git ejecuta un comando git en dir y falla el test si devuelve error.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v en %s: %v\n%s", args, dir, err, out)
	}
}

// Init crea un repo git vacío en dir (branch main) con identidad local.
func Init(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@gitdash.local")
	git(t, dir, "config", "user.name", "gitdash tests")
}

// CommitFiles escribe ficheros (map ruta→contenido) en dir y los commitea.
func CommitFiles(t *testing.T, dir string, files map[string]string, msg string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", msg)
}

// Marker escribe el marcador .repo.toml con los metadatos dados
// ("" como valor = clave omitida). Con malformed=true escribe TOML inválido.
func Marker(t *testing.T, dir, name, group string, malformed bool) {
	t.Helper()
	var content string
	if malformed {
		content = "name = [roto\n"
	} else {
		if name != "" {
			content += "name = \"" + name + "\"\n"
		}
		if group != "" {
			content += "group = \"" + group + "\"\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".repo.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// InitBare crea un repo bare que hace de origin.
func InitBare(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "--bare", "-b", "main")
}

// AddUpstream conecta dir con el bare origin, pushea la rama main y la deja
// como upstream trackeado.
func AddUpstream(t *testing.T, dir, origin string) {
	t.Helper()
	git(t, dir, "remote", "add", "origin", origin)
	git(t, dir, "push", "-u", "origin", "main")
}

// PushUpstreamCommits crea n commits en un clone temporal del bare origin
// y los pushea, dejando el repo local detrás (behind/diverged).
func PushUpstreamCommits(t *testing.T, bare string, n int, prefix string) {
	t.Helper()
	clone := t.TempDir()
	git(t, clone, "clone", "--quiet", bare, ".")
	git(t, clone, "config", "user.email", "test@gitdash.local")
	git(t, clone, "config", "user.name", "upstream")
	for i := 0; i < n; i++ {
		CommitFiles(t, clone, map[string]string{"up_" + prefix + string(rune('a'+i)) + ".txt": "up"}, prefix)
	}
	git(t, clone, "push", "--quiet", "origin", "main")
}

// Detach pone el repo en HEAD detached sobre el commit actual.
func Detach(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "checkout", "--detach", "--quiet", "HEAD")
}

// MakeWorktree añade un worktree del repo en wtDir (su .git es un fichero).
func MakeWorktree(t *testing.T, dir, wtDir, branch string) {
	t.Helper()
	git(t, dir, "worktree", "add", "--quiet", wtDir, "-b", branch)
}

// NewRepo crea un repo con un commit base; con upstream=true además lo
// conecta a un origin bare (remoto local) con la rama trackeada. Devuelve
// el repo y el path del bare origin.
func NewRepo(t *testing.T, upstream bool) (dir, origin string) {
	t.Helper()
	dir = t.TempDir()
	Init(t, dir)
	CommitFiles(t, dir, map[string]string{"base.txt": "base"}, "base")
	if upstream {
		origin = filepath.Join(t.TempDir(), "origin.git")
		InitBare(t, origin)
		AddUpstream(t, dir, origin)
	}
	return dir, origin
}

// WriteUncommitted modifica ficheros ya trackeados sin commitear.
func WriteUncommitted(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// WriteUntracked crea ficheros nuevos sin trackear.
func WriteUntracked(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	WriteUncommitted(t, dir, files)
}

// BreakGit corrompe el .git de un repo para probar el estado error (S5.6).
func BreakGit(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("basura"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// FetchLocal actualiza los remote-tracking refs del repo local (los tests
// que simulan behind/diverged necesitan fetch para ver el upstream nuevo).
func FetchLocal(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "fetch", "--quiet", "origin")
}
