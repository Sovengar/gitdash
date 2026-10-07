---
feature: 0002-refactor-ui-restructure
freshness: 2ad0f32698dd0ab289779ba1154bfaed8efebdaa
worktree: /home/buble/dev/projects/gitdash/.worktrees/gitdash.refactor-ui-restructure
codegraph: ready
engram_index: codebase-index/gitdash — REFRESHED at 2ad0f32698dd0ab289779ba1154bfaed8efebdaa (branch refactor/ui-restructure, this worktree); usable as navigation, but this file is the executor's input
generated_by: codebase-researcher
---

# Context: dashboard restructure — table + commits above, fields + files below

Every path is relative to the `worktree:` above (absolute: `/home/buble/dev/projects/gitdash/.worktrees/gitdash.refactor-ui-restructure`). Line numbers are from commit `2ad0f32698dd0ab289779ba1154bfaed8efebdaa` (HEAD of this worktree).
Executor must not glob/grep: everything is here. Read `behavior.feature` for the literal constants and scenarios; this file only maps code → change.

## Scope
- In: split the dashboard into a rectangular top band (repos table left, COMMITS panel right) and a bottom card split into fields (left) + worktrees/files (right); drop ACTIVITY/FETCH table columns; move the transient fetch glyph into a fixed 2-cell slot left of NAME; add `activity` as a card field; delete the commits block from the card; `detailHeadLines` 5→6; `--print` unchanged.
- Out: new git data/subprocesses, keybindings, `State.Score`/`group.Arrange`, terminal handoff, forge, command-log semantics, `proverlay.go`/`cmdlogpanel.go` internals (they keep reading `m.width`).

## Files to Touch
Base = `/home/buble/dev/projects/gitdash/.worktrees/gitdash.refactor-ui-restructure`

| Symbol / Area | File | Lines | Why |
|---|---|---|---|
| `layout` struct | `internal/tui/layout.go` | 20-27 | Add width fields (`showPanel`, `tableWidth`, `panelWidth`, `cardWidth`, `cardSplit`) so one value owns width+height. `computeLayout` stays height-only; the width split lives in `m.layout()`. |
| `computeLayout` | `internal/tui/layout.go` | 30-52 | Keep height-only. It is the additive-degradation entry; do not add width here. |
| `panelHeight` / `panelCandidates` / `fitLayout` / `mismaChromeQue` | `internal/tui/layout.go` | 55-73 / 76-116 / 119-123 | Height floor moves with `detailHeadLines` automatically. `fitLayout` degrades hints→keybinds→stats; `mismaChromeQue` is the additivity predicate. Width gate is separate. |
| `m.section` | `internal/tui/sections.go` | 12-17 | Hardcodes `m.width` (`bordered.RenderWithTitle(..., m.width)`). Must accept a width so table/panel/card can be sub-widths. This is the central change of the "single budget source". |
| `m.layout()` | `internal/tui/sections.go` | 20-37 | Add the width split (table vs panel, card left vs right) and the width-driven panel gate. Keep the log/pr height giveback (`freed := 1` + preview) — the panel adds no height, so no extra giveback. |
| `compose` | `internal/tui/sections.go` | 64-80 | `middle` becomes the horizontally-joined top band (table + panel) instead of the table alone. |
| `tableSection` | `internal/tui/sections.go` | 148-164 | Must take the table width (currently `m.section("repos", ...)` + `headerColumns(m.width)` at :162). Add the 2-cell fetch slot to the header prefix. |
| `previewSection` | `internal/tui/sections.go` | 167-185 | Card dispatch (repo/worktree/header/empty). Add a width arg; keep the title on the border and `fitLines`. |
| `renderGroupSummary` | `internal/tui/sections.go` | 230-252 | Extract the pure aggregate text so the COMMITS panel (header row) and the card share it. `cardTail` must NOT be applied in the panel (the `!` input is card-only). |
| (new) `panelSection` / top-band join | `internal/tui/sections.go` | new | New renderer over `Snap.Commits` (30 cells + 1 gap), dispatch by entry kind; horizontal join with the table. Model the vertical padding on `fitLines`/`rellenaHasta` (:254-270). |
| `tableColumns` | `internal/tui/table.go` | 283-291 | Remove `{"ACTIVITY", colActivity}` and `{"FETCH", colFetch}` → 5 columns (sum 82). |
| `fitColumns` | `internal/tui/table.go` | 293-302 | Width deduction: currently called with `-4` (2 borders + 2 cursor). With the 4-cell prefix (cursor + fetch) it must be `-6`. See "Constants" — this is what makes the 119 boundary true. |
| `headerColumns` | `internal/tui/table.go` | 304-310 | Same deduction; header must reserve the same 4-cell prefix as rows (`"  "` cursor + 2-cell fetch slot). |
| `renderWorktreeRow` | `internal/tui/table.go` | 313-337 | Drop the ACTIVITY/FETCH cells; add the blank 2-cell fetch slot so BRANCH/↑↓up/SYNC align with repo rows. |
| `renderRow` | `internal/tui/table.go` | 436-468 | Drop the ACTIVITY/FETCH cells; render the fetch glyph in a fixed 2-cell slot left of NAME (`"⟳ "`/`"✗ "`/`"  "`). Names never shift. |
| `fetchCell` | `internal/tui/table.go` | 512-521 | Still the glyph data source. If the painted text changes from `"⟳ fetch"` to `"⟳ "`, update callers/tests; the slot is 2 cells wide. |
| `activityCell` / `colActivity` / `colFetch` | `internal/tui/table.go` :568-570 / `internal/tui/styles.go` :14-15 | delete | Orphaned by the column removal → coverage/mutation failure if left. `relativeTime` (:630-635) stays (reused by the `activity` card field). |
| `groupStats` | `internal/tui/table.go` | 386-415 | Reused as the panel's header-row aggregate. |
| `entryAt` / `entryKind` | `internal/tui/table.go` | 429-434 / 23-28 | Panel dispatch: `kindRepo`→commits, `kindWorktree`→its own commits, `kindPrimary/Secondary`→`groupStats`, `!ok`→empty hint. |
| `detailHeadLines` | `internal/tui/detail.go` | 11 | 5→6 (adds the `activity` field). Moves the card floor 21→22. |
| `listBudget` | `internal/tui/detail.go` | 16-28 | Reused for BOTH right-column lists (worktrees, files), independently. |
| `cmdInputLines` / `cardTail` | `internal/tui/detail.go` | 37-46 | Unchanged contract: `!` input always at the end. Must be applied AFTER the split join so the prompt spans the full card width. |
| `renderDetail` | `internal/tui/detail.go` | 49-185 | Split: left 36 cells (path/branch/upstream/state/sync + new dimmed `activity`), right lists; separator; equal-height pad before join; collapse below 61 inner cells. Delete the commits block (:146-156). Reads `m.width` at :59,:119,:138,:153,:166,:178 → take width as arg. |
| `renderWorktreeDetail` / `renderWorktreeMinimal` | `internal/tui/detail.go` | 188-215 | Worktree with marker+snapshot delegates to the split repo card (no commits); undiscovered stays one-column minimal. Reads `m.width` at :202. |
| `View` / `renderDashboard` | `internal/tui/update.go` | 631-637 / 640-650 | Compose the top band; branch log/pr BEFORE composing (panel+card only in the normal branch). |
| `toastReserve` | `internal/tui/update.go` | 652-658 | Derives from `m.layout()`; must keep conserving lines when the panel is present. |
| `Model` fields / `New` | `internal/tui/app.go` | 182-239 / 247-292 | `states map[string]gitstatus.Snapshot` (:185) is the panel's data source (`Snap.Commits`). No new field expected; `fetchStates` (:209) is the glyph source. |
| `fetchBatchCmd` | `internal/tui/app.go` | 388-435 | Emits `fetchStateMsg{path,state}` (fetching/failed/ok) that drives `fetchCell`. No change. |
| `bordered.RenderWithTitle` / `contentLines` | `internal/tui/bordered/bordered.go` | 20-22 / 74-91 | Contract: width INCLUDES borders, content clipped+padded to inner width (`width-2`). No change, but every new width must respect it. |
| `runPrint` / `printRowOf` | `cmd/gitdash/print.go` | 56-96 / 24-54 | Deliberately UNCHANGED (keeps ACTIVITY + ordering). Verify no shared symbol gets deleted that print.go imports — it shares nothing from `tui`. |
| AGENTS.md gotchas | `AGENTS.md` | §"Design gotcha: the preview panel" 280-310 (and its "header is `detailHeadLines` = 5 lines" at :307); table/columns mention in §Architecture; fetch/card gotchas | Update the invalidated text: 5→6 head lines, ACTIVITY/FETCH columns gone, commits leave the card, the new panel/split boundaries. |

## Contracts
- **Single budget source** (`layout` / `m.layout()`): the ADR `layout-budget-ownership` (proposed in `plan.md`, no file yet in `docs/`). `computeLayout` stays height-only; `m.layout()` owns the width split and every pane renderer receives its width as an argument. Any renderer still reading `m.width` after this re-creates the "two width truths" bug class.
- **`section()` / `bordered.RenderWithTitle` width semantics**: `width` includes borders; inner = `width-2`; content is ANSI-clipped and padded to inner (`bordered.go:25-27,74-91`). `m.section` currently fixes `m.width` (`sections.go:16`).
- **Cells return `(text, style)` and the render pads BEFORE styling** (`table.go:252`, `:462`): `c.style.Render(pad(truncate(c.text, c.width), c.width))`. Styling first breaks the width.
- **`truncate(s, w)` panics for `w <= 0`** (`table.go:595-601`: `runes[:w-1]` → negative index). Clamp every new width (panel subject, card columns, slot math) to ≥1; `width - panel - gap` must be impossible to go negative.
- **`cardTail`/`cmdInputLines`**: the `!` input is always painted at the end (`detail.go:37-46`); overflow is cut from the top. Keep in both split and collapsed cards.
- **`fetchCell`/`fetchStates`**: the glyph data source (`table.go:512-521`, `app.go:209`); only its placement changes.
- **`groupStats`** (`table.go:392-415`): reused verbatim for the panel's group aggregate; must count over `m.rows()` BEFORE folding.
- **`mismaChromeQue`** (`layout.go:119-123`): the additivity predicate for the height-driven panel; the width-driven commits panel needs its own gate (table keeps the same column count as at full width).

## Pattern to Follow
- **Additive degradation** → `computeLayout` (`layout.go:30-52`): compute `sinPanel`, then walk `panelCandidates` descending and accept the first `fitLayout` with `bodyLines >= minBodyLines` and `mismaChromeQue(sinPanel)`. The width gate is the same shape: try the split, accept only if the table keeps its columns.
- **Section composition** → `m.section` (`sections.go:12-17`), `compose` (`:64-80`), `previewSection` dispatch (`:167-185`). A section is `bordered.RenderWithTitle(..., width)`; compose joins sections with `\n`.
- **Table cell/prefix rendering** → `renderRow` (`table.go:436-468`) and `renderWorktreeRow` (`:313-337`): build a `[]struct{text,style,width}`, slice with `fitColumns`, then `pad(truncate(...))` + `style.Render`, prefixed by the cursor slot (`"▸ "`/`"  "`). Add the fetch slot to the same prefix.
- **List budget + `… N more`** → `listBudget` (`detail.go:16-28`) and its use at `:109-124` (worktrees) and `:134-144` (files): reserve the warning line when the list does not fit whole. The right column applies it independently per list.
- **Direct model tests** → `newTestModel` (`app_test.go:26-39`, width 120 height 30, `config.Defaults()`), `press` (`:123-130`, needs `Code`+`Text`), `cursorOn` (`pull_selector_test.go:15-30`, locates by path — ordering is attention-first). No teatest.

## Tests
Runner / infra:
- `go test -race -count=1 -coverpkg ./... -coverprofile=coverage.out ./...` (unit only; no DB/broker).
- Diff-coverage gate `scripts/diff-coverage.sh` (diff at 100%) + `scripts/coverage-floor` (currently `100.00`). New branches/boundaries must be asserted statement-by-statement.
- Mutation gate: `.mutation-allowlist` is compared **by line** (`<MUTATOR> <file>:<line>`). Inserting/removing lines shifts entries → re-verify, do NOT regenerate. Dead arms (`activityCell`, `colActivity`, `colFetch`, the removed column arms) are a gate failure, not a leftover.
- `make lint` (gofmt + golangci-lint), `make install` at the end.

Pinned tests that MUST change (exact anchors):
- `internal/tui/table_test.go`
  - `TestFitColumnsFitExact` (:123-154): asserts `total != colName+colBranch+colWT+colUpDown+colSync+colActivity+colFetch` → drop the last two terms.
  - `TestHeaderUsesTheWidthInterior` (:157-168): uses `fitColumns(width-4)` and `tableColumns[:want]` → `-6` + 5 columns.
  - `TestHeaderAndRowsFitTheSameColumns` (:382-429): header width vs `len(row)-2`; the row now has a 4-cell prefix (cursor+fetch) so the expected offset changes. Worktree-row loop (:412-424) uses `fitColumns(width-4)` → `-6`.
- `internal/tui/cells_test.go`
  - `TestFetchCellTransient` (:86-102): expects `"⟳ fetch"`/`"✗ fetch"` → update if the slot text becomes `"⟳ "`/`"✗ "`.
  - `TestHeaderSpacing` (:104-122): asserts `ACTIVITY` and `FETCH` are present → assert absent; keep `Work Tree`,`↑↓up`,`SYNC`.
- `internal/tui/sections_test.go`
  - `TestHeaderSkipsColumnsNarrow` (:335-352): asserts `FETCH` not fit at width 60 → replace with the new column set.
  - `TestFetchStateForItsValue` (:927-955): searches `"⟳ fetch"`/`"✗ fetch"` in `renderDashboard` → update to the new glyph text.
  - `TestPreviewPanelIsItLastInFall` (:126-160) + `minPanelHeight` (:193-202): derive `first` from `detailHeadLines`, so they adapt (first 21→22) — verify the `for h := 14; h < first` range still exercises the no-panel regime.
  - `TestLayoutHeightExactInAllTheHeights` (:162-190) / `TestLayoutInTheFitExact` (:222-264) / `TestLayoutDegradesInTerminalDrops` (:68-124): height-only; recompute if any hardcoded height (14/41) now lands on the other side of the 5→6 floor. `totalHeight` (:204-219) has no width term — leave unless a new height is added.
  - `TestPreviewWithoutHeightNotDrawsBox` (:723-738): counts `"╭ "` sections == 3; with the panel drawn at width 120 it becomes 4 — the test sets height 14 (no panel) so still 3, but verify.
  - `TestSummaryOfGroupOnlyPaintsWhatIsThere` (:650-717) / `renderGroupSummary(groupHeader("backend"), 20)`: signature may gain a width; and the shared aggregate extraction changes the call.
- `internal/tui/preview_test.go`
  - `TestPreviewShowsTheRepoOfTheCursor` (:37-55), `TestPreviewInSubRowOfWorktree` (:69-94), `TestPreviewFillsUntilItsHeight` (:141-156, box line count), `TestPreviewAnnouncesTheListsThatDoNotFit` (:172-200, `"files (2)"` / `"more"`), `TestPreviewSummaryOfGroup` (:96-127): all read the CARD; re-verify after the split + commits removal.
  - `TestPreviewNotBreaksTable` (:202-228): full-width every line; the top band join must keep it true.
- `internal/tui/detail_test.go`
  - `TestCardClipsEachFieldAItsWidth` (:188-231): `m.width` + `renderDetail(r, 40)` signature; the field clip constants (`width-13/-8/-24/-30`) change to the left-column width.
  - `TestCardClipsTheListsToLiteralWidths` (:234-265): literal run lengths for the file/commit rows; the commit row is gone and the file row width is the right column.
  - `TestCardTheGapMinimumForTheCommits` (:537-547): asserts the card paints `commits` → delete/invert (behavior: no commits in the card at any width).
  - `TestCardTheListsShareTheBudget` (:442-...) / `TestCardTheWarningCountsTheMissingWorktrees` (:478-...) / `TestCardTheGapMinimumItSpendsTheFirstList` (:501-...) / `TestCardTheWarningCountsTheMissingFiles` (:293-...) / `TestCardClipsTheCommandToTheWidthThatIsLeft` (:709-...): the worktrees/files budgets become independent (right column), not shared.
- `internal/tui/app_test.go`: `newTestModel` uses width 120 → at 120 the commits panel IS drawn (≥119), so every render assertion (`TestViewContainsTable` :374, `TestPanelShowsTheFiles` :294, `TestSummary` :365, etc.) now sees the panel. Audit or pin the width to <119 in tests that must not see it.

Suggested scenario→area mapping (add new tests): ACTIVITY/FETCH gone + header/row offset (table_test); fetch slot glyph + no name shift + follows the repo (cells_test/table_test); panel gate at 118/119 + 80x24 absent (new layout/width test); panel content repo/empty/header/worktree/no-row (new panel test); card split at 63/62 + `!` visible (detail_test); activity field (detail_test); commits absent from card (detail_test); floor 21/22 (sections_test); `--print` unchanged (cmd/gitdash print test).

## Conventions & Boundaries
- Comments in English, WHY-only, one line each (repo floor: ≤170 chars, ≤6 contiguous comment lines in the scripts).
- No SDD/requirement IDs in code; `docs/planning/` is allowed scaffolding.
- Direct model tests only; no teatest.
- Derived states precedence `diverged > dirty > ahead > behind > detached > no-upstream > clean`; `State.Score()` ordering shared with `--print`.
- `--print` (`cmd/gitdash/print.go`) deliberately keeps its columns and ordering; extract no shared column model.
- Run `go build ./... && go vet ./... && go test ./...`, then `make install` (the user runs `~/.local/bin/gitdash`).

## Constants validated against the code (arithmetic)
| Constant | Verdict | Arithmetic (governing lines) |
|---|---|---|
| Table after removal: 5 cols = 82 cells | ✅ | 26+24+11+9+12 = 82 (`tableColumns` table.go:283-291; col constants styles.go:9-15). |
| Width 80 keeps NAME/BRANCH/Work Tree/↑↓up | ✅ | content = 80−2 borders−4 prefix = 74; cumulative 26/50/61/70/82 → 4 cols stop at 70. Before (prefix 2, content 76) also stops at 70. `fitColumns` table.go:293-302. |
| Commits panel: absent at 118, present at 119 (30+1 gap) | ✅ **iff** the deduction is 6, not 4 | table outer = W−30−1; need `tableOuter−2−4 ≥ 82` → `W ≥ 119`. At 119: outer 88, content 82 = exactly 5 cols; at 118: outer 87, content 81 → 4 cols, panel dropped. **`headerColumns(width-4)` (table.go:306) and `fitColumns(m.width-4)` (:328,:458) must become `-6`** (2 borders + 4-cell prefix). If left at `-4` the row overflows the border by 2 and the boundary becomes 117. |
| Card split: 63 split / 62 collapsed | ✅ | inner = W−2; 36+1 sep+24 = 61 → `W ≥ 63`. `m.section` width semantics (`sections.go:16`, `bordered.go:25-27`). |
| `detailHeadLines` 5→6: card absent at 21, present at 22 | ✅ | `fitLayout` reserve with preview=detailHeadLines: 5→`3+3+2+3+2+5=18`, body=`h−18 ≥ 3`→`h≥21`; 6→`19`, `h≥22`. `panelCandidates` starts at `panelHeight` (layout.go:55-73); floor derived by `minPanelHeight` (sections_test.go:193). |
| `--print` unchanged | ✅ | `print.go:90` header + `:24-54` keep `ACTIVITY`; shares nothing with `internal/tui`. |

No numeric discrepancy found. The single implementation-critical dependency is the width deduction change (`-4`→`-6`) — without it the 119 boundary and the border fit are both wrong.

## Integration Points (non-obvious)
- **Log/PR body takeover + height giveback**: `m.layout()` (`sections.go:27-35`) returns `freed := 1` (the table's column header) + `previewChrome+previewLines`, adds it to `bodyLines`, zeroes `previewLines`. The commits panel shares the table's height, so it gives back nothing extra; it must be composed ONLY in the normal branch (`renderDashboard` update.go:640-650 branches before `compose`). Total terminal lines must stay conserved.
- **Toasts**: `overlayToasts(..., m.width, ...)` and `toastReserve()` (update.go:633,652-658) use the layout; the top-band join must not change the number of body lines.
- **Filters feed both table and panel**: `m.entries()` applies `onlyDirty`/search (`rows()` table.go:64-83). Pass the SAME `entries` to the table and the panel so the cursor and the panel agree; `entryAt` (table.go:429-434) is the cursor lookup.
- **Cursor kinds**: `kindRepo`→`Snap.Commits`; `kindWorktree`→its own commits only if `discoveredByPath` + `states[path]` exists, else dim placeholder (never the parent's) — mirror `renderWorktreeDetail` (detail.go:188-195); `kindPrimary/Secondary`→`groupStats` aggregate; `!ok`→`emptyTableHint` (sections.go:192-197).
- **`m.width` readers to convert**: `detail.go:59,119,138,153,166,178,202`; `sections.go:16,162`; `table.go:328,458`. `proverlay.go:145-159` and `cmdlogpanel.go:78,97` legitimately keep `m.width` (full-width sections) — out of scope.
- **`fetchStates` lifecycle**: `fetchBatchCmd` sends `fetching`→`ok`/`failed` (`app.go:409-426`); `fetchCell` reads it per path. The glyph follows the repo, not the cursor, because the slot is per-row.

## Risks / Assumptions
- **Two width truths**: any pane renderer still reading `m.width` after the split (list above) disagrees with what the layout painted → overflow/clipping. The layout must pass every width.
- **`truncate` panic for `w ≤ 0`** (`table.go:595`): `width - panel - gap` and `cardWidth - 36 - 1` must clamp to ≥1. `truncate("x", 0)` → `runes[:-1]` panics.
- **Horizontal join order / ANSI**: the split card (and the table+panel band) must pad/clip each side to equal line counts and to its pane width BEFORE styling and joining, or ANSI breaks the width and the `!` tail moves. `fitLines`/`rellenaHasta` (sections.go:254-270) only pad vertically; a horizontal join is new.
- **Dead-code removal under the mutation gate**: `activityCell`, `colActivity`, `colFetch` and the removed column arms must be deleted, not orphaned (unreachable code fails the 100% diff-coverage and the mutation gate). `.mutation-allowlist` is line-keyed: re-verify entries after shifting lines; do not regenerate.
- **Floor shifts**: `detailHeadLines` 5→6 moves the card floor 21→22 and changes tight-height list budgets; tests with hardcoded 21 / `h:=14` ranges must be revisited.
- **Tests pinned to old literals**: `TestFitColumnsFitExact`, `TestHeaderUsesTheWidthInterior`, `TestHeaderAndRowsFitTheSameColumns`, `TestFetchCellTransient`, `TestHeaderSpacing`, `TestCardClipsEachFieldAItsWidth`, `TestCardClipsTheListsToLiteralWidths`, `TestCardTheGapMinimumForTheCommits`, plus every render test in `app_test.go` that now sees the panel at width 120.
- **Assumption**: the ADR `layout-budget-ownership` is decided but not yet written to `docs/`; the executor should treat the `layout`/`m.layout()` single-source rule as binding regardless.
