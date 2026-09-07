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
	"strings"
	"sync"

	"gitdash/internal/discovery"
)

// Snapshot es el estado completo y vivo de un repo, listo para la UI.
type Snapshot struct {
	Status     Status
	Files      []FileEntry
	Commits    []Commit // últimos 5 (S10.1)
	LastCommit int64    // epoch del último commit (0 si sin commits)
	Err        string   // "" = recolección ok (S5.6)
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

// Collect recolecta el estado del repo en dir. Nunca falla duro: el error
// (si lo hay) viaja dentro del Snapshot para verse en la UI (S5.6).
func Collect(ctx context.Context, dir string) Snapshot {
	var snap Snapshot

	out, err := runGit(ctx, dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		snap.Err = firstLine(err.Error())
		return snap
	}
	st, files := ParsePorcelain(string(out))
	st.Branch = normalizeBranch(st)
	snap.Status, snap.Files = st, files

	logOut, err := runGit(ctx, dir, "log", "-5", "--format=%h%x00%ct%x00%s")
	if err == nil {
		snap.Commits = ParseLog(string(logOut))
		if len(snap.Commits) > 0 {
			snap.LastCommit = snap.Commits[0].When
		}
	}
	// Un repo sin commits es legítimo: el error de log se ignora.
	return snap
}

// normalizeBranch completa la rama para detached con el sha corto.
func normalizeBranch(st Status) string {
	if st.Detached && st.Branch == "" && len(st.OID) >= 7 {
		return st.OID[:7] // S5.5: "<sha-corto> (detached)" en UI
	}
	return st.Branch
}

// StreamPool recolecta los snapshots de todos los proyectos en paralelo
// (máximo concurrency a la vez) invocando emit por cada uno. Bloquea hasta
// terminar o cancelarse por contexto.
func StreamPool(ctx context.Context, projects []discovery.Project, concurrency int, emit func(path string, snap Snapshot)) {
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
		go func(path string, hasRepo bool) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if !hasRepo {
				emit(path, Snapshot{}) // S3.3: estado no-repo inmediato
				return
			}
			emit(path, Collect(ctx, path))
		}(p.Path, p.HasRepo)
	}
	wg.Wait()
}

// Fetch ejecuta `git fetch --prune` en dir. El caller aplica el timeout
// vía contexto (R8).
func Fetch(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "fetch", "--prune")
	return err
}

// Pull ejecuta `git pull --ff-only` (safe default, R9) y devuelve la
// salida combinada para el detalle.
func Pull(ctx context.Context, dir string) (string, error) {
	return runGitCombined(ctx, dir, "pull", "--ff-only")
}

// Push ejecuta `git push` y devuelve la salida combinada (R9).
func Push(ctx context.Context, dir string) (string, error) {
	return runGitCombined(ctx, dir, "push")
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
