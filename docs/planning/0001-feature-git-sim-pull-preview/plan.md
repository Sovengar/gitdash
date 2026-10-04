# Plan — Visual pull preview with `git-sim`

adr_required: false
(There is no architectural decision with heavy tradeoffs nor a breaking change: the
feature is additive and reuses three already-proven rails — the prefix-key
selector, the terminal handoff and the command log. The decisions taken are
documented below.)

## Wanted outcome

With the cursor on a repo, `v` arms a selector and the second key opens a visual
`git-sim` preview (`pull` / `merge <upstream>` / `rebase <upstream>`), handing the
terminal to the child. git-sim draws without touching the real repo. On return,
gitdash records the exec with its real argv and re-collects.

## Approach (high level)

A **parallel path** to `p`/`pullArmed`: the same mechanism (arm with the first key,
consume the variant before the normal routing, cancel without consuming the
rest), the same warning painting (`promptLine()`), the same handoff
(`execDoneMsg`). `PullKinds`, `commands.pull*` and `[ai.pull]` are not touched.

Key design points:

- **Single source of visual variants**, analogous to `pullOptions`, with p/m/r.
  The prompt and the labels derive from it. No hardcoded slices.
- **The media-dir is mandatory**, not cosmetic: without it git-sim writes
  `git-sim_media/` inside the repo and gitdash would mark it dirty (it uses real
  `git status`). It has to be created if missing; if it cannot be created, it
  aborts with a toast instead of launching (launching would dirty the repo).
- **Auto-open active**: `-d` is not passed. No `--animate`. No
  `--output-only-path` (it would mean capturing stdout, and the handoff does not
  capture; the image's path is non-deterministic due to the timestamp and is not
  needed — the log's argv is enough to know what ran).
- **`git-sim` is NOT git**: it is resolved through PATH and goes through the
  handoff; it does not go through `runGit`/`runGitCombined`.
- **A rebase in progress does NOT block** the selector: git-sim does not mutate
  the real repo (the network operations run in a temporary clone). Previewing a
  half-done rebase is precisely the useful thing. Consistent with the AI variant,
  which does not consult `RebaseInProgress` either.

Documented secondary decisions:

- Visual `p` is allowed with **no upstream** (mirror of gitdash's plain `p`, which
  delegates to git; git-sim pull simulates and does not require a ref). `m`/`r` do
  require an upstream (they need the explicit `<upstream-ref>`): without it, a
  toast and nothing is launched.
- Arming **captures path + upstream** when `v` is pressed (the upstream comes from
  `Snapshot.Status.Upstream`, e.g. `origin/main`), so that the argv is
  deterministic with respect to the chosen row.
- The cache dir comes from `os.UserCacheDir()` + `gitdash/git-sim`; if that fails,
  it degrades with a toast (never to the repo).

## Steps and the files each one touches

1. **Config** — `internal/config/config.go`
   - `DefaultKeybindings()`: add `"visual": "v"`.
   - `hintLabels`: add `"visual": "visual"` (label WITHOUT the key).
   - `HintBarLines()`: add `"visual"` to the action list; it lands on the tools
     row (`default`) through the current `switch`.
   - `LoadFrom`'s stale-action validation keeps working on its own (it validates
     against `DefaultKeybindings()`).

2. **Model and variants** — `internal/tui/app.go`
   - `visualOption{key, kind, label, needsUpstream}` type + `visualOptions` slice
     (p/m/r) as the single source.
   - `armedVisual{path, upstream string}` type and `visualArmed *armedVisual` field
     in `Model` (ephemeral, like `pullArmed`).
   - `visualOptions` does NOT go into `PullKinds`.
   - Pure, testable functions: `visualMediaDir()` (cache + dir creation),
     `visualArgv(kind, upstream, mediaDir) []string` (composes
     `git-sim --media-dir <dir> <sub> [upstream]`), `visualVariantLabel(kind)`.
   - `startVisualCmd(path, kind, argv)`: "an action is already running" guard,
     `exec.LookPath("git-sim")` (missing → toast), `m.running[path]="visual"`,
     `tea.ExecProcess` returning `execDoneMsg{action: kind, argv}`.
   - Reuses the existing `execDoneMsg` handler: logging with `Dur=0` and
     re-collect (no new path is invented).

3. **Routing and state machine** — `internal/tui/update.go`
   - New `if m.visualArmed != nil { ... }` block right **after** the `pullArmed`
     block and **before** the log panel and the normal routing:
     - consumes `p`/`m`/`r` as variants;
     - records the INTENT (`cmdlog.RecordIntent` with key + variant + repo) and
       launches `startVisualCmd`;
     - for `m`/`r` with no upstream → "no upstream" toast and no launch;
     - any other key disarms and does NOT return (it carries on normally).
   - Add `"visual"` to `commandActions` (leaves an intent) and to `rowActions`
     (needs a row).
   - In the `switch action`, the `"visual"` case: no row or not a repo → toast;
     with a repo → `m.visualArmed = &armedVisual{path, upstream}` (without
     running).
   - `visualPrompt()`: warning derived from `visualOptions`, analogous to
     `pullPrompt()`.

4. **Painting the warning** — `internal/tui/sections.go`
   - `promptLine()`: add `case m.visualArmed != nil: return m.visualPrompt()`
     (the armed states have priority over the log's legend; only one armed at a
     time because the second press disarms the previous one).
   - `keybindsLines()`/`keepKeybinds`/`computeLayout` derive from `promptLine()` on
     their own: they need no changes.

5. **Tests** — `internal/tui/` (+ `internal/config/`)
   - `visual_selector_test.go` (new), direct pattern: build the Model, `press`,
     inspect state (no teatest).

## Mandatory test cases

- **Selector arming/cancel**: `v` arms without launching (`running` untouched); a
  non-variant key disarms and carries on; `esc` disarms.
- **No row / row with no repo**: `v` on a header or a project with no repo → toast
  and no arming.
- **Variant by variant with the exact argv**: pure `visualArgv` →
  `["git-sim","--media-dir",<dir>,"pull"]`,
  `[...,"merge","origin/main"]`, `[...,"rebase","origin/main"]`.
- **Media-dir present** in every variant and pointing at the cache, NOT the repo;
  the dir is created if missing.
- **No upstream**: `m`/`r` → "no upstream" toast and nothing launched; `p` is
  allowed.
- **Binary missing**: `exec.LookPath("git-sim")` fails → "git-sim not installed"
  toast and no handoff (check the returned `tea.Cmd`).
- **Uncreatable media-dir** → error toast, no handoff.
- **Config/hints**: `visual` in `DefaultKeybindings`; the "v visual" hint (label
  without key); `HintBarLines` includes the row.
- **Warning**: `promptLine()` with `visualArmed` set; `keybindsLines()==1`.
- **Intent + exec**: picking a variant records the intent; on a simulated
  `execDoneMsg` the exec is recorded with the real argv (including `--media-dir`
  and the ref) and `Dur=0`.

Fixture with an upstream: use `pull_selector_test.go`'s pattern + `testutil`
(`AddUpstream`/`PushUpstreamCommits`/`FetchLocal`). For the "no upstream" case,
reuse a no-upstream fixture.

## Risks and how to verify them

- **`git-sim` not installed on the user's machine** → only a toast; no handoff.
  Verifiable by hand with a PATH without git-sim.
- **`--media-dir` omitted on some path** → the repo gets dirtied with
  `git-sim_media/`. Verification: a `visualArgv` test (exact argv) + manual with
  `git status` after a preview.
- **git-sim's version changes flags** (0.4.0 in the overhaul uses skia). Check
  against real git-sim 0.3.5; note that the plan assumes 0.3.5.
- **Terminal handoff + auto-open** (xdg-open) is not testable in the suite: manual
  verification with tmux (`tmux capture-pane`) and checking that the image opens
  and that the command log records the argv.
- **Overlap with the armed states** (`visual` vs `pull` vs `remove`): only one
  armed at a time; the arming/cancel tests cover it.
- **AGENTS.md forbids SDD artefacts in the repo** (`docs/planning/`) while the
  orchestrator insists on depositing the planning package in `planning_dir`.
  Known conflict: these artefacts must be removed before the merge (or the PR
  excludes them), because the repo's convention does not allow `docs/planning/`.

## Work order (rough)

1. config (action/keybind/hint) → 2. model + pure argv + handoff (app.go) →
3. routing + arming + intent (update.go) → 4. warning in promptLine
(sections.go) → 5. tests → 6. build/vet/test + `make install` and manual
verification with tmux.