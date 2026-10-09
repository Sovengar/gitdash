# Summary: pr-picker-enter-advance

## Metadata
- **Completed:** 2026-10-09 17:28
- **Duration:** ~51 minutes
- **Plan Number:** 0002

## Scenarios
| Scenario (behavior.feature) | Status |
|-----------------------------|--------|
| `enter` commits the highlighted branch and advances focus to the next field in tab order (base → head → draft) | ✅ Passed |
| `enter` with a verbatim typed ref (no match) commits it and advances | ✅ Passed |
| `enter` with nothing to commit (nothing typed, list not ready) is a no-op: focus and value stay | ✅ Passed |

## Commits
- feat(tui): advance to the next field when a PR branch picker commits

## Files
- Created: none
- Modified: internal/tui/proverlay.go, internal/tui/proverlay_test.go, docs/FEATURES.md

## Tests
- Added: 1 new test (`TestPROverlayPickerEnterWithoutACommitStays`); 2 existing picker tests extended with advance assertions
- System Tests: ✅ Passed (`make test` green, 14 packages; CI run 37950677898: Lint, Test, Mutation, CI fast all success)

## Documentation
- Changelog: ✅ Updated (Unreleased → Changed)
- Docs: docs/FEATURES.md (branch-pickers bullet: enter commits and moves on to the next field)
- ADR: No required

## Code Review Issues
- Critical Found: 0
- High Found: 0
- User Decision: Approved to continue

## Tooling Debt (not patched here)
- The CI Mutation red that preceded this fix was tooling, not the change: gremlins v0.6.0's `--diff` scope formula assumes contiguous added lines, so the measured range depends on the gitconfig (`diff.interhunkcontext`/`diff.algorithm` merge hunks locally, CI's clean config splits them), and mutants on `switch` `case` conditions are structurally NOT COVERED for gremlins (Go cover blocks start after the colon), which the gate reads as "no measurement". The `if/else-if` restructure moved the conditions into covered blocks (3/3 killed in CI). Reproduce CI locally with `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null make mutate-all-diff`. Same family as the previously recorded gremlins NOT COVERED lookup bug; scripts intentionally untouched.

## Next Step
Merge approval: integrate `fix/pr-picker-enter-advance` into `main` (PR #38, green and mergeable).
