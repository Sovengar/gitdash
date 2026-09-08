// Package discovery descubre proyectos por fichero marcador en los roots
// configurados (spec 0001 R2-R4).
//
// Un directorio es proyecto si contiene el marcador (default .gitdash.toml).
// El repo git se resuelve en la MISMA carpeta del marcador: .git directorio
// = repo normal, .git fichero (gitdir:) = worktree, ausencia = proyecto sin
// repo. El walk es ilimitado en profundidad pero poda directorios ocultos,
// excluidos y ilegibles.
package discovery

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"gitdash/internal/config"
)

// Project es un proyecto descubierto por el marcador.
type Project struct {
	Path           string // ruta absoluta (carpeta del marcador)
	Name           string // name del marcador o nombre del directorio
	PrimaryGroup   string // primary_group del marcador o "" (0003 R18)
	SecondaryGroup string // secondary_group del marcador; "" = sin segundo nivel (0003 R18)
	SyncBranch     string // sync_branch del marcador (0002 R14); "" = usar la global
	HasRepo        bool   // existe .git (directorio o fichero)
	IsWorktree     bool   // .git es un fichero gitdir: (R3.2)
	MainRepo       string // 0002 R15: repo principal si IsWorktree ("" = no aplica)
	MarkerErr      string // error de parseo del marcador (S4.3)
}

// MainPath es el repo al que pertenece el proyecto (sí mismo salvo worktrees).
func (p Project) MainPath() string {
	if p.MainRepo != "" {
		return p.MainRepo
	}
	return p.Path
}

// Scan recorre los roots de la config y devuelve los proyectos ordenados
// por ruta. Los roots ilegibles se reportan como error agregado sin abortar
// el resto.
func Scan(cfg config.Config) ([]Project, error) {
	var projects []Project
	var errs []string

	for _, root := range cfg.Roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", root, err))
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			errs = append(errs, fmt.Sprintf("root ilegible: %s", root))
			continue
		}
		ps, err := scanRoot(abs, cfg)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", root, err))
			continue
		}
		projects = append(projects, ps...)
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Path < projects[j].Path })

	var err error
	if len(errs) > 0 {
		err = fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return projects, err
}

// scanRoot hace el walk de un root con podas (R2).
func scanRoot(root string, cfg config.Config) ([]Project, error) {
	exclude := make(map[string]bool, len(cfg.Exclude))
	for _, name := range cfg.Exclude {
		exclude[name] = true
	}

	var projects []Project
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // ilegible: se salta sin abortar (R2)
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && (isHidden(d.Name()) || exclude[d.Name()]) {
			return fs.SkipDir // S2.2
		}
		if hasMarker(path, cfg.Marker) {
			projects = append(projects, inspect(path, cfg.Marker)) // S2.1
			// seguimos descendiendo: proyectos anidados válidos (S2.4)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// inspect clasifica un directorio con marcador (R3, R4).
func inspect(dir, marker string) Project {
	p := Project{
		Path: dir,
		Name: filepath.Base(dir),
	}

	metadata, err := parseMarker(filepath.Join(dir, marker))
	if err != nil {
		p.MarkerErr = err.Error() // S4.3: visible, sin excluir
	} else {
		if metadata.Name != "" {
			p.Name = metadata.Name
		}
		p.PrimaryGroup = metadata.PrimaryGroup
		// 0003 R18/S18.3: secondary sin primary se ignora (cae en ungrouped)
		if metadata.PrimaryGroup != "" {
			p.SecondaryGroup = metadata.SecondaryGroup
		}
		p.SyncBranch = metadata.SyncBranch
	}

	switch k, main := classifyGit(filepath.Join(dir, ".git")); k {
	case gitDir:
		p.HasRepo = true // S3.1
	case gitFile:
		p.HasRepo = true
		p.IsWorktree = true // S3.2
		p.MainRepo = main   // 0002 R15: para plegar wt bajo su repo principal
	default:
		// S3.3: sin repo, queda visible con HasRepo=false
	}
	return p
}

// markerMeta son los metadatos opcionales del marcador (R4).
// 0003 R18: la clave `group` desaparece (cambio duro, sin fallback).
type markerMeta struct {
	Name           string `toml:"name"`
	PrimaryGroup   string `toml:"primary_group"`
	SecondaryGroup string `toml:"secondary_group"`
	SyncBranch     string `toml:"sync_branch"` // R14: override de la sync branch
}

// parseMarker lee name/primary_group/secondary_group del marcador; campos
// ausentes = vacío (S4.2).
func parseMarker(path string) (markerMeta, error) {
	var meta markerMeta
	raw, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	if err := toml.Unmarshal(raw, &meta); err != nil {
		return meta, fmt.Errorf("marker: %v", err)
	}
	return meta, nil
}

// hasMarker comprueba la existencia del fichero marcador en dir.
func hasMarker(dir, marker string) bool {
	info, err := os.Stat(filepath.Join(dir, marker))
	return err == nil && !info.IsDir()
}

// gitKind clasifica la ruta .git de un proyecto.
type gitKind int

const (
	gitNone gitKind = iota
	gitDir
	gitFile
)

// classifyGit distingue repo normal de worktree (R3). Devuelve el path
// del repo principal cuando .git es un fichero gitdir: (`<main>/.git/...`).
func classifyGit(gitPath string) (gitKind, string) {
	info, err := os.Lstat(gitPath)
	if err != nil {
		return gitNone, ""
	}
	if info.IsDir() {
		return gitDir, ""
	}
	raw, err := os.ReadFile(gitPath)
	if err != nil {
		return gitNone, ""
	}
	if rest, ok := strings.CutPrefix(string(raw), "gitdir:"); ok {
		main := strings.TrimSpace(rest)
		// worktrees registrados: <main>/.git/worktrees/<nombre>
		main = filepath.Dir(filepath.Dir(main))
		return gitFile, filepath.Dir(main)
	}
	return gitNone, ""
}

// isHidden reporta si un nombre de directorio está oculto.
func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}
