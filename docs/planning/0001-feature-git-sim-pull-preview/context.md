---
feature: 0001-feature-git-sim-pull-preview
freshness: 904c1fed0357058fc818a4593f30de7858e174ef
codegraph: ready
generated_by: codebase-researcher
---

# Context: Visual pull/merge/rebase preview with `git-sim`

## Scope
- In: the `visual` key (`v`) that arms a prefix-key selector (path + upstream captured), variants `p`/`m`/`r` → terminal handoff `git-sim --media-dir <cache>/gitdash/git-sim <sub> [ref]`; media-dir mandatory and created if missing; intent + exec in the command log; warning in keybinds via `promptLine()`.
- Out: git's `PullKinds`/`pullOptions`, `commands.pull*`, `[ai.pull]`, `runGit`/`runGitCombined`, the `pullArmed`/`removePrompt` state machine, git-sim's squash variant, parsing the generated image.

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---|---|---|---|
| `DefaultKeybindings()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config.go` | 197-222 | add `"visual": "v"` (default). `LoadFrom` validates against this map → an old config with a nonexistent action still only warns. |
| `hintLabels` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config.go` | 321-340 | add `"visual": "visual"` — label WITHOUT the key (`HintBarLines` prepends the key). |
| `HintBarLines()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config.go` | 345-380 | add `"visual"` to the action list (lines 350-354); its `switch` (365-372) sends it to `row3` (tools) through the `default`. |
| `visualOption` + `visualOptions` (p/m/r) single source | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app.go` | next to `pullOption`/`pullOptions` (116-138) | analogous to `pullOptions`; the prompt and the labels derive from here. It does NOT go into `PullKinds`. |
| `armedVisual{path, upstream string}` + `visualArmed *armedVisual` field in `Model` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app.go` | next to `armedPull` (164-169) and the `armed`/`pullArmed` fields (222-228) | ephemeral, like `pullArmed`; captures the upstream when arming. |
| `visualMediaDir()`, `visualArgv()`, `visualVariantLabel()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app.go` | next to `pullAIArgv`/`startPullAICmd` (677-732) | pure testable functions; exact argv `git-sim --media-dir <dir> <sub> [ref]`. |
| `startVisualCmd` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app.go` | model it on `openLazygitCmd` (605-619) and `startPullAICmd` (692-732) | running guard, `exec.LookPath("git-sim")`, `m.running[path]="visual"`, `tea.ExecProcess` → `execDoneMsg{action: kind, argv}`. |
| `if m.visualArmed != nil { ... }` block | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go` | insert after the `pullArmed` block (286-314) and BEFORE `if m.logOpen` (321) | consumes `p`/`m`/`r`; records the intent + `startVisualCmd`; `m`/`r` with no upstream → toast; any other key disarms and does NOT return. |
| `commandActions` and `rowActions` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go` | 546-561 | add `"visual": true` to both (precedent: `pull` is in the two). |
| `case "visual"` of the `switch action` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go` | inside the 428-506 switch (model it on `case "pull"` 449-458) | no row / no repo → toast `no git repo — nothing to do`; with a repo → arms `m.visualArmed = &armedVisual{path, upstream}` WITHOUT running. |
| `visualPrompt()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go` | model it on `pullPrompt()` (630-641) | warning derived from `visualOptions`. |
| `promptLine()` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/sections.go` | 67-77 | add `case m.visualArmed != nil: return m.visualPrompt()` (before `m.logOpen`). `keybindsLines`/`keepKeybinds`/`computeLayout` derive on their own — do not touch. |
| (new) `visual_selector_test.go` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/visual_selector_test.go` | new | tests for the selector + argv + media-dir + intent/exec. |
| (extend) hint/keybinding tests | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config_test.go` | ~156-179 (the `TestDefaultKeybindingsWorktreeRemove` pattern) | `visual` in `DefaultKeybindings` and the `v visual` hint. |

## Contracts (respect, do not modify)
- Pull selector: `pullOptions` (app.go:132-138), `PullKinds` (144-152), `IsPullKind` (155-162), the `pullArmed` block (update.go:286-314), `armedPull` (app.go:167-169). The visual path is **parallel**; do not add `visual_*` to `PullKinds`.
- `runGit` / `runGitCombined` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/gitstatus/status.go:283-316`): **git only**. `git-sim` does NOT go through here; it goes through the `tea.ExecProcess` handoff.
- `[ai.pull]`: `config.AICommand` (config.go:302-304), `BuildAIArgv`, `startPullAICmd` (app.go:692-732). Do not touch.
- `commands.pull*`: `DefaultCommands` (config.go:230-239). Do not touch.
- `hintLabels`/`HintBarLines`: the label goes WITHOUT the key; `HintBarLines` prepends it. Do not put the key in the label.
- `execDoneMsg` handler (update.go:149-167): reuse as is; it records the exec with `Dur=0` and re-collects. Do not create a new logging path.

## Pattern to Follow
- **Prefix-key armed selector** → the `pullArmed` block in `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/update.go:286-314`. Keys: (1) it disarms at the start (`armed := *m.pullArmed; m.pullArmed = nil`), (2) it consumes the variant before the normal routing, (3) any other key is NOT consumed (fall-through → esc/navigation carry on).
- **Terminal handoff** → `openLazygitCmd` (app.go:605-619) and `startPullAICmd` (app.go:692-732): `m.running[path]` guard, `exec.LookPath` → toast, `tea.ExecProcess` returning `execDoneMsg{path, action, argv, err}`. Re-collect through the existing handler.
- **Warning in keybinds** → `promptLine()`/`keybindsLines()` (`sections.go:55-77`) and `keybindsSection` (250-263): the warning REPLACES the hints; a single line.
- **Action/hint config** → `DefaultKeybindings` (config.go:197-222) + `hintLabels` (321-340) + `HintBarLines` (345-380). An exact mirror of the `worktree_remove` pair.

## Tests
- Framework/runner: `go test ./...` (nothing else). The repo's pattern: **direct Model** — build it with `newTestModel`, send `press`, inspect state; no teatest. See `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/app_test.go:1-137`.
- Helpers (same `tui` package):
  - `newTestModel(t, projects, states)` — app_test.go:19-31.
  - `press(m, key)` — app_test.go:102-117 (needs `Code`+`Text`).
  - `fixtureProjects()` — app_test.go:119-137 (`/tmp/old-clean` with upstream `origin/main`; `/tmp/no-up-cli` with no upstream; `/tmp/no-repo-docs` with no repo).
  - `cursorOn(t, m, path)` — pull_selector_test.go:21-31 (locates by path, not by index: attention-first ordering).
  - `snapClean`/`snapNoUpstream` — app_test.go:37-75.
  - `logModel(t)` — cmdlogpanel_test.go:23-29 (installs the recorder and cleans it up); `sectionContent`, `stripANSI`.
  - Real fixture with an upstream: `testutil.NewRepo(t, true)` / `testutil.AddUpstream` / `testutil.PushUpstreamCommits` / `testutil.FetchLocal` — `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/testutil/testutil.go:82-102,133-144,173-176`.
- Files to extend / add:
  - NEW `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/tui/visual_selector_test.go` (arming/cancel, no row, exact argv, media-dir, no upstream, no binary, uncreatable media-dir, intent+exec).
  - EXTEND `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/config/config_test.go` (default `visual` + `v visual` hint) and optionally `pull_policy_test.go`.
  - Style reference: `pull_selector_test.go` (arming/cancel) and `pull_ai_test.go:321-354` (`TestExecDoneRecordsPullAI`: simulates `execDoneMsg` and looks the entry up by action, with `Dur=0`).
- Infra: unit only. The real handoff + `xdg-open` is NOT testable in the suite → manual verification with tmux.
- Scenario→test mapping: arming (update block), argv (pure `visualArgv`), media-dir (`visualMediaDir` with `t.Setenv("XDG_CACHE_HOME", ...)`), no binary (PATH without git-sim + checking that `cmd()` is a `notifyMsg`), config/hints (config_test), warning (`promptLine`/`keybindsLines`).

## Conventions & Boundaries
- Comments in **English**, with no references to specs/IDs. Do not create `docs/planning/` in the repo: these artefacts are removed before the merge (known conflict, see plan.md:145-148).
- Direct model tests, no teatest (AGENTS.md).
- Derived states with precedence `diverged > dirty > ...`; `cursorOn` because the cursor is not the fixture's.
- When done: `go build ./... && go vet ./... && go test ./...` and `make install` (the repo's rule).
- Single source of variants: no hardcoded slices in the prompt/labels (the latent bug already seen in `pullPrompt`).

## Integration Points (non-obvious)
- **Media-dir**: `os.UserCacheDir()` honours `$XDG_CACHE_HOME` (otherwise `~/.cache`). The gitdash subdir can be composed with `cache.DirName` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/cache/cache.go:18`, = `"gitdash"`); `cache.Path()` returns the `repos.json` FILE, not the dir. Create the dir with `os.MkdirAll(dir, 0o755)`; if it fails → toast and NO handoff (launching would dirty the repo with `git-sim_media/`).
- **Upstream when arming**: it comes from `r.snap.Status.Upstream` / `HasUpstream` of the row returned by `m.selected()` (`update.go`/`app.go:913-926`); the row carries `snap gitstatus.Snapshot` (table.go:17-21). `gitstatus.Status.Upstream`/`HasUpstream` are parsed from `# branch.upstream` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-git-sim-pull-preview/internal/gitstatus/parse.go:14-15,145-146`). For a worktree with no snapshot, `worktreeRow` (app.go:932-943) brings no upstream → `m`/`r` with a toast.
- **Fall-through order in `handleKey`**: `armed` (267) → `pullArmed` (286) → **`visualArmed` (new, ~315)** → `logOpen` (321) → `cmdOpen` (327) → `searchActive` (358) → fixed keys (393) → `switch action` (428). Insert the visual block AFTER `pullArmed` and BEFORE `logOpen`.
- **`actionNeedsRow`/`launchesCommand`**: `logIntent` (app.go:839-853) only records with a row if `actionNeedsRow`. `pull` is in `commandActions` AND `rowActions` (update.go:546-561) → the precedent for `visual`.
- **`promptLine()` is the single place**: `keybindsLines` (sections.go:55-60), `keepKeybinds` and `computeLayout` derive from it. Adding a `case` is enough; do not touch the layout budgets.
- **`running` per path**: using the kind `"visual"` does not collide with `IsPullKind`/`runningActions` (sections.go:141-154 only lists pull/push/worktree_remove). `startVisualCmd`'s `m.running[path]` guard avoids overlapping with pull/lazygit.
- **Cancellation**: as in `pullArmed`, the block ALWAYS disarms on entry (assign nil before looking at the key); esc falls through to the normal routing (update.go:397-400) and does nothing. Do not consult `RebaseInProgress` (git-sim does not mutate the repo).

## Risks / Assumptions
- `git-sim` 0.3.5 assumed; a version with different flags (0.4.0) would break the argv. Check against the real binary.
- `--media-dir` omitted on some path dirties the repo (`git-sim_media/`): cover it with an exact-argv test + a manual `git status`.
- Handoff + `xdg-open` is not testable in the suite → manual verification with tmux (`tmux capture-pane`) and the command log.
- The double intent (arming + variant) is intentional and has a precedent in `pull`; it is not a bug.
- AGENTS.md forbids SDD artefacts in the repo; remove `docs/planning/` before the merge.