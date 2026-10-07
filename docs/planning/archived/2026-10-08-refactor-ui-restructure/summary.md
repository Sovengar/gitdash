# Summary: refactor-ui-restructure

## Metadata
- **Completed:** 2026-10-08 00:06
- **Duration:** ~177 minutes (21:09 → 00:06)
- **Plan Number:** 0002

## Scenarios
| Scenario (behavior.feature) | Status |
|-----------------------------|--------|
| ACTIVITY and FETCH are no longer table columns | ✅ Passed |
| removing the two columns does not shrink the table at 80 columns | ✅ Passed |
| a fetching repo paints the spinner glyph in its fixed slot | ✅ Passed |
| a failed fetch paints the failure glyph in the same slot | ✅ Passed |
| names never shift between fetch states | ✅ Passed |
| the glyph follows the repo, not the cursor | ✅ Passed |
| worktree subrows and group header rows keep the same left edge | ✅ Passed |
| the panel enters only when it costs the table no column | ✅ Passed |
| the panel is dropped when it would cost the table a column | ✅ Passed |
| at 80x24 the panel is absent and the table is unchanged | ✅ Passed |
| the top band is rectangular at every height | ✅ Passed |
| the panel lists the commits of the repo under the cursor | ✅ Passed |
| a repo with no commits shows a placeholder | ✅ Passed |
| cursor on a group header shows the group aggregate | ✅ Passed |
| cursor on a worktree subrow shows that worktree's own commits | ✅ Passed |
| no row under the cursor keeps a stable box | ✅ Passed |
| the log panel or the PR overlay replaces the whole body | ✅ Passed |
| the card is two columns | ✅ Passed |
| the commits block no longer lives in the card | ✅ Passed |
| the right column owns its own list budgets | ✅ Passed |
| the card collapses to one column when narrow | ✅ Passed |
| the `!` input is always visible at the end of the card | ✅ Passed |
| a group header keeps its aggregate in the card | ✅ Passed |
| worktree subrows keep today's card behavior | ✅ Passed |
| activity raises the card floor by one line | ✅ Passed |
| tiny terminals never break the render | ✅ Passed |
| --print keeps its columns and ordering | ✅ Passed |

## Commits
- docs(planning): the ui restructure planning package
- docs(adr): single layout budget ownership
- refactor(tui): restructure the dashboard into side-by-side panes

## Files
- Created:
  - docs/adr/0001-layout-budget-ownership.md
  - docs/planning/0002-refactor-ui-restructure/issue.md
  - docs/planning/0002-refactor-ui-restructure/behavior.feature
  - docs/planning/0002-refactor-ui-restructure/plan.md
  - docs/planning/0002-refactor-ui-restructure/context.md
  - docs/planning/0002-refactor-ui-restructure/diagrams/feature-flow.md
  - docs/planning/0002-refactor-ui-restructure/diagrams/process-flow.md
  - internal/tui/restructure_test.go
- Modified:
  - AGENTS.md
  - internal/tui/app_test.go
  - internal/tui/cells_test.go
  - internal/tui/cmdlogpanel.go
  - internal/tui/detail.go
  - internal/tui/detail_test.go
  - internal/tui/layout.go
  - internal/tui/proverlay.go
  - internal/tui/sections.go
  - internal/tui/sections_test.go
  - internal/tui/styles.go
  - internal/tui/table.go
  - internal/tui/table_test.go
  - internal/tui/update.go
  - internal/tui/worktree_test.go

## Tests
- Added: 28 test functions
- System Tests: ✅ Passed (`make test` -race green; CI Lint ✓, Test ✓, Mutation ✓ — 139 killed / 0 lived in scope)

## Documentation
- Changelog: ✅ Updated (CHANGELOG.md created)
- Docs: AGENTS.md (design gotchas for the commits panel, the split card and the table's fetch slot), docs/adr/0001-layout-budget-ownership.md
- ADR: ✅ Created

## Code Review Issues
- Critical Found: 0
- High Found: 0
- User Decision: Approved to continue (0 CRITICAL / 0 HIGH / 0 MEDIUM after the fix; remaining LOWs accepted — AGENTS.md wording fixed, 2 informative NOT-COVERED mutants, one pre-existing bubbletea padding quirk that is not ours)

## Next Step
Integrate the change into `main` through PR #32 and release the working environment.
