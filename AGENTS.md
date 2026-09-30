# AGENTS.md — gitdash

Guía para agentes sin contexto previo sobre este proyecto.

## Qué es

TUI (Go + Bubbletea v2) que muestra el estado de todos los repos git del
usuario descubiertos por **fichero marcador** `.gitdash.toml`: branch, cambios
pendientes (dirty) y **↑ahead/↓behind**, con fetch automático en batches y
acciones rápidas (pull/push/editor). Inspirado en
[bircni/git-statuses](https://github.com/bircni/git-statuses) — no es un
fork: solo comparte la idea.

## Stack

- Go 1.26+, module `gitdash`
- `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`
  (import paths `charm.land`, NO github.com/charmbracelet)
- `github.com/BurntSushi/toml`
- Sin cgo: git vía subprocess (`git status --porcelain=v2 --branch`)

## Comandos

```bash
go build ./... && go vet ./... && go test ./...   # build + lint + tests
go build -o bin/gitdash ./cmd/gitdash              # binario (artefacto de build)
make install                                       # instala bin/gitdash en ~/.local/bin (PREFIX/DESTDIR)
go mod tidy                                        # tras añadir deps
```

```bash
./scripts/gen-fixtures.sh                          # regenera testdata/playground
bin/gitdash --print                                # modo tabla one-shot
# smoke test de la TUI (ver Gotcha 3):
tmux new-session -d -s gd 'XDG_CONFIG_HOME=<tmp> bin/gitdash' && sleep 3 && tmux capture-pane -t gd -p
```

**REGLA**: al terminar cualquier cambio de código, INSTALAR el binario
(`make install`). El usuario ejecuta el de `~/.local/bin`: un bin stale con
cambios ya hechos causa síntomas falsos (ej. "no encuentra repos" por el
rename del marcador).

## CI y protección de `main`

CI vive en `.github/workflows/ci.yml` y corre en **todo PR** y en **todo push
a `main`** (sin filtros `paths`: un workflow skipeado deja los required checks
en pending para siempre y bloquea todos los PRs). Tres jobs:

- **`Build`**: `go build ./...` y `go vet ./...`.
- **`Lint`**: `make lint` → vet + fmt-check (gofmt) + golangci-lint
  **v2.13.2** (versión pineada en el `Makefile`; no hay `.golangci.yml`, corre
  el set de linters por defecto).
- **`Test`**: `go test -race -coverprofile=coverage.out ./...` (suite completa,
  sin `-short`) y un resumen de cobertura en el step summary. La suite es
  **autocontenida**: cada fixture git se crea bajo `t.TempDir()` con
  `internal/testutil`, así que CI **no** necesita `make fixtures` ni
  `testdata/playground`.

Reglas de la rama `main` (ruleset **`protect-main`**, reproducible con
`scripts/setup-repo-protection.sh`, idempotente y con `--dry-run`):

- Merge **solo vía PR**, con los tres checks en verde; force-push y borrado de
  `main` bloqueados.
- Existe **bypass de admin** y es **deliberado** (aprobado por el usuario): un
  admin *podría* pushear directo, pero la intención de trabajo es siempre el
  camino PR. Ningún actor no-admin puede hacerlo.
- `delete_branch_on_merge=true`: GitHub borra la rama remota al mergear.

Ante un merge: verificar que el workflow `push` de `main` quedó verde y que el
badge del README reporta `passing` (el badge cachea unos segundos).

`make smoke` (tmux + pty, ver Gotcha 3) queda **manual y fuera de CI**: necesita
un terminal interactivo que el runner no garantiza.

**Trampa del smoke**: aísla `XDG_CONFIG_HOME`, y git lee su config global de
`$XDG_CONFIG_HOME/git/config`. Con la config del usuario oculta, un `git pull`
sobre un repo divergido **falla** ("divergent branches") donde en tu terminal
rebasa, y parece un bug de gitdash. Para probar la política real del usuario,
enlaza la config de git al directorio aislado
(`ln -s ~/.config/git/config "$tmp/git/config"`).

## Arquitectura (flujo de datos)

```
config → discovery (walk por marcador) → gitstatus (subprocess por repo, pool)
       → eventos por canal → tui (Update/View) → render
```

| Package | Rol |
|---|---|
| `internal/config` | TOML XDG. `Load()` nunca falla: defaults + warning string |
| `internal/discovery` | `Project{Path,Name,Group,SyncBranch,HasRepo,IsWorktree,MainRepo,MarkerErr}`. La carpeta del marcador ES el repo (no se busca `.git` hacia arriba). Poda ocultos + `exclude`. `MainRepo` enlaza worktree→repo principal |
| `internal/gitstatus` | `parse.go` puro (ParsePorcelain, ParseWorktrees, Derive, Score) + `status.go` (Collect, StreamPool, Run, Fetch, RemoteURL, RebaseInProgress, RemoveWorktree) + `outcome.go` (Classify: qué hizo git de verdad). El `Snapshot` lleva `Err` embebido y también la desviación vs sync branch (`SyncBehind`) y sus worktrees; nunca falla duro. `runGit`/`runGitCombined` son el **único** punto por el que sale un subprocess git, y ambos dejan entrada en el command log |
| `internal/forge` | Puro (sin I/O): `RepoRef` + `ParseRemoteURL` (remote → forge/host/proyecto, con el prefijo de subcarpeta), `WebURL`, `ForgeForHost`/`PublicHosts` (hosts públicos) y `BuildCreateArgv`/`CreateBin`/`PromptEnv` (el argv de `gh pr create` / `glab mr create`). La ejecución NO es de aquí: es de `internal/forge/tool` (Runner con plazo de 30 s y `Error` con exit code) |
| `internal/cache` | `repos.json` para pintar instantáneo al arrancar; validación por existencia del marcador; corrupto = silencioso |
| `internal/cmdlog` | Ring acotado en memoria (500) de lo que se ejecutó: entries de `intent` (tecla) y `exec` (proceso con argv, exit, duración y resultado). Global con default no-op; solo la TUI lo instala (`tui.New`) |
| `internal/tui` | `app.go` (modelo + pipelines de fondo), `update.go` (Update/View/teclas), `table.go` (filas/orden/celdas/agrupación), `detail.go`, `proverlay.go` (formulario de PR) + `prcreate.go` (su ejecución), `cmdlogpanel.go` (panel del log), `styles.go` |
| `internal/group` | Arrangement de la vista agrupada a 2 niveles (estilo vroom): `Arrange` + `IsPrimaryHeader`/`IsSecondaryHeader` |
| `internal/testutil` | helpers para crear repos git fixture reales en `t.TempDir()` (bare origin, push upstream, worktrees, ramas) |
| `cmd/gitdash` | `main.go` (TUI) + `print.go` (modo `--print`, tabwriter, mismo orden) |

## Convenciones

- Comentarios de código en **español**, **sin referencias a specs ni a IDs de
  requisito/escenario**: el código es la fuente de verdad. No hay artefactos
  SDD en el repo y no se crean (`proposal.md`, `spec.md`, specs con IDs de
  requisito/escenario, `R#n SHALL`, `S#n.#`).
- `docs/planning/<NNNN>-<tipo>-<slug>/` NO es un artefacto SDD: es el paquete
  que deposita el pipeline de planificación como andamiaje suyo, así que es
  válido en el repo y no se retira.
- Tests del modelo **directo** (construir Model, enviar msgs con Update,
  inspeccionar estado) — sin teatest. Patrón: `internal/tui/app_test.go`.
- Estados derivados con precedencia: `diverged > dirty > ahead > behind >
  detached > no-upstream > clean`. `State.Score()` (gitstatus) da el orden
  atención-primero compartido por TUI y `--print`.
- Las celdas de tabla devuelven `(texto, estilo)`: el render hace
  `pad(texto)` ANTES de aplicar estilo (el ANSI rompe el cálculo de ancho).

## Gotchas críticos

1. **Event pump**: cada `tea.Cmd` lee UN evento del canal. SIEMPRE rearmar
   `waitForEvent` (helper `withPump`) en Update tras consumir un evento del
   canal. Sin esto solo llega el primer mensaje y los estados nunca pintaan.
2. **ahead/behind requieren fetch**: los remote-tracking refs solo se
   actualizan con `git fetch`. Los tests que simulan behind/diverged deben
   hacer `testutil.FetchLocal` tras pushear al origin.
3. **TUI smoke tests**: usar **tmux** (`capture-pane`). `script` NO funciona:
   bubbletea v2 bloquea el primer render esperando las respuestas a las
   queries de capacidades kitty del pty tonto (síntoma: alt-screen en
   blanco, proceso vivo, sin stderr).
4. **porcelain v2**: líneas `1 ` → 7 campos antes del path; `2 ` (renames) →
   8 y emite `<nuevo>\t<viejo>`; `u ` → 9. `# branch.oid` da el sha para
   detached. La ruta es TODO lo restante (puede contener espacios).
5. **Fixtures**: el marcador debe commitearse en el commit base, si no
   aparece como untracked y ensucia el estado dirty de todos los repos.
6. **textinput v2 con teclas sintéticas**: `tea.KeyPressMsg` necesita
   `Code` Y `Text` — solo `Code` no inserta runas en el input.
7. **La política de pull es del usuario, no nuestra**: `commands.pull` va
   **sin flags** a propósito. Los flags en la línea de comandos pisan el
   gitconfig, así que un `--ff-only` hardcodeado anulaba un `pull.rebase=true`
   del usuario (comprobado: el mismo repo divergente rebasea con `git pull` pelado
   y no con `git pull --ff-only`). Las variantes con flags existen solo para el
   selector de `p`, que ofrece la política explícita. **No reintroduzcas flags en
   el default.**
8. **Un pull --rebase que choca no es un fallo limpio**: deja el rebase a medias
   (`rebase-merge`/`rebase-apply` en el dir del worktree). Por eso
   `gitstatus.RebaseInProgress` existe y el aviso tiene prioridad sobre los
   hints de divergencia/upstream: decir "falló" invita a reintentar sobre un
   rebase sin resolver. Se resuelve con `git rev-parse --git-path`, no mirando
   `.git/rebase-*` a pelo, porque en un worktree `.git` es un fichero.
9. **El argv resuelto viaja en `actionMsg`/`actionResult`**: con la política
   delegada en el gitconfig, el kind ya no implica los flags. El detalle lo
   muestra; sin eso la UI miente sobre lo que reconcilió.
10. **Las filas se ordenan attention-first**: la posición del cursor NO es la del
   fixture de test. Los tests que necesitan una fila concreta la localizan por
   path (`cursorOn`), no por índice.

## Gotcha de diseño: el selector de pull

`p` no ejecuta: arma `pullArmed` con el path capturado, y la **siguiente** tecla
elige variante (`p`/`r`/`f`/`m` en `PullKinds`, más `a` para la variante AI, que
no es un pull de git y se resuelve aparte). Dos reglas:

- Las teclas de variante **chocan con acciones reales** de la tabla (`p`=pull,
  `r`=rescan, `f`=fetch), así que el estado armado tiene que consumir la tecla
  **antes** del enrutado normal en `handleKey`.
- Cualquier otra tecla **cancela y sigue su curso normal** (no se consume): es
  lo que evita que la app quede pegada esperando una segunda pulsación. Es el
  patrón de prefix-key, no un modo bloqueante.

El prompt se pinta en la **sección keybinds, sustituyendo las hints**, no en un
toast: los toasts expiran a los 3 s y el selector vive hasta la siguiente tecla.
Keybinds es su sitio porque comparte función con las hints ("qué hago ahora"), y
el banner de stats queda para el resumen y la actividad en curso. Lo mismo aplica
a `removePrompt`: los dos avisos salen de `armedPrompt()`, que devuelve uno u
otro, y `keybindsLines()` deriva de ahí el presupuesto de alto (1 línea con
aviso armado, `defaultHintLines` sin él) para que la caja nunca mida más que su
contenido. Con un aviso armado, `computeLayout` degrada **stats antes que
keybinds** (`keepKeybinds`): si la caja del aviso cayera, la app quedaría
esperando una tecla sin decir cuáles.

## Gotcha de diseño: el panel de preview

Debajo de la tabla hay una ficha del repo bajo el cursor (estilo prdash), entre
el listado y los keybinds. **Es la única vista de detalle**: no hay `enter`
detalle, ni `detailSection`, ni `detailOpen`. Decisiones que no son evidentes:

- **El título va solo en el borde.** `detailTitle`/`worktreeTitle` componen el
  título de la caja y `renderDetail` NO lo repite como primera línea: pintado en
  los dos sitios, el mismo texto salía duplicado justo bajo el borde.
- **El panel es aditivo.** `computeLayout` busca el mayor alto de panel que
  (a) deje `minBodyLines` filas de tabla y (b) no obligue a recortar hints ni a
  ocultar stats/keybinds (`mismaChromeQue`). Si no hay ninguno, el panel no se
  dibuja. Por debajo de ~21 líneas (config por defecto) el dashboard es
  exactamente el que había antes de la feature. Ese suelo sale de
  `detailHeadLines`, así que `minPanelHeight` (test) lo deriva en vez de
  mendigar el número.
- **El share se mide sobre el alto LIBRE**, no sobre el terminal: contra el
  total, una ventana de 30 líneas se quedaba con 12 para la ficha y 3 para la
  tabla.
- **El presupuesto de la ficha son sus líneas**, no las de la terminal:
  `renderDetail(r, rows)` reserva `detailHeadLines` para la cabecera de estado y
  reparte el resto con `listBudget`, que reserva la línea del aviso `… N más`
  cuando la lista no cabe entera. `rows` es `lay.previewLines`. Sin eso, las
  listas se cuentan como si cupieran y luego las recorta la caja sin avisar.
- **El path es un campo, no una línea suelta**: `path` va en la misma columna
  clave/valor que `branch`/`upstream`/`state`/`sync`, y su valor sale atenuado
  (es contexto, no estado). En línea propia con el hueco que la separaba se
  llevaba una altura que las listas necesitan; al ser campo, la cabecera son
  `detailHeadLines` = 5 líneas.
- **La ficha no repite las teclas de la fila**: `g lazygit · ! cmd` ya están en
  la sección de keybinds, así que la ficha no lleva pie (`fichaTail` solo añade
  el input de `!`). Duplicarlas costaba una línea de alto útil y dos fuentes que
  podían divergir con un rebind.
- **El input de `!` va al FINAL de la ficha y siempre se ve** (`fichaTail`): si
  la ficha llenó la caja, se recorta la ficha por arriba. Escribir un comando sin
  ver el prompt es escribir a ciegas.
- **La caja se rellena** (`fitLines`): el alto lo dice el layout, no la ficha. Sin
  el relleno, una ficha corta haría subir los keybinds y la vista no ocuparía la
  terminal.

Con el cursor sobre un **header de grupo** el panel no tiene ficha que enseñar:
muestra el agregado del grupo (`groupStats`, sobre `rows()` y antes del
plegado, que es lo mismo que cuenta su header). Los estados a cero no se pintan.

## Gotcha de diseño: `enter` es la única tecla de plegado

`enter` (acción `fold`) pliega **lo que hay bajo el cursor**, y cada fila tiene una
cosa distinta debajo: un header pliega su bloque, una fila de repo pliega sus
sub-filas de worktree, y una sub-fila de worktree no tiene nada que plegar (no-op,
y a propósito: plegar el grupo del padre desde la sub-fila sería una sorpresa).
No hay vista de detalle que abrir, así que `enter` quedó libre para esto; `tab`
(plegado) y `space` (expansión) se eliminaron por redundantes.

Los dos estados se siguen persistiendo en el mismo `collapsed.json` (worktrees
bajo su prefijo), así que el plegado sobrevive entre sesiones.

Los hints llevan la acción SIN la tecla dentro (`hintLabels`): la tecla la
antepone `HintBarLines`. Si la etiqueta la llevara, un rebind producía hints
como `w enter fold`. Y `config.LoadFrom` avisa (toast + stderr) de las acciones
de `[keybindings]` que ya no existen: sin ese aviso, un `detail = "enter"` de una
config vieja deja `enter` muerta y parece un bug de la TUI.
## Gotcha de diseño: el command log (`l`)

El argv **no** dice qué política de pull aplicó git. `commands.pull` va sin flags
a propósito, así que `p` `p` ejecuta `git pull` y con `pull.rebase=true` en el
gitconfig del usuario eso integró con rebase. El log resuelve el "qué pasó de
verdad" con dos piezas:

- **`gitstatus.Classify(args, out, exit)`** deduce el resultado de la salida que
  git ya imprimió (`Successfully rebased and updated` → `rebase`,
  `Applied autostash` → `rebase+autostash`, `Merge made by` → `merge`,
  `Fast-forward`, `up to date`, `Not possible to fast-forward` → `diverged`,
  `could not apply`/`CONFLICT` → `conflict`…). Subprocess extra: **cero**.
- Las **intenciones** (tecla + acción + repo) las registra `handleKey`, porque el
  argv no distingue "pulsé p y elegí rebase" de "el gitconfig decidió por mí".

**No sondees `git config` para deducir la política**: `branch.<name>.rebase`
pisa al `pull.rebase` global, esa precedencia cambia entre versiones de git
(`branch.<name>.rebase` está deprecado a favor de `branch.<name>.pullrebase`) y
gitdasharía devolviendo una respuesta plausible y equivocada. Lo que git HIZO
está en su output, y con `LC_ALL=C` forzado en `gitEnv` los mensajes no se
localizan.

**El reflog se descartó como fuente** (comprobado con git real, no de memoria):
`git pull` pelado deja `pull (start)/(pick)/(finish)` si rebasea,
`pull: Merge made by the 'ort' strategy.` si hace merge y `pull: Fast-forward`
si ff, pero **no escribe ninguna entrada** cuando ya estaba al día — justo el caso
en que se pregunta "¿qué pasó con el pp?" — y no distingue un pull de gitdash de
uno manual en tu terminal, ni cubre push/fetch/`!`/worktree remove. Si algún día
se quiere como modo forense, es un `git reflog show --date=iso` **bajo demanda**
(una llamada al abrir el detalle de un repo), no por acción.

Reglas del panel (`internal/tui/cmdlogpanel.go`):

- Es un **view mode**, no un overlay: `logOpen` toma el cuerpo y comparte el
  chrome del detalle. Sus teclas se consultan **antes** del enrutado normal
  (como los estados armados) porque `j`/`k` chocan con la navegación; el resto de
  teclas sigue su curso normal, así la app no queda encerrada.
- `promptLine()` (antes `armedPrompt`) es el **único** punto por el que keybinds
  pinta un aviso: los dos armados y la leyenda del panel. `keybindsLines()` y
  `keepKeybinds` derivan de ahí, así que añadir un aviso nuevo es añadir un
  `case` y nada más.
- Abrir el panel **suelta los estados armados**: el aviso queda sin sentido fuera
  de su vista y dejarlo armado obligaría a acertar la tecla siguiente desde un
  panel que ya no está.
- `launchesCommand` / `actionNeedsRow` deciden qué acciones dejan intención. Las
  de navegación pura (filtro, plegado, detalle, el propio panel) no: el log es de
  comandos, no de teclas. Sin fila bajo el cursor tampoco (una `key p` sin repo
  ni `exec` confunde).
- El offset cuenta **desde la cola** (0 = lo más reciente al final): en un log se
  mira lo último, y las entradas nuevas no te sacan de sitio si estabas
  scrolleado arriba. Se recorta contra las líneas visibles, nunca deja huecos.
- **El argv es texto no confiable** (lleva el prompt del marcador en `pull_ai`):
  `logLine` lo pasa por `sanitizeLogText` antes de pintarlo. Sin eso, una
  secuencia OSC/CSI inyectada se renderiza tal cual, un carácter de formato
  (bidi/zero-width) reordena la línea y un prompt multilínea rompe el alto del
  panel (una entrada = una línea). El saneo quita control (C0/C1/DEL), Cf
  (bidi, zero-width) y U+2028/U+2029, además de las secuencias ESC, y es **solo
  de pintura**: el argv ejecutado y el registrado no se tocan.
- `computeLogColumns` degrada columnas por valor (veredicto → resultado → repo →
  kind) y da al argv lo que sobra: por debajo de `logMinArgv` (20, lo que cabe
  `git pull --ff-only`) el comando se lee a medias, que es lo que el panel existe
  para evitar.

## Gotcha de diseño: el guard de no-op del preview visual (`v m` / `v r`)

git-sim **aborta con código 1** cuando el ref que le pasas ya está contenido en
HEAD: `merge.py` y `rebase.py` imprimen `Branch 'origin/main' is already
included in the history of active branch 'main'` y salen. Eso no es un bug de
gitdash, es la respuesta correcta, pero reproducirla en pantalla cuesta un
handoff completo de terminal para leer un error que el snapshot ya anticipaba:
`Status.Behind == 0` **es** la condición que git-sim comprueba
(`git branch --contains <ref>`).

Por eso el selector bloquea con toast antes de ceder la terminal, y por eso
`armedVisual` captura `behind` **al armar**, junto al path y al upstream: el
guard tiene que decidirse sobre la fila elegida, no sobre la que esté bajo el
cursor cuando llegue la segunda tecla.

- **El guard es de las variantes con ref (`merge`/`rebase`), no del selector.**
  `pull` no lleva argumento posicional y git-sim `pull` clona y simula de
  verdad, sin ese chequeo: con `behind == 0` se lanza igual. Guardarlo sería
  inventar una restricción que la herramienta no tiene.
- **`ahead` no afloja el bloqueo**: un repo con `↑2 ↓0` sigue teniendo el
  upstream contenido en HEAD, así que git-sim fallaría igual.
- **El aviso nombra la tecla de fetch con `cfg.KeyFor("fetch")`**, nunca un
  `f` hardcodeado: `behind` viene del último fetch, así que si el
  remote-tracking está viejo la simulación bloqueada sí tenía contenido y el
  aviso tiene que decir cómo arreglarlo. Los hints del dashboard se pintan
  igual (etiqueta sin tecla, la antepone `HintBarLines`); un toast no pasa por
  ahí, y es el único sitio donde la tecla se escribe a mano.

## Gotcha de diseño: abrir un PR/MR (`O`)

`O` abre un overlay que recoge título, cuerpo, base y draft (`proverlay.go`), y
`ctrl+s` lo envía a `gh pr create` / `glab mr create` (`prcreate.go`). Decisiones
que no son evidentes:

- **NO es un handoff de terminal.** gh y glab son no interactivos con todos los
  flags dados, y `BuildCreateArgv` garantiza eso (el cuerpo se emite siempre, y
  glab lleva su `-y`). Así que no hay TTY que ceder: se captura la salida con
  `forge/tool.Runner`, como el comando `!`. Por eso, y solo por eso, esta acción
  **sí mide duración** en el command log: los handoffs van con `Dur = 0` porque
  medir su proceso exigiría guardar el arranque en el modelo.
- **La cadena es larga y cada paso corta antes de ejecutar**: remote →
  `ParseRemoteURL` → `BuildCreateArgv` → `LookPath` → ejecución. Un PR creado
  contra el repo equivocado no falla visiblemente (gh deduciría el destino), así
  que “no sé de dónde es esto” es un toast que dice qué hacer y NADA ejecutado.
  Los cuatro rechazos son `m.running[path]` liberado, sin exec en el log y sin
  recollect: no pasó nada.
- **El remote se lee on demand** (`gitstatus.RemoteURL`, `ClassRead`), no en
  `Collect`: sumarlo al scan sería un `git remote get-url` por repo y por ciclo
  para un dato que casi nadie mira. Va por `runGit` como todo lo demás, así que
  es auditable desde el panel con `a` (show all).
- **El forge sale de la config, no de una heurística**: `forge.ForgeForHost` solo
  conoce `github.com` y `gitlab.com`, y adivinar el proveedor de un host
  desconocido produce enlaces que abren 404 sin que nada falle. `Config.ForgeHosts()`
  y `Config.ForgePrefixes()` (`internal/config/forge.go`) resuelven los dos mapas
  que consume `ParseRemoteURL`; los hosts públicos salen de `forge.PublicHosts()`
  para que la lista no pueda duplicarse, y los self-managed se declaran:

  ```toml
  [forge.gitlab]
  api_base = "https://git.example.com/git/api/v4/"
  hosts = ["git.example.com"]
  ```

  El `api_base` **absoluto nombra el host al que aplica**, y de su path sale el
  prefijo de la subcarpeta (`/git/api/v4/` → `git`, con
  `forge.PrefixFromAPIBase`): es lo que hace que un GitLab en `/git/` resuelva
  `grupo/sub/widget` y no `git/grupo/sub/widget`. Un `api_base` relativo no
  nombra ningún host y aplica a todos los del proveedor; los hosts que no nombra
  se quedan con el default del proveedor. Un proveedor que no soportamos avisa al
  cargar en vez de aceptarse en silencio.
- **El argv se registra CRUDO y lo sanea quien pinta.** El título y el cuerpo los
  escribió una persona y acaban en el panel del log, así que pasan por
  `sanitizeLogText` (ver la sección del command log). El registro guarda lo que
  se ejecutó, tal cual: un log “limpiado” puede mentir.
- **Aceptar y ejecutar son dos pasos.** El overlay publica el envío en
  `m.prPending` y devuelve un `tea.Cmd` que emite `prStartMsg`; `prCreateCmd` lo
  consume. El seam existe para que un test vea el envío aceptado sin que ningún
  proceso haya salido, que es la mitad que un handoff no tiene.
- **La intención la deja el enrutado genérico**, como toda acción de
  `commandActions`: `key O pr` en el log, y debajo el exec con el argv. Con el
  panel del log abierto la tecla está en el guard que impide abrir overlays (si
  no, dejaría una intención por algo que no ocurrió).
- **La tecla de abrir la resuelve la config** (`cfg.KeyFor("pr")`) hasta en el
  aviso del panel: un texto fijo dejaría mintiendo al usuario tras un rebind. La
  de enviar (`ctrl+s`) NO sale de la config porque no es una acción
  rebindeable.

## Gotcha de diseño: pull con IA (`p a`)

El selector de `p` tiene una quinta variante, `a`, que hace handoff al comando AI
configurado. `p a` **lanza directamente** (sin preview ni confirmación: la
segunda tecla es la decisión). Decisiones que no son evidentes:

- **El límite de confianza es el eje de la feature**: el ejecutable/argv sale
  SOLO de la config global (`[ai.pull] command` en
  `~/.config/gitdash/config.toml`); el marcador commiteado
  (`.gitdash.toml`, input no fiable) aporta SOLO el texto del prompt. Ese texto
  entra como **un único elemento de argv** (`config.BuildAIArgv`), jamás
  interpolado en un `sh -c`. Si duplicas la sustitución en otro sitio, rompes el
  límite.
- **`pull_ai` NO es un `PullKind`**: `PullKinds` alimenta
  `startActionCmd → cfg.CmdArgs → gitstatus.Run` y el guard de
  `RebaseInProgress`, que son caminos de git. `pull_ai` se resuelve como un
  `case` explícito de `a` dentro del bloque `pullArmed` de `update.go` (el
  estado armado consume la tecla antes del enrutado normal, como p/r/f/m).
- **El prompt se relee on demand** con `discovery.MarkerPrompt` y **no** se
  guarda en `discovery.Project`: así no engorda `repos.json`, no queda obsoleto
  tras editar el marcador y se resuelve en la pulsación, no por frame.
- **`pullOptions` es la fuente única de variantes**: `PullKinds`, `pullPrompt()`
  y `pullVariantLabel()` derivan de ella. El bug latente que arregla es real: el
  prompt tenía `[]string{"p","r","f","m"}` hardcodeado y una variante nueva no
  aparecía.
- **Un rebase a medias no bloquea la variante AI**: sin preview no hay nada que
  surfacear, y resolver el rebase puede ser justo la intención del prompt. No se
  consulta `RebaseInProgress` (a diferencia de los pull de git).
- **Handoff sin timeout ni captura**, como lazygit: la terminal es del hijo y al
  volver `execDoneMsg` registra el exec en el command log (`Dur=0`, argv con el
  prompt íntegro) y re-colecta el estado. Sin prompt, sin comando o sin binario
  (`exec.LookPath`) solo hay toast.

## Gotchas de cableado

- **Todo exec de git pasa por `runGit`/`runGitCombined`** (`gitstatus`), que miden
  y registran. Si añades un verbo git nuevo fuera de ahí, no aparece en el log.
  Los 6 `exec.Command` de `tui/app.go` (editor, lazygit, `pull_ai`, visual, `!`,
  shell) están fuera: se registran a mano, en `execDoneMsg` (handoffs, al
  volver) y en `openCmdCmd` (el `!`, que sí mide duración). Los handoffs van con
  `Dur = 0`: medirlo exigiría guardar el arranque en el modelo. `gh`/`glab` no
  son git: salen por `forge/tool.Runner` y los registra `prCreateCmd`, también a
  mano (y SÍ midiendo).
- **`gitstatus.Fetch` recibe la `cmdlog.Class` del caller**: `git fetch --prune`
  es el mismo comando lo lanzan el scan automático y la tecla `f`, y solo el
  origen los separa. Es el único exec cuya clase no se deduce del argv.
- `RemoveWorktreeArgv` existe para que el log, el detail y `RemoveWorktree` no
  puedan discrepar. Si duplicas la construcción del argv en otro sitio, el log
  puede mentir sobre lo que se ejecutó.
- `forge.BuildCreateArgv` es la fuente única del argv de creación (y
  `forge.CreateBin`/`PromptEnv`, la de la puerta y la del host): si los armas en
  otro sitio, el log puede enseñar un comando que no es el que salió.

## Probar

```bash
# config real del usuario (roots default: ~/dev)
bin/gitdash
```

```bash
# aislada contra los fixtures
XDG_CONFIG_HOME=$(mktemp -d) bin/gitdash --print
# con config: crea <tmp>/gitdash/config.toml con roots=["<repo>/testdata/playground"]
```
