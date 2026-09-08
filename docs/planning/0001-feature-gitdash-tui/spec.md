# Spec: gitdash — panel de estados git

Cambio: `0001-feature-gitdash-tui`
Base: feature nueva (greenfield)

## Requirements

### R1: Configuración XDG

El sistema SHALL leer `~/.config/gitdash/config.toml` (respetando `$XDG_CONFIG_HOME`)
al arrancar con claves opcionales: `marker` (string, default `".gitdash.toml"`),
`roots` (array de strings, default `["~/dev"]`), `exclude` (array de nombres de
directorio a podar), `editor` (string, default `$EDITOR` o `"vi"`),
`fetch.auto` (bool, default `true`), `fetch.concurrency` (int, default `4`),
`fetch.timeout` (string duración, default `"30s"`). Los valores `~/` SHALL
expandirse al home del usuario. Si el fichero falta, está vacío o es
malformado SHALL usarse la config por defecto; en el caso malformado SHALL
notificarse el error de parseo sin abortar el arranque.

#### S1.1: defaults sin config

- GIVEN que `~/.config/gitdash/config.toml` no existe
- WHEN arranca gitdash
- THEN se usan marker `.gitdash.toml`, roots `["~/dev"]`, exclusiones por
  defecto (node_modules, target, vendor, dist, build, .venv, .cache, ...),
  `fetch.auto=true`, `fetch.concurrency=4`, `fetch.timeout=30s`

#### S1.2: override parcial

- GIVEN una config con solo `roots = ["~/code", "~/work"]`
- WHEN arranca gitdash
- THEN se escanean esos dos roots y el resto de claves conservan sus defaults

#### S1.3: config malformada

- GIVEN una config con TOML inválido
- WHEN arranca gitdash
- THEN se notifica `config: <error>` en la barra de mensajes y se continúa
  con los defaults

#### S1.4: expansión de tilde

- GIVEN una config con `roots = ["~/dev"]`
- WHEN se resuelve la config
- THEN el root se expande a `<home>/dev`

### R2: Discovery por marcador

El sistema SHALL recorrer cada root de la config en profundidad ilimitada
encontrando los directorios que contengan el fichero marcador. El walk SHALL
podar: directorios cuyo nombre empiece por `.` (excepto el root mismo), los
nombres de la lista `exclude`, y directorios ilegibles (sin abortar el
escaneo). Al encontrar un marcador SHALL dejar de descender dentro de ese
directorio salvo para subdirectorios que también contengan marcadores
(monorepos con proyectos anidados válidos).

#### S2.1: detección por marcador

- GIVEN un root con `~/dev/projects/api/.gitdash.toml`
- WHEN se escanea el root
- THEN `~/dev/projects/api` aparece como proyecto descubierto

#### S2.2: poda de ocultos y artefactos

- GIVEN un root con `~/dev/.hidden/proj/.gitdash.toml` y
  `~/dev/api/node_modules/dep/.gitdash.toml`
- WHEN se escanea
- THEN ninguno de los dos aparece (ocultos y exclusiones se podan)

#### S2.3: profundidad ilimitada con raíles

- GIVEN `~/dev/a/b/c/d/proj/.gitdash.toml` (4 niveles)
- WHEN se escanea
- THEN el proyecto aparece (no hay cap de profundidad; solo podas)

#### S2.4: anidado válido

- GIVEN `~/dev/mono/.gitdash.toml` y `~/dev/mono/sub/.gitdash.toml`
- WHEN se escanea
- THEN ambos aparecen como proyectos separados

### R3: Resolución del repo git

Para cada directorio con marcador, el sistema SHALL clasificarlo según su
`.git`: directorio `.git` → repo normal; fichero `.git` (con `gitdir:`) →
worktree (flag `isWorktree`, etiquetado en UI); ausencia de `.git` → proyecto
sin repo, visible con estado `no repo`. El path del proyecto es la carpeta del
marcador (decisión de diseño: no se busca `.git` hacia arriba).

#### S3.1: repo normal

- GIVEN un proyecto con marcador y directorio `.git`
- WHEN se resuelve
- THEN se registra como repo normal con path = carpeta del marcador

#### S3.2: worktree

- GIVEN un proyecto cuyo `.git` es un fichero `gitdir: <ruta>`
- WHEN se resuelve
- THEN se registra con `isWorktree=true` y se muestra el tag `worktree` en UI

#### S3.3: marcador sin repo

- GIVEN un proyecto con marcador y sin `.git`
- WHEN se resuelve
- THEN aparece en el panel con estado `no repo`, sin fetch ni acciones

### R4: Metadatos del marcador

El marcador `.gitdash.toml` SHALL aceptar claves opcionales `name` (string) y
`group` (string). Si `name` falta, el nombre mostrado SHALL ser el nombre del
directorio; si `group` falta, el grupo SHALL ser vacío (mostrado como `-`).
Un marcador malformado SHALL verse con estado de error de parseo visible sin
excluir el proyecto.

#### S4.1: metadatos presentes

- GIVEN un marcador con `name = "api"` y `group = "vsocial"`
- WHEN se escanea
- THEN el panel muestra `api` en grupo `vsocial`

#### S4.2: metadatos ausentes

- GIVEN un marcador vacío
- WHEN se escanea
- THEN el panel muestra el nombre del directorio y grupo `-`

#### S4.3: marcador malformado

- GIVEN un marcador con TOML inválido
- WHEN se escanea
- THEN el proyecto aparece con nombre del directorio y estado de error
  `marker: <error>`, sin abortar el scan

### R5: Recolección de estado git

El sistema SHALL recolectar por repo (subprocess `git`, en paralelo con worker
pool): branch actual, presencia de upstream, ahead/behind, número de cambios
tracked y untracked, y epoch del último commit, parseando
`git status --porcelain=v2 --branch` + `git log -1 --format=%ct`. Cada repo
SHALL tener un estado derivado: `clean`, `dirty` (cambios tracked o
untracked), `ahead`, `behind`, `diverged` (ahead>0 y behind>0),
`no-upstream`, `detached`, `no-repo`, `error`. La recolección SHALL ser
cancelable por contexto.

#### S5.1: repo limpio sincronizado

- GIVEN un repo sin cambios y con upstream sincronizado
- WHEN se recolecta el estado
- THEN el estado es `clean` con ahead=0, behind=0

#### S5.2: ahead/behind/diverged

- GIVEN repos con 2 commits locales sin subir, 3 commits del upstream sin
  bajar, o ambos
- WHEN se recolecta
- THEN los estados son `ahead (2)`, `behind (3)`, `diverged (+2/-3)`
  respectivamente, visibles como `↑2`, `↓3`, `↑2↓3`

#### S5.3: dirty por tracked y untracked

- GIVEN un repo con 1 fichero modificado y 2 sin trackear
- WHEN se recolecta
- THEN el estado es `dirty` con cambios=1, untracked=2 (visible `●1 ?2`)

#### S5.4: sin upstream

- GIVEN un repo sin upstream configurado
- WHEN se recolecta
- THEN el estado es `no-upstream` (visible `-` en la columna ↑/↓) y el repo
  se excluye del fetch automático

#### S5.5: detached HEAD

- GIVEN un repo con HEAD detached
- WHEN se recolecta
- THEN la columna branch muestra `<sha-corto> (detached)`

#### S5.6: error de git

- GIVEN un directorio con `.git` corrupto
- WHEN se recolecta
- THEN el estado es `error` con el mensaje visible en el detalle, sin
  abortar la recolección global

### R6: Dashboard

La TUI SHALL mostrar una tabla con columnas: `Name`, `Group`, `Branch`,
`State` (dirty/untracked/errores con iconos), `↑↓` (ahead/behind),
`Activity` (fecha relativa del último commit) y un indicador de estado de
fetch. El orden por defecto SHALL ser: primero repos con atención (dirty,
ahead, behind, diverged, error), luego por último commit descendente, y como
desempate nombre. La barra inferior SHALL mostrar el resumen
(`<n> repos · <d> dirty · <a> ahead · <b> behind`) y las notificaciones
transitorias.

#### S6.1: render inicial

- GIVEN un scan con repos en varios estados
- WHEN termina la recolección
- THEN la tabla muestra todas las columnas pobladas con el orden
  atención-primero y el resumen correcto en la barra

#### S6.2: navegación

- GIVEN la tabla con N repos
- WHEN se pulsan `j/k` (o flechas) y `g/G`
- THEN el cursor se mueve con scroll respetando límites (no sale por arriba
  ni por abajo)

#### S6.3: estado de fetch visible

- GIVEN un repo cuyo fetch está en curso
- WHEN la tabla se renderiza
- THEN su fila muestra el indicador `⟳ fetching` hasta que termina

### R7: Filtros y rescan

Las teclas `n` y `/` SHALL filtrar la vista: `n` alterna "solo no-limpios"
(estado distinto de clean y sin error de parseo); `/` abre un input de
búsqueda incremental por nombre/grupo (esc cancela, enter confirma, el filtro
persiste hasta limpiarlo con esc de nuevo en input vacío). La tecla `r`
SHALL relanzar discovery + recolección completa (con fetch automático si
`fetch.auto`); `R` SHALL re-coleccionar solo el repo del cursor. Los filtros
no afectan al rescan (se reaplican sobre los datos nuevos).

#### S7.1: filtro no-limpios

- GIVEN repos limpios y sucios mezclados con el filtro `n` activado
- WHEN se renderiza
- THEN solo se ven los no-limpios; al desactivar `n` vuelven todos

#### S7.2: búsqueda incremental

- GIVEN el input `/` abierto
- WHEN se teclea `api`
- THEN la tabla se reduce en vivo a los repos cuyo nombre o grupo contiene
  `api` (case-insensitive)

#### S7.3: rescan completo

- GIVEN la TUI abierta con datos en pantalla
- WHEN se pulsa `r`
- THEN se relanza discovery+estado+fetch-auto y la tabla se refresca al
  terminar (spinner visible durante el proceso)

#### S7.4: re-colección de un repo

- GIVEN el cursor sobre un repo
- WHEN se pulsa `R`
- THEN solo ese repo re-colecciona estado (spinner en su fila)

### R8: Fetch automático en batches

Tras terminar cada scan/rescan, SI `fetch.auto` está activo, el sistema SHALL
lanzar `git fetch --prune` automáticamente sobre todos los repos con upstream,
en batches de `fetch.concurrency` fetches en paralelo, cada uno con timeout
`fetch.timeout`. Cada repo SHALL pasar por los estados `fetching → fetched`
(ok, re-colecta estado tras fetch) / `failed` (visible en UI, no fatal).
Los fetches SHALL poder cancelarse (contexto global al cerrar la TUI). El
fetch manual queda disponible: `f` sobre el repo del cursor, `F` fetch de
todos (mismo mecanismo de batches).

#### S8.1: fetch automático tras scan

- GIVEN `fetch.auto=true` y un scan que encuentra 10 repos con upstream
- WHEN termina el scan
- THEN se lanzan fetches en batches de 4 sin bloquear la UI, y cada fila
  transita `⟳ → ✓` (con ahead/behind actualizado tras su fetch)

#### S8.2: timeout y fallos

- GIVEN un repo cuyo fetch tarda más que `fetch.timeout` (o falla por red)
- WHEN vence el timeout
- THEN la fila marca `✗ fetch failed`, el resto de batches continúa y la
  barra notifica el fallo; el estado anterior del repo no se corrompe

#### S8.3: auto desactivado

- GIVEN `fetch.auto=false`
- WHEN termina el scan
- THEN no se lanza ningún fetch automático (solo `f`/`F` manuales)

#### S8.4: repos sin upstream se saltan

- GIVEN un repositorio sin upstream
- WHEN corre el fetch automático
- THEN se omite sin marcar fallo (estado `no-upstream` persiste)

#### S8.5: fetch manual

- GIVEN el cursor sobre un repo
- WHEN se pulsa `f`
- THEN se lanza su fetch (reutilizando el pipeline con re-colección); `F`
  lanza el fetch de todos los repos con upstream

### R9: Acciones

Sobre el repo del cursor: `p` SHALL ejecutar `git pull --ff-only` y `P`
`git push`, capturando su salida. Éxito → notificación `pull ok (1.2s)` /
`push ok` y re-colección de estado; fallo → notificación `pull failed`
con hint (`diverged? try pull --rebase manually`) y la salida visible en el
detalle. `e` SHALL abrir `$EDITOR` en el directorio del repo con handoff de
terminal (la TUI se suspende y restaura al salir). Mientras una acción corre
sobre un repo, otra acción sobre el MISMO repo SHALL bloquearse con
notificación; repos distintos SHALL poder actuar en paralelo. Acciones sobre
repos `no-repo` SHALL bloquearse con notificación.

#### S9.1: pull fast-forward

- GIVEN el cursor sobre un repo behind con posibilidad de ff
- WHEN se pulsa `p`
- THEN `git pull --ff-only` corre capturado, al acabar notifica `pull ok`,
  re-colecciona y behind pasa a 0

#### S9.2: pull divergido

- GIVEN un repo diverged
- WHEN se pulsa `p`
- THEN el pull falla, notifica `pull failed` con hint y la salida queda
  visible en el detalle; el estado diverged persiste

#### S9.3: push

- GIVEN el cursor sobre un repo ahead
- WHEN se pulsa `P`
- THEN `git push` corre capturado y al acabar ahead pasa a 0 con notificación

#### S9.4: editor con handoff

- GIVEN `$EDITOR=nvim` y el cursor sobre un repo
- WHEN se pulsa `e`
- THEN la TUI se suspende, nvim abre en el directorio del repo y al salir la
  TUI se restaura y re-colecciona el estado del repo

#### S9.5: bloqueo por repo

- GIVEN un pull en curso sobre un repo
- WHEN se pulsa `P` (o `p`) sobre el mismo repo
- THEN se notifica `pull already running in <name>` y no se lanza nada
- WHEN se pulsa `p` sobre OTRO repo
- THEN corre en paralelo sin interferencias

#### S9.6: guards

- GIVEN el cursor sobre un repo `no-repo`
- WHEN se pulsa `p`/`P`/`f`
- THEN se notifica `no git repo — nothing to do` y no se lanza nada

### R10: Vista de detalle

La tecla `enter` SHALL abrir un panel de detalle del repo del cursor con:
path, branch, upstream, estado derivado, ficheros cambiados (con su código de
estado porcelain: M/A/D/R/?), últimos 5 commits (`%h %cr %s`), y la salida de
la última acción si existe. `esc` SHALL cerrarlo volviendo a la tabla. El
detalle SHALL refrescarse con los datos vivos del repo (no snapshot muerto
del scan).

#### S10.1: apertura y contenido

- GIVEN el cursor sobre un repo dirty
- WHEN se pulsa `enter`
- THEN el panel muestra path, branch, upstream, los ficheros cambiados con
  sus códigos y los últimos commits; `esc` vuelve a la tabla

#### S10.2: salida de última acción

- GIVEN un pull que acaba de fallar en el repo
- WHEN se abre el detalle
- THEN la sección de última acción muestra la salida capturada del pull

### R11: Cache de descubrimiento

El sistema SHALL persistir el descubrimiento en `~/.cache/gitdash/repos.json`
(XDG_CACHE_HOME): por repo, path, name, group, isWorktree. Al arrancar SHALL
pintar inmediatamente los repos del cache (validando que marcador y `.git`
sigan existiendo; los inválidos se descartan silenciosamente) y SHALL lanzar
el rescan en background que reemplaza la lista al terminar. Cache corrupto
SHALL ignorarse sin ruido. El cache SHALL actualizarse al final de cada
rescan.

#### S11.1: arranque con cache válido

- GIVEN un cache válido de un arranque anterior
- WHEN arranca gitdash
- THEN la tabla se pinta al instante con los repos del cache y el rescan
  corre en background refrescando al terminar

#### S11.2: entradas stale

- GIVEN un cache con un repo cuyo directorio fue borrado
- WHEN arranca gitdash
- THEN esa entrada se descarta silenciosamente y no aparece en la tabla

#### S11.3: cache corrupto

- GIVEN un cache con JSON inválido
- WHEN arranca gitdash
- THEN se ignora (sin crash, sin notificación) y el scan corre normal

### R12: Modo print

Con `gitdash --print`, el sistema SHALL ejecutar discovery + recolección
(sin fetch) e imprimir una tabla en texto plano con las mismas columnas del
dashboard, ordenada igual, y salir con código 0. Este modo SHALL reutilizar
los mismos paquetes sin UI.

#### S12.1: print de tabla

- GIVEN repos descubiertos en los roots
- WHEN se ejecuta `gitdash --print`
- THEN se imprime la tabla alineada por stdout y el proceso termina en 0

#### S12.2: print sin repos

- GIVEN un root sin marcadores
- WHEN se ejecuta `gitdash --print`
- THEN se imprime `no repositories found` y termina en 0

## Fixtures de prueba

`testdata/playground/` (creado por `scripts/gen-fixtures.sh`, repos git
reales, deterministicos):

| Fixture | Estados cubiertos |
|---|---|
| `clean-go` | clean sincronizado con upstream |
| `dirty-java` | 1 modificado + 2 untracked |
| `ahead-rust` | 2 commits locales sin push |
| `behind-python` | 3 commits del upstream sin pull |
| `diverged-node` | ahead 2 + behind 3 |
| `no-upstream-cpp` | sin upstream configurado |
| `detached-shell` | HEAD detached |
| `worktree-wt` | proyecto en worktree (`.git` fichero) |
| `no-repo-plain` | marcador sin `.git` |
| `bad-marker-toml` | marcador con TOML inválido |
| `nested/mono-*` | marcadores anidados válidos |

Los tests de parsing de `porcelain=v2` SHALL ser unitarios con salidas
canned; los de discovery SHALL correr contra fixtures; la TUI SHALL tener
smoke tests con el modelo directo (patrón de vroom, sin teatest).

## Revisiones (desviaciones de la implementación)

- **R5 (precedencia del estado derivado)**: `dirty` es independiente del
  upstream — un repo dirty sin upstream se reporta `dirty`, no
  `no-upstream`. La columna ↑↓ sigue mostrando `—` y el fetch sigue
  excluyéndolo (comprueba `HasUpstream`, no el estado derivado). Precedencia
  final: diverged > dirty > ahead > behind > detached > no-upstream > clean.
- **R7 (S7.1, alcance del filtro `n`)**: "no-limpios" = estados con cambios
  pendientes reales (dirty, ahead, behind, diverged). Los estados
  `no-upstream`, `detached`, `no-repo` y `error` se excluyen del filtro
  (no son "trabajo pendiente"), aunque sí puntúan en el orden de atención.
- **R2 (nested)**: el walk desciende siempre dentro de un proyecto con
  marcador; los proyectos anidados con marcador propio aparecen como
  entradas separadas (S2.4). Sin deduplicación: dos marcadores = dos
  entradas.
- **Detalle de implementación verificado en integración**: los mensajes de
  Bubbletea leen UN evento del canal por Cmd — el rearmado de la bomba de
  eventos tras cada evento es obligatorio o los estados nunca llegan a la
  UI (detectado en el smoke test con tmux).
