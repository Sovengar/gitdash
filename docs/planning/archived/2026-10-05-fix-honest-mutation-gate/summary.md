# Summary: fix-honest-mutation-gate

## Metadata
- **Completed:** 2026-10-05 16:06
- **Duration:** ~20 hours wall clock (planning package opened 2026-10-04 20:19, closed 2026-10-05 16:06)
- **Plan Number:** 0002

## What was wrong

The required check `Mutation (diff)` was a **no-op that went green while measuring
nothing**. The `Mutate` step called a supervisor that only exists after a chezmoi
deploy (`~/.local/lib/swe/lib/watchdog.sh`, absent on a runner), died instantly,
carried `continue-on-error: true`, and the `Gate` step then ran an explicit
`no report.json -> no result, gate passes / exit 0`. Confirmed in four real runs
(`37069167074`, `37070797637`, `37074649740`, `37075347525`): green, zero mutants,
zero artefacts. The last run that measured anything was `36688020576` (30-Sep).

## What changed

- The supervisor is **vendored into the repo** (`scripts/watchdog.sh` +
  `scripts/watchdog_test.sh`), so the workflow no longer depends on the runner's
  home directory.
- One script (`scripts/mutate.sh`) now **measures and decides the verdict**:
  the measured set is the mutants the **report** names, the survivor count is
  read from the report instead of inferred from a literal, a cut reason survives
  the trip from the supervisor to the summary, and a dry-run that failed is not
  a measurement (its duration is parsed so the ceiling is honest).
- "Was there a run" and "what did it say" are **two verdicts**, so
  *measuring nothing* can no longer be read as a disguised green.
- The **job budget** makes a timed-out mutant still observable, and the count of
  expired mutants is cross-checked against itself so the efficacy cannot inflate.
- The announced scope and the measured scope are **verified with the same ref**.
- `AGENTS.md`'s mutation section became **declared policy**, and the three claims
  that were not what they said were corrected.

## Scenarios
| Scenario (behavior.feature) | Status |
|-----------------------------|--------|
| New survivors block the merge | ✅ Passed |
| The comparison with the allowlist is by line, not by substring | ✅ Passed |
| All already-known survivors give green | ✅ Passed |
| A PR that touches no Go code does not measure, and says so | ✅ Passed |
| A tests-only PR has scope but zero mutants, and that is NOT a failure | ✅ Passed |
| The three reasons for a red are told apart from each other | ✅ Passed |
| Measuring nothing is a verdict of its own, not a disguised green | ✅ Passed |
| Without report.json the check is red, not green | ✅ Passed |
| The engine says there are no results and there are no mutants to mutate | ✅ Passed |
| The supervisor cuts the run on a stall or on the ceiling | ✅ Passed |
| The engine finishes with a non-zero code | ✅ Passed |
| A tool failure while preparing the verdict does not degenerate into green | ✅ Passed |
| A run with no log is red even though the engine said it exited fine | ✅ Passed |
| The job cannot go green for not having run the gate | ✅ Passed |
| Mutants that timed out block even when the efficacy comes out perfect | ✅ Passed |
| The count of expired mutants is cross-checked against itself | ✅ Passed |
| The measured scope is the diff the check announced | ✅ Passed |
| The engine cannot measure more than the check announced, not even by accident | ✅ Passed |
| The scope is never announced differently from what is measured | ✅ Passed |
| The supervisor makes the cut, not the platform | ✅ Passed |
| A previous run's report cannot be taken for this run's | ✅ Passed |
| Without an allowlist the job fails saying how to seed it | ✅ Passed |
| This PR checks its own gate with a survivors fixture | ✅ Passed |
| Before the fix, the same fixture went green | ✅ Passed |
| A staged-only fixture proves nothing | ✅ Passed |

System tests: `make test` (go test -race -count=1 ./...) → all 13 packages **ok**
(run again at close, on the final shape of the branch).

## Commits
- `test(mutation): a verdict read from the report, never inferred`
- `fix(ci): a timed-out mutant still fails the gate`
- `refactor(build): leave mutation in a single script`
- `chore(mutation): undo the deliberate red run left behind`
- `docs(planning): the planning package for this change`

History was finalized to `one-per-concern` (5 concerns, 5 units) over 23 linear
commits. The content invariant was verified independently: the tree hash
(`e38ff99…`) and the sha256 of `git diff main...HEAD` are byte-identical before
and after the rewrite.

## Files
- **Created:** `scripts/mutate.sh`, `scripts/mutate_test.sh`,
  `scripts/watchdog.sh`, `scripts/watchdog_test.sh`,
  `docs/planning/0002-fix-honest-mutation-gate/{issue.md,plan.md,behavior.feature}`
- **Modified:** `.github/workflows/mutation.yml`, `Makefile`, `.gitignore`, `AGENTS.md`
- **Deleted:** `scripts/mutate-all.sh` (mutation left in a single script)

## Tests
- **Added:** 15 cases in `scripts/mutate_test.sh`, 6 in `scripts/watchdog_test.sh`
  (both run as a step of the mutation workflow, so the gate's own suite is inside
  the gate)
- **System Tests:** ✅ Passed (`make test`; GitHub `Build`, `Mutation (diff)`,
  `Lint`, `Test` all `success` on PR #23)

## Documentation
- **Changelog:** ✅ Not required — the change ships no user-facing behaviour and
  the repo has **no `CHANGELOG.md`**; creating one for a CI-internal fix would be
  a new convention decided silently.
- **Docs:** `AGENTS.md` (the mutation section is now declared policy; the three
  claims that were not what they said are corrected)
- **ADR:** No required

## Code Review Issues
- Critical Found: 0
- High Found: 0
- User Decision: Approved to continue (change #23 `open_for_integration: true`,
  no requested changes, `ready-to-integrate: {ok:true, ready:true}`)

## Notes for the next close

- The swe mechanism reported `integration_base: 'dev'`, but **this repo has no
  `dev` branch** (default is `main`, `origin/HEAD -> origin/main`). Every
  base-sensitive verb has to run with `SWE_INTEGRATION_BASE=main`. With the
  default, `shape-change-history plan` resolves the base to nothing and folds
  **97 steps including `main`'s own history and its merges** — a plan that, if
  applied, would have rewritten the base's commits into this change.
- `docs/planning/` is listed in `.git/info/exclude`, so the artifacts need
  `git add -f`.

## Next Step

Merge PR #23 into `main` through the branch-protection path (`protect-main`
demands PR + the four green checks). The change on the chezmoi/swe side — `swe`
ceasing to be the source of the supervisor — is explicitly **out of scope** and
is a separate PR.