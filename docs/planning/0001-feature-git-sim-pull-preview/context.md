---
feature: 0001-feature-git-sim-pull-preview
freshness: 904c1fed0357058fc818a4593f30de7858e174ef
codegraph: ready
generated_by: codebase-researcher
---

# Context: Preview visual de pull/merge/rebase con `git-sim`

## Scope
- In: tecla `visual` (`v`) que arma un selector prefix-key (path + upstream capturados), variantes `p`/`m`/`r` → handoff de terminal `git-sim --media-dir <caché>/gitdash/git-sim <sub> [ref]`; media-dir obligatorio y creado si falta; intención + exec en el command log; aviso en keybinds vía `promptLine()`.
- Out: `PullKinds`/`pullOptions` de git, `commands.pull*`, `[ai.pull]`, `runGit`/`runGitCombined`, la máquina de `pullArmed`/`removePrompt`, la variante squash de git-sim, parseo de la imagen generada.

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---|---|---|---|
| `DefaultKeybindings()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config.go` | 197-222 | añadir `"visual": "v"` (default). `LoadFrom` valida contra este mapa → config vieja con acción inexistente sigue avisando solo. |
| `hintLabels` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config.go` | 321-340 | añadir `"visual": "visual"` — etiqueta SIN la tecla (`HintBarLines` antepone la tecla). |
| `HintBarLines()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config.go` | 345-380 | añadir `"visual"` a la lista de acciones (línea 350-354); su `switch` (365-372) lo manda a `row3` (tools) por el `default`. |
| `visualOption` + `visualOptions` (p/m/r) fuente única | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app.go` | junto a `pullOption`/`pullOptions` (116-138) | análogo a `pullOptions`; derivan de aquí prompt y etiquetas. NO entra en `PullKinds`. |
| `armedVisual{path, upstream string}` + campo `visualArmed *armedVisual` en `Model` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app.go` | junto a `armedPull` (164-169) y campos `armed`/`pullArmed` (222-228) | efímero, como `pullArmed`; captura upstream al armar. |
| `visualMediaDir()`, `visualArgv()`, `visualVariantLabel()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app.go` | junto a `pullAIArgv`/`startPullAICmd` (677-732) | funciones puras testeables; argv exacto `git-sim --media-dir <dir> <sub> [ref]`. |
| `startVisualCmd` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app.go` | modelar sobre `openLazygitCmd` (605-619) y `startPullAICmd` (692-732) | guard running, `exec.LookPath("git-sim")`, `m.running[path]="visual"`, `tea.ExecProcess` → `execDoneMsg{action: kind, argv}`. |
| bloque `if m.visualArmed != nil { ... }` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go` | insertar tras el bloque `pullArmed` (286-314) y ANTES de `if m.logOpen` (321) | consume `p`/`m`/`r`; registra intención + `startVisualCmd`; `m`/`r` sin upstream → toast; otra tecla desarma y NO retorna. |
| `commandActions` y `rowActions` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go` | 546-561 | añadir `"visual": true` a ambos (precedente: `pull` está en los dos). |
| case `"visual"` del `switch action` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go` | dentro del switch 428-506 (modelar sobre `case "pull"` 449-458) | sin fila / sin repo → toast `no git repo — nothing to do`; con repo → arma `m.visualArmed = &armedVisual{path, upstream}` SIN ejecutar. |
| `visualPrompt()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go` | modelar sobre `pullPrompt()` (630-641) | aviso derivado de `visualOptions`. |
| `promptLine()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/sections.go` | 67-77 | añadir `case m.visualArmed != nil: return m.visualPrompt()` (antes de `m.logOpen`). `keybindsLines`/`keepKeybinds`/`computeLayout` derivan solos — no tocar. |
| (nuevo) `visual_selector_test.go` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/visual_selector_test.go` | nuevo | tests del selector + argv + media-dir + intención/exec. |
| (extender) tests de hints/keybind | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config_test.go` | ~156-179 (patrón `TestDefaultKeybindingsWorktreeRemove`) | `visual` en `DefaultKeybindings` y hint `v visual`. |

## Contracts (respetar, no modificar)
- Selector de pull: `pullOptions` (app.go:132-138), `PullKinds` (144-152), `IsPullKind` (155-162), bloque `pullArmed` (update.go:286-314), `armedPull` (app.go:167-169). El camino visual es **paralelo**; no añadir `visual_*` a `PullKinds`.
- `runGit` / `runGitCombined` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/gitstatus/status.go:283-316`): **solo git**. `git-sim` NO pasa por aquí; va por handoff `tea.ExecProcess`.
- `[ai.pull]`: `config.AICommand` (config.go:302-304), `BuildAIArgv`, `startPullAICmd` (app.go:692-732). No tocar.
- `commands.pull*`: `DefaultCommands` (config.go:230-239). No tocar.
- `hintLabels`/`HintBarLines`: la etiqueta va SIN la tecla; `HintBarLines` la antepone. No meter la tecla en la etiqueta.
- `execDoneMsg` handler (update.go:149-167): reutilizar tal cual; registra exec con `Dur=0` y re-colecta. No crear camino de logging nuevo.

## Pattern to Follow
- **Prefix-key armed selector** → bloque `pullArmed` en `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go:286-314`. Claves: (1) desarma al inicio (`armed := *m.pullArmed; m.pullArmed = nil`), (2) consume la variante antes del enrutado normal, (3) cualquier otra tecla NO se consume (fall-through → esc/navegación siguen).
- **Handoff de terminal** → `openLazygitCmd` (app.go:605-619) y `startPullAICmd` (app.go:692-732): guard `m.running[path]`, `exec.LookPath` → toast, `tea.ExecProcess` devolviendo `execDoneMsg{path, action, argv, err}`. Recolecta vía handler existente.
- **Aviso en keybinds** → `promptLine()`/`keybindsLines()` (`sections.go:55-77`) y `keybindsSection` (250-263): el aviso SUSTITUYE las hints; una sola línea.
- **Config acción/hint** → `DefaultKeybindings` (config.go:197-222) + `hintLabels` (321-340) + `HintBarLines` (345-380). Mirror exacto del par `worktree_remove`.

## Tests
- Framework/runner: `go test ./...` (nada más). Patrón del repo: **Model directo** — construir con `newTestModel`, enviar `press`, inspeccionar estado; sin teatest. Ver `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app_test.go:1-137`.
- Helpers (mismos paquete `tui`):
  - `newTestModel(t, projects, states)` — app_test.go:19-31.
  - `press(m, key)` — app_test.go:102-117 (necesita `Code`+`Text`).
  - `fixtureProjects()` — app_test.go:119-137 (`/tmp/old-clean` con upstream `origin/main`; `/tmp/no-up-cli` sin upstream; `/tmp/no-repo-docs` sin repo).
  - `cursorOn(t, m, path)` — pull_selector_test.go:21-31 (localiza por path, no por índice: orden attention-first).
  - `snapClean`/`snapNoUpstream` — app_test.go:37-75.
  - `logModel(t)` — cmdlogpanel_test.go:23-29 (instala recorder y lo limpia); `sectionContent`, `stripANSI`.
  - Fixture real con upstream: `testutil.NewRepo(t, true)` / `testutil.AddUpstream` / `testutil.PushUpstreamCommits` / `testutil.FetchLocal` — `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/testutil/testutil.go:82-102,133-144,173-176`.
- Archivos a extender / añadir:
  - NUEVO `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/visual_selector_test.go` (armado/cancelación, sin fila, argv exacto, media-dir, sin upstream, sin binario, media-dir no creable, intención+exec).
  - EXTENDER `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config_test.go` (default `visual` + hint `v visual`) y opcionalmente `pull_policy_test.go`.
  - Referencia de estilo: `pull_selector_test.go` (armado/cancelación) y `pull_ai_test.go:321-354` (`TestExecDoneRegistraPullAI`: simula `execDoneMsg` y busca la entry por acción, con `Dur=0`).
- Infra: unit only. El handoff real + `xdg-open` NO es testeable en suite → verificación manual con tmux.
- Mapeo escenario→test: armado (update block), argv (`visualArgv` puro), media-dir (`visualMediaDir` con `t.Setenv("XDG_CACHE_HOME", ...)`), sin binario (PATH sin git-sim + comprobar `cmd()` es `notifyMsg`), config/hints (config_test), aviso (`promptLine`/`keybindsLines`).

## Conventions & Boundaries
- Comentarios en **español**, sin referencias a specs/IDs. No crear `docs/planning/` en el repo: estos artefactos se retiran antes del merge (conflicto conocido, ver plan.md:145-148).
- Tests del modelo directo, sin teatest (AGENTS.md).
- Estados derivados con precedencia `diverged > dirty > ...`; `cursorOn` porque el cursor no es el del fixture.
- Al terminar: `go build ./... && go vet ./... && go test ./...` y `make install` (regla del repo).
- Fuente única de variantes: nada de slices hardcodeados en el prompt/etiquetas (bug latente ya visto en `pullPrompt`).

## Integration Points (non-obvious)
- **Media-dir**: `os.UserCacheDir()` respeta `$XDG_CACHE_HOME` (si no, `~/.cache`). El subdir gitdash se puede componer con `cache.DirName` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/cache/cache.go:18`, = `"gitdash"`); `cache.Path()` devuelve el FICHERO `repos.json`, no el dir. Crear el dir con `os.MkdirAll(dir, 0o755)`; si falla → toast y NO handoff (lanzar ensuciaría el repo con `git-sim_media/`).
- **Upstream al armar**: sale de `r.snap.Status.Upstream` / `HasUpstream` del `row` devuelto por `m.selected()` (`update.go`/`app.go:913-926`); la fila lleva `snap gitstatus.Snapshot` (table.go:17-21). `gitstatus.Status.Upstream`/`HasUpstream` se parsean de `# branch.upstream` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/gitstatus/parse.go:14-15,145-146`). Para worktree sin snapshot, `worktreeRow` (app.go:932-943) no trae upstream → `m`/`r` con toast.
- **Orden de fall-through en `handleKey`**: `armed` (267) → `pullArmed` (286) → **`visualArmed` (nuevo, ~315)** → `logOpen` (321) → `cmdOpen` (327) → `searchActive` (358) → teclas fijas (393) → `switch action` (428). Insertar el bloque visual DESPUÉS de `pullArmed` y ANTES de `logOpen`.
- **`actionNeedsRow`/`launchesCommand`**: `logIntent` (app.go:839-853) solo registra con fila si `actionNeedsRow`. `pull` está en `commandActions` Y `rowActions` (update.go:546-561) → precedente para `visual`.
- **`promptLine()` es punto único**: `keybindsLines` (sections.go:55-60), `keepKeybinds` y `computeLayout` derivan de él. Añadir un `case` basta; no tocar presupuestos de layout.
- **`running` por path**: usando kind `"visual"` no colisiona con `IsPullKind`/`runningActions` (sections.go:141-154 solo lista pull/push/worktree_remove). El guard `m.running[path]` de `startVisualCmd` evita solapar con pull/lazygit.
- **Cancelación**: como en `pullArmed`, el bloque desarma SIEMPRE al entrar (asignar nil antes de mirar la tecla); esc cae al enrutado normal (update.go:397-400) y no hace nada. No consultar `RebaseInProgress` (git-sim no muta el repo).

## Risks / Assumptions
- `git-sim` 0.3.5 asumido; una versión con flags distintos (0.4.0) rompería el argv. Verificar contra el binario real.
- `--media-dir` omitido en algún camino ensucia el repo (`git-sim_media/`): cubrir con test de argv exacto + `git status` manual.
- Handoff + `xdg-open` no testeable en suite → verificación manual con tmux (`tmux capture-pane`) y command log.
- Doble intención (armado + variante) es intencional y tiene precedente en `pull`; no es bug.
- AGENTS.md prohíbe artefactos SDD en el repo; retirar `docs/planning/` antes del merge.
