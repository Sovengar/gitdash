# Plan: dashboard restructure — table + commits above, fields + files below

adr_required: true — reason: the restructure forces an architectural decision on where the
layout budget lives: one `layout` value, assembled only by `m.layout()`, owning both width and
height, with every pane renderer receiving its widths as arguments — versus a new composition
layer or self-sizing sections. It has discarded alternatives (pattern with tradeoffs) and
long-lived consequences for the additive-degradation guarantee and the coverage/mutation gates.
Proposed ADR title: `layout-budget-ownership`.

## Intended outcome

Wide terminals show a rectangular top band (repos table left, commits of the row under the
cursor right) and a bottom card split into fields (left) and worktrees/files lists (right).
Narrow/short terminals degrade additively to today's dashboard: the commits panel and the card
split are dropped before the table loses a column or the card loses height, and the `!` input
stays visible in every regime. Behavior is fixed by `behavior.feature`; that file is the source.

## Approach

- **Widths become part of the layout budget.** `computeLayout` stays height-only; `m.layout()`
  gains the width split (table vs. commits panel, card left vs. right) and the pane renderers
  stop reading `m.width` — they receive their width from the layout. One source of truth, so
  degradation decisions cannot disagree with what is painted.
- **Table.** ACTIVITY and FETCH leave `tableColumns`; the transient fetch glyph moves into a
  fixed 2-cell prefix slot after the cursor prefix (`⟳ `/`✗ `/blank`), reserved on every row
  kind so names never shift and worktree/header rows keep the same left edge.
- **Commits panel.** New renderer over `Snap.Commits` (already collected, max 5, no new git
  call): same sha/age/subject line as today's card block. Fixed 30 cells + 1-cell gap; additive
  gate: drawn only when the table keeps the same columns it would show at full width (width 119
  with current constants). No height-driven drop; the panel box always matches the table box
  height. Row kinds: repo → commits; worktree subrow → its own commits when its path has a live
  snapshot, dim placeholder otherwise (never the parent's); group header → the group aggregate,
  shared with the card through one helper; no row → the same empty hint as table/card. No
  fallback into the card when the panel is dropped.
- **Card.** One renderer with a split budget: left 36 cells of fields (path, branch, upstream,
  state, sync + new dimmed `activity`), right the worktrees and files lists with their own
  `… N more`; dim separator; both columns padded to the same height before the join; under
  61 cells of inner width it collapses to today's single-column stack. Commits block deleted
  from the card. `cardTail`/`cmdInputLines` keep the `!` input at the end, always.
- **`--print` is explicitly unchanged** (keeps ACTIVITY and ordering); no shared column model
  is extracted, so nothing in `print.go` becomes dead code.
- Update the AGENTS.md design-gotcha sections this change invalidates (table columns/fitColumns,
  preview panel head lines, card content).

## Risks

- **Two width truths** are the failure mode this cut exists to prevent: any renderer still
  reading `m.width` after the split re-creates the bug class the single layout struct avoids.
- **`truncate(s, w)` panics for `w <= 0`**: every new width (panel subject, card columns, slot
  math) must clamp to ≥ 1; negative arithmetic from `width - panel - gap` must be impossible.
- **Join order**: card columns must be padded/clipped to equal line counts and to their pane
  width before styling and joining, or ANSI breaks the width and the tail moves.
- **Removals must delete, not orphan**: `activityCell`/`colActivity`/`colFetch` and the column
  arms of `renderRow`/`renderWorktreeRow` disappear outright — unreachable code is a
  coverage/mutation-gate failure, not a leftover.
- **Pinned literals**: existing tests pin the old prefix width (`m.width-4`), the 7-column
  header, `TestFetchCellTransient`, commits-in-card and the preview floor; they need literal
  (not derived) expectations in the mutation-safe style.
- **Floors shift**: `detailHeadLines` 5→6 moves the card entry floor from height 21 to 22 and
  changes tight-height list budgets; the exact boundary tests change accordingly.
- **Log/PR takeover**: the panel and card must be composed only in the normal body branch;
  the existing height giveback (log/pr freed height) must keep conserving total lines.

## Ordering (coarse)

1. Layout width ownership: extend `layout`/`m.layout()`; pass widths into the table render path.
2. Table: drop the two columns, add the fixed fetch-slot prefix, update header/width math.
3. Commits panel: renderer + dispatch (repo/worktree/header/empty) + additive gate.
4. Card: split, lists right, `activity` field, commits removal, collapse rule.
5. Verify `--print` untouched; update AGENTS.md gotchas.
6. Suite + gates: update pinned tests, add per-scenario tests, `go build/vet/test`,
   coverage-check, mutation loop, `make install`.

## Out of scope

No new git subprocesses or dependencies, no keybinding/ordering/grouping changes, no commits
fallback in the card, no `--print` format change.
