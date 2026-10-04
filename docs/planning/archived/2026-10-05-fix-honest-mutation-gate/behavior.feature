# Expected behaviour of the mutation gate.
# Single source of behaviour, for the user's verification.
# It is NOT Cucumber: no step definitions, no runner. Keywords in English.
# Descriptions in the repo's language (English).
#
# The check's promise, in one line: "Mutation (diff)" is GREEN only if the mutation
# really was measured and no new survivor is left untested. "Could not measure" is a
# RED verdict, never a green one.
#
# And there is a second principle, just as important: not every green is the same
# green. A green that measured NOTHING has to say so, because otherwise the team
# reads it as a measurement.

Feature: The required check "Mutation (diff)" only goes green if it really measured
  The check measures mutation testing (gremlins) over the PR's diff against its base
  branch, using merge-base, and blocks the merge if a mutant appears that survives
  the tests and is not in .mutation-allowlist.

  A required check green while measuring nothing is worse than one in red: the red
  says "this is not verified" and gets fixed; the green says "this is verified" and
  the team stops looking.

  Background:
    Given the repo has a committed .mutation-allowlist
    And the check's scope is the diff against the PR's base branch
    And the job cannot stay pending: there are no paths filters nor conditions that could skip it

  # --- The promise: green = measured and clean -------------------------------

  Scenario: New survivors block the merge
    Given the mutation run measured and produced report.json
    And the report holds 46 mutants and 2 of them came out LIVED
    And neither of the 2 survivors is in .mutation-allowlist
    When the gate evaluates the run
    Then the check is RED
    And the step summary lists the 2 survivors, with mutator, file and line
    And the summary says how to resolve it: a test that kills them, or an allowlist entry if they are provably equivalent

  Scenario: The comparison with the allowlist is by line, not by substring
    Given the run produced a CONDITIONALS_BOUNDARY mutant at internal/tui/table.go:292
    And the allowlist has CONDITIONALS_BOUNDARY at internal/tui/table.go:295
    When the gate evaluates the run
    Then that mutant counts as NEW
    And the check is RED
    Because an entry naming the file at a different line does not cover the mutant that appeared

  Scenario: All already-known survivors give green
    Given the run produced 46 mutants and 4 came out LIVED
    And the 4 survivors are in .mutation-allowlist with their justification
    When the gate evaluates the run
    Then the check is GREEN
    And the summary says how many survivors there were, how many were allowlisted and how many are new
    And the summary publishes the measurement's numbers

  # --- Nothing to mutate: green, but a green that says so -------------------

  Scenario: A PR that touches no Go code does not measure, and says so
    Given the PR only touches scripts, workflows, Makefile and documentation
    When the gate evaluates the PR's scope
    Then the check is GREEN
    And the job finishes instead of being skipped
    And the summary says there are no .go changes to mutate
    And the summary leaves a record that no mutation was measured

  Scenario: A tests-only PR has scope but zero mutants, and that is NOT a failure
    Given the PR only touches _test.go files
    And the diff contains no production code, so there are no mutants to generate
    When the gate evaluates the run
    Then the check is GREEN
    And the summary says the diff held no mutants
    And the summary does NOT say the measurement failed
    Because there is Go code in the diff, so the scope exists, but there is nothing to mutate: that is a result, not an absence of a result
    And if the gate treated it as a failure it would block the repo's test-only PRs

  Scenario: The three reasons for a red are told apart from each other
    Given a run can end without a useful verdict for three different reasons
    When the check is RED
    Then the reason says whether the measurement could not be made, whether it was cut short, or whether the engine exited fine without writing anything
    And whoever reads it does not have to guess which of the three it was
    Because "there was nothing to mutate", "it could not be measured" and "it was cut" demand opposite actions: write a test, fix the infra, or retry

  Scenario: Measuring nothing is a verdict of its own, not a disguised green
    Given the check knows the diff has nothing to mutate
    When the check finishes
    Then it reports a "no measurement" verdict distinguishable from the "measured and clean" verdict
    And it does not re-derive it from the absence of a report, but from the prior count having been zero
    Because if "nothing to mutate" were deduced from "no report", the lie would not have been fixed: it would only have moved

  # --- Without measurement: red ---------------------------------------------

  Scenario: Without report.json the check is red, not green
    Given the mutation run never got to write report.json
    When the gate evaluates the run
    Then the check is RED
    And the reason says there was no measured result
    And the reason includes the tail of the run's log so it can be diagnosed
    And no report is published as if it had measured

  Scenario: The engine says there are no results and there are no mutants to mutate
    Given the engine announces there are no results to report
    And the prior count of anticipated mutants was zero
    When the gate evaluates the run
    Then the check is GREEN
    And the green's reason says there were no mutants, not that the measurement failed
    Because the absence of a report and the "no results" phrase travelling together mean zero mutants, not a lost measurement

  Scenario: The supervisor cuts the run on a stall or on the ceiling
    Given the run made no progress and the supervisor cut it
    When the gate evaluates the run
    Then the check is RED
    And the reason tells a cut for lack of progress apart from a cut on the absolute ceiling
    Because a cut is not a failure of the tests: it is a failure of the measurement

  Scenario: The engine finishes with a non-zero code
    Given the mutation run finished with a non-zero code
    When the gate evaluates the run
    Then the check is RED

  Scenario: A tool failure while preparing the verdict does not degenerate into green
    Given a tool the verdict needs is unavailable or fails halfway
    When the gate evaluates the run
    Then the check is RED
    And the summary is not published half-written, looking like a valid measurement
    Because an extractor that fails leaves its output empty, and an empty output read as "zero new survivors" is exactly the green-without-measurement this check forbids

  Scenario: A run with no log is red even though the engine said it exited fine
    Given the run's log is empty and the engine exited with code zero
    When the gate evaluates the run
    Then the check is RED
    Because an empty log is what a flag that silences the engine's output produces: the engine exits with code zero and with the report present, and yet nothing was measured

  Scenario: The job cannot go green for not having run the gate
    Given the PR's scope stayed indeterminate because the base ref could not be fetched
    When the check cannot establish which files it is going to mutate
    Then the check is RED
    And the reason says the base could not be resolved
    And no path executes that ends in green without measuring
    Because an empty diff from an unresolved base is indistinguishable from a PR with no changes, and treating them the same reopens the hole this check fixes

  # --- Mutants that expired: the invisible failure --------------------------

  Scenario: Mutants that timed out block even when the efficacy comes out perfect
    Given the run produced report.json with 46 mutants and 100% efficacy
    And the run's log shows 3 mutants that timed out
    Because the engine excludes the expired ones from the total and from the report, so an incomplete count is presented as a perfect measurement
    When the gate evaluates the run
    Then the check is RED
    And the reason says how many mutants went unchecked on time
    And the reason clarifies that the report did not count them

  Scenario: The count of expired mutants is cross-checked against itself
    Given the engine publishes an aggregate of the expired ones and also a line per expired mutant
    When the gate counts the expired ones
    Then the count comes from the aggregate total, which is a number and does not get truncated when the log is cut
    And the per-line count is used as the cross-check
    And if the two counts do not match, the check is RED
    Because a count derived from partial text would give a number that looks fine and is not

  # --- Integrity of the scope and of the state ------------------------------

  Scenario: The measured scope is the diff the check announced
    Given the PR touches Go code in a single package
    When the gate measures the mutation
    Then it measures the mutants of that diff
    And it does not mutate the packages the PR does not touch
    And the set of files it mutates is the same as the set the check announced
    Because both sides use merge-base: the check's scope is the three-dot diff, and the engine internally runs git diff --merge-base
    And that equivalence holds with a clean tree, which is what the runner's checkout guarantees

  Scenario: The engine cannot measure more than the check announced, not even by accident
    Given the engine, handed an empty diff, silently falls back to the whole module
    And this PR is one of scripts, so its diff has no Go code
    When the check evaluates the scope before measuring
    Then it decides there is nothing to mutate and launches no measurement
    And it does not go measure the whole module without saying so
    Because measuring the whole module inside a budget of minutes does not fit, and it would also be measuring a different thing from what the check offers as its verdict

  Scenario: The scope is never announced differently from what is measured
    Given the check decides before mutating which files it is going to mutate
    When the measurement finishes
    Then the gate checks that what was measured corresponds to what was announced
    And if it does not, the check is RED instead of giving a verdict over a set that was never measured
    Because a verdict over a set different from the announced one is a verdict about something else

  Scenario: The supervisor makes the cut, not the platform
    Given the run went idle
    When the job's budget runs out before the supervisor cuts
    Then the red is produced by the supervisor, with its reason and the tail of the log
    And the platform does not cancel the job underneath, because a cancelled job is left with no reason, no report and no log
    Because the numbers have to fit: the job's ceiling has to be above the supervisor's ceiling, and that one above the per-mutant cap

  Scenario: A previous run's report cannot be taken for this run's
    Given a report from a previous run exists in the working tree
    When a new measurement starts
    Then the old report is deleted before starting
    And if the new measurement produces no report, the check is RED
    Because locally the tree does survive between runs, and an old report read as new is a verdict about a measurement nobody made

  # --- Calibration: the allowlist is the gate's reference --------------------

  Scenario: Without an allowlist the job fails saying how to seed it
    Given the repo has no .mutation-allowlist
    When the mutation job starts
    Then the check is RED
    And the reason explains that without an allowlist the gate has nothing to compare against
    And the reason gives the exact command to seed it
    And the job runs anyway and does not stay pending

  # --- The gate tests itself -------------------------------------------------

  Scenario: This PR checks its own gate with a survivors fixture
    Given the PR includes a temporary Go fixture whose purpose is to leave survivors
    And the fixture is committed and pushed, not merely staged
    When the required check runs on the PR
    Then it really measures: the summary brings the measurement's numbers
    And it lists the fixture's survivors as new
    And the check is RED
    Because a fixture that survives produces a red, and that red can only come from a real measurement: from the supervisor that ran, the engine that measured, the report that existed and the comparison that was made
    And removing the fixture brings the check back to green for having nothing to mutate

  Scenario: Before the fix, the same fixture went green
    Given the same fixture, on the old check
    When the required check runs on the PR
    Then the scope is not empty, because there is Go code in the diff
    And the check still comes out GREEN
    And the only possible reason for the green is the "no result, the gate passes" path
    Because that contrast is the proof of the original failure: the scope existed and the check measured nothing anyway

  Scenario: A staged-only fixture proves nothing
    Given the fixture is in the index but not committed
    When the required check runs on the PR
    Then the check measures nothing and demonstrates no guarantee of the gate