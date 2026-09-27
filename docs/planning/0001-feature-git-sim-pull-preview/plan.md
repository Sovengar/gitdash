# Plan — Preview visual de pull con `git-sim`

adr_required: false
(No hay decisión arquitectónica con tradeoffs de peso ni breaking change: la
feature es aditiva y reutiliza tres raíles ya probados — selector prefix-key,
handoff de terminal y command log. Las decisiones tomadas quedan documentadas
aquí abajo.)

## Resultado buscado

Con el cursor sobre un repo, `v` arma un selector y la segunda tecla abre una
previsualización visual de `git-sim` (`pull` / `merge <upstream>` /
`rebase <upstream>`), cediendo la terminal al hijo. git-sim dibuja sin tocar el
repo real. Al volver, gitdash registra el exec con su argv real y re-colecta.

## Enfoque (alto nivel)

Un **camino paralelo** al de `p`/`pullArmed`: mismo mecanismo (armar con la
primera tecla, consumir la variante antes del enrutado normal, cancelar sin
consumir el resto), misma pintura del aviso (`promptLine()`), mismo handoff
(`execDoneMsg`). No se toca `PullKinds`, ni `commands.pull*`, ni `[ai.pull]`.

Puntos clave del diseño:

- **Fuente única de variantes visuales** análoga a `pullOptions`, con p/m/r.
  Derivan de ella el prompt y las etiquetas. Nada de slices hardcodeados.
- **El media-dir es obligatorio**, no cosmético: sin él git-sim escribe
  `git-sim_media/` dentro del repo y gitdash lo marcaría dirty (usa `git status`
  real). Debe crearse si falta; si no se puede crear, se aborta con toast en vez
  de lanzar (lanzar ensuciaría el repo).
- **Auto-open activo**: no se pasa `-d`. Sin `--animate`. Sin
  `--output-only-path` (implicaría capturar stdout, y el handoff no captura;
  la ruta de la imagen es no determinista por el timestamp y no se necesita — el
  argv del log basta para saber qué se ejecutó).
- **`git-sim` NO es git**: se resuelve por PATH y va por handoff; no pasa por
  `runGit`/`runGitCombined`.
- **Rebase en curso NO bloquea** el selector: git-sim no muta el repo real
  (las operaciones de red corren en un clon temporal). Previsualizar un rebase
  a medias es precisamente útil. Coherente con la variante AI, que tampoco
  consulta `RebaseInProgress`.

Decisiones secundarias documentadas:

- `p` visual se permite **sin upstream** (espejo del `p` pelado de gitdash, que
  delega en git; git-sim pull simula y no exige ref). `m`/`r` sí exigen upstream
  (necesitan el `<upstream-ref>` explícito): sin él, toast y nada se lanza.
- El armado **captura path + upstream** al pulsar `v` (el upstream sale del
  `Snapshot.Status.Upstream`, p. ej. `origin/main`), para que el argv sea
  determinista respecto a la fila elegida.
- El dir de caché sale de `os.UserCacheDir()` + `gitdash/git-sim`; si falla, se
  degrada con toast (nunca al repo).

## Pasos y ficheros que toca cada uno

1. **Config** — `internal/config/config.go`
   - `DefaultKeybindings()`: añadir `"visual": "v"`.
   - `hintLabels`: añadir `"visual": "visual"` (etiqueta SIN la tecla).
   - `HintBarLines()`: añadir `"visual"` a la lista de acciones; cae en la fila
     de tools (`default`) por el `switch` actual.
   - La validación de acciones obsoletas de `LoadFrom` sigue funcionando sola
     (valida contra `DefaultKeybindings()`).

2. **Modelo y variantes** — `internal/tui/app.go`
   - Tipo `visualOption{key, kind, label, needsUpstream}` + slice `visualOptions`
     (p/m/r) como fuente única.
   - Tipo `armedVisual{path, upstream string}` y campo `visualArmed *armedVisual`
     en `Model` (efímero, como `pullArmed`).
   - `visualOptions` NO entra en `PullKinds`.
   - Funciones puras y testeables: `visualMediaDir()` (caché + creación del
     dir), `visualArgv(kind, upstream, mediaDir) []string` (compone
     `git-sim --media-dir <dir> <sub> [upstream]`), `visualVariantLabel(kind)`.
   - `startVisualCmd(path, kind, argv)`: guard de "ya hay acción en curso",
     `exec.LookPath("git-sim")` (si falta → toast), `m.running[path]="visual"`,
     `tea.ExecProcess` que devuelve `execDoneMsg{action: kind, argv}`.
   - Reutiliza el handler existente de `execDoneMsg`: logging con `Dur=0` y
     re-colecta (no se inventa camino nuevo).

3. **Enrutado y máquina de estados** — `internal/tui/update.go`
   - Nuevo bloque `if m.visualArmed != nil { ... }` justo **después** del bloque
     `pullArmed` y **antes** del panel del log y del enrutado normal:
     - consume `p`/`m`/`r` como variantes;
     - registra la INTENCIÓN (`cmdlog.RecordIntent` con tecla + variante + repo)
       y lanza `startVisualCmd`;
     - para `m`/`r` sin upstream → toast "no upstream" y no lanza;
     - cualquier otra tecla desarma y NO retorna (sigue su curso normal).
   - Añadir `"visual"` a `commandActions` (deja intención) y a `rowActions`
     (necesita fila).
   - En el `switch action`, caso `"visual"`: si no hay fila o no es repo → toast;
     si hay repo → `m.visualArmed = &armedVisual{path, upstream}` (sin ejecutar).
   - `visualPrompt()`: aviso derivado de `visualOptions`, análogo a
     `pullPrompt()`.

4. **Pintura del aviso** — `internal/tui/sections.go`
   - `promptLine()`: añadir `case m.visualArmed != nil: return m.visualPrompt()`
     (los armados tienen prioridad sobre la leyenda del log; un solo armado a
     la vez porque la segunda pulsación desarma el anterior).
   - `keybindsLines()`/`keepKeybinds`/`computeLayout` derivan solos de
     `promptLine()`: no requieren cambios.

5. **Tests** — `internal/tui/` (+ `internal/config/`)
   - `visual_selector_test.go` (nuevo), patrón directo: construir Model, `press`,
     inspeccionar estado (sin teatest).

## Casos de test (obligatorios)

- **Armado/cancelación del selector**: `v` arma sin lanzar (`running` intacto);
  tecla no-variante desarma y sigue su curso; `esc` desarma.
- **Sin fila / fila sin repo**: `v` sobre header o proyecto sin repo → toast y
  sin armado.
- **Variante por variante con argv exacto**: `visualArgv` puro →
  `["git-sim","--media-dir",<dir>,"pull"]`,
  `[...,"merge","origin/main"]`, `[...,"rebase","origin/main"]`.
- **Media-dir presente** en todas las variantes y apuntando a la caché, NO al
  repo; el dir se crea si falta.
- **Ausencia de upstream**: `m`/`r` → toast "no upstream" y nada lanzado; `p`
  sí se permite.
- **Ausencia del binario**: `exec.LookPath("git-sim")` falla → toast
  "git-sim not installed" y sin handoff (comprobar el `tea.Cmd` devuelto).
- **Media-dir no creable** → toast de error, sin handoff.
- **Config/hints**: `visual` en `DefaultKeybindings`; hint "v visual"
  (etiqueta sin tecla); `HintBarLines` incluye la fila.
- **Aviso**: `promptLine()` con `visualArmed` set; `keybindsLines()==1`.
- **Intención + exec**: elegir variante registra la intención; al simular
  `execDoneMsg` se registra el exec con el argv real (incluye `--media-dir` y
  el ref) y `Dur=0`.

Fixture con upstream: usar el patrón de `pull_selector_test.go` + `testutil`
(`AddUpstream`/`PushUpstreamCommits`/`FetchLocal`). Para el caso "sin upstream",
reutilizar un fixture sin upstream.

## Riesgos y cómo verificarlos

- **`git-sim` no instalado en la máquina del usuario** → solo toast; no hay
  handoff. Verificable manualmente con PATH sin git-sim.
- **`--media-dir` omitido en algún camino** → el repo se ensucia con
  `git-sim_media/`. Verificación: test de `visualArgv` (argv exacto) + manual
  con `git status` tras un preview.
- **La versión de git-sim cambia flags** (0.4.0 en overhaul usa skia). Verificar
  contra git-sim 0.3.5 real; anotar que el plan asume 0.3.5.
- **Handoff de terminal + auto-open** (xdg-open) no es testeable en la suite:
  verificación manual con tmux (`tmux capture-pane`) y comprobando que la imagen
  se abre y que el command log registra el argv.
- **Solape con los estados armados** (`visual` vs `pull` vs `remove`): un solo
  armado a la vez; lo cubren los tests de armado/cancelación.
- **AGENTS.md prohíbe artefactos SDD en el repo** (`docs/planning/`) mientras el
  orquestador exige depositar el paquete de planificación en `planning_dir`.
  Conflicto conocido: estos artefactos deben retirarse antes del merge (o el PR
  los excluye), porque la convención del repo no permite `docs/planning/`.

## Orden de trabajo (grueso)

1. config (acción/keybind/hint) → 2. modelo + argv puro + handoff (app.go) →
3. enrutado + armado + intención (update.go) → 4. aviso en promptLine
(sections.go) → 5. tests → 6. build/vet/test + `make install` y verificación
manual con tmux.
