# Spec: sync branch, worktrees plegados, grupos y renombrado del marcador

Cambio: `0002-feature-sync-worktrees-groups`
Base: 0001 (amplía R1/R4/R5/R6/R10/R12; estructura de secciones paralela)

## Requirements

### R13: Renombrado del marcador

El marcador por defecto SHALL ser `.gitdash.toml` (config default `marker`).
No SHALL haber fallback a `.gitdash.toml` (cambio duro). La clave `marker` de
config.toml sigue teniendo precedencia sobre el default.

#### S13.1: nuevo default

- GIVEN una config sin clave `marker`
- WHEN se escanea un root
- THEN solo los directorios con `.gitdash.toml` se descubren como proyectos

#### S13.2: marcador custom sigue

- GIVEN una config con `marker = ".custom.toml"`
- WHEN se escanea
- THEN se descubre por `.custom.toml`, no por `.gitdash.toml`

### R14: Sync branch

La sync branch SHALL resolverse: override del marcador (`sync_branch` en
`.gitdash.toml`) > global (`sync_branch` en config.toml, default `"main"`) >
ninguna. La desviación SHALL ser solo **behind**: commits alcanzables desde
la sync branch y no desde la rama actual (`git rev-list --count
<rama>..<sync>`). La desviación vs sync SHALL mostrarse en la columna
adicional `SYNC` y SHALL reemplazar el estado vacío de una celda con:
`↓N` (behind), `✓` (0), `—` (sin comparación posible). La desviación vs sync
SHALL NO alterar el estado derivado ni el Score del orden (informativa).

#### S14.1: global aplica por defecto

- GIVEN `sync_branch = "main"` global y un repo en `feat/x` con main a 3
  commits por delante
- WHEN se recolecta
- THEN la SYNC muestra `↓3`

#### S14.2: override del marcador

- GIVEN un marcador con `sync_branch = "develop"` y una config global
  `sync_branch = "main"`
- WHEN se recolecta ese repo
- THEN la desviación se calcula contra `develop`

#### S14.3: rama actual == sync branch

- GIVEN HEAD en la propia sync branch
- WHEN se recolecta
- THEN la SYNC muestra `✓`

#### S14.4: sync branch inexistente

- GIVEN un repo cuya sync branch resuelta no existe como ref local
- WHEN se recolecta
- THEN la SYNC muestra `—`

#### S14.5: comparación contra SHA de merge-base

- GIVEN la rama actual divergió de la sync (commits propios y ajenos)
- WHEN se calcula el behind
- THEN `rev-list --count` cuenta solo los commits de sync ausentes en la
  rama actual (union con merge-base, no diff de tips)

### R15: Worktrees plegados

La recolección SHALL ejecutar `git worktree list --porcelain` por repo con
repo y SHALL registrar por worktree: path, branch y head. Las filas de
proyectos worktree (`IsWorktree`) SHALL estar ocultas por defecto en la
tabla. El repo principal SHALL mostrar el indicador `(N wt)` en la celda
NAME cuando N > 0. Filas de worktree SHALL volver a ser visibles solo si su
repo principal (`gitdir:` parseado) no está entre los proyectos descubiertos
(fallback de trazabilidad). El detalle (Enter) SHALL abrir siempre y SHALL
añadir la sección `worktrees (N)` con rama y ruta relativa de cada worktree.
El modo `--print` SHALL ocultar las filas de worktree igual que la TUI y
SHALL añadir columna `WT` con el contador.

#### S15.1: worktree oculto por defecto

- GIVEN un repo principal con 3 worktrees descubiertos (con marcador)
- WHEN se renderiza la tabla
- THEN solo la fila del repo principal es visible con `(3 wt)` en NAME

#### S15.2: lista en el detalle

- GIVEN el cursor en un repo con worktrees
- WHEN se pulsa `enter`
- THEN el detalle muestra la sección `worktrees (3)` con rama y ruta por wt

#### S15.3: fallback de huérfanos

- GIVEN un worktree con marcador cuyo repo principal no está descubierto
- WHEN se renderiza
- THEN el worktree sigue visible como fila propia

#### S15.4: conteo incluye no descubiertos

- GIVEN un repo con worktrees dentro y fuera de los roots de discovery
- WHEN se recolecta
- THEN `(N wt)` cuenta TODOS los de `git worktree list`, no solo los con marcador

### R16: Agrupación plegable (estilo vroom)

SI existe algún proyecto con `group` no vacío, la vista SHALL agrupar
siempre (sin modo toggle): tras el sort attention-first, cada grupo SHALL
formar un bloque de filas contiguas emitido en la posición de su primer
miembro; un grupo de un único miembro SHALL mostrar header igualmente. Los
proyectos sin group SHALL ir a una sección `(ungrouped)` plegable al final.
Los headers SHALL ser filas navegables dibujadas `▾ <nombre> (n)` /
`▸ <nombre> (n)` plegado, y el plegado SHALL alternarse con `tab` sobre el
cursor (header o miembro) y con `enter` sobre el header. El estado plegado
SHAL persistir solo en la sesión (map en memoria). Los filtros `n` y `/`
SHALL aplicarse antes de agrupar; los grupos sin miembros visibles tras
filtrar SHALL desaparecer de la vista.

#### S16.1: agrupación automática

- GIVEN 2 repos con `group="backend"` y 3 sin group
- WHEN termina la recolección
- THEN aparece header `▸ backend (2)` y sección `(ungrouped)` con los otros

#### S16.2: plegado y desplegado

- GIVEN el cursor sobre el header `backend`
- WHEN se pulsa `tab`
- THEN los miembros se ocultan y el header pasa a `▸ backend (2)`
- WHEN se pulsa `tab` de nuevo
- THEN los miembros vuelven a verse y el header pasa a `▾ backend (2)`

#### S16.3: bloque en posición del primer miembro

- GIVEN repos intercalados (backend, sin grupo, backend) ordenados
  attention-first
- WHEN se agrupa
- THEN los 2 backend quedan contiguos desde la posición del primero y las
  demás filas conservan su orden relativo

#### S16.4: un solo miembro

- GIVEN un grupo con un único repo
- WHEN se agrupa
- THEN el header se muestra igualmente

#### S16.5: filtros + grupos

- GIVEN agrupación activa y el filtro `/api`
- WHEN se renderiza
- THEN solo los grupos con miembros cuyo name/group matchean aparecen, y la
  sección `(ungrouped)` desaparece si queda vacía

### R17: Propagación al modo print

El modo `--print` SHALL reflejar las columnas nuevas del dashboard
(SYNC, WT) y el orden de la tabla plana (attention-first), sin headers de
grupo (la columna GROUP ya existe ahí).

#### S17.1: print con columnas nuevas

- GIVEN repos con sync branch y worktrees
- WHEN se ejecuta `gitdash --print`
- THEN la tabla incluye las columnas SYNC y WT con los mismos valores que la TUI

## Revisiones

- **R15**: se eligió `git worktree list --porcelain` (tool-agnóstico: cubre
  worktrees nativos y worktrunk, que son git worktrees reales; y ve worktrees
  fuera de los roots de discovery) frente al enlace vía gitdir de marcadores.
- **R14**: default global `sync_branch="main"` (vetable), sin opt-in vacío.
- **R16**: se combina el modelo vroom (siempre agrupado si hay groups, bloque
  en posición del primer miembro) con la sección `(ungrouped)` plegable
  decidida en la conversación (vroom deja los ungrouped inline).
