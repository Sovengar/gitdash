# Refactor: restructure the dashboard into side-by-side panels (commits + files)

## Why

The user confirmed a new layout from a UI sketch. The dashboard today stacks
vertically: a repos table on top and a single detail card below that packs
fields, files and commits into one column. That wastes horizontal room, pushes
commits to the bottom of the card where they are the first thing cropped, and
keeps two table columns (ACTIVITY, FETCH) that are better surfaced elsewhere.

Goal: reclaim vertical space and surface commits and files more usefully by
splitting the dashboard into two side-by-side sections — table + commits on top,
fields + files at the bottom — without regressing the existing degradation
contract.

## Confirmed target layout (authoritative)

### 1. Top section — repos table + COMMITS panel, side by side

- The table keeps columns **NAME, BRANCH, Work Tree, ↑↓up, SYNC**.
  - `ACTIVITY` (`activityCell`, `relativeTime(Snap.LastCommit)`) is **removed**
    from the table and reappears as a new `activity` field in the detail card.
  - `FETCH` (`fetchCell`: transient `⟳ fetch` while fetching / `✗ fetch` on
    failure) is **not dropped**: it moves attached to the **left of the NAME
    cell**, with the name, NOT as a column and NOT moved to the detail.
- A new **COMMITS panel to the right of the table** shows the commits of the
  repo under the cursor (same data as today's card commits block: sha, when,
  subject). For a **group-header row** it shows the equivalent of today's
  `groupStats` aggregate.

### 2. Bottom section — detail card split into two columns

- **Left**: existing fields `path`, `branch`, `upstream`, `state`, `sync`, plus
  the new `activity` field (last-commit age).
- **Right half**: the **FILES** list.
- The commits block **leaves the card** (it now lives in the top-right panel).
- The `!` command input stays at the **end of the card, always visible**.

## Scope

### In scope

- Remove the ACTIVITY column (`activityCell`, `colActivity`, header entry) and
  wire `activity` as a card field.
- Attach the transient fetch icon to the left of NAME in the row render, with a
  composition that does **not shift names while a fetch runs** (reserved slot —
  the row already carries a 2-char cursor prefix `▸ `/`  `). `fetchCell` output
  and `fetchStates` stay the source; only its placement changes.
- New top-right COMMITS panel: data source, empty/folded/group-header cases,
  height/width budget, and its own title/border.
- Card restructure into two columns: left fields + new `activity`, right files.
  Remove the commits block from the card.
- Keep `cardTail`/`cmdInput` behavior: `!` input always painted at the end.
- Worktree subrows and group headers: define what the commits panel and the card
  show (sane, not empty/incoherent).
- **`--print` mode decision** (`cmd/gitdash/print.go`): decide whether it changes.
  It shares ordering (`Score` + `lastCommit`) with the TUI and today prints
  `ACTIVITY` as its own column. The decision (keep as-is vs. mirror the new
  layout) must be explicit in scope, not left implicit.

### Out of scope

- New git data or new subprocesses: the commits panel reuses `Snap.Commits`,
  already collected. No new fetch/timestamp storage (none exists today).
- Changing the pull/push/fetch/PR/AI/log behaviors or their keybindings.
- Redesigning the ordering (`State.Score`) or the grouping/`Arrange` logic.
- Terminal handoff, forge or command-log semantics.

## Constraints and non-negotiables

- **Degradation contract stays additive.** Today: the preview panel is dropped
  below a floor (`computeLayout`/`panelCandidates`/`mismaChromeQue`), stats
  degrade before keybinds (`fitLayout`, `keepKeybinds`), and `fitColumns` trims
  table columns by width. The issue must **acknowledge that the new commits
  panel (min width/height), the files right column, and the `activity` field
  each need their own boundary** — exact behavior is settled later in
  `behavior.feature`/`plan.md`, but these are in-scope decisions, not
  afterthoughts.
- **`!` command input always visible** at the end of the card (`fichaTail`/
  `cardTail`); the card may crop from the top, never hide the prompt.
- **FETCH icon stays with NAME in the table.** Do not move it to the detail. Do
  not let the icon appear/disappear in a way that shifts the name.
- **Panel layout is width-driven too.** A side-by-side split needs a minimum
  width per side; below it the split must degrade (stacked/omitted) consistently
  with the existing height-driven degradation. Define the fallback.
- **The card does not repeat keys already in keybinds** and its title lives only
  on the border (existing rule).
- No cgo, no new deps. Git access only through `gitstatus` (`runGit`/
  `runGitCombined`) if any read is ever needed — but none is expected.
- Comments in English, WHY-only, one line each.

## Test and coverage expectations

- **Direct model tests only** (build `Model`, send msgs with `Update`, inspect
  state/render) — no teatest. Pattern: `internal/tui/app_test.go`.
- Repo enforces **100% diff coverage + mutation gates**: every new branch,
  boundary and degradation step must be reachable and asserted
  statement-by-statement. Column/fit/height decisions need tests that fail if a
  boundary is off by one.
- Cover at least: activity as a card field; fetch icon left-of-NAME with and
  without an in-flight fetch (no name shift); commits panel populated / empty /
  group-header / folded; two-column card with files; commits absent from the
  card; `!` input visible with a full card; width/height degradation of the new
  panels; the `--print` decision (whichever way it goes).
- Update existing assertions that encode the old layout (e.g. `tableColumns`
  width sum in `table_test.go`, commits-in-card tests in `detail_test.go`).

## Decisions to settle in `behavior.feature` / `plan.md`

- Exact fetch-icon slot composition (fixed width vs. glyph + space).
- Min width/height of the commits panel and the two-column card, and the
  fallback when width is insufficient.
- `--print`: unchanged vs. mirrored.
- Group-header commits-panel content (reuse `groupStats` fields how?).
- Whether the card keeps any commits fallback when the panel is not drawn.
