# Plan: worktrees expandibles y operables

Cambio: `0006-feature-worktree-expand-ops`
Estado: pendiente de aprobación
`adr_required: true` — razón: decisión de modelo de datos con tradeoffs reales
(persistir la expansión como clave namespaced de polaridad inversa dentro de
`collapsed.json` vs. fichero/schema separado) y decisión de modelo de filas
(sub-fila sintetizada en la capa TUI vs. nuevo tipo de dominio). ADR propuesto:
`adr-0006-worktree-expansion-state-and-row-model`.

Base: 0002 (R15 worktrees plegados), 0003 (plegado de grupos), 0004 (render de
celdas). `behavior.feature` (R30–R40) es la fuente de comportamiento; `issue.md`
el por qué y el alcance.

## Resultado buscado

Desde el menú principal, un repo con worktrees se expande/pliega con `space`
(configurable). Expandido, cada worktree de `git worktree list --porcelain`
aparece como sub-fila navegable y operable con **toda** la superficie de acciones
de una fila de repo (fetch, pull, sync, push, lazygit, update, editor, recollect,
comando `!`, shell y detalle). La expansión persiste entre sesiones; `/` encuentra
worktrees por rama o basename y revela su padre expandido de forma transitoria.

## Enfoque (alto nivel)

Todo el comportamiento nuevo vive en la capa TUI, más una convención de clave en
el store de estado y una entrada de keybinding en config. `gitstatus`,
`discovery`, `group` y el modo `--print` **no se tocan**: los worktrees ya vienen
en `Snapshot.Worktrees` (path/rama/head) y las operaciones ya son por `path`.

- **Modelo de filas**: nuevo tipo de fila navegable `kindWorktree`. Las sub-filas
  se inyectan en la lista de entradas **justo después de la fila de su repo padre**,
  y solo si el padre está visible y el repo está expandido. Se inyectan después de
  la agrupación, así no alteran headers, orden, contadores ni resumen. El render
  usa un camino dedicado (no el de filas de repo), porque un worktree no tiene
  estado git propio y no debe heredar celdas que prometan dirty/ahead/behind/sync.
- **Selección/operaciones**: la selección bajo el cursor se amplía para aceptar la
  sub-fila y resolver su `path`; las operaciones existentes (ya parametrizadas por
  path) funcionan sin cambios. Los guards actuales (`HasRepo`, `MarkerErr`) se
  mantienen.
- **Cobertura y dedupe**: se listan **todos** los worktrees del snapshot, incluidos
  los que no tienen marcador y los de fuera de roots. El repo principal nunca es
  sub-fila (ya viene excluido del snapshot). Un worktree que además fue descubierto
  con marcador aparece **una sola vez**, como sub-fila (reutilizando su nombre y su
  sync override, y su snapshot vivo si existe), nunca como fila top-level: el
  ocultado actual de worktrees bajo su principal se conserva. El fallback de
  huérfanos (principal no descubierto) se preserva: sigue top-level con `[wt]` y no
  es expandible. La igualdad de paths se normaliza para evitar duplicados por
  symlinks o barras finales.
- **Expansión y persistencia**: el estado de expansión por repo es un mapa propio
  en el modelo (por defecto **plegado**), persistido en el mismo `collapsed.json`
  que el plegado de grupos, bajo un namespace de clave propio con el path canónico
  del repo principal. La polaridad del valor es la inversa a la de las claves de
  grupo (expandido vs. plegado), así que la carga separa ambos espacios por prefijo
  y el guardado los compone; sin migración (los ficheros existentes solo tienen
  claves de grupo). Estado ausente o corrupto degrada a plegado por defecto.
- **Render**: sub-fila indentada con glyph propio, distinta de una fila de repo y
  de un header de grupo; muestra el basename del worktree y su rama (o `(detached)`),
  y deja vacías/dim las celdas de estado por-worktree. El NAME del repo principal
  gana el glyph plegado/expandido junto al contador `(N wt)`.
- **Interacción con filtros y grupos**: `n` (dirty) y el plegado de grupos mandan
  sobre la visibilidad del padre; las sub-filas siguen a su padre. La búsqueda `/`
  es la excepción: matchea rama/basename de worktrees y, al matchear uno, muestra su
  padre expandido con la sub-fila que matchea. Esa expansión es **transitoria**
  (solo de vista): no modifica el estado persistido.
- **Teclas**: nueva acción configurable (default `space`, resuelta por el mapa de
  keybindings, no como tecla fija). Verificado en bubbletea v2: la barra espaciadora
  se stringifica como `"space"`. `space` es no-op fuera de una fila de repo con
  worktrees; `tab` sobre una sub-fila es no-op (no pliega la sección sin grupo).
- **Detalle**: `enter` sobre una sub-fila muestra path, rama (o `(detached)`) y head;
  si el worktree fue descubierto con marcador y tiene snapshot vivo, se muestra; si
  no, un panel mínimo sin inventar estado git. Las notificaciones identifican el
  worktree por basename, no por ruta absoluta.

## Orden de trabajo (grueso)

1. Persistencia: mapa de expansión en el modelo + convención de clave
   namespaced en el store, con carga/guardado y default plegado.
2. Modelo de filas y selección: nuevo tipo de sub-fila, inyección bajo el padre
   expandido, resolución de path para operaciones y navegación/cursor.
3. Toggle `space` + keybinding configurable + hint.
4. Render de la sub-fila, glyph/contador en el padre y panel de detalle mínimo.
5. Dedupe (incl. normalización de path) y huérfanos.
6. Búsqueda `/` por worktree con expansión transitoria.
7. Tests derivados de `behavior.feature` (R30–R40) y actualización de
   `README.md`/hints si aplica.

## Riesgos y puntos críticos

- **Falsa información de estado**: si una sub-fila cayera en el render de filas de
  repo, un snapshot vacío se leería como "no-up"/error. Es la trampa principal;
  exige render dedicado.
- **Polaridad y colisión de claves** en `collapsed.json`: mezclar espacios de
  nombres corrompería el plegado de grupos. La carga debe partir por prefijo.
- **Dedupe por path**: sin normalizar (symlink/barra final) el mismo worktree
  aparece dos veces.
- **Coherencia del cursor**: expandir/plegar debe reclutar el cursor dentro de
  rango; los filtros pueden hacer desaparecer la fila bajo el cursor.
- **Búsqueda transitoria**: el auto-expandido por búsqueda no debe escribir estado
  persistido (evitar efectos laterales sorprendentes).
- **Alcance explícito**: sin recolección de estado git por worktree, sin cambios en
  discovery/group/print, sin nuevas acciones git.

## Fuera de alcance

Recolección completa de estado git por worktree (dirty/ahead/behind/sync/actividad);
cambios en el walk de discovery o en `Project`; nuevas acciones git; refactors no
relacionados; cambios en el modo `--print`.
