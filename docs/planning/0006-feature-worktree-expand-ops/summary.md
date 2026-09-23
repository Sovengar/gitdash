# Summary: 0006-feature-worktree-expand-ops

## Metadata

- **Completed:** 2026-09-23
- **Plan Number:** 0006
- **Type:** feature
- **Branch:** `feat/0006-worktree-expand-ops`
- **Base:** `854ce68`
- **Worktree:** `~/dev/projects/gitdash.feat-0006-worktree-expand-ops`
- **Reviewer:** success — sin findings abiertos (re-review de `c5c4405` + `fc092dd`)
- **Push:** pendiente de autorización explícita del usuario

## Qué se hizo

Desde el menú principal, un repo con worktrees se expande/pliega con `space`
(configurable). Expandido, cada worktree de `git worktree list --porcelain`
aparece como **sub-fila navegable y operable** con toda la superficie de acciones
de una fila de repo (fetch, pull, sync, push, lazygit, update, editor,
recollect, comando `!` y detalle). La expansión persiste entre sesiones; `/`
encuentra worktrees por rama o basename y revela su padre expandido de forma
transitoria (sin tocar la persistencia).

Todo el comportamiento nuevo vive en la capa TUI, más una convención de clave en
`state.Store` y una entrada de keybinding en config. `gitstatus`, `discovery`,
`group` y el modo `--print` **no se tocan**.

### Mapa de cambios por commit

| Commit | Contenido |
|--------|-----------|
| `899d9fe` | ADR-0006 (estado de expansión + modelo de filas) |
| `ac1d4d9` | Estado namespaced de expansión + keybinding `expand` (R35, R37) |
| `42c37b7` | Sub-filas expandibles y operables; render dedicado, selección por path, búsqueda, `--print` no-regresión (R30–R36, R38–R40) |
| `e5497aa` | README: expansión y operaciones por worktree |
| `4e330cf` | Refactor: `discoveredByPath` para lookup de path de worktree |
| `c5c4405` | Fixes de review (HIGH-1, MEDIUM-2 + tests LOW) |
| `fc092dd` | Docs: decisión S33.6 + reformulación S32.3 en "Revisiones" |

## Decisiones (ADR-0006)

1. **Persistencia** — reutilizar `state.Store` / `collapsed.json` (sin fichero
   nuevo, R35) bajo un namespace propio `wt/<path canónico>` con polaridad
   **inversa** a las claves de grupo: valor `true` = **expandido**, ausencia =
   plegado. Carga/guardado parten y recomponen ambos espacios por el prefijo
   `state.WorktreePrefix` (`"wt/"`). Sin migración (los ficheros existentes solo
   tienen claves de grupo). Ausente/corrupto → plegado por defecto.
2. **Modelo de filas** — nuevo `entryKind` `kindWorktree` sintetizado en la capa
   TUI con los datos crudos de `gitstatus.Worktree` (path/rama/head). La sub-fila
   se **inyecta** en `entries()` tras `group.Arrange`, justo después de la fila
   del padre, sin alterar headers, orden, contadores ni resumen. Render dedicado
   (`renderWorktreeRow`) que **no lee `Snapshot`** y deja vacías/dim las celdas de
   estado por-worktree. La selección resuelve un `row` sintético
   (`HasRepo=true`, `MarkerErr=""`, `Name=basename`) para reutilizar las
   operaciones ya parametrizadas por path.
3. **Cobertura y dedupe** — se listan **todos** los worktrees del snapshot
   (incluidos sin marcador y fuera de roots); el principal nunca es sub-fila. Un
   worktree descubierto con marcador aparece **una sola vez** como sub-fila
   (dedupe reutilizando el ocultado de 0002 R15); el fallback de huérfanos se
   preserva (top-level con `[wt]`, no expandible). Igualdad de paths normalizada
   con `filepath.Clean` en ambos lados.
4. **Búsqueda transitoria** — `entries()` calcula la expansión efectiva como
   `expanded[path] || (búsqueda activa && algún worktree matchea)` **sin** escribir
   `m.expanded`; al limpiar la búsqueda el repo vuelve a su estado persistido.
5. **Detalle** — sub-fila sin snapshot: panel mínimo (path, rama o `(detached)`,
   head) sin inventar estado git. Sub-fila descubierta con snapshot vivo: muestra
   ese snapshot (decisión del usuario; ver MEDIUM-1).

## Findings de review y resolución

| ID | Severidad | Finding | Resolución |
|----|-----------|---------|------------|
| HIGH-1 | High | `nameCell` pintaba glyph/contador `▸/▾ (N wt)` en worktree huérfano (`kindRepo` + `IsWorktree`) donde `space` es no-op → affordance muerta | Gateado con el mismo guard `m.expandable(r)` (no solo `len(Worktrees)`); test S31.4 con snapshot realista |
| MEDIUM-1 | Medium | S33.6 (texto literal "no muestra estado git derivado") contradecía la decisión del usuario: un worktree descubierto con snapshot vivo debe mostrarlo | Documentado como desviación intencional en "Revisiones" de `behavior.feature`; `detail.go` sin cambios; test S33.6 verifica la intención real |
| MEDIUM-2 | Medium | `worktreeHidden` no normalizaba el path (solo `discoveredByPath`) → duplicado por symlink/barra final | `filepath.Clean` en ambos lados; test `TestWorktreeDedupeNormalizedPathsMEDIUM_2` |
| LOW-a | Low | S32.3 incoherente con R30.6 (`space` sobre sub-fila es no-op, no pliega el padre) | Escenario reformulado con un camino real (filtro `d`); test pulsa `d` dos veces |
| LOW-b | Low | S33.1 no verificaba el path destino del fetch | El test lee el `fetchStateMsg` con el path destino del canal de eventos |
| LOW-c | Low | Sub-fila no ejercida vía `renderEntry`/`View` | Cobertura añadida |
| LOW-d | Low | R34.1 no comparaba estrictamente | Comparación estricta de líneas wt ≠ repo/header |

Re-review de los fixes (`c5c4405` + `fc092dd`): todos los findings resueltos, sin
regresiones. Evidencia: `go build/vet` OK; `go test -count=1 ./...` OK;
`go test -race -count=5 -run TestWorktree ./internal/tui/` OK.

## Escenarios

`behavior.feature` (R30–R40, 40 escenarios + sección "Revisiones") cubiertos por
37 funciones de test nuevas derivadas escenario a escenario (S30.1–S40.5, más
`TestWorktreeExpandRealFixture` con fixture git real).

## Tests

- **Added:** 37 funciones de test
  - `internal/tui/worktree_test.go` (R30–R36, R39, R40)
  - `internal/config/config_test.go` (R37)
  - `internal/state/state_test.go` (R35)
  - `cmd/gitdash/print_test.go` (R38 no-regresión)
- **System tests:** ✅ `go build ./... && go vet ./... && go test ./...` verdes.

## Documentación

- **Changelog:** N/A — el repo no usa `CHANGELOG.md` (no se crea).
- **Docs:** `README.md` (expansión + operaciones por worktree, filtro `/`).
- **ADR:** `adr-0006-worktree-expansion-state-and-row-model.md` (aceptado).
- **SDD:** `issue.md`, `behavior.feature`, `plan.md`, `context.md` — **no se
  archiva ni se borra** (el repo mantiene el SDD por cambio como audit trail).

## Riesgos residuales

- **Falsa información de estado**: mitigado por el render dedicado de sub-fila y
  el panel mínimo de detalle; si en el futuro una sub-fila cayera en `renderRow`
  o `renderDetail` normal, un snapshot vacío se leería como `no-up`/`clean`.
- **Colisión de namespace** en `collapsed.json`: el prefijo `wt/` queda reservado;
  una clave de grupo llamada literalmente `wt/...` colisionaría (improbable:
  los grupos son nombres de directorio de proyecto).
- **Fuera de alcance declarado**: no hay recolección de estado git por worktree
  (dirty/ahead/behind/sync); la sub-fila muestra a lo sumo path/rama/head.
- **Race pre-existente** en `gitstatus.TestStreamPool` (map write en
  `parse_test.go`), **no** relacionada con 0006 (`gitstatus` intacto): fuera de
  alcance, no es regresión.
- **Artefacto local** `.codegraph/.gitignore` untracked (tooling): no se commitea.

## Next Step

Push de la rama tras autorización explícita del usuario:

```
git push -u origin feat/0006-worktree-expand-ops
```
