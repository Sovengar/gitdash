---
feature: 0002-feature-pr-overlay-ux
freshness: 359998a
codegraph: ready
generated_by: codebase-researcher
---

# Context: PR/MR form as a floating modal with searchable branch pickers

## Scope
- In: `O` form floats over the still-visible dashboard (ANSI splice, no layout budget); base+head become type-to-filter pickers fed by a new bounded on-demand `for-each-ref` read (`ClassRead`, through `runGit`); body gets `ctrl+t` skeleton; five-field tab order; resize re-fits/closes.
- Out: submit/execution path, forge, config, discovery, `Collect` scan, terminal handoff, dimming, popup picker, per-picker esc, refresh key, output cap, remote-as-head, structured body.

## Files to Touch

| Symbol / Area | File | Lines | Why |
|---|---|---|---|
| `prField` enum + `prFieldCount` (4→5) | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay.go` | 35-49 | Add `prFieldHead` between base and draft; `next()` wrap-around gains a field |
| `prDraft` struct | same | 52-64 | `head string` → `headIn textinput.Model` (editable); add picker state (refs/filter/highlight/err/loading) ×2 |
| `newPROverlay` | same | 72-96 | Pre-fill `headIn` with the row's branch (like `baseIn` at :82-85) |
| `openPR` | same | 104-141 | After `m.pr = newPROverlay(...)`, launch the async ref read (goroutine + `sendEvent`, pattern `recollectCmd`); keep the fit-refusal toast |
| `prFits` | same | 144-146 | **Rewire**: currently `m.layout().bodyLines >= prMinBodyLines()`; after layout migration the fit check is the modal's derived minimum vs `m.height`/`m.width` |
| `prValueWidth` / `prFit` | same | 149-165 | Size widgets from the modal's derived width/height, not `m.layout().bodyLines` |
| `closePR` | same | 168-175 | Blur the new `headIn` widget too |
| `prFocus` | same | 177-196 | Add `prFieldHead` case (focus `headIn`) |
| `handlePRKey` | same | 199-239 | Add `prFieldHead` case; intercept up/down/enter for branch pickers (highlight/selection) BEFORE forwarding runes/backspace to the textinput; add `ctrl+t` skeleton intercept for body |
| `prSubmit` / `prParams` | same | 242-269 | `Head` now reads `m.pr.headIn.Value()` (trimmed), not `m.pr.head` |
| `prSection` | same | 271-285 | **Rewire**: content-sized (no `rows` clip); render head as a field line with `headIn.View()`; render the focused branch field's picker pane under it; width = modal's derived width, not `m.width` |
| `prMinBodyLines` / `prMinWidth` | same | 29-32 | Evolve into the modal's derived minimum height/width functions (parts, not constants) |
| `prPrompt` | same | 326-332 | Update legend text (head is now editable); keep `cfg.KeyFor("pr")` + `ctrl+s` + `esc` |
| `prFieldCount`/`prLabelFor` (test helper) | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay_test.go` | 72, 419-431 | `prLabelFor` gains a `prFieldHead` case |
| `Update` WindowSizeMsg | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go` | 17-25 | `prFits` rewire lands here; `prFit()` re-centers |
| new `prRefsMsg` case | same | ~172 (near `prStartMsg`) | Handle the async ref result: `withPump`, path guard (`m.pr != nil && m.pr.path == msg.path`), store refs in draft |
| `handleKey` `m.pr != nil` guard | same | 254-258 | Stays (modal consulted first) |
| `handleKey` logOpen `pr` guard | same | 391-396 | Stays |
| `View` | same | 658-664 | Compose: `overlayToasts(spliceModal(renderDashboard(), modal), toasts, ...)` — modal first, toasts last |
| `renderDashboard` | same | 667-682 | **Delete** the `m.pr != nil` branch (:672-674) — the dashboard paints normally behind the modal |
| `toastReserve` | same | 684-690 | Unchanged (layout no longer has formMin, so reserve is identical with/without modal) |
| `layout()` formMin + freed branch | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/sections.go` | 21-36 | **Delete** `formMin` (:22-26); reduce `if m.logOpen \|\| m.pr != nil` to `if m.logOpen` (:28) |
| `computeLayout` signature | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/layout.go` | 56-78 | **Delete** the `formMin` parameter and the `formMin <= 0` guard (:65-74); panel search always runs |
| `layoutTest` helper | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/sections_test.go` | 16-18 | Drop the `0` arg |
| direct `computeLayout` call | same | 963 | Drop the `0` arg |
| `overlayToasts` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/toast.go` | 198-231 | Extract a line-splice helper; add a centered variant for the modal. **Keep `overlayToasts` signature stable** so the 12 toast tests don't break |
| `Model.pr` / `Model.prPending` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/app.go` | 232-234 | Unchanged fields; add the new `prRefsMsg` type near `prStartMsg` |
| `recollectCmd` (pattern) | same | 565-577 | The async-read goroutine template (call + `sendEvent`, conditional-free) |
| `pullOptions` (selector-struct pattern) | same | 99-105 | Model for the picker's single-source-of-list |
| `prCreateCmd` / `prStartMsg` / `prResultMsg` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/prcreate.go` | 20-88 | **No change** — `forge.Params` already carries Base/Head; submit seam stays |
| `sanitizeLogText` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/cmdlogpanel.go` | 160-188 | Reuse for untrusted ref names in the picker pane |
| `logEntries` ClassRead filter | same | 57-73 | The ref read is `ClassRead` → hidden by default in the panel; test pins via `rec.Entries()`, not the painted panel |
| `runGit` / `runGitCombined` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/gitstatus/status.go` | 299-330 | The only exec paths; the ref read leaves through `runGit` with `ClassRead` |
| `RemoteURL` (pattern precedent) | same | 217-223 | The on-demand read template (trim, error propagation) |
| `recordExec` | same | 333-361 | Auto-records the read; no extra wiring |
| new `BranchRefs` read | same (or new `refs.go`) | near `RemoteURL` | `for-each-ref --format=%(refname) refs/heads refs/remotes` via `runGit(ctx, dir, cmdlog.ClassRead, ...)` |
| `ParsePorcelain` (pure-parser pattern) | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/gitstatus/parse.go` | 115-162 | Model for `ParseRefs` (pure, canned-output testable) |
| new `ParseRefs` | same | near `ParseWorktrees` (:214) | Strip `refs/heads/`→local, `refs/remotes/`→remote, drop `*/HEAD` symrefs |
| `Params` / `BuildCreateArgv` | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/forge/create.go` | 6-13, 43-96 | **No change** — `-B`/`-H` (gh) and `-b`/`-s` (glab) already emitted; do not break |
| `docs/FEATURES.md` `O` entry | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/docs/FEATURES.md` | 79-84 (quote :81) | Update in the same change |
| new ADR | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/docs/adr/0002-modal-overlay-composition.md` | new | Style template: `docs/adr/0001-layout-budget-ownership.md` |

## Contracts
- `runGit(ctx, dir, class, args...)` / `runGitCombined(...)` — the ONLY git exec paths; the ref read leaves through `runGit` with `cmdlog.ClassRead` and is auto-recorded by `recordExec` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/gitstatus/status.go:299-361`).
- Event pump: every channel event must rearm `waitForEvent` via `withPump` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go:198-200`); the `prRefsMsg` handler must use it.
- `handleKey` precedence: modal (`m.pr != nil`) → armed removals → pullArmed → visualArmed → logOpen → cmdOpen → search → logOpen action guard → normal routing (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go:250-520`).
- `promptLine()` is the single keybinds-warning source (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/sections.go:74-89`); `keybindsLines()` returns 1 when it is non-empty (:67-72).
- `input is the only source of truth`: `prParams` reads widget `Value()`s, never a parallel string (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay.go:98-101, 258-269`).
- Bordered exact-width: `bordered.contentLines` pads every line to `innerWidth` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/bordered/bordered.go:86-88`); the modal's lines are exactly `modalWidth` wide, so the splice preserves the dashboard's `m.width`.
- `sanitizeLogText` for untrusted text (ref names, marker prompts) — `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/cmdlogpanel.go:160-188`.
- pad-before-style: `pad(text)` BEFORE `style.Render` (ANSI breaks width) — `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay.go:310-316`.
- `forge.Params{Title,Body,Base,Head,Draft}` → `BuildCreateArgv` → `-B`/`-H` (gh), `-b`/`-s` (glab) — `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/forge/create.go:55-96`; the submit seam `prPending → prStartMsg → prCreateCmd` must stay pinned.

## Pattern to Follow
- Async read → `RemoteURL` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/gitstatus/status.go:217-223`) + `recollectCmd` goroutine (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/app.go:565-577`).
- Per-repo-running guard → `m.running[path]` + `busyActionCmd` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/app.go:529-534`).
- View-mode key capture → `handleLogKey` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/cmdlogpanel.go:278-299`).
- Scroll/offset windowing → `syncOffset` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go:692-700`) + `logScroll` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/cmdlogpanel.go:273-276`).
- Fixed window with `… N more` → `filesList` + `listBudget` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/sections.go:318-330`).
- Derived minima as functions → `prMinBodyLines` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay.go:29`).
- Selector structs with a single source of list → `pullOptions` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/app.go:99-105`).
- Event msg + `withPump` → `recollect` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go:44-48`).
- Pure parser with canned output → `ParsePorcelain` / `ParseWorktrees` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/gitstatus/parse.go:115, 214`).

## Tests

### Existing affected (re-pin)
- `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay_test.go`: `TestPROverlayTabWalksTheFields` (:433, 4-field order), `TestPROverlayHighlightsTheLabelOfTheFieldWithTheFocus` (:408), `TestPROverlayIsFitsInTheTerminal` (:644, table NOT painted → now painted behind), `TestPROverlayPrSectionOnlyFromTheHeightMinimum` (:798, `prSection` rows clip → content-sized), `TestPROverlayTheWidgetsIsSizeOnTheGap` (:818, sizes from `m.layout().bodyLines` → modal-derived), `TestPROverlayResizeInsufficientCloses` (:843) / `TestPROverlayResizeEnoughNotCloses` (:856), `TestPROverlayOpensFairInTheHeightMinimum` (:761) / `TestPROverlayOnlyOpensInTheBorderExact` (:902) / `TestPROverlayTheMinimumOfHeightNotIsAFloorOfParty` (:959), `TestPROverlayNotOpensInTerminalSmall` (:697) / `TestPROverlayNotOpensInTerminalNarrow` (:718), `TestPROverlayTheBudgetIsTheOneTheCommentStates` (:981, `prFixedLines` 4→5 fields + notice + body label), `TestPROverlayCapturesTheRowOnTheArm` (:107, `m.pr.head` → `m.pr.headIn.Value()`), `TestPROverlayCloseDropsTheFocusOfTheFields` (:603, add head), `TestPROverlayMarksTheFieldWithTheFocus` (:452), `highlightedLabels` (:389, label list already includes "head").
- `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/restructure_test.go`: `TestPROverlayReplacesTheWholeBody` (:765, form replaces body → now floats over it).
- `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/prcreate_test.go`: `TestPRCreationCompletesRecordsTheArgvResolved` (:140, `-B`/`-H` argv — add a picker-chosen-branch case), `TestPRTheHeadExitsOfTheSnapshot` (:246, head now from picker).
- `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/toast_test.go`: the 12 `overlayToasts` tests (:98-481) — keep `overlayToasts` signature stable.
- `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/sections_test.go`: `layoutTest` (:16) and `computeLayout` (:963) — drop the `formMin` arg.
- `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/gitstatus/cmdlog_exec_test.go`: `TestRemoteURLExitsForRunGitAndStaysInTheLog` (:139) is the template for the ref-read test; `installRecorder` (:14).

### New tests (one line per scenario/group)
- Picker pure helpers (filter case-insensitive contains, highlight clamp, scroll window, pane state) → `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/picker_test.go` (new).
- Ref read: one subprocess through `runGit`, `ClassRead`, in `rec.Entries()`; parse local+remote, drop `*/HEAD`, deterministic order → `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/gitstatus/refs_test.go` (new) or `parse_test.go`.
- Modal splice: wider/taller/exact width+height, ANSI-safe, dashboard preserved on both sides → `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay_test.go` (or new `modal_test.go`).
- Modal fit boundary: exact minimum opens, one below refuses; resize re-fit (text preserved) and close-with-warning; stale-result drop (no modal, wrong path) and accept → `proverlay_test.go`.
- Picker interactions: type-to-filter narrows + highlight to first match; arrows move + clamp; enter commits highlighted (filter cleared, full list back); verbatim fallback on no match; head excludes remotes; loading/empty/error pane states → `proverlay_test.go` + `picker_test.go`.
- Tab order: five fields wrap, leaving a branch field clears the filter → `proverlay_test.go`.
- Body: `ctrl+t` inserts Summary/Test-plan skeleton at cursor, existing text preserved, body reaches argv verbatim (newlines) → `proverlay_test.go`.
- Submit: base/head flags carry picker-chosen values → `prcreate_test.go`.

### Framework / runner
- `go test ./...` (direct model tests, no teatest); `make coverage-check` (diff at 100%, total floor); `make mutate-all` / `make mutate-all-diff` (mutation gate).
- Integration infra: unit only — fixtures via `internal/testutil` (`NewRepo`, `PushUpstreamCommits`, `FetchLocal`, `NewBranch`, `Checkout`); forge via `forgeStub` (PATH stub); git via real repos in `t.TempDir()`.
- Synthetic-key gotcha: `tea.KeyPressMsg` needs `Code` AND `Text` for runes (see `press` `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/app_test.go:123-130` and `namedKeys` :109-121); modifiers need `Mod` (e.g. `ctrl+t` = `{Code: 't', Mod: tea.ModCtrl}`).
- Mutation diet: no package const expressions (use functions), pure helpers for filter/clamp/scroll, conditional-free goroutine (call + `sendError`).

## Conventions & Boundaries
- Comments: English, WHY-only, one line; no HOW, no godoc repeating the name.
- No SDD artefacts / no requirement or scenario IDs in code.
- `docs/FEATURES.md` must be updated in the same change (user-visible `O` change).
- New feature → ADR for the overlay decision (`docs/adr/0002-modal-overlay-composition.md`), style per `docs/adr/0001-layout-budget-ownership.md`.
- Direct model tests (build `Model`, send msgs via `Update`, inspect state) — no `teatest`.
- `internal/tui/bordered` is the only box renderer; the modal uses `bordered.RenderWithTitle` like every other section.
- Coverage cannot depend on the machine: no `0o000` permission tests (use EISDIR), no `t.Skip` for missing tools (use PATH stubs).

## Integration Points (non-obvious)
- `WindowSizeMsg` resize path (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go:17-25`): `prFits` rewire + `prFit()` re-center both land here.
- `m.toastReserve()` and overlay order in `View` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go:658-664`): modal spliced first, toasts last; reserve is computed from the layout (now formMin-free).
- `keybindsLines()`/`prPrompt()` interplay (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/sections.go:67-89`): the legend is 1 line; the dashboard behind still paints keybinds with the form's legend.
- `layout()` freed-lines branch (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/sections.go:28-36`) must remain for `logOpen` ONLY — the `m.pr != nil` half is deleted.
- The two `m.pr != nil` guards in `handleKey` (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go:254, 391`) stay.
- The ref read is `ClassRead` → hidden by default in the log panel (`logEntries` filter `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/cmdlogpanel.go:62-72`); the scenario "appears in the command log panel as a read entry" is satisfied by the recorder (visible with `a` = show all), NOT by changing the default filter.
- `for-each-ref refs/heads refs/remotes` lists `refs/remotes/origin/HEAD` (symref) — `ParseRefs` must drop `*/HEAD`.
- bubbles v2.2.1 APIs (confirmed via `go doc`): `textarea.Model.InsertString(s)` inserts at cursor (the `ctrl+t` skeleton); `textarea.Model.SetValue` replaces; `textinput.Model.SetValue`/`SetCursor`/`CursorEnd`/`CursorStart` for filter editing.

## Risks (ranked)
1. **Layout migration** — removing `m.pr` special-cases from `layout()`/`computeLayout`/`renderDashboard` changes what the dashboard paints behind the modal. Watch `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/sections.go:21-36`, `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/layout.go:56-78`, `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/update.go:667-682`. Pinned by `TestPROverlayIsFitsInTheTerminal` (re-pinned) + new width/height-exact splice tests.
2. **`prFits`/`prFit`/`prSection` rewire** — the fit check and widget sizing must move from `m.layout().bodyLines` to the modal's derived minimum vs `m.height`/`m.width`. Watch `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay.go:144-165, 271-285`. Pinned by the height/width-minimum tests (re-pinned).
3. **ANSI splice correctness** — untrusted ref names + exact-width lines; the modal's lines are `modalWidth` wide (bordered pads), the dashboard's are `m.width`. Watch the new splice helper + `sanitizeLogText` + `truncate`/`pad`-before-style. Pinned by new splice edge tests.
4. **Event pump / stale results** — a dropped rearm stalls the UI; a missing path guard paints another repo's branches. Watch the `prRefsMsg` handler (`withPump` + `m.pr.path == msg.path`). Pinned by stale-result drop tests.
5. **Key precedence** — picker keys (up/down/enter/runes) must not leak to the table nor steal modal-global keys (tab/esc/ctrl+s/ctrl+t). Watch `handlePRKey` `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay.go:199-239`. Pinned by `TestPROverlayCapturesTheKeyboardInteger` + new picker-key tests.
6. **`prFieldCount` 4→5** — `prLabelFor` default case, `focusField` helper, tab-walk/highlight tests. Watch `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/proverlay_test.go:72, 419-431`.

## Blockers
None — the plan is implementable as written. `textarea.InsertString` exists (ctrl+t skeleton is feasible without SetValue+reposition); `computeLayout` callers are enumerable (3 sites); removing the `m.pr` branch from `renderDashboard` is safe because the toast reserve is computed from the now formMin-free layout.

One clarification for the executor (not a blocker): the scenario "it appears in the command log panel as a read entry" — the ref read is `ClassRead`, which `logEntries` hides by default (`/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-pr-overlay-ux/internal/tui/cmdlogpanel.go:62-72`). The correct implementation records it as `ClassRead` (auditable via `rec.Entries()` and visible in the panel with the `a` = show-all toggle); do NOT change the default panel filter to show reads (the comment at :65 forbids it — 240 lines/scan with 60 repos).

## Picker model — binding clarification (overrides any ambiguous row above)

The approved plan and behavior.feature bind this shape; where a row above reads as "the field input is the filter", apply this instead:

- Branch field state: `value string` (committed — what `prParams` reads and what the field line paints) + a transient `filter textinput.Model` + `cursor int` (highlight) + `scroll int`, per picker. `refs`/`loading`/`err` stay shared in the draft.
- The pane appears only while the field is focused: the filter input's `View()` line, then the fixed-height filtered window with the highlight and a `… N more` tail.
- Keys while a branch field is focused: runes/backspace edit the FILTER only (the committed value is never edited char-by-char); up/down move the highlight (clamped, scroll follows); enter commits the highlighted ref — or the filter's literal text when the list has no match and the filter is non-empty — then clears the filter (pane returns to the full list with the committed value highlighted); tab/shift+tab discards the filter and moves field; esc closes the modal; ctrl+s submits with the committed values.
- The filter starts empty on focus (pane opens showing all refs). `prParams` never reads the filter. `newPROverlay` seeds `value`: head = snapshot branch, else worktree label, else empty; base = sync branch, else `syncOf` fallback.
