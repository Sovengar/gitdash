# gitdash

[![CI](https://github.com/Sovengar/gitdash/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Sovengar/gitdash/actions/workflows/ci.yml)

Panel de estados git en TUI, estilo GitHub Desktop: todos tus repos marcados
con `.gitdash.toml` en un dashboard, con branch, cambios pendientes
(dirty) y **↑ahead / ↓behind** (qué hay que subir y bajar de un vistazo),
más acciones rápidas.

Inspirado en [bircni/git-statuses](https://github.com/bircni/git-statuses)
(Rust, CLI one-shot) — no es un fork: la idea del escaneo y el modelo de
datos se reimplementaron en Go + Bubbletea v2 como TUI interactiva, con
descubrimiento por **fichero marcador** en lugar de buscar `.git` suelto y
**fetch automático en batches**.

## Instalación

```bash
go install gitdash/cmd/gitdash@latest   # o, desde el repo:
make install                            # instala en ~/.local/bin (soporta PREFIX/DESTDIR)
```

Requiere el binario `git` en el PATH (subprocess; sin cgo ni librerías git).

## Uso

```bash
gitdash            # TUI
gitdash --print    # tabla one-shot en stdout (útil para scripts/debug)
```

Marcador `.gitdash.toml` en la raíz de cada proyecto (campos todos opcionales):

```toml
name = "api"                # nombre mostrado (default: nombre del directorio)
primary_group = "vsocial"   # grupo primario (nivel 1 plegable)
secondary_group = "backend" # grupo secundario (nivel 2 plegable, dentro del primario)
sync_branch = "main"        # rama de referencia de la columna SYNC (override del global)
```

### Config

`~/.config/gitdash/config.toml` (todo opcional, con estos defaults):

```toml
marker      = ".gitdash.toml"
roots       = ["~/dev"]
exclude     = ["node_modules", "target", "vendor", "dist", "build", "out",
               "coverage", ".venv", "__pycache__", ".gradle", ".terraform"]
editor      = "vi"        # $EDITOR si está definida
sync_branch = "main"     # rama de referencia para la columna SYNC

[fetch]
auto        = true    # fetch automático tras cada scan/rescan
concurrency = 4       # fetches en paralelo (batches)
timeout     = "30s"   # timeout por fetch
```

## Columnas de la tabla

La tabla es *quieta*: las celdas quedan vacías cuando no hay nada que
comunicar y solo se pinta lo que pide atención (el orden sigue siendo
attention-first).

- **BRANCH** rama actual; en detached `<sha> (detached)` (única mención del
  estado en la fila).
- **Work Tree** solo el working tree: `N ?M` = N ficheros trackeados
  cambiados + M sin trackear; `∅` sin repo; `⚠` error git. Sin bola ni
  flechas: los estados de commits viven en ↑↓up.
- **↑↓up** deriva de commits contra el *upstream* de la rama actual (qué hay
  que subir/bajar): `↑N↓N` diverged, `↑N`/`↓N`, `no-up` = rama sin upstream
  trackeado ("sin cuerda al remoto": no hay contra qué comparar).
- **SYNC** desviación respecto a la *sync branch* (`sync_branch` global,
  overridable por repo en el marcador): `<rama> ↓N` = hay N commits en la
  sync branch que tu rama no tiene — la pregunta CI-first "¿mi `feat/x`
  tiene los últimos cambios de `main`?"; `<rama>` a secas = al día;
  `<rama> —` = la ref no existe; `—` = sin rama resuelta. La rama elegida
  es siempre visible. La frescura depende de que la ref local de la sync
  branch esté actualizada; el fetch automático solo refresca remote-tracking
  refs.
- **ACTIVITY** último commit en tiempo relativo.
- **FETCH** solo transitorio: `⟳ fetch` corriendo, `✗ fetch` fallo
  (persistente hasta el próximo fetch de ese repo); el éxito no ocupa celda
  (lo señala la notificación de la barra).

## Teclas

| Tecla | Acción |
|---|---|
| `j/k`, `↑↓` | mover cursor (`g`/`G` extremos) |
| `n` | alternar solo repos con cambios pendientes |
| `/` | filtrar por nombre/grupo o por rama/basename de worktree (en vivo; `enter` confirma, `esc` limpia) |
| `space` | expandir/plegar los worktrees del repo bajo el cursor (configurable, acción `expand`) |
| `D` | borrar el worktree bajo el cursor (configurable, acción `worktree_remove`; solo el worktree, la rama se conserva) |
| `tab` | plegar/desplegar el grupo bajo el cursor |
| `r` | rescan completo (discovery + estados + fetch auto) |
| `R` | re-coleccionar el repo del cursor |
| `f` / `F` | fetch del repo / fetch de todos |
| `p` | pull (`--ff-only`; si divergió, falla visible con hint) |
| `P` | push |
| `e` | abrir `$EDITOR` en el directorio del repo |
| `enter` | detalle: ficheros cambiados, commits, worktrees, última acción; sobre un header de grupo pliega/despliega |
| `q` | salir |

## Worktrees y grupos

- Los **worktrees** se muestran plegados bajo el repo principal con un
  indicador `▸ (N wt)` y se **expanden/pliegan con `space`** sobre su fila.
  Expandido, cada worktree de `git worktree list` aparece como **sub-fila
  navegable y operable** (incluidos los que no tienen marcador y los que
  están fuera de los roots): el cursor puede posarse en ella y *todas* las
  acciones de repo (fetch, pull, sync, push, lazygit, update, editor,
  recollect, `!` y detalle) se ejecutan contra el path de ese worktree. La
  sub-fila muestra su rama (o `(detached)`) y deja vacías las celdas de
  estado por-worktree (no se inventa dirty/ahead/behind/sync). El estado
  expandido/plegado persiste entre sesiones (mismo `collapsed.json` que el
  plegado de grupos, en un namespace propio). El filtro `/` encuentra
  worktrees por rama o basename y revela su padre expandido de forma
  transitoria (sin alterar la persistencia). Queda visible como fila propia
  solo un worktree cuyo repo principal no está descubierto (tag `[wt]`).
- Con el cursor sobre una **sub-fila de worktree**, `D` (acción
  `worktree_remove`, configurable) borra el worktree: carpeta y registro en
  `.git/worktrees`, **nunca la rama**. El primer `D` arma una confirmación
  persistente (`remove worktree <nombre>? D to confirm, esc to cancel`); el
  segundo `D` ejecuta `git worktree remove` desde el repo principal y, al
  terminar, re-colecciona el padre para que la sub-fila desaparezca. Si el
  worktree tiene cambios sin commitear o untracked, git falla: se muestra el
  motivo real y se arma un segundo nivel (`D to force`) que reintenta con
  `--force`; un fallo del forzado reporta el error y desarma (sin bucle).
  `esc` cancela en cualquier punto, y cualquier otra tecla desarma. `D` sobre
  una fila de repo, un header de grupo o la tabla vacía no hace nada (avisa
  `select a worktree`).
- Si algún marcador define `primary_group`, la tabla se **agrupa en dos
  niveles** (patrón vroom): el bloque de cada grupo desde la posición de su
  primer miembro con header `▾ nombre (n)`; dentro de un primario, cada
  `secondary_group` forma un sub-bloque con header indentado. Ambos niveles
   son plegables (plegar el primario oculta sus secundarios). Los repos sin
   primario van a la sección `(ungrouped)` al final; un `secondary_group`
   sin `primary_group` se ignora. La TUI no repite el grupo en una columna
   (los headers plegables ya lo dicen); `--print` —tabla plana, sin
   headers— sí muestra la columna GROUP. Sin grupos la tabla es plana.

## Cómo descubre repos

Recorre los `roots` en profundidad ilimitada (podando ocultos y
`exclude`), buscando el marcador. La carpeta del marcador ES el repo:
`.git` directorio = repo normal, `.git` fichero = worktree (etiquetado
`[wt]`), sin `.git` = visible como `no repo`. Los estados derivados:
`clean`, `dirty`, `ahead`, `behind`, `diverged`, `no-upstream`,
`detached`, `no repo`, `error` — ordenados atención-primero.

## Desarrollo

```bash
go build ./... && go vet ./... && go test ./...
./scripts/gen-fixtures.sh    # genera testdata/playground (repos fixture)
XDG_CONFIG_HOME=$(mktemp -d) bin/gitdash --print   # prueba headless
```

Arquitectura: `internal/config` (TOML XDG), `internal/discovery` (walk por
marcador), `internal/gitstatus` (subprocess git + parsing `porcelain=v2`),
`internal/cache` (pintura instantánea al arrancar), `internal/tui`
(dashboard Bubbletea v2).

## Roadmap

- Fetch de la sync branch (frescura de la columna SYNC sin pull manual)
- Acciones grupales (fetch/pull de todo un grupo)
- Acciones extra: git update, stash, PRs
- Fetch programado en background

MIT
