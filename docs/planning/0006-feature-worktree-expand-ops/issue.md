# Issue: operaciones sobre worktrees expandibles

> **Nota de formato (pipeline)**: este repo usó históricamente el SDD recortado
> `proposal.md` + `spec.md`. Este pipeline usa `issue.md` + `behavior.feature` +
> `plan.md` + `context.md`. Mapeo: `issue.md` ≈ `proposal.md` (por qué + alcance);
> `behavior.feature` ≈ escenarios `S#n.#` GIVEN/WHEN/THEN; `plan.md`/`context.md`
> cubren lo que antes era implementación.

Cambio: `0006-feature-worktree-expand-ops`
Base: 0002 (R15: worktrees plegados bajo su repo principal), 0003 (plegado de
grupos), 0004 (render de celdas).

## Por qué

Hoy un repo con worktrees solo muestra un indicador `(N wt)` en la celda NAME:
los worktrees son visibles como repos de primera clase **solo** si tienen su
propio `.gitdash.toml` descubierto, y en ese caso quedan plegados bajo el repo
principal (0002 R15) sin forma de operar sobre ellos. No hay manera de
seleccionar un worktree concreto y aplicarle acciones (fetch, pull, sync, push,
lazygit, update, editor, `!`, detalle).

El usuario quiere que, desde el menú principal, un repo con worktrees sea
**expandible** con `space` y que al expandir aparezcan sus worktrees como
sub-filas navegables y operables, con la **misma** superficie de acciones que una
fila de repo. Además, la expansión debe cubrir **todos** los worktrees que
reporta git, no solo los que tienen marcador o caen dentro de los roots de
discovery.

## Alcance (IN)

1. **Sub-fila navegable de worktree**: al expandir un repo, insertar bajo su fila
   una sub-fila por cada worktree de `git worktree list --porcelain`, incluidos
   los que **no** tienen marcador y los que están **fuera** de los roots de
   discovery. Las sub-filas participan del cursor, scroll y selección como
   cualquier fila navegable.
2. **Cobertura total desde el snapshot**: la lista de worktrees sale de lo que ya
   expone `gitstatus` (`Snapshot.Worktrees` / `ParseWorktrees`); no se requiere
   un segundo descubrimiento por marcador. Como mínimo se dispone de path, rama y
   head, que es lo que devuelve `worktree list`.
3. **Toggle `space` configurable**: nueva acción de keybinding (p. ej. `expand`)
   con default `space`, resoluble vía `config.Keybindings` / `actionForKey` como
   el resto. `space` hoy no está bindeado, por lo que no colisiona.
4. **Persistencia de expansión**: el estado expandido/plegado de worktrees por
   repo persiste entre sesiones reutilizando el mismo mecanismo que el plegado de
   grupos (`state.Store` / `collapsed.json`), sin introducir un archivo nuevo.
5. **Operaciones por path sobre el worktree**: todas las acciones existentes a
   nivel de repo se aplican al worktree bajo el cursor — `fetch`, `pull`, `sync`,
   `push`, `lazygit`, `update`, `editor`, `recollect`, comando `!` y detalle
   (`enter`). Consistencia total con las filas de repo (mismos guards de
   `HasRepo`, mismas notificaciones, mismo comportamiento de handoff).
6. **Dedupe con worktrees ya descubiertos**: un worktree que además fue
   descubierto con marcador no debe aparecer dos veces (ni como fila de repo
   plegada por 0002 R15 y como sub-fila). El fallback de huérfanos (0002 S15.3:
   worktree cuyo repo principal no está descubierto sigue visible como fila de
   repo) se preserva.
7. **Render de la sub-fila**: indentación/tag visual que distinga la sub-fila de
   worktree de una fila de repo, reutilizando la convención de celdas
   `(texto, estilo)` con `pad()` antes del estilo. La celda NAME del repo
   principal puede reflejar el estado expandido (glyph `▾`/`▸` análogo a los
   headers de grupo).
8. **`--print` sin cambios**: el modo no interactivo (`cmd/gitdash/print.go`) no
   incorpora sub-filas ni expansión; mantiene su salida actual.
9. **Búsqueda `/` sobre worktrees (v1)**: el filtro `/` matchea además el
   basename/nombre y la rama de los worktrees; cuando matchea un worktree, su repo
   padre queda visible y expandido mostrando la sub-fila que matchea (aunque el
   padre no matchee por sí mismo). Esta expansión inducida por búsqueda es
   **transitoria** (de vista) y no altera el estado de expansión persistido.

## Fuera de alcance (OUT)

- **Recolección completa de estado git por worktree**: no se hace `git status` ni
  fetch por worktree. La sub-fila muestra a lo sumo lo que ya trae
  `worktree list` (path/rama/head). El estado derivado rico (dirty, ahead/behind,
  sync) sigue siendo a nivel de repo.
- **Cambios en el modelo de descubrimiento** (`internal/discovery`): no se
  modifica el walk por marcador ni la semántica de `Project`.
- **Refactors no relacionados** de tabla, grupo o pipelines de fondo.
- **Nuevas acciones git** más allá de las ya existentes.

## Intención de aceptación (resumen)

- Dado un repo con worktrees, `space` sobre su fila alterna expandido/plegado; al
  expandir aparecen sub-filas por cada worktree de `worktree list`, incluyendo
  worktrees sin marcador y fuera de roots.
- El cursor puede posarse en una sub-fila y las acciones de repo (fetch/pull/
  sync/push/lazygit/update/editor/recollect/`!`/detalle) se ejecutan contra el
  path de ese worktree.
- El estado expandido/plegado se conserva al reiniciar la app (mismo store que el
  plegado de grupos).
- La búsqueda `/` encuentra worktrees por rama o basename y revela su repo padre
  expandido de forma transitoria (sin alterar la persistencia).
- No hay duplicados: un worktree descubierto con marcador aparece una sola vez.
- `--print` permanece inalterado.

## Preguntas abiertas

Ninguna: las decisiones de alcance están cerradas.
