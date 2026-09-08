// Package gitstatus recolecta el estado git de los repos vía subprocess
// (spec 0001 R5). Un solo `git status --porcelain=v2 --branch` por repo da
// branch, upstream, ahead/behind y ficheros cambiados; `git log` aporta
// actividad y commits recientes. Sin librerías git: el binario git es la
// única dependencia.
package gitstatus

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"gitdash/internal/discovery"
)

// Snapshot es el estado completo y vivo de un repo, listo para la UI.
type Snapshot struct {
	Status     Status
	Files      []FileEntry
	Commits    []Commit   // últimos 5 (S10.1)
	LastCommit int64      // epoch del último commit (0 si sin commits)
	Worktrees  []Worktree // worktrees del repo (0002 R15), sin el principal
	SyncBranch string     // sync branch usada en la comparación (0002 R14)
	SyncBehind int        // commits de sync ausentes en HEAD (0002 R14)
	SyncKnown  bool       // comparación vs sync calculable
	Err        string     // "" = recolección ok (S5.6)
}

// Worktree es un worktree registrado del repo, listo para el detalle
// (0002 R15).
type Worktree struct {
	Path   string // ruta absoluta del worktree
	Branch string // refs/heads/x o "" si detached/bare
	Head   string // sha corto de HEAD ("" si vacío/prunable)
}

// State deriva el estado visible considerando errores y proyectos sin repo.
func (s Snapshot) State(hasRepo bool) State {
	if s.Err != "" {
		return StateError // S5.6
	}
	if !hasRepo {
		return StateNoRepo // S3.3
	}
	return s.Status.Derive()
}

// Collect recolecta el estado del repo en dir, con la desviación vs la
// sync branch dada (R14, "" = sin comparación). Nunca falla duro: el error
// (si lo hay) viaja dentro del Snapshot para verse en la UI (S5.6).
func Collect(ctx context.Context, dir, syncBranch string) Snapshot {
	var snap Snapshot

	out, err := runGit(ctx, dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		snap.Err = firstLine(err.Error())
		return snap
	}
	st, files := ParsePorcelain(string(out))
	st.Branch = normalizeBranch(st)
	snap.Status, snap.Files = st, files

	if syncBranch != "" {
		// 0004 R23: la rama resuelta se rellena siempre (visible en UI
		// aunque la comparación falle → "<rama> —").
		snap.SyncBranch = syncBranch
		if n, ok := syncBehind(ctx, dir, syncBranch); ok {
			snap.SyncBehind = n // R14: commits de sync ausentes en HEAD
			snap.SyncKnown = true
		}
		// sync ref inexistente o error: SyncKnown=false → "<rama> —" en UI (0004 S23.4)
	}

	logOut, err := runGit(ctx, dir, "log", "-5", "--format=%h%x00%ct%x00%s")
	if err == nil {
		snap.Commits = ParseLog(string(logOut))
		if len(snap.Commits) > 0 {
			snap.LastCommit = snap.Commits[0].When
		}
	}
	// Un repo sin commits es legítimo: el error de log se ignora.

	// 0002 R15: inventario de worktrees (sin el repo principal).
	if wtOut, err := runGit(ctx, dir, "worktree", "list", "--porcelain"); err == nil {
		snap.Worktrees = ParseWorktrees(string(wtOut), dir)
	}
	return snap
}

// syncBehind cuenta los commits de sync ausentes en HEAD con
// `git rev-list --count HEAD..<sync>` (R14): valen para ramas y detached,
// y respectan el merge-base (no es un diff de tips). Cualquier error
// (ref inexistente, repo roto) devuelve known=false.
func syncBehind(ctx context.Context, dir, sync string) (int, bool) {
	out, err := runGit(ctx, dir, "rev-list", "--count", "HEAD.."+sync)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, false
	}
	return n, true
}

// normalizeBranch completa la rama para detached con el sha corto.
func normalizeBranch(st Status) string {
	if st.Detached && st.Branch == "" && len(st.OID) >= 7 {
		return st.OID[:7] // S5.5: "<sha-corto> (detached)" en UI
	}
	return st.Branch
}

// StreamPool recolecta los snapshots de todos los proyectos en paralelo
// (máximo concurrency a la vez) invocando emit por cada uno. defaultSync es
// la sync branch global (R14): cada proyecto puede overriddenla desde el
// marcador. Bloquea hasta terminar o cancelarse por contexto.
func StreamPool(ctx context.Context, projects []discovery.Project, defaultSync string, concurrency int, emit func(path string, snap Snapshot)) {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > runtime.NumCPU()*4 {
		concurrency = runtime.NumCPU() * 4
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for _, p := range projects {
		wg.Add(1)
		go func(p discovery.Project) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if !p.HasRepo {
				emit(p.Path, Snapshot{}) // S3.3: estado no-repo inmediato
				return
			}
			emit(p.Path, Collect(ctx, p.Path, SyncFor(p, defaultSync)))
		}(p)
	}
	wg.Wait()
}

// SyncFor resuelve la sync branch efectiva de un proyecto (R14):
// override del marcador > global.
func SyncFor(p discovery.Project, defaultSync string) string {
	if p.SyncBranch != "" {
		return p.SyncBranch
	}
	return defaultSync
}

// Fetch ejecuta `git fetch` en dir con los args dados (default: --prune).
// El caller aplica el timeout vía contexto (R8).
func Fetch(ctx context.Context, dir string, args ...string) error {
	if len(args) == 0 {
		args = []string{"fetch", "--prune"}
	}
	_, err := runGit(ctx, dir, args...)
	return err
}

// Pull ejecuta `git pull` con los args dados (default: --ff-only) y
// devuelve la salida combinada para el detalle (R9).
func Pull(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) == 0 {
		args = []string{"pull", "--ff-only"}
	}
	return runGitCombined(ctx, dir, args...)
}

// Push ejecuta `git push` con los args dados y devuelve la salida
// combinada (R9).
func Push(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) == 0 {
		args = []string{"push"}
	}
	return runGitCombined(ctx, dir, args...)
}

// runGit ejecuta git en dir y devuelve stdout.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := stderr.String(); msg != "" {
			return nil, fmt.Errorf("git %v: %s", args, firstLine(msg))
		}
		return nil, fmt.Errorf("git %v: %w", args, err)
	}
	return out, nil
}

// runGitCombined ejecuta git y devuelve stdout+stderr mezclados.
func runGitCombined(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// firstLine recorta un mensaje a su primera línea (para UI compacta).
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
