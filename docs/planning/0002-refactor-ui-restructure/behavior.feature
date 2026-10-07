# Expected behavior — dashboard restructure: table + commits on top, fields + files below.
# This file captures the behavior the user verifies. It is NOT wired to a runner:
# the executor turns these scenarios into direct model tests (no teatest).
# Numbers below are the decided design constants; tests must pin them literally.

Feature: Dashboard restructure — repos and commits above, detail card with fields and files below

  Background:
    Given a rendered dashboard over a Model with the projects and snapshots named in the scenario
    And the terminal width and height are the ones named in the scenario

  # ── Table columns ────────────────────────────────────────────────────────────

  Scenario: ACTIVITY and FETCH are no longer table columns
    Given a table rendered at width 120
    Then the box header line lists exactly NAME, BRANCH, Work Tree, ↑↓up, SYNC
    And no ACTIVITY or FETCH column header is painted
    And no repo row paints a last-commit age cell

  Scenario: removing the two columns does not shrink the table at 80 columns
    Given a table rendered at width 80
    Then the same columns are visible as before the change (NAME, BRANCH, Work Tree, ↑↓up)
    And the header line and every row start their first column at the same offset (cursor slot + fetch slot)

  # ── Fetch glyph attached left of NAME ────────────────────────────────────────

  Scenario: a fetching repo paints the spinner glyph in its fixed slot
    Given repo A is fetching and repo B is idle
    Then A's row paints "⟳ " immediately left of the NAME cell
    And B's row paints two spaces in that slot

  Scenario: a failed fetch paints the failure glyph in the same slot
    Given repo A's last fetch failed
    Then A's row paints "✗ " immediately left of the NAME cell

  Scenario: names never shift between fetch states
    Given the same repo rendered idle, then fetching, then failed
    Then the NAME text starts at the same terminal column in all three states

  Scenario: the glyph follows the repo, not the cursor
    Given repos A and B, with A fetching
    When the cursor moves from A to B
    Then A's row still paints "⟳ " and B's row has a blank slot

  Scenario: worktree subrows and group header rows keep the same left edge
    Given a repo with an expanded worktree subrow and a group header row
    Then both reserve the same 2-cell fetch slot (blank) as repo rows
    And the worktree subrow's BRANCH/↑↓up/SYNC columns align with the repo row's

  # ── COMMITS panel (top right) ────────────────────────────────────────────────

  Scenario: the panel enters only when it costs the table no column
    Given width 119
    Then the commits panel is drawn to the right of the table (30 cells + 1-cell gap)
    And the table keeps the same visible columns it would have at full width

  Scenario: the panel is dropped when it would cost the table a column
    Given width 118
    Then no commits panel is drawn and the table takes the full width
    And the commits of the repo under the cursor are not shown anywhere

  Scenario: at 80x24 the panel is absent and the table is unchanged
    Given width 80 and height 24
    Then the table shows its 4 columns (NAME, BRANCH, Work Tree, ↑↓up)
    And no commits panel is drawn

  Scenario: the top band is rectangular at every height
    Given the panel is drawn
    Then the table box and the panel box span exactly the same number of lines
    And every line of the dashboard has the terminal width

  Scenario: the panel lists the commits of the repo under the cursor
    Given a repo with 3 commits and width 119
    Then the panel paints one line per commit: short sha, age, subject, newest first
    And the panel shows at most one line more than the table's body rows
    And a long subject is truncated to the panel's inner width, never wrapped

  Scenario: a repo with no commits shows a placeholder
    Given a repo with an empty commit list under the cursor
    Then the panel box is still drawn with a dim "no commits" placeholder

  Scenario: cursor on a group header shows the group aggregate
    Given the cursor is on a group header
    Then the panel is titled with the group name
    And it paints the same aggregate as the card does for a header today: repos always; errors, dirty, ahead, behind and worktrees only when non-zero

  Scenario: cursor on a worktree subrow shows that worktree's own commits
    Given the cursor is on a worktree subrow whose path has a live snapshot
    Then the panel paints that worktree's commits
    And it never paints the parent repo's commits
    And with no live snapshot for that path it paints a dim placeholder

  Scenario: no row under the cursor keeps a stable box
    Given the filtered table has no entries
    Then the panel is drawn with the same empty hint the table and card use

  Scenario: the log panel or the PR overlay replaces the whole body
    Given the log is open, or the PR overlay is open
    Then no commits panel and no detail card are drawn; the body is the log or the form

  # ── Detail card (bottom) ─────────────────────────────────────────────────────

  Scenario: the card is two columns
    Given width 80 and a repo with worktrees and files
    Then the card's left column paints path, branch, upstream, state, sync and activity
    And the right column paints the worktrees list when non-empty and the files list
    And a vertical separator divides the columns

  Scenario: the commits block no longer lives in the card
    Given any repo row
    Then the card paints no commits list at any width

  Scenario: the right column owns its own list budgets
    Given more worktrees and files than fit the card
    Then the worktrees list paints "… N more" for what it omits
    And the files list paints "… N more" for what it omits, independently

  Scenario: the card collapses to one column when narrow
    Given width 62 (inner width below 61 = 36 + separator + 24)
    Then the card stacks fields, then worktrees, then files, as before the restructure

  Scenario: the `!` input is always visible at the end of the card
    Given the `!` input is open and the card content would overflow
    Then the prompt and the input are painted at the end of the card in both the split and the collapsed card

  Scenario: a group header keeps its aggregate in the card
    Given the cursor is on a group header
    Then the card keeps painting the aggregate (repos/errors/dirty/ahead/behind/worktrees, non-zero only) as today

  Scenario: worktree subrows keep today's card behavior
    Given a worktree subrow with a discovered project and live snapshot
    Then the card is the repo card (split, no commits)
    And an undiscovered worktree gets the minimal card (path, branch, head, repo) in one column

  # ── Degradation floors ───────────────────────────────────────────────────────

  Scenario: activity raises the card floor by one line
    Given the default configuration
    Then at height 21 the card is not drawn (it used to appear with the 5-line header)
    And at height 22 the card is drawn with the activity field (6-line header) and the lists are omitted

  Scenario: tiny terminals never break the render
    Given width 1 and height 1
    Then the dashboard renders without panicking, with the panel and the split omitted

  # ── --print mode ─────────────────────────────────────────────────────────────

  Scenario: --print keeps its columns and ordering
    Given `bin/gitdash --print`
    Then the output still has its NAME, GROUP, BRANCH, WT, ↑↓up, SYNC, WTS, ACTIVITY, PATH columns
    And the row order is unchanged (attention first, then last commit, then name)
