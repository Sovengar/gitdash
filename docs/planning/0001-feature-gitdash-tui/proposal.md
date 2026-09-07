# Proposal: gitdash — panel de estados git

## Intent

TUI interactiva estilo GitHub Desktop: un panel con **todos los repos marcados
por un fichero marcador en disco** mostrando branch, estado dirty y
**↑ahead / ↓behind** (cambios pendientes de subir/bajar), con acciones básicas
por repo. Inspirado en `bircni/git-statuses` (Rust, CLI one-shot) — no es un
fork: reescribimos el modelo de datos en Go + Bubbletea v2 y lo convertimos en
TUI con estado.

La innovación clave vs git-statuses: **el descubrimiento no busca `.git`,
busca un marcador** (`.repo.toml`) — "esto es un proyecto que me importa" — y
de esa carpeta se extrae el repo git. El fetch no es manual: tras cada scan se
lanza **automáticamente en batches** (concurrencia limitada, con timeout) para
mantener ahead/behind fresco sin esperas bloqueantes.

## Scope

### In Scope
- Config XDG (`~/.config/gitdash/config.toml`): marcador, roots, exclusiones,
  editor, opciones de fetch (auto on/off, concurrencia, timeout).
- Discovery: walk ilimitado de los roots configurados con **podas agresivas**
  (directorios ocultos, artefactos), detección de marcador, parseo opcional de
  `name`/`group`, resolución del repo (`.git` dir = repo, `.git` file =
  worktree flag básico).
- Recolección de estado por repo vía subprocess `git status --porcelain=v2
  --branch` + `git log -1`: branch/detached, upstream, ahead/behind, cambios
  tracked, untracked, último commit. Worker pool paralelo.
- Dashboard Bubbletea v2: tabla ordenada por "necesita atención" y actividad,
  filtros (`n` no-limpios, `/` búsqueda), rescan (`r`), vista de detalle
  (`enter`).
- **Fetch automático en batches** tras scan/rescan + manual (`f` repo, `F`
  todos), estados por repo (idle/fetching/ok/failed/sin upstream), timeout por
  fetch.
- Acciones: `p` pull (`--ff-only`, safe default), `P` push, `e` abrir editor
  ($EDITOR con handoff de terminal). Bloqueo de acciones concurrentes sobre el
  mismo repo.
- Detalle por repo: ficheros cambiados, últimos commits, salida de la última
  acción.
- Cache en `~/.cache/gitdash/repos.json` para pintar instantáneo al arrancar y
  rescan en background.
- Modo `--print`: tabla CLI one-shot (homenaje/debug, reutiliza discovery+estado).

### Out of Scope (futuras iteraciones)
- Agrupaciones colapsables (vsocial-backend, vsocial-frontend, ...) — v2.
- Integración profunda de worktrees (listar/mutar; v1 solo detecta y etiqueta).
- Acciones extra (git update, stash, checkout de branches, PRs vía gh CLI).
- Fetch programado/periódico (v1: solo tras scan/rescan y manual).
- Cache incremental por mtime (v1: cache simple validada por existencia).

## Capabilities

### New Capabilities
- `config`: configuración TOML XDG con defaults sensatos y expansión `~`.
- `discovery`: walk por marcador con podas y resolución de repo.
- `gitstatus`: recolección de estado git por subprocess con parsing de
  `porcelain=v2`.
- `autofetch`: fetch automático en batches con concurrencia, timeout y
  re-colección de estado por repo.
- `actions`: pull/push con salida capturada y editor con handoff.
- `tui-core`: dashboard, filtros, orden, detalle, barra de estado.

### Modified Capabilities
- (ninguna — feature 0001 es la base)

## Approach

### Estructura de módulos

```
cmd/gitdash/main.go          # entrypoint + flag --print
internal/config/config.go    # TOML XDG + defaults
internal/discovery/scan.go   # walk por marcador + resolución repo
internal/gitstatus/status.go # subprocess git + parsing porcelain=v2
internal/gitstatus/parse.go  # parsing puro (unit-testeado con fixtures)
internal/cache/cache.go      # persistencia JSON del descubrimiento
internal/tui/app.go          # modelo Bubbletea, teclas, mensajes async
internal/tui/table.go        # tabla, orden, filtros
internal/tui/detail.go       # vista de detalle
internal/tui/styles.go       # lipgloss
testdata/playground/         # repos fixture creados por script
```

### Decisiones

- **Subprocess git, no librerías**: `go-git` es lento para status y `git2go`
  arrastra cgo. `git status --porcelain=v2 --branch` da branch, upstream,
  ahead/behind y cambios en UNA llamada. El binario git es la única
  dependencia externa.
- **Branch desde disco cuando se pueda, subprocess para el resto**: leer
  `.git/HEAD` es instantáneo (patrón de vroom gitinfo), pero dirty/ahead/
  behind requieren el subprocess; un solo comando `status --porcelain=v2`
  gana por simplicidad.
- **Podas, no límite de profundidad**: los roots son curados por el usuario;
  podar ocultos + artefactos (`node_modules`, `target`, ...) hace el walk
  ilimitado barato.
- **Fetch en batches con worker pool acotado** (`fetch.concurrency`, default 4)
  y timeout por fetch (default 30s): nunca bloquea la UI, los estados cambian
  por repo en tiempo real, y tras cada fetch se re-colecciona el estado de ese
  repo para refrescar ahead/behind.
- **Pull `--ff-only`**: safe default; si divergió, falla visible con hint en
  lugar de crear merge sorpresa.
- **Acciones capturadas, editor con handoff**: pull/push corren en background
  con salida capturada (notificación + detalle); el editor usa `tea.ExecProcess`
  (suspende la TUI, restaura al salir).
- **Cache como pintura instantánea**: al arrancar se pinta del cache validado
  (existe marcador + `.git`) y el rescan corre en background; el cache
  corrupto se ignora sin ruido.
