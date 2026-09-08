# Spec: claridad de la tabla

Cambio: `0004-feature-table-clarity`
Base: 0001 (S6.3), 0002 (R14/R17), 0003 (R19-R21). Revisa SOLO el render de
celdas/headers: `Derive()`, `Score()`, filtro `n` y orden attention-first
quedan intactos.

## Requirements

### R23: Celda SYNC con rama visible

La celda SYNC SHALL incluir el nombre de la sync branch resuelta (override
del marcador > global). El formato SHALL ser: `<rama> ↓N` (behind, estilo
warn), `<rama>` a secas (sin desviación, estilo dim: rama visible sin tick),
`<rama> —` (rama resuelta pero ref inexistente o comparación fallida),
`—` (sin rama resuelta ni comparación, estilo dim). El snapshot SHALL
rellenar `SyncBranch` siempre que haya rama resuelta, aunque la comparación
falle (`SyncKnown=false`), para poder mostrar `<rama> —`.

#### S23.1: behind

- GIVEN sync global `main` y un repo en `feat/x` con main 3 commits por delante
- WHEN se renderiza la tabla
- THEN la celda SYNC muestra `main ↓3`

#### S23.2: override del marcador visible

- GIVEN un marcador con `sync_branch = "develop"` y global `main`
- WHEN se renderiza la fila
- THEN la celda muestra `develop ↓N` (la rama elegida aparece por fila)

#### S23.3: en sync (tabla quieta)

- GIVEN HEAD en la propia sync branch o sin commits ausentes
- WHEN se renderiza
- THEN la celda muestra solo `main` (dim), sin `✓`

#### S23.4: ref inexistente

- GIVEN un repo cuya sync branch resuelta no existe como ref
- WHEN se recolecta y renderiza
- THEN la celda muestra `main —`

#### S23.5: sin rama resuelta

- GIVEN un repo sin repo git (o sin sync branch configurada)
- WHEN se renderiza
- THEN la celda muestra `—`

### R24: FETCH solo transitorio

La celda FETCH SHALL mostrar estado SOLO mientras es relevante: `⟳ fetch`
(mientras corre) y `✗ fetch` (fallo, persistente hasta el próximo fetch de
ese repo). En éxito SHALL quedar vacía. El éxito agregado se señala por la
notificación transitoria de la barra inferior (0001 S8.x), no por celda.

#### S24.1: éxito en silencio

- GIVEN el auto-fetch terminó ok para un repo
- WHEN se renderiza la fila
- THEN la celda FETCH está vacía (sin `✓` permanente)

#### S24.2: fallo visible y persistente

- GIVEN un fetch fallido por red/credenciales
- WHEN se renderiza la fila
- THEN la celda muestra `✗ fetch` hasta que un nuevo fetch de ese repo cambie el estado

#### S24.3: en curso

- GIVEN un fetch corriendo en un repo
- WHEN se renderiza
- THEN la celda muestra `⟳ fetch`

### R25: Conjunto de columnas y headers

La TUI SHALL mostrar: NAME, BRANCH, Work Tree, ↑↓up, SYNC, ACTIVITY, FETCH.
La columna GROUP SHALL eliminarse de la TUI (los headers plegables de 0003 ya
identifican el grupo). El modo `--print` SHALL conservar GROUP (tabla plana
sin headers, R17). El header de deriva SHALL ser `↑↓up` (explícito: contra el
upstream) y el de working tree SHALL ser `Work Tree`. Todo ancho de columna
SHALL ser estrictamente mayor que la longitud de su header (prohibe el
pegado tipo `ACTIVITYFETCH`).

#### S25.1: sin GROUP en TUI

- GIVEN repos agrupados con headers plegables visibles
- WHEN se renderiza la tabla TUI
- THEN no existe columna GROUP y las filas siguen bajo su header plegable

#### S25.2: GROUP en print

- GIVEN `gitdash --print` con repos agrupados
- WHEN se imprime
- THEN la columna GROUP aparece (única fuente del grupo en print)

#### S25.3: headers separados

- WHEN se renderiza el header
- THEN cada título está separado del siguiente por al menos un espacio
  (`… Work Tree  ↑↓up … ACTIVITY  FETCH …`, nunca `ACTIVITYFETCH`)

### R26: Work Tree solo working tree; detached y no-upstream sin duplicar

La columna Work Tree SHALL mostrar únicamente el estado del working tree:
`N ?M` (N tracked, M untracked, `?M` solo si M>0), vacío si limpio, `∅`
(no repo) y `⚠` (error git). SHALL NO mostrar: `●` decorativa, `⇅`, el
sufijo `detached` ni `no-upstream`. El estado detached SHALL verse solo en
BRANCH (`<sha> (detached)`), y un repo detached con ficheros cambiados SHALL
seguir mostrando sus counts en Work Tree. La condición sin upstream SHALL
verse solo en ↑↓up como `no-up`.

#### S26.1: counts sin bola

- GIVEN un repo con 2 tracked y 1 untracked
- WHEN se renderiza
- THEN Work Tree muestra `2 ?1` (sin `●`)

#### S26.2: limpio en silencio

- GIVEN un repo clean
- WHEN se renderiza
- THEN Work Tree está vacío (sin `✓`)

#### S26.3: detached conserva dirty

- GIVEN un repo detached con 3 ficheros cambiados
- WHEN se renderiza
- THEN BRANCH muestra `<sha> (detached)` y Work Tree muestra `3` (la info
  dirty no se suprime)

#### S26.4: errores y no-repo

- GIVEN un repo con error git / un directorio sin repo
- WHEN se renderiza
- THEN Work Tree muestra `⚠` / `∅` respectivamente

### R27: ↑↓up explícito

La columna ↑↓up SHALL mostrar la deriva de commits contra el upstream:
`↑N↓N`/`↑N`/`↓N` (diverged visible aquí, sin `⇅` en Work Tree), `no-up`
(estilo warn) cuando no hay upstream trackeado, y vacío cuando HEAD está en
sync con su upstream (sin placeholder `·`).

#### S27.1: diverged vive aquí

- GIVEN ahead=2 behind=3
- WHEN se renderiza
- THEN ↑↓up muestra `↑2↓3` y Work Tree no muestra flechas

#### S27.2: no-upstream como no-up

- GIVEN una rama local sin upstream
- WHEN se renderiza
- THEN ↑↓up muestra `no-up` (y Work Tree puede seguir mostrando counts)

#### S27.3: en sync en silencio

- GIVEN ahead=0 behind=0 con upstream
- WHEN se renderiza
- THEN ↑↓up está vacía (sin `·`)

### R28: Tabla quieta

Las celdas SHALL quedar vacías cuando no haya nada que comunicar: Work Tree
vacío si clean, ↑↓up vacío si en sync, FETCH vacío tras éxito, SYNC solo el
nombre de rama en dim si sin desviación. Las filas de atención (dirty,
behind, diverged, no-up, error) SHALL seguir destacando por contenido y
estilo; el orden attention-first (R6) no varía.

#### S28.1: fila limpia mínima

- GIVEN un repo clean, en sync con upstream y con sync branch al día
- WHEN se renderiza
- THEN sus celdas Work Tree, ↑↓up y FETCH están vacías y SYNC muestra solo
  el nombre de la rama

### R29: Propagación al modo print

El modo `--print` SHALL reflejar la nueva semántica de celdas: SYNC con rama
(S23), ↑↓up con `no-up` (R27), working tree solo counts con detached solo en
BRANCH (R26). La columna GROUP SHALL conservarse (S25.2). El header de
working tree SHALL ser `WT` y el contador de worktrees (columna `WT` de 0002
R15) SHALL renombrarse a `WTS` para evitar la colisión de siglas.

#### S29.1: print espejo

- GIVEN repos con sync behind, sin upstream y dirty
- WHEN `gitdash --print`
- THEN SYNC muestra `<rama> ↓N`, ↑↓up muestra `no-up` donde corresponda y WT
  muestra los counts

#### S29.2: sin colisión de siglas

- GIVEN un repo con worktrees y ficheros cambiados
- WHEN `gitdash --print`
- THEN la fila muestra `WT` con los counts del working tree y `WTS` con el
  número de worktrees (dos columnas distintas)

## Revisiones

- **0002 R14 / S14.1-S14.4**: la celda SYNC ya no es el número a secas:
  pasa a `<rama> ↓N` / `<rama>` / `<rama> —` / `—` (S23). `SyncBranch` se
  rellena aunque la comparación falle (antes vacía si la ref no existía).
- **0001 S6.3**: la celda FETCH pierde el `✓` de éxito (transitoria, R24).
- **0001 S5.3/S5.5 y 0002 (render)**: `●`, `⇅`, `·`, `detached` y
  `no-upstream` desaparecen de STATE, que pasa a Work Tree (R26/R27); el
  detached en BRANCH se conserva, el de STATE se elimina (dedupe + bug de
  dirty suprimido en detached).
- **Tabla quieta**: durante la aprobación se revisó el `rama ✓` previsto
  inicialmente para SYNC en limpio: sin tick, solo rama dim (R23/S23.3, R28).
- **0003 R21**: GROUP se elimina de la TUI pero se conserva en print (R25).
- **0002 R15 (print)**: el contador de worktrees se renombra `WT` → `WTS`
  para liberar la sigla a la columna de working tree (S29.2).
