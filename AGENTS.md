# AGENTS.md — gitdash

Guía para agentes sin contexto previo sobre este proyecto.

## Qué es

TUI (Go + Bubbletea v2) que muestra el estado de todos los repos git del
usuario descubiertos por **fichero marcador** `.gitdash.toml`: branch, cambios
pendientes (dirty) y **↑ahead/↓behind**, con fetch automático en batches y
acciones rápidas (pull/push/editor). Inspirado en
[bircni/git-statuses](https://github.com/bircni/git-statuses) — no es un
fork: solo comparte la idea.

## Stack

- Go 1.26+, module `gitdash`
- `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`
  (import paths `charm.land`, NO github.com/charmbracelet)
- `github.com/BurntSushi/toml`
- Sin cgo: git vía subprocess (`git status --porcelain=v2 --branch`)

## Comandos

```bash
go build ./... && go vet ./... && go test ./...   # build + lint + tests
go build -o bin/gitdash ./cmd/gitdash              # binario
go build -o ~/.local/bin/gitdash ./cmd/gitdash     # binario instalado en PATH
go mod tidy                                        # tras añadir deps
```

```bash
./scripts/gen-fixtures.sh                          # regenera testdata/playground
bin/gitdash --print                                # modo tabla one-shot
# smoke test de la TUI (ver Gotcha 3):
tmux new-session -d -s gd 'XDG_CONFIG_HOME=<tmp> bin/gitdash' && sleep 3 && tmux capture-pane -t gd -p
```

**REGLA**: al terminar cualquier cambio de código, RECOMPILAR el binario
instalado (`go build -o ~/.local/bin/gitdash ./cmd/gitdash`). El usuario
ejecuta el de `~/.local/bin`: un bin stale con cambios ya hechos causa
síntomas falsos (ej. "no encuentra repos" por el rename del marcador).

## Arquitectura (flujo de datos)

```
config → discovery (walk por marcador) → gitstatus (subprocess por repo, pool)
       → eventos por canal → tui (Update/View) → render
```

| Package | Rol |
|---|---|
| `internal/config` | TOML XDG. `Load()` nunca falla: defaults + warning string |
| `internal/discovery` | `Project{Path,Name,Group,SyncBranch,HasRepo,IsWorktree,MainRepo,MarkerErr}`. La carpeta del marcador ES el repo (no se busca `.git` hacia arriba). Poda ocultos + `exclude`. `MainRepo` enlaza worktree→repo principal |
| `internal/gitstatus` | `parse.go` puro (ParsePorcelain, ParseWorktrees, Derive, Score) + `status.go` (Collect, StreamPool, Fetch, Pull, Push). El `Snapshot` lleva `Err` embebido y también la desviación vs sync branch (`SyncBehind`) y sus worktrees; nunca falla duro |
| `internal/cache` | `repos.json` para pintar instantáneo al arrancar; validación por existencia del marcador; corrupto = silencioso |
| `internal/tui` | `app.go` (modelo + pipelines de fondo), `update.go` (Update/View/teclas), `table.go` (filas/orden/celdas/agrupación), `detail.go`, `styles.go` |
| `internal/group` | Arrangement de la vista agrupada a 2 niveles (estilo vroom R24, 0003): `Arrange` + `IsPrimaryHeader`/`IsSecondaryHeader` |
| `internal/testutil` | helpers para crear repos git fixture reales en `t.TempDir()` (bare origin, push upstream, worktrees, ramas) |
| `cmd/gitdash` | `main.go` (TUI) + `print.go` (modo `--print`, tabwriter, mismo orden) |

## Convenciones

- Comentarios de código en **español** referenciando la spec (p. ej. `S5.2`, `R8`).
- **SDD recortado** (contrato): `docs/planning/000X-feature-NAME/{proposal.md,spec.md}`
  con requisitos `R#n SHALL` + escenarios `S#n.#` GIVEN/WHEN/THEN. Desviaciones
  → sección "Revisiones" de spec.md. SIN impl-plan ni tasks.md.
- Tests del modelo **directo** (construir Model, enviar msgs con Update,
  inspeccionar estado) — sin teatest. Patrón: `internal/tui/app_test.go`.
- Estados derivados con precedencia: `diverged > dirty > ahead > behind >
  detached > no-upstream > clean`. `State.Score()` (gitstatus) da el orden
  atención-primero compartido por TUI y `--print`.
- Las celdas de tabla devuelven `(texto, estilo)`: el render hace
  `pad(texto)` ANTES de aplicar estilo (el ANSI rompe el cálculo de ancho).

## Gotchas críticos

1. **Event pump**: cada `tea.Cmd` lee UN evento del canal. SIEMPRE rearmar
   `waitForEvent` (helper `withPump`) en Update tras consumir un evento del
   canal. Sin esto solo llega el primer mensaje y los estados nunca pintaan.
2. **ahead/behind requieren fetch**: los remote-tracking refs solo se
   actualizan con `git fetch`. Los tests que simulan behind/diverged deben
   hacer `testutil.FetchLocal` tras pushear al origin.
3. **TUI smoke tests**: usar **tmux** (`capture-pane`). `script` NO funciona:
   bubbletea v2 bloquea el primer render esperando las respuestas a las
   queries de capacidades kitty del pty tonto (síntoma: alt-screen en
   blanco, proceso vivo, sin stderr).
4. **porcelain v2**: líneas `1 ` → 7 campos antes del path; `2 ` (renames) →
   8 y emite `<nuevo>\t<viejo>`; `u ` → 9. `# branch.oid` da el sha para
   detached. La ruta es TODO lo restante (puede contener espacios).
5. **Fixtures**: el marcador debe commitearse en el commit base, si no
   aparece como untracked y ensucia el estado dirty de todos los repos.
6. **textinput v2 con teclas sintéticas**: `tea.KeyPressMsg` necesita
   `Code` Y `Text` — solo `Code` no inserta runas en el input.

## Probar

```bash
# config real del usuario (roots default: ~/dev)
bin/gitdash
```

```bash
# aislada contra los fixtures
XDG_CONFIG_HOME=$(mktemp -d) bin/gitdash --print
# con config: crea <tmp>/gitdash/config.toml con roots=["<repo>/testdata/playground"]
```
