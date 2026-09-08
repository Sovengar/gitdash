// Package cache persiste el descubrimiento en disco (spec 0001 R11) para
// pintar la tabla al instante al arrancar mientras el rescan corre en
// background.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gitdash/internal/discovery"
)

// FileName es el fichero de cache dentro del dir XDG de cache.
const FileName = "repos.json"

// DirName es el subdirectorio de gitdash bajo $XDG_CACHE_HOME.
const DirName = "gitdash"

// version del formato; un fichero de otra versión se ignora.
// v2 añade sync_branch y main_repo (spec 0002).
// v3 reemplaza group por primary_group/secondary_group (spec 0003 R22).
const version = 3

// Entry es un repo persistido.
type Entry struct {
	Path           string `json:"path"`
	Name           string `json:"name"`
	PrimaryGroup   string `json:"primary_group,omitempty"`
	SecondaryGroup string `json:"secondary_group,omitempty"`
	SyncBranch     string `json:"sync_branch,omitempty"`
	HasRepo        bool   `json:"has_repo"`
	IsWorktree     bool   `json:"is_worktree"`
	MainRepo       string `json:"main_repo,omitempty"`
}

// File es el documento JSON completo.
type File struct {
	Version int     `json:"version"`
	Repos   []Entry `json:"repos"`
}

// Path devuelve la ruta del fichero de cache ($XDG_CACHE_HOME/gitdash).
func Path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, FileName), nil
}

// Load lee el cache y devuelve los proyectos aún válidos (el marcador debe
// seguir existiendo en su carpeta; S11.2). Cache corrupto o versión
// desconocida = lista vacía sin error (S11.3).
func Load(path, marker string) []discovery.Project {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != version {
		return nil
	}
	var projects []discovery.Project
	for _, e := range f.Repos {
		if e.Path == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.Path, marker)); err != nil {
			continue // S11.2: directorio/marcador borrado → descartar
		}
		projects = append(projects, discovery.Project{
			Path:           e.Path,
			Name:           e.Name,
			PrimaryGroup:   e.PrimaryGroup,
			SecondaryGroup: e.SecondaryGroup,
			SyncBranch:     e.SyncBranch,
			HasRepo:        e.HasRepo,
			IsWorktree:     e.IsWorktree,
			MainRepo:       e.MainRepo,
		})
	}
	return projects
}

// Save persiste los proyectos descubiertos (se llama al final de cada
// rescan; R11). Best-effort: los errores no son fatales.
func Save(path string, projects []discovery.Project) error {
	f := File{Version: version, Repos: make([]Entry, 0, len(projects))}
	for _, p := range projects {
		f.Repos = append(f.Repos, Entry{
			Path:           p.Path,
			Name:           p.Name,
			PrimaryGroup:   p.PrimaryGroup,
			SecondaryGroup: p.SecondaryGroup,
			SyncBranch:     p.SyncBranch,
			HasRepo:        p.HasRepo,
			IsWorktree:     p.IsWorktree,
			MainRepo:       p.MainRepo,
		})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
