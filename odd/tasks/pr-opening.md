# Feature: PR opening (crear PR/MR desde gitdash)

## Objetivo

Crear un pull/merge request desde la TUI de gitdash, sin salir de ella: un panel
overlay captura título, cuerpo, branch base y draft, y `gh`/`glab` ejecuta la
creación por debajo con esos parámetros.

## Decisiones tomadas

Estas decisiones vienen del usuario y condicionan todo lo demás. No reabrirlas
sin consultarlo.

| Decisión | Valor | Por qué |
|---|---|---|
| Flujo | gitdash arma el argv completo y ejecuta `gh`/`glab` | gh y glab son no-interactivos con `-t`/`-b`/`-B`/`-d`/`-l`/`-y`; no hace falta handoff de terminal ni navegador |
| Provider | automático desde `git remote get-url` | sin heurísticas en config; `ParseRemoteURL` de prdash ya resuelve scp/ssh/https y subcarpeta |
| Base del PR | `sync_branch` del repo, prellenada | el snapshot ya lo tiene; no se elige a ojo |
| Puertas | GitHub → `gh pr create`, GitLab → `glab mr create` | cubre github.com y el GitLab self-managed del usuario |
| Subcarpeta | `clone_base` derivado de `api_base` | el GitLab del usuario vive en `/git/`, sin esto toda URL apunta mal |
| Body del PR | **no** se lee del marcador commiteado | límite de confianza de AGENTS.md: el marcador es input no fiable |

## Fuera de alcance

- **Panel de PRs para aprobar/mergear/listar.** Eso es prdash, que ya lo hace y
  ya tiene 48 KB de CHANGELOG de bugs resueltos. Acá se **crean**.
- Comentarios, worktrees de review, simulación, auto-review.
- Backoff, paginación, snapshot en disco, polling global: son cosas del inbox
  global de prdash. Acá es una acción sobre el repo bajo el cursor.
- Bitbucket, Azure DevOps, Codeberg.

## Mapa de lo que ya existe (relevante)

- `discovery.Project` **no** tiene branch. La branch viva es
  `r.snap.Status.Branch`; la base es `r.snap.SyncBranch` / `SyncFor(p, default)`.
- **No existe ninguna lectura de remote en `gitdash`.** `internal/gitstatus`
  lanza 4 verbos en `Collect` y ninguno es de remote. Hay que agregarlo y tiene
  que pasar por `runGit` para que quede en el command log.
- El tabla de "acciones válidas" **es** `DefaultKeybindings()` (`config.go:197`).
  Una acción sin estar ahí hace que `[keybindings]` la liste como obsoleta.
- Cuatro listas de acciones que hay que tocar de forma coherente:
  `DefaultKeybindings`, `hintLabels`, la lista literal dentro de `HintBarLines`,
  y `commandActions`/`rowActions`.
- `exec.Command` en `internal/tui/app.go` **ya son 6** (editor, lazygit,
  `pull_ai`, visual, `!`, shell). AGENTS.md todavía dice 4: corregir.
- `bubbles/v2` trae `textarea`: el cuerpo del PR no necesita textarea casero.
- Gotcha 6: `tea.KeyPressMsg` necesita `Code` **y** `Text` para insertar runas.
- Gotcha 1: todo `tea.Cmd` que lee del canal debe rearmar `withPump`.
- Gotcha 10: los tests localizan la fila **por path** (`cursorOn`), nunca por
  índice, porque el orden es attention-first.
- `Config.KeyByAction()` está muerto: la TUI usa `actionForKey`.

## Portable desde prdash (mismas versiones de charm.land/*, sin tocar go.mod)

- `internal/forge/tool` → Runner con timeout, env homogéneo (`LC_ALL=C`,
  `GIT_TERMINAL_PROMPT=0`), `Error` con exit code.
- `internal/reporesolver.ParseRemoteURL` → normaliza remote → RepoRef.
- `internal/forge/model` → `RepoRef` y la convención `Known bool`.
- Derivación `clone_base` desde `api_base` (`/git/api/v4/` → `git`).

NO portar: streams, paginación, polling, backoff, snapshot, comentarios,
worktrees, Herdr. Son la otra arquitectura de eventos.

## Tasks

### T1 — `internal/forge`: resolver el repo a un forge
`RepoRef` (Forge, Host, Project, ClonePrefix) + `ParseRemoteURL` + detección de
provider por host + derivación de `clone_base`. Puro, sin I/O.

Puro y table-driven con la tabla de casos de prdash: scp-like
`git@host:o/r.git`, `ssh://git@host/o/r`, `https://host/o/r.git`, sin `.git`,
con barra final, con prefijo de subcarpeta, host desconocido, ruta local.

**Done when** los casos de la tabla pasan y `HostUnknown` no se confunde con
`RutaLocal`. **Checks:** `go test ./internal/forge/...`

### T2 — `internal/forge`: construir y ejecutar el argv de creación
`BuildCreateArgv(ref, params) []string` para gh y para glab, + `Runner` con
timeout y env homogéneo.

`Params`: Title, Body, Base, Head, Draft, Labels.

Mapa de flags (verificado contra las CLIs instaladas):

| | gh | glab |
|---|---|---|
| título | `-t` | `-t` |
| cuerpo | `-b` | `-d` |
| base | `-B` | `-b` |
| head | `-H` | `-s` |
| draft | `-d` | `--draft` |
| labels | `-l` (repetible) | `-l` (repetible) |
| sin prompt | — | `-y` |

Ojo: en gh `-b` es body y `-B` es base; en glab `-b` es base y `-d` es
description. Es la trampa más fácil de esta task.

**Done when** ambos argv salen con los flags correctos y cada valor es **un
elemento** de argv (nunca shell-concatenado). **Checks:** `go test ./internal/forge/...`

### T3 — Overlay de TUI
`prArmed` (captura path, branch, sync branch al armar), el panel overlay, el
textinput del título, el textarea del cuerpo, la selección de base y el toggle
de draft.

Es un **view mode** como `logOpen`, no un estado armado de prefix-key: vive
varias pulsaciones, así que la tecla de submit lo distingue de "elegir
variante".

Prompt de keybinds vía `promptLine()` (un `case` más), `keybindsLines()` deriva
solo, `computeLayout` recibe el presupuesto.

**Done when** `esc` cierra sin crear, submit valida (título no vacío, base no
vacía), y el prompt aparece en la sección keybinds. **Checks:** `go test ./internal/tui/...`

### T4 — Ejecución, wiring y docs
Registrar `pr` en `DefaultKeybindings`, `hintLabels`, la lista de
`HintBarLines`, `commandActions`, `rowActions`. Conectar el argv con el Runner,
capturar stdout (sin handoff: no hay TTY que ceder), toast + recollect al volver,
entrada de command log con el argv resuelto.

Documentar en AGENTS.md: la sección de gotchas de diseño, la corrección de
"los 4 exec.Command" → el número real, y la fila de `internal/forge` en la
tabla de paquetes.

**Done when** `make lint` en verde, suite completa en verde, `make install`
hecho, smoke con tmux. **Checks:** `go build && go vet && go test ./...`

## Riesgos

- **`gh`/`glab` no instalados**: `exec.LookPath` + toast, sin handoff. Es el
  mismo trato que `lazygit`.
- **No autenticados**: no se detecta antes de ejecutar; `gh`/`glab` fallan
  solos y el mensaje va al toast. No inventar auth: gitdash no la gestiona.
- **`glab` en gitlab.com sin token** (verificado en la máquina del usuario): el
  fallo es del entorno, no del código. Mencionarlo, no workaroundearlo.
- **El cuerpo del PR no sale del marcador.** Si algún día se quiere, es una
  decisión de trust boundary aparte, no un default.

## Evidencia (commits)

| Task | Commit | Checks |
|---|---|---|
| (previo) guard visual | `8b2c87a` | build + vet + gofmt + `go test -race ./...` + `make lint` |
