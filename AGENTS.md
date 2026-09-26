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
go build -o bin/gitdash ./cmd/gitdash              # binario (artefacto de build)
make install                                       # instala bin/gitdash en ~/.local/bin (PREFIX/DESTDIR)
go mod tidy                                        # tras añadir deps
```

```bash
./scripts/gen-fixtures.sh                          # regenera testdata/playground
bin/gitdash --print                                # modo tabla one-shot
# smoke test de la TUI (ver Gotcha 3):
tmux new-session -d -s gd 'XDG_CONFIG_HOME=<tmp> bin/gitdash' && sleep 3 && tmux capture-pane -t gd -p
```

**REGLA**: al terminar cualquier cambio de código, INSTALAR el binario
(`make install`). El usuario ejecuta el de `~/.local/bin`: un bin stale con
cambios ya hechos causa síntomas falsos (ej. "no encuentra repos" por el
rename del marcador).

## CI y protección de `main`

CI vive en `.github/workflows/ci.yml` y corre en **todo PR** y en **todo push
a `main`** (sin filtros `paths`: un workflow skipeado deja los required checks
en pending para siempre y bloquea todos los PRs). Tres jobs:

- **`Build`**: `go build ./...` y `go vet ./...`.
- **`Lint`**: `make lint` → vet + fmt-check (gofmt) + golangci-lint
  **v2.13.2** (versión pineada en el `Makefile`; no hay `.golangci.yml`, corre
  el set de linters por defecto).
- **`Test`**: `go test -race -coverprofile=coverage.out ./...` (suite completa,
  sin `-short`) y un resumen de cobertura en el step summary. La suite es
  **autocontenida**: cada fixture git se crea bajo `t.TempDir()` con
  `internal/testutil`, así que CI **no** necesita `make fixtures` ni
  `testdata/playground`.

Reglas de la rama `main` (ruleset **`protect-main`**, reproducible con
`scripts/setup-repo-protection.sh`, idempotente y con `--dry-run`):

- Merge **solo vía PR**, con los tres checks en verde; force-push y borrado de
  `main` bloqueados.
- Existe **bypass de admin** y es **deliberado** (aprobado por el usuario): un
  admin *podría* pushear directo, pero la intención de trabajo es siempre el
  camino PR. Ningún actor no-admin puede hacerlo.
- `delete_branch_on_merge=true`: GitHub borra la rama remota al mergear.

Ante un merge: verificar que el workflow `push` de `main` quedó verde y que el
badge del README reporta `passing` (el badge cachea unos segundos).

`make smoke` (tmux + pty, ver Gotcha 3) queda **manual y fuera de CI**: necesita
un terminal interactivo que el runner no garantiza.

## Arquitectura (flujo de datos)

```
config → discovery (walk por marcador) → gitstatus (subprocess por repo, pool)
       → eventos por canal → tui (Update/View) → render
```

| Package | Rol |
|---|---|
| `internal/config` | TOML XDG. `Load()` nunca falla: defaults + warning string |
| `internal/discovery` | `Project{Path,Name,Group,SyncBranch,HasRepo,IsWorktree,MainRepo,MarkerErr}`. La carpeta del marcador ES el repo (no se busca `.git` hacia arriba). Poda ocultos + `exclude`. `MainRepo` enlaza worktree→repo principal |
| `internal/gitstatus` | `parse.go` puro (ParsePorcelain, ParseWorktrees, Derive, Score) + `status.go` (Collect, StreamPool, Run, Fetch, Pull, Push, RebaseInProgress). El `Snapshot` lleva `Err` embebido y también la desviación vs sync branch (`SyncBehind`) y sus worktrees; nunca falla duro |
| `internal/cache` | `repos.json` para pintar instantáneo al arrancar; validación por existencia del marcador; corrupto = silencioso |
| `internal/tui` | `app.go` (modelo + pipelines de fondo), `update.go` (Update/View/teclas), `table.go` (filas/orden/celdas/agrupación), `detail.go`, `styles.go` |
| `internal/group` | Arrangement de la vista agrupada a 2 niveles (estilo vroom): `Arrange` + `IsPrimaryHeader`/`IsSecondaryHeader` |
| `internal/testutil` | helpers para crear repos git fixture reales en `t.TempDir()` (bare origin, push upstream, worktrees, ramas) |
| `cmd/gitdash` | `main.go` (TUI) + `print.go` (modo `--print`, tabwriter, mismo orden) |

## Convenciones

- Comentarios de código en **español**, **sin referencias a specs ni a IDs de
  requisito/escenario**: el código es la fuente de verdad. No hay artefactos
  SDD en el repo y no se crean (nada de `docs/planning/`, `proposal.md`,
  `spec.md`, `R#n SHALL`, `S#n.#`).
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
7. **La política de pull es del usuario, no nuestra**: `commands.pull` va
   **sin flags** a propósito. Los flags en la línea de comandos pisan el
   gitconfig, así que un `--ff-only` hardcodeado anulaba un `pull.rebase=true`
   del usuario (comprobado: el mismo repo divergente rebasea con `git pull` pelado
   y no con `git pull --ff-only`). Las variantes con flags existen solo para el
   selector de `p`, que ofrece la política explícita. **No reintroduzcas flags en
   el default.**
8. **Un pull --rebase que choca no es un fallo limpio**: deja el rebase a medias
   (`rebase-merge`/`rebase-apply` en el dir del worktree). Por eso
   `gitstatus.RebaseInProgress` existe y el aviso tiene prioridad sobre los
   hints de divergencia/upstream: decir "falló" invita a reintentar sobre un
   rebase sin resolver. Se resuelve con `git rev-parse --git-path`, no mirando
   `.git/rebase-*` a pelo, porque en un worktree `.git` es un fichero.
9. **El argv resuelto viaja en `actionMsg`/`actionResult`**: con la política
   delegada en el gitconfig, el kind ya no implica los flags. El detalle lo
   muestra; sin eso la UI miente sobre lo que reconcilió.
10. **Las filas se ordenan attention-first**: la posición del cursor NO es la del
   fixture de test. Los tests que necesitan una fila concreta la localizan por
   path (`cursorOn`), no por índice.

## Gotcha de diseño: el selector de pull

`p` no ejecuta: arma `pullArmed` con el path capturado, y la **siguiente** tecla
elige variante (`p`/`r`/`f`/`m` en `PullKinds`). Dos reglas:

- Las cuatro teclas **chocan con acciones reales** de la tabla (`p`=pull,
  `r`=rescan, `f`=fetch), así que el estado armado tiene que consumir la tecla
  **antes** del enrutado normal en `handleKey`.
- Cualquier otra tecla **cancela y sigue su curso normal** (no se consume): es
  lo que evita que la app quede pegada esperando una segunda pulsación. Es el
  patrón de prefix-key, no un modo bloqueante.

El prompt se pinta en la **sección keybinds, sustituyendo las hints**, no en un
toast: los toasts expiran a los 3 s y el selector vive hasta la siguiente tecla.
Keybinds es su sitio porque comparte función con las hints ("qué hago ahora"), y
el banner de stats queda para el resumen y la actividad en curso. Lo mismo aplica
a `removePrompt`: los dos avisos salen de `armedPrompt()`, que devuelve uno u
otro, y `keybindsLines()` deriva de ahí el presupuesto de alto (1 línea con
aviso armado, `defaultHintLines` sin él) para que la caja nunca mida más que su
contenido. Con un aviso armado, `computeLayout` degrada **stats antes que
keybinds** (`keepKeybinds`): si la caja del aviso cayera, la app quedaría
esperando una tecla sin decir cuáles.

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
