# Proposal: claridad de la tabla (sync visible, fetch transitorio, columnas coherentes)

Cambio: `0004-feature-table-clarity`
Base: 0001/0002/0003 (revisa el render de columnas; NO toca estados internos,
Score ni filtros)

## Problema

El feedback de uso de la tabla reveló siete problemas de legibilidad:

1. El header pega `ACTIVITY`+`FETCH` (`colActivity` == longitud de "ACTIVITY",
   `pad()` no añade separador).
2. La rama sync elegida no aparece en la tabla (solo en el detalle); además
   `Snapshot.SyncBranch` ni se rellena cuando la ref no existe.
3. `↓N` es ambiguo: en ↑↓ mide contra el upstream, en SYNC contra la sync
   branch. El header `↑↓` no lo explica.
4. Tras el auto-fetch, la columna FETCH muestra un `✓` permanente sin
   información (los propios números ↑↓ ya prueban frescura).
5. La columna GROUP es redundante en la TUI: los headers plegables de 0003 ya
   muestran el grupo de cada fila.
6. `detached` aparece dos veces (BRANCH y STATE) y, por precedencia de
   `Derive()`, un repo detached con ficheros dirty suprime la info de dirty.
7. Iconos incoherentes: `⇅` de diverged en STATE colisiona con las flechas de
   la columna ↑↓; `●` es decorativa sin dato; `·` es un placeholder vacío.

## Propuesta

Rediseño del render de columnas (semántica de celdas y headers), sin tocar
`Derive`/`Score`/filtro `n`/orden attention-first:

| Columna | Limpio/sin novedad | Con novedad |
|---|---|---|
| NAME | igual | igual (+ `[wt]`, `(N wt)`) |
| GROUP | **eliminada en TUI** (headers plegables) | se conserva en `--print` |
| BRANCH | rama | `<sha> (detached)` única mención |
| Work Tree | vacío (tabla quieta) | `N ?M` · `∅` · `⚠` |
| ↑↓up | vacío | `↑N↓N`/`↑N`/`↓N` · `no-up` |
| SYNC | `<rama>` dim (rama siempre visible, sin tick) | `<rama> ↓N` · `<rama> —` · `—` |
| ACTIVITY | igual | igual |
| FETCH | vacío | `⟳ fetch` · `✗ fetch` (persistente) |

"Tabla quieta": solo se pinta lo que pide atención; los ticks de
confirmación desaparecen (incluido el `rama ✓` previsto en un primer diseño
de este cambio, revisado durante la aprobación).

## Alcance

- `internal/gitstatus`: `SyncBranch` siempre rellena cuando hay rama resuelta.
- `internal/tui`: celdas, headers, anchos, detalle.
- `cmd/gitdash`: modo print espejo (GROUP se queda allí).
- `README.md`: semántica de columnas.
- Tests de celdas actualizados y nuevos casos.
