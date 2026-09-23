# ADR-0006: Estado de expansión y modelo de filas de worktree

- **Estado**: aceptado
- **Fecha**: 2026-09-23
- **Cambio**: `0006-feature-worktree-expand-ops`
- **Contexto**: `behavior.feature` R30–R40, `plan.md`, `context.md`

## Contexto

gitdash ya plegaba **grupos** persistidos en `state.Store` / `collapsed.json`
como un mapa plano `map[string]bool` (clave = nombre de grupo o
`primario/secundario`, valor `true` = **plegado**). El cambio 0006 necesita:

1. **Persistir** el estado expandido/plegado de los worktrees por repo, sin
   introducir un fichero nuevo (R35) y sin colisionar con las claves de grupo.
2. **Modelar** la sub-fila de worktree en la tabla, que es navegable y
   operable pero **no** tiene estado git propio (`Snapshot`/`State`), a
   diferencia de una fila de repo.

La decisión de modelo de filas es la de mayor riesgo: el render de una fila
de repo lee un `Snapshot`; si una sub-fila cayera en ese camino, un snapshot
vacío se leería como `no-up`/`clean` (información falsa de estado).

## Decisión 1 — Persistencia: namespace propio en `collapsed.json` con polaridad inversa

Se reutiliza el mismo `state.Store` y el mismo fichero `collapsed.json`. Las
claves de expansión de worktrees viven bajo un namespace propio
`wt/<path canónico del repo principal>` con polaridad **inversa** a las de
grupo: valor `true` = **expandido**, ausencia = **plegado** (R35.2).

La carga (`Model.loadPersisted`) y el guardado (`Model.saveCollapsed`) parten
y recomponen ambos espacios por el prefijo `state.WorktreePrefix` (`"wt/"`).
Sin migración: los ficheros existentes solo contienen claves de grupo.

### Alternativas consideradas

- **Fichero/schema separado** (p. ej. `expanded.json` o
  `{"groups": {...}, "worktrees": {...}}`): aísla los espacios de nombres de
  forma explícita, pero introduce un segundo fichero de estado y una segunda
  ruta de carga/guardado que mantener, contradiciendo el requisito explícito
  R35 ("sin fichero nuevo") y el alcance del plan. Un schema con dos mapas
  exigiría migrar el formato plano actual o tolerar dos formatos, ampliando
  el riesgo sin beneficio funcional.
- **Misma polaridad que grupos** (`true` = plegado) bajo namespace propio:
  evitaría la asimetría conceptual, pero obliga a invertir el valor al
  cargar/guardar igualmente, con el mismo riesgo de confusión y peor lectura
  (`wt/<path> = true` significaría "no expandido"). Se prefiere que el valor
  del namespace de expansión se lea naturalmente (`true` = expandido) y que
  la asimetría quede documentada en el prefijo.

### Consecuencias

- `collapsed.json` sigue siendo un único mapa plano; la separación es una
  convención de clave, no de esquema.
- El prefijo debe reservarse: una clave de grupo llamada literalmente `wt/...`
  colisionaría (improbable: los grupos son nombres de directorio de proyecto).
- Fichero corrupto o ausente → `nil` → todo plegado (R35.4, ya soportado).

## Decisión 2 — Modelo de filas: sub-fila sintetizada en la capa TUI

La sub-fila es un nuevo `entryKind` (`kindWorktree`) dentro de `tableEntry`,
con los datos crudos del worktree (`gitstatus.Worktree`) y el path del padre.
La sub-fila se **inyecta** en `entries()` justo después de la fila de su repo
padre, **después** de `group.Arrange`, de modo que headers, orden,
contadores y resumen no cambian.

El render usa un camino **dedicado** (`renderWorktreeRow`) que no lee
`Snapshot` y deja vacías/dim las celdas de estado por-worktree. La selección
bajo el cursor resuelve la sub-fila a un `row` sintético (path del worktree,
`HasRepo=true`, `MarkerErr=""`) para que las operaciones existentes —ya
parametrizadas por path— funcionen sin cambios.

### Alternativas consideradas

- **Nuevo tipo de dominio** (p. ej. `discovery.Project` con un flag
  "es sub-fila", o un `gitstatus` enriquecido por worktree): contamina el
  modelo de descubrimiento/gitstatus (fuera de alcance, R: no tocar
  `discovery`/`gitstatus`/`group`) y promete un estado por-worktree que el
  propio cambio declara fuera de alcance (R34.4, issue OUT). El estado rico
  por worktree sigue siendo a nivel de repo.
- **Reutilizar `row` con `Snapshot` vacío** y el render de repo: es
  precisamente la trampa de "falsa información de estado"; un snapshot vacío
  deriva `no-up`/`clean`. Descartado explícitamente (riesgo principal del
  plan).
- **Tipo `row` polimórfico** (interfaz): sobre-ingeniería para un caso con
  dos formas y campos disjuntos; el `tableEntry` con campos por-kind ya es la
  convención existente (`group` solo en headers).

### Consecuencias

- La sub-fila no participa del sort base ni del resumen; sigue a su padre en
  visibilidad (filtros y plegado de grupos, R36).
- `groupOfEntry` debe tratar `kindWorktree` como no-op (R39) para no plegar la
  sección `(ungrouped)` por accidente.
- La expansión inducida por búsqueda (R40) es una función de vista
  (`repoExpanded`) que NO escribe `m.expanded`; al limpiar la búsqueda el repo
  vuelve a su estado persistido (S40.5).
- Dedupe (R31.3) y fallback de huérfanos (S31.4) se resuelven reutilizando el
  ocultado existente de worktrees bajo su principal: un worktree descubierto
  aparece una sola vez, como sub-fila.
