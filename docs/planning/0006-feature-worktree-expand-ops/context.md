---
feature: 0006-feature-worktree-expand-ops
freshness: a681588d857dcd5157c79ffb5a73c02357a72849
codegraph: ready
generated_by: codebase-researcher
---

# Context: worktrees expandibles y operables (R30–R40)

## Scope
- In: sub-fila navegable y operable por worktree en la TUI; toggle `space`
  configurable; persistencia de expansión en `collapsed.json`; dedupe/huérfanos;
  render dedicado de sub-fila; búsqueda `/` por rama/basename con expansión
  transitoria; panel de detalle mínimo.
- Out: recolección de estado git por worktree; cambios en `discovery`,
  `gitstatus`, `group` o `cmd/gitdash/print.go`; nuevas acciones git.

## Files to Touch

| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `entryKind` consts | internal/tui/table.go | 23-30 | Añadir `kindWorktree` al iota (`kindRepo, kindPrimary, kindSecondary, kindWorktree`). |
| `tableEntry` struct | internal/tui/table.go | 36-40 | La sub-fila necesita datos de worktree (path/branch/head). Añadir campo propio (p. ej. `wt gitstatus.Worktree`) — NO reutilizar `r row` (no hay `Snapshot`/`State` de worktree). |
| `rows()` | internal/tui/table.go | 74-93 | NO debe emitir sub-filas: `rows()` es la base filtrada+ordenada que consume `group.Arrange`. Debe seguir excluyendo worktrees descubiertos (85-87) y mantiene `matchSearch` (82-84). Aquí vive el `snap` para la búsqueda de worktrees (R40). |
| `entries()` | internal/tui/table.go | 99-129 | Punto de inyección: tras `out = append(out, kindRepo...)` (línea 126) inyectar las sub-filas si el padre está visible y expandido. El guard de plegado de grupo (`skipPrim`/`skipSec`, 123-125) ya garantiza que no se inyecten bajo un grupo plegado (R36.2). `group.Arrange` se llama ANTES (101), así que nunca ve sub-filas. |
| `worktreeHidden()` | internal/tui/table.go | 168-178 | Dedupe R31.3 y fallback huérfano R31.4: se conserva tal cual. Un wt descubierto queda oculto si su `MainRepo` está entre `m.projects`. |
| `groupOfEntry()` | internal/tui/table.go | 134-148 | `tab` sobre sub-fila debe ser no-op (R39). El `default` accede a `e.r.project` (zero para `kindWorktree`) → devolvería `(ungrouped)` y plegaría la sección por accidente. Guardar en `update.go` (case `fold`) o tratar `kindWorktree` explícitamente. |
| `renderEntry()` | internal/tui/table.go | 240-257 | Añadir `case kindWorktree:` → camino de render DEDICADO (`renderWorktreeRow`). Es la trampa principal: si cae en `renderRow` (default 254-255) leerá un snapshot vacío como `no-up`/error. |
| `renderRow()` / `nameCell()` | internal/tui/table.go | 310-356 | `nameCell` añade `(N wt)` en 348-351. Añadir glyph `▸`/`▾` junto al contador (R34.5). `renderRow` solo sirve a `kindRepo`; la sub-fila usa su propio render (no tocar las celdas de estado por-repo). |
| `matchSearch()` | internal/tui/table.go | 182-187 | R40: extender para matchear también rama/basename de los `Worktrees` del repo. Hoy solo recibe `(p, q)`; necesita el `snap` (disponible en `rows()` línea 77) o un helper `matchSearchRepo(p, snap, q)`. |
| `summary()` | internal/tui/table.go | 212-231 | Sin cambios funcionales (los worktrees no cuentan como repos); verificar que no se altera. |
| `handleKey()` | internal/tui/update.go | 151-343 | Nuevo `case "expand":` (junto a `fold`, 304-315) que toglea `m.expanded[path]` solo sobre `kindRepo` no-worktree con worktrees; `clampCursor()` + `saveCollapsed()`. Guard R39 en `case "fold"`. `case "detail"` (316-330): `enter` sobre sub-fila abre detalle. Búsqueda en vivo ya re-evalúa `entries()` (198-204). |
| `selected()` | internal/tui/app.go | 524-534 | Extender para aceptar `kindWorktree` y resolver el path del worktree (sintetizar `row` con `project{Path: wt.Path, Name: filepath.Base(wt.Path), HasRepo: true}` o exponer un `selectedPath()`). Todas las operaciones usan `r.project.Path`/`HasRepo`/`MarkerErr`. |
| `actionForKey()` | internal/tui/update.go | 347-354 | Resuelve `"space"` → `"expand"` vía `cfg.Keybindings`; sin cambios salvo que el default exista en config (ver abajo). |
| `clampCursor()` / `syncOffset()` | internal/tui/update.go | 140-146 / 450-461 | Ya operan sobre `len(m.entries())`: funcionan con sub-filas sin cambios (R32.2/R32.3). |
| `Model` fields | internal/tui/app.go | 84-126 | Añadir `expanded map[string]bool` (por path canónico del repo principal; default ausente = plegado, R35.2). `collapsed` se mantiene. |
| `New()` | internal/tui/app.go | 138-180 | Inicializar `expanded` y restaurar el estado persistido (bloque 171-178) separando por prefijo de namespace (R35.1/S35.3). |
| `saveCollapsed()` | internal/tui/app.go | 515-520 | Componer namespace de grupos + namespace de expansión antes de `SaveCollapsed` (R35.3). |
| `nameOf()` / `syncOf()` | internal/tui/app.go | 493-511 | Solo escanean `m.projects`; un path de worktree no descubierto cae a la ruta absoluta. R33.7 exige basename: añadir fallback `filepath.Base(path)` (y `syncOf` ya devuelve global, correcto). |
| Cmds de operación | internal/tui/app.go | 271-490 | `startActionCmd`, `fetchBatchCmd`, `recollectCmd`, `openEditorCmd`, `openLazygitCmd`, `openUpdateCmd`, `openCmdCmd`, `openShellCmd` ya reciben `path` — sin cambios; operan sobre el worktree al pasarle su path. |
| `renderDetail()` | internal/tui/detail.go | 20-155 | Añadir camino mínimo para sub-fila de worktree (path, rama o `(detached)`, head; R33.6). NO reutilizar el bloque de estado (51-59 convierte vacío en `"clean"` → inventa estado). |
| `SaveCollapsed`/`LoadCollapsed` | internal/state/state.go | 62-94 | Store plano `map[string]bool` JSON. Definir convención de clave namespaced (p. ej. `wt/<path canónico>`) con polaridad invertida (valor `true` = expandido). No se crea fichero nuevo. `LoadCollapsed` devuelve el mapa completo; la TUI parte por prefijo. |
| `DefaultKeybindings()` | internal/config/config.go | 163-182 | Añadir `"expand": "space"` (R37.1). |
| `hintLabels` | internal/config/config.go | 263-282 | Añadir `"expand": "space expand"` (R37.3). |
| `HintBarLines()` | internal/config/config.go | 287-322 | Incluir `"expand"` en la lista de acciones (292-296) y agruparlo en `row1` con `fold`/`detail` (switch 307-314). |
| Tests TUI | internal/tui/worktree_test.go | 1-121 | Añadir tests R30–R36/R39/R40. `TestWorktreeHiddenS15_1` (13-51) debe seguir pasando: `rows()` no cambia su contrato. |
| Tests TUI | internal/tui/app_test.go | 19-91 | Reutilizar `newTestModel`, `press`, helpers `snapClean`/`proj`. `press(m, " ")` ya stringifica `"space"` (ver Gotchas). |
| Tests TUI | internal/tui/group_test.go | 13-254 | Regresión de plegado (R36.2/R36.3/R39): la expansión no debe alterar headers ni contadores. |
| Tests TUI | internal/tui/commands_test.go | 1-166 | Base para R33.5 (`!`/shell) sobre sub-fila. |
| Tests state | internal/state/state_test.go | 1-123 | Patrón `NewStoreAt(t.TempDir())`; añadir R35 (namespaces sin colisión, corrupto/ausente). |
| Tests gitstatus | internal/gitstatus/worktree_test.go | 10-41 | Regresión: `ParseWorktrees` no cambia (no tocar `gitstatus`). |
| Fixtures | internal/testutil/testutil.go | 110-130 | `NewRepo` + `MakeWorktree(t, dir, wtDir, branch)` para fixtures git reales de worktrees. |

## Contracts

- `gitstatus.Snapshot.Worktrees []Worktree{Path,Branch,Head}` — `internal/gitstatus/status.go:27,36-40`. `Path` absoluto, `Branch` ya sin `refs/heads/` (vacío si detached/bare), `Head` sha corto (7). Excluye el principal.
- `ParseWorktrees(out, mainPath)` — `internal/gitstatus/parse.go:238-269`: excluye `mainPath`, tolera prunable/desconocidas. No modificar.
- `discovery.Project{Path,Name,PrimaryGroup,SecondaryGroup,SyncBranch,HasRepo,IsWorktree,MainRepo,MarkerErr}` — `internal/discovery/scan.go:25-35`; `MainPath()` 37-43. No modificar el modelo.
- `state.Store` plano `map[string]bool` JSON — `internal/state/state.go:62-94`. Fichero `collapsed.json`, atómico (tmp+rename), missing→nil, corrupto→nil.
- `config.Keybindings`/`KeyFor`/`actionForKey` — `internal/config/config.go:232-237`, `internal/tui/update.go:347-354`.
- Contrato de celda `(text, style)` con `pad()` ANTES del estilo — `internal/tui/table.go:233-235,335`. La sub-fila debe respetarlo.
- Semántica `entryKind`: `kindRepo` = fila operable de repo; headers no seleccionables (`selected()` 530-532). `kindWorktree` debe ser navegable y operable.
- Contrato `group.Arrange` — `internal/group/group.go:39-100`: recibe solo filas base; las sub-filas se inyectan después en `entries()` para no alterar headers/orden/contadores.
- Guards `HasRepo`/`MarkerErr` en operaciones — `internal/tui/update.go:243-340`. La sub-fila debe satisfacer `HasRepo=true` (es un repo real) y `MarkerErr=""` (sin marcador).

## Pattern to Follow

- Tests directos de Model → `internal/tui/app_test.go:19-30` (`newTestModel`) y `76-91` (`press`). Construir `Model`, enviar `tea.KeyPressMsg` por `Update`, inspeccionar estado. Sin teatest.
- Fixtures git reales de worktrees → `internal/testutil/testutil.go:110-114` (`MakeWorktree`) + `119-130` (`NewRepo`); ejemplo de recolección real en `internal/gitstatus/worktree_test.go:27-41`.
- Tests de celdas `(text,style)` → `internal/tui/cells_test.go:14-44`.
- Tests de plegado/entradas → `internal/tui/group_test.go:13-66` (conteo de `entries()` e inspección de `kind`).
- Tests del store → `internal/state/state_test.go:9-34` (`NewStoreAt`).

## Tests

- Existing affected: `internal/tui/worktree_test.go:13-51` (rows con wt descubierto), `internal/tui/group_test.go` (conteo de entries), `internal/tui/app_test.go:244-260` (`TestNavigationBoundsS6_2` usa `len(m.rows())`), `internal/state/state_test.go`.
- Framework / runner: `go build ./... && go vet ./... && go test ./...` (AGENTS.md).
- Integration infra: fixtures git reales en `t.TempDir()` (bare origin, worktrees); unit-only para store y modelo. Los tests de behind/diverged requieren `testutil.FetchLocal` (Gotcha 2).
- Suggested targets por escenario:
  - S30.x/S31.x/S34.x/S36.x/S39.x → `internal/tui/worktree_test.go` (nuevo bloque) sobre `entries()`/`View()`.
  - S32.x → `internal/tui/app_test.go` (cursor/scroll).
  - S33.x → `internal/tui/commands_test.go` (path de operación; inspeccionar `m.running[wtPath]`).
  - S35.x → `internal/state/state_test.go` + arranque `New()`.
  - S37.x → `internal/config` (default/rebind/hint) + `internal/tui` (actionForKey).
  - S38.1 → `cmd/gitdash/print.go` sin cambios (test de no-regresión si existe).
  - S40.x → `internal/tui` (matchSearch + expansión transitoria en `entries()`).

## Conventions & Boundaries

- Comentarios en **español** referenciando spec (`R30`, `S30.2`…) — AGENTS.md.
- SDD recortado: `docs/planning/000X-feature-NAME/{proposal,spec}` en el repo; este pipeline usa `issue.md`/`behavior.feature`/`plan.md`/`context.md`.
- Orden atención-primero (`State.Score()`) intacto: las sub-filas no participan del sort base.
- NO tocar `internal/discovery`, `internal/gitstatus`, `internal/group`, `cmd/gitdash/print.go`.
- El binario instalado (`~/.local/bin/gitdash`) se recompila al terminar (AGENTS.md).

## Integration Points (non-obvious)

- **Spacebar stringifica `"space"`** (verificado en `ultraviolet v0.0.0-20260811164956`, `go.mod:15`): `KeySpace` está en `keyTypeString` → `"space"`. En tests, `press(m, " ")` crea `Code:' '` (== `KeySpace`) → `msg.String()=="space"`. Por eso `DefaultKeybindings` debe usar `"space"` como valor, no `" "`.
- **Bubbletea v2 event pump**: cada `tea.Cmd` lee UN evento del canal; en `Update` rearmar con `withPump` (`internal/tui/update.go:126-128`). Aplica a cualquier nuevo `tea.Cmd`.
- **Huérfano R31.4**: un worktree descubierto cuyo `MainRepo` no está en `m.projects` NO se oculta (`worktreeHidden` false, `table.go:168-178`) → queda `kindRepo` top-level. OJO: su propio `Snapshot.Worktrees` (de `git worktree list` lanzado en el worktree) incluye el principal y los demás worktrees → si se permite expandir un `kindRepo` con `IsWorktree==true`, aparecerían sub-filas falsas. La expansión debe restringirse a `!p.IsWorktree`.
- **`nameOf`/`syncOf` solo escanean `m.projects`** (`app.go:493-511`): un path de worktree no descubierto no se resuelve → hay que añadir fallback `filepath.Base` para R33.7.
- **Polaridad vs. claves de grupo**: las claves de grupo usan `true = plegado`; la expansión debe usar namespace propio (p. ej. `wt/<path>`) con `true = expandido`. `LoadCollapsed` devuelve el mapa plano completo: la carga debe partir por prefijo y `saveCollapsed` recomponer ambos espacios (S35.3). Ficheros existentes solo tienen claves de grupo → sin migración.
- **Expansión transitoria por búsqueda (R40)**: `entries()` debe calcular la expansión efectiva como `expanded[path] || (search activa && algún worktree matchea)`, SIN escribir `m.expanded` (S40.5). Al limpiar la búsqueda, el repo vuelve a su estado persistido. Cuando hay búsqueda activa, solo se muestran las sub-filas que matchean (S40.3).
- **Snapshot aún sin recolectar (R31.6)**: `m.states[p.Path]` vacío → `len(snap.Worktrees)==0` → sin glyph y `space` no-op. `nameCell` ya comprueba `len(r.snap.Worktrees)`.
- **Cursor tras togglear (R32.3)**: `clampCursor()` (`update.go:140-146`) se llama tras `expand`/`fold`; si el cursor quedaba en una sub-fila que desaparece, cae dentro de rango.

## Risks / Assumptions

- **Falsa información de estado**: si una sub-fila cae en `renderRow` (default de `renderEntry`) o en `renderDetail` normal, un `Snapshot` vacío se lee como `no-up`/`clean`. Exige render dedicado (tabla) y panel mínimo (detalle). Trampa principal.
- **Colisión de claves** en `collapsed.json`: mezclar namespaces corrompería el plegado de grupos; la carga DEBE partir por prefijo.
- **Dedupe por path**: comparar paths normalizados (`filepath.Clean`) para evitar duplicados por symlink/barra final entre `snap.Worktrees[i].Path` y `m.projects[j].Path` (S31.3).
- **Colisión de tecla**: `space` no está bindeado hoy; `actionForKey` itera un `map` (orden no determinista) — asegurar que ningún otro action mapea `"space"`.
- **Alcance**: sin estado git por worktree; sin cambios en discovery/gitstatus/group/print; sin nuevas acciones git.
- Plan factible contra el código actual; no se detectó bloqueo. El único ajuste de diseño no trivial es que `selected()` devuelve `row` y las operaciones leen `project.Path`/`HasRepo`/`MarkerErr`: resolverlo con un `row` sintético para `kindWorktree` (HasRepo=true, MarkerErr="", Name=basename).
