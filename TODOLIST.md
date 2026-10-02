# TODOLIST — cobertura al 100%

Objetivo: que ningún statement del módulo quede sin ejecutar, y que un gate de CI
lo impida.

## Cómo medir (no te fíes de `go test -cover` a secas)

`go test -coverpkg ./...` escribe **cada bloque una vez por test binary** (13
paquetes aquí, y en la práctica 22 copias). Sumar en crudo da ~10% y no significa
nada. Hay que deduplicar por rango y quedarse con el `count` máximo:

```bash
go clean -testcache && go test -coverpkg ./... -coverprofile=/tmp/cov.out ./...
# -> deduplicar por bloque, max(count); covered/total
```

Estado medido al abrir este documento: **96.67% (2697/2790), 85 bloques sin cubrir**.
Progreso: **98.18% (2745/2796), 45 sin cubrir**.

Meta: 2790/2790.

## Regla de este todolist

- Cada casilla se marca `[x]` **solo** cuando el bloque tiene `count>0` en un
  perfil deduplicado, no cuando el test pasa.
- Un test que pasa no prueba cobertura: `TestToastVacioNoSeEncola` falló
  primero por un caso mal entendido. Medir después de escribir.
- Si un bloque resulta inalcanzable (ver §inalcanzables), se documenta aquí con
  el porqué y se excluye del gate explícitamente. No se borra ni se maquilla.

---

## [x] Base de partida

- [x] Medir la cobertura real con el denominador deduplicado (era 94.9%, no 100)
- [x] Clasificar los 132 bloques: `count=0` real / bug de gremlins / sin bloque

## [x] `const` de paquete — no los instrumenta Go (8 mutantes)

- [x] `actionTimeout`, `commandTimeout`, `toastDuration` → funciones de una línea
- [x] `tool.DefaultTimeout` → función
- [x] `prMinBodyLines`, `prMinWidth` → funciones
- [x] `TestPlazosEnUnidades` afirma en unidades, no contra la fuente

## [x] Seam de handoff (el techo de la TUI)

- [x] Campo `handoff handoffFunc` en `Model`, default `tea.ExecProcess`
- [x] `handoffSpy` que no cede la terminal y expone `salir(err)`
- [x] Los 5 handoffs ejecutados enteros: editor, lazygit, shell, `p a`, visual

## [x] Ramas que existían y nadie miraba

- [x] `renderGroupSummary`: `errors` y `wt` (nunca activados)
- [x] `activityIndicator`: `case m.scanning` (`New` deja `scanning=true`)
- [x] `renderGroupSummary` en modo worktree

---

## Pendiente por fichero

### `internal/tui/update.go` — 13 → quedan 4

- [x] `31.23,34.16` — `spinner.TickMsg` (hay que mandarlo a mano: lo emite el spinner)
- [x] `221.2` — `return m, nil` para un msg desconocido
- [x] `421,432` — `!HasRepo` y sin fila en el comando `!` + `enter`
- [x] `592,596` — `busyActionCmd`: ocupado y libre
- [x] `635` — `toggleFold` sin fila (no-op, cursor quieto)
- [x] `770,783` — `visualPrompt`/`removePrompt`/`promptLine` sin armar
- [ ] `66.74,68.5` — `statusMsg.err` / `rebaseInProgress`
- [ ] `538.22` — `sendEvent` de un collect que sí terminó
- [ ] `711` — `vars["branch"]` de un worktree

### `internal/tui/app.go` — 22 → quedan 15

- [x] `200` — `visualOptionForKey` con tecla desconocida
- [x] `412` — `NotifyConfig` (el aviso tiene que verse, no irse a stderr)
- [x] `616` — `removeWorktreeCmd` con el padre ocupado
- [x] `647` — `recollectCmd` con el repo ocupado
- [x] `701` — `openLazygitCmd` con el repo ocupado (orden de las dos guardas)
- [x] `930` — `startVisualCmd` con el repo ocupado
- [x] `998` — `execExit` con un `*exec.ExitError` real (exit 3)
- [x] `1013` — `logIntent` con recorder apagado
- [x] `1066` — `saveCollapsed` con store nil
- [ ] `417` — `Init()` (arranca scan + ticker de 1s)
- [ ] `430` — `startScanCmd` con evento que no es `scanMsg`
- [ ] `446` — `tickCmd` (devuelve `tea.Tick`; no ejecutable sin reloj)
- [ ] `468` — `ctx.Err() != nil` en el colector
- [ ] `477` — `sendEvent(collectDoneMsg)`
- [ ] `487` — `!p.HasRepo` en el fetch automático
- [ ] `494` — repo ya en `fetchStates["fetching"]`
- [ ] `766` — worktree no encontrado
- [ ] `789` — marcador ilegible en `p a`
- [ ] `821` — `openPullAICmd`, camino feliz con el seam
- [ ] `835` — `pullAIArgv` con error
- [ ] `879` — `startVisualCmd`, camino feliz con git-sim en PATH

### `internal/tui/table.go` — 0 · `detail.go` — 0 · `sections.go` — 2

- [x] `table.go` — `styleFor` entero, glifo de header, `worktreeHidden` en
      `summary`, `repoExpanded` inducido por búsqueda, `(detached)` sin sha
- [x] `detail.go` — detached con rama, rama vacía, sin upstream, worktree en otra raíz
- [ ] `sections.go 128` — `onlyDirty` en la cabecera de stats
- [ ] `sections.go 289` — `[worktree]` en el título de la ficha

### `internal/tui/proverlay.go` — 0

- [x] `193` — `openPR` con `logOpen` (segunda red de la guarda)
- [x] `408` — `prParams()` con `m.pr == nil`
- [x] `492` — `draftLabel` con draft a true
- [x] `507` — `prPrompt()` con `m.pr == nil`

### `internal/tui/toast.go` — 3

- [ ] `183` — `head == ""` en el corte por anchura (salvaguarda de progreso)
- [ ] `206` — `len(lines) == 0` tras envolver
- [ ] `248` — bloque de altura 0 en el apilado

### `internal/tui/prcreate.go` — 2 · `cmdlogpanel.go` — 1 · `bordered.go` — 0

- [ ] `prcreate.go 62` — `sub == nil` al resolver el flag de base
- [ ] `prcreate.go 101` — `bin == "" || len(argv) == 0`
- [ ] `cmdlogpanel 176` — `cmdline` vacía → `-`
- [x] `bordered 90,93` — `leftChar`/`rightChar` vacíos

### `cmd/gitdash/` — 11 → quedan 4

- [x] `main.go 52,57,58` — `depsProd`: las cinco funciones no son nil y `load` lee de verdad
- [x] `main.go 65` — `run(printMode, eout)` con deps reales
- [x] `print.go 51,57,62,65,69` — worktree duplicado, sufijo `[wt]`, detached,
      rama vacía, contador de worktrees (se extrajo `printRowOf` para poder
      probarlos sobre filas fabricadas: un HEAD real no se puede dejar detached)
- [ ] `main.go 21` — `func main()` (llama a `os.Exit`; ver §inalcanzables)
- [ ] `print.go 29` — error de `discovery.Scan` a stderr

### `internal/discovery/scan.go` — 7 → quedan 5

- [x] `47,57` — roots inexistentes y que son un fichero (error agregado)
- [x] `85` — `WalkDir` sobre un directorio sin permiso (se salta, no aborta)
- [x] `173` — `MarkerPrompt` con marcador inexistente (vacío, no error)
- [x] `187` — `MarkerPrompt` con marcador malformado (sí error)
- [x] `222` — `.git` ilegible por permisos
- [ ] `100` — el walk que ABORTA (devuelve error, no solo salta)

Nota: los tests de permisos se saltan si el proceso corre como root, que no
puede impedirse leer un 0o000. Aquí no es root, pero en un contenedor de CI puede
darse: si se saltan, el bloque vuelve a estar sin cubrir sin que nadie se entere.

### `internal/gitstatus/status.go` — 2 · `state.go` — 2 · `cache.go` — 1

- [x] `status.go 140` — `syncBehind` a una ref inexistente → `known=false`
- [x] `status.go 176` — `StreamPool` con contexto ya cancelado
- [x] `status.go 333` — `FailureReason` sin salida / solo hints
- [ ] `status.go 263` — `rev-parse` devolviendo vacío
- [ ] `state.go 70` — `json.Marshal` fallando (mapa no serializable)
- [ ] `state.go 78` — `os.WriteFile` fallando
- [ ] `cache.go 106` — `json.MarshalIndent` fallando

### `internal/config/config.go` — 1 · `internal/forge/parse.go` — 1

- [ ] `config.go 417` — `label = key` **hecho con `t.Cleanup`, reverificar**
- [ ] `parse.go 70` — `RepoRef` con `Host` conocido y sin segmentos

---

## Inalcanzables (decidir antes del gate)

Estos no los cubre ningún test sin cambiar el código o la métrica:

**Nada de esto quedó inalcanzable.** Los tres casos que había dado por perdidos
salen, y la lección es que "no se puede testear" y "no lo he pensado" se parecen
mucho:

1. ~~`internal/testutil` — 8 bloques de `t.Fatal`.~~ **Resuelto, 8 → 1.** Los
   helpers tomato un `TB` (interface con `Helper`/`Fatal`/`Fatalf`/`TempDir`) en
   vez de `*testing.T`, y `MockTB` registra el fallo en vez de matar el proceso.
   `*testing.T` satisface el interface, así que las 160 llamadas del repo no
   cambian. El fallo se provoca DE VERDAD: un fichero donde debería ir el
   directorio da ENOTDIR.
2. ~~`func main()`.~~ **Resuelto** (ver `cmd/main.go` en el todolist).
3. ~~`tea.Tick`.~~ **Resuelto**: `tea.Tick` devuelve una `tea.Cmd` que se puede
   invocar directamente. Cuesta 1s por test y ejecuta el closure.

Lo que **sí** queda fuera del objetivo, y no por inalcanzable:

4. **Los 83 NOT COVERED de mutación.** No son cobertura: 36 son un bug del
   lookup de gremlins (ver `WATCHDOG-PLAN.md` §0) y 47 son condiciones de `case`,
   que Go instrumenta desde la columna del cuerpo. El refactor `switch`→`if` se
   probó y **no los baja**.

---

## El gate (último paso, no antes)

Falta escribirlo en `.github/workflows/ci.yml`. Solo tiene sentido cuando lo de
arriba esté hecho o excluido, porque si no falla en todos los PRs desde el
primer día.

- [ ] Objetivo del gate: ¿100% literal, o 100% con lista de exclusión visible?
- [ ] La lista de exclusión vive en el repo (un fichero, junto al allowlist de
      mutación), no en el workflow: si es una excepción tiene nombre y motivo.
- [ ] El gate **no baja**: si la cobertura sube, se sube el número del gate en el
      mismo commit. Un gate que solo sube es un gate que alguien puede relajar
      sin querer.
- [ ] El step resume la cobertura por paquete en el summary, como ya hace el de
      tests.