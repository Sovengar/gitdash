// Package state persiste el estado de la UI (grupos plegados) entre sesiones.
// Las claves son el nombre del primario o "primario/secundario":
// el composite key evita colisión de nombres entre primarios distintos.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DirName es el subdirectorio de gitdash bajo $XDG_STATE_HOME.
const DirName = "gitdash"

// FileName es el nombre del fichero de estado de UI.
const FileName = "collapsed.json"

// WorktreePrefix es el namespace de las claves de expansión de worktrees
// dentro de collapsed.json. Convención: `wt/<path canónico>` con
// valor true = expandido. La polaridad es la inversa a la de las claves de
// grupo (donde true = plegado); la carga separa ambos espacios por prefijo.
const WorktreePrefix = "wt/"

// Store accede al directorio de estado persistente.
type Store struct {
	base string
}

// DefaultBaseDir resuelve el directorio base de estado según plataforma:
// $XDG_STATE_HOME/gitdash si está definida, si no ~/.local/state/gitdash.
func DefaultBaseDir() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, DirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", DirName), nil
}

// NewStore crea el store usando DefaultBaseDir.
func NewStore() (*Store, error) {
	base, err := DefaultBaseDir()
	if err != nil {
		return nil, err
	}
	return &Store{base: base}, nil
}

// NewStoreAt crea un store sobre un directorio arbitrario (usado en tests).
func NewStoreAt(base string) *Store {
	return &Store{base: base}
}

// Base devuelve el directorio raíz del store.
func (s *Store) Base() string { return s.base }

// CollapsedFile devuelve la ruta del fichero collapsed.json.
func (s *Store) CollapsedFile() string {
	return filepath.Join(s.base, FileName)
}

// SaveCollapsed persiste el mapa de grupos colapsados a disco (átomico).
// Las claves son el nombre del primario o "primario/secundario".
// Best-effort: los errores no son fatales (no bloquear la UI).
func (s *Store) SaveCollapsed(groups map[string]bool) error {
	data, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal collapsed.json: %w", err)
	}
	if err := os.MkdirAll(s.base, 0o755); err != nil {
		return fmt.Errorf("could not create state directory %s: %w", s.base, err)
	}
	target := s.CollapsedFile()
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("could not write collapsed.json: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("could not replace collapsed.json: %w", err)
	}
	return nil
}

// LoadCollapsed lee el mapa de grupos colapsados desde disco.
// Si el fichero no existe devuelve nil sin error; si está corrupto
// devuelve un mapa vacío (el usuario pierde el estado pero no la sesión).
func (s *Store) LoadCollapsed() map[string]bool {
	data, err := os.ReadFile(s.CollapsedFile())
	if err != nil {
		return nil // primer arranque o limpieza: todos expandidos
	}
	groups := make(map[string]bool)
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil // corrupto: empezar limpio
	}
	return groups
}
