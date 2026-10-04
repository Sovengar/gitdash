---
adr_required: false
---

# Plan: an honest mutation testing gate

> `adr_required: false` — the design was **derived from constraints**, not chosen
> among alternatives: the runner does not run chezmoi (vendoring is forced),
> `report.json` physically cannot hold the expired ones (`report.go:178`), there is
> no absolute per-mutant cap flag (`executor.go:101` only has `elapsed × coef`),
> and extracting the gate into a script is the house convention with
> `diff-coverage.sh` as the precedent already established. This repo's decision
> record **is** `AGENTS.md`, and its "Design gotcha" sections are de facto ADRs.
> What this change does need is that section rewritten.

## Sought result

The required check `Mutation (diff)` goes green **only if the mutation really was
measured and no new survivor was left untested**. "Could not measure" is a **red**
verdict, never a green one. And since not every green is the same green, a green
that measured nothing has to **say so**, or the team reads it as a measurement.

## Approach

Five pieces, in this order because each one is a cheaper check than the next:

1. **Vendor the supervisor** into `scripts/watchdog.sh` + its suite. It is a
   **frozen** copy: only path adaptation and removing the deployed/source fallback.
   The header records its origin and that diverging from chezmoi is a later PR.
   **No features are added in this PR**: the suite asserts the behaviour of *this*
   copy.
2. **`scripts/mutate.sh`** absorbs `scripts/mutate-all.sh` and the Makefile's
   mutation logic. Owner of: scope precheck, warm-up, `elapsed` measurement,
   coefficient computation, forbidden flags, denominator, supervisor invocation, run
   log, exit code. **Not** owner of: allowlist, `comm -23`, nor the red/green
   decision.
3. **The verdict**, as a pure function of paths. No gremlins, no Go, no git, no
   clock, no network. That is what makes piece 4 possible.
4. **The watchdog's suite as its own step** before mutating. Cheap, and it is the
   only thing that proves the supervisor works *on the runner* — which is exactly
   what cannot be checked today.
5. **`AGENTS.md`** rewrites the mutation section as the repo's declared policy. It
   is the interface the tooling will read.

## Cut decisions

### Where the gate's logic lives: in the script, invoked as **a single step**

**Decision.** `scripts/mutate.sh` produces *and* verifies, in the **same process**,
passing the exit status explicitly. The workflow keeps checkout, `fetch-depth`, the
base ref resolution and the artefact upload — **paths and refs, not logic**. The
inline gate in the YAML disappears, the `Gate` step included.

**Why, and why not "both".** The decisive argument is that **the red paths are pure
functions of files**. The verdict's three inputs — `report.json`, the run log,
`.mutation-allowlist` — are fabricated in a `mktemp -d` in under a second: no Go, no
gremlins, no diff, no network. "No `report.json` → RED" and "`TIMED OUT` > 0 → RED"
are **file** conditions, not run conditions. Exercising them inside the YAML would
require a synthetic Actions run, which is not a test but a hope — and it is exactly
what the repo's lesson ("a new gate is not tested until it has actually passed")
forbids.

**Why a single step and not producer + gate step.** The run's state has to travel
**in-band**. Between steps that requires `continue-on-error` (forbidden by the
design) or `outputs` (which requires the producing step to be soft). **Both are the
mechanisms that manufactured the current green.** One mutation step, and "there is
no `report.json`" stops being an unhandled state: it becomes a verdict.

**The cost, stated.** "What does the gate do?" can no longer be answered by reading
the workflow. Mitigation: the verdict prints its lines to stdout and the workflow
carries them to the step summary, so **the reason of a red is still in the step's
log**.

### The run/verdict seam: one file on disk, an explicit path

The supervisor **already has** its own buffer, and it is not fit for this: it is
transient (its trap deletes it), it does not survive `SIGKILL`, and it mixes stdout
with stderr. The run log is written by `mutate.sh` at a fixed path **in both
modes** — if the local mode did not write it, the invariant "the log the verdict
reads is the log the run produced" would only hold in one mode, i.e. it would not
hold. The verdict receives it as an **explicit positional argument**, never by
convention; an empty path is a hard error, never "I count 0". **It is never
deleted**: a script that deletes it destroys the evidence exactly on the red, which
is the only moment it is wanted.

The exit status is taken with `${PIPESTATUS[0]}` immediately after: with `pipefail`,
a pipeline's status is "the last non-zero", and a `tee` failing on ENOSPC becomes
indistinguishable from a failed mutation.

**Bonus that only exists because of this cut:** "empty log + exit 0" is a **third**
shape of red, and it is exactly the one `--silent` produces: the engine exits with
code 0 **and with the report present**, and yet nothing was measured.

### The warm-up is NOT the coefficient's denominator

`executor.go:101` multiplies by the duration of **gremlins' coverage pass**. The warm
up with `-run '^$'` **does not run the suites**: it is build time. Feeding
`ceil(CAP/elapsed)` with build time does not give the CAP.

And the real risk is not a huge coefficient (with `coef = ceil(CAP/elapsed)` the
ceiling normalises itself to ≈CAP): it is **`elapsed` = 0 or unreadable** →
division by zero → `coef 0` → `executor.go:86` (`if tCoefficient != 0`) is false →
it falls back to the default and **masks** the error instead of failing.

**Resolution:** keep measuring where it is already measured — the `done in` of the
**dry-run**, which is the faithful proxy because the dry-run runs the same
`coverage.Run()` before the engine exists — and use the warm-up **only as a cache
primer**, placed before. The dry-run must carry **the same scope flags** as the real
run, or the denominator and the `elapsed` describe different runs. Absent or zero
measurement = **hard error**, never a silent 0. Today `mutate-all.sh:93` does
`cov_secs=0` in silence and the script *announces* a ceiling that is not the one
that applies: a live bug that `mutate.sh` must not inherit.

Incidentally: today there are **two** dry-runs. Only one, tee'd to a file, gives the
`elapsed` and the denominator.

### The supervisor is a direct child, and `exec` is out

`mutate-all.sh:162` uses `exec` because it had nothing left to do. The new script
always has work afterwards, so `exec` is no longer available — and **it is what
would break the 124**: 124 means "run cut short" and it has to reach the verdict as
a distinct value, because the verdict needs to say *why*. `exec` would destroy that
capability.

Dropping `exec` does not break the group-kill: the two invariants the supervisor
needs (being the direct parent, and the engine having its own group) are properties
of `watchdog.sh`, not of how it was launched — it creates its pgid with `set -m` and
**refuses to run** if they collided. Contract rule: **no layer translates the
status**. `130/143` pass through intact: they mean the supervisor **was** signalled
(the runner's cancellation), not a mutation verdict; classifying them as "new
mutants" would be lying.

### Configuration surface

One table inside `mutate.sh`, **two rows per scope** (the cut's axis is run |
verdict, not local | CI: that way the formula exists once and both scopes come out of
the same table). The environment wins over the defaults, but that alone is not
enough: the failure mode of "env wins" is **a forgotten variable falling to the local
default**, i.e. CI loosening its own budget with no diff. That is why the `--ci` flag
means **"these knobs are mandatory"**: empty or unset = hard error naming the one
that is missing. Classification: *budget* (CAP, workers, stall, ceiling) mandatory in
CI; *correctness* (`exclude`, `coverpkg`) pinned in CI and overridable locally.
**`COEF` is not a knob**: it is the direct route to the unsafe number that
`ceil(CAP/elapsed)` exists to eliminate.

Two safety releases, and both are taken: **invariant assertions** under `--ci`
(`stall < ceiling < job's timeout` and `CAP ≤ ceiling`) instead of magic numbers,
and the **verdict prints the budget it ran with** in every run's summary. Without
the second, the coupling with `timeout-minutes` is invisible.

## How the gate is tested in its own PR (smoke A/B/C)

Trap declared in `AGENTS.md`: *"a new gate is not tested until it has actually
passed"*. And the case is worse here, because if the PR touches no `.go` the scope
says "nothing to mutate" and measures nothing — **and this PR is one of scripts**.

The branch is today exactly at `main`, so the control comes for free. The instrument
is a temporary Go fixture, **committed and pushed** (a staged-only file is invisible
to the scope: the scope greps the committed diff), whose behaviour was already
measured before: two mutants killed and **two survivors that are not in the
allowlist**.

| Run | What is on the branch | What must happen | What it proves |
|-----|---------------------|------------------|---------------|
| **A** | Only the fixture, with `main`'s **old** workflow | 🟢 **green**, and the only possible reason is `no report.json → the gate passes` | The original failure, with a **non-empty** scope. It isolates the dishonesty: there was `.go` and the check still measured nothing |
| **B** | The fix + the fixture | 🔴 **red listing the 2 survivors** | The supervisor ran, the engine measured, the report existed, the allowlist was compared. **A single run discriminates all four** |
| **C** | The fix, fixture removed | 🟢 **green** "nothing to mutate" | The mergeable end state; `main` does not inherit dead code |

Run B is the one that proves the most, because the red is **read** in
`$GITHUB_STEP_SUMMARY`: if it lists survivors, the report existed and was compared;
if it said "no result", the fix did not measure. The reason of the red is not
assumed.

**Push order:** A first (before the fix), then B, then C. Each push waits for its run
with `gh run watch <id> --exit-status`, never with `sleep` + `gh pr checks` (runs go
*stale* after a force-push and the id has to be asked for again).

## Build order

Each failure is caught by the cheapest possible check:

1. **The forbidden flags first**, tested with a test that throws `-s` at them and a
   grouped one like `-Sk`. Pure shell, <1s, local. It is the wall the rest leans on:
   if this fails, everything else is irrelevant.
2. **`mutate.sh` with its verdict**, with the scope precheck inside and `rm -f` of the
   report at the start. Tested dry with **three** file fixtures (§ Local validation).
3. **The gate in the workflow**, and a test push with the flag that silences the
   output deliberately injected → it must come out **red**. It is the proof that the
   gate cannot go green again without measuring.
4. **The committed and pushed fixture** → intermediate red (runs A and B).
5. **The watchdog's suite as a step**, uploaded before step 3 if time is tight: it is
   30s and it validates the only component whose failure is invisible.
6. **`AGENTS.md`**, and `make mutate-all` locally to calibrate CAP and the coefficient
   with real data.

## Risks

| Risk | Why it matters | What bounds it |
|---|---|---|
| **The engine silently falls back to the whole module if the diff is empty** (`Makefile:241-242`) | This PR is one of scripts: without the precheck it would launch a 10 min run inside a 5 min job → cut → red | Scope precheck inside the script, over the **committed** diff |
| **"Nothing to mutate" re-derived from "no report"** | The lie is not fixed: it moves | A third explicit verdict, derived from the prior count, not from the absence |
| **The job is cancelled underneath the supervisor** | If the setup eats the margin: no 124, no reason, no report. The most important red is replaced by "cancelled" | The assertions `stall < ceiling < job's timeout` and `CAP ≤ ceiling` |
| **`MUTATE_LINE_RE` duplicated** | Numerator and denominator have to come from the same expression; if not, the supervisor declares a healthy run finished | Moved **whole, once**, into the script |
| **Fixture inside `coverpkg ./...`** | It enters the total floor, which **only goes up**: if it lowers it, `make coverage-check` fails and cannot be fixed without raising the floor | Measure the total **before** committing |
| **Allowlist indexed by line number** | Any refactor that moves code invalidates its entries | This PR does not trigger it (it touches no `.go`); it is noted as the next trap |
| **Several wiring boundaries rotting in silence** | See below | They are written down in `context.md` |

**Boundaries the executor will fail if they are not written** (they go into
`context.md`, not here): `MUTATE_BASE` is **not** removed from the Makefile —
`coverage-check` and `diff-coverage.sh` share it; the `Calibration` message points at
`make mutate`, which is deleted, and it has to be repointed to `make mutate-all`; the
`.PHONY` list and `AGENTS.md`'s **Commands** section name targets that disappear;
`Makefile` in `cache-dependency-path` goes stale when the wiring moves, and changing
the script does not bust the cache; the new log needs `.gitignore` (the repo's
Gotcha 5); the report's path has to be **one** variable shared by producer, verdict
and upload; `jq` as a declared precondition, not an assumption; and the `runGit`
rule is for Go, not for scripts, so it is said out loud so the convention is not
fought over.

## Local validation before pushing

The executor can close almost everything without touching the runner. In this order,
stopping at the first failure:

```bash
# 1. Syntax. NOTHING lints bash in this repo (make lint is Go only), so this is
#    the only net before executing anything.
bash -n scripts/watchdog.sh && bash -n scripts/mutate.sh

# 2. The vendored watchdog's suite (<1min, the repo's only shell harness)
bash scripts/watchdog_test.sh

# 3. The verdict dry, with the three file fixtures. All three modes have to give:
#    no report + "no results" -> GREEN;
#    no report without that line -> RED; report with timeouts -> RED.
#    Plus: report absent -> RED, and a failing tool -> RED.

# 4. The scope precheck: a repo with no .go in the diff cannot launch a measurement
#    (the engine would fall back to the whole module).

# 5. The forbidden flags: mutate the invocation with -s and with -Sk and check
#    that they are rejected before anything executes.

# 6. The repo's four checks, which the PR has to leave green
go build ./... && go vet ./... && make lint && go test ./...

# 7. Optional and expensive (~10min): a full run to calibrate CAP and the
#    coefficient with real data from the machine, before fixing the numbers.
make mutate-all

# 8. Only while the fixture is committed (runs A and B)
make coverage-check
```

Item 3 is the one that pays the most: it is the reason the verdict is a pure function
of paths. Without that property, the red paths can only be tested on a runner, and
then the A/B/C smoke is the only thing left.

## Conventions

- **Code comments in English, one line, only the WHY.** The vendored watchdog already
  comes in English and stays exempt. The new scripts and `AGENTS.md`'s mutation
  section follow it, and it is enforced: a suite assertion fails the build on a
  comment over 170 characters or a run of more than 6, because a convention nobody
  checks is only a comment. If a comment needs more than one line, the design goes
  into the section below or into `docs/`, not into the code.
- Paths and path names in English; the prose of `AGENTS.md` in the repo's language
  (English since #24).
- No SDD artefacts outside `docs/planning/`.

## Out of scope

- **The chezmoi/swe side**: `swe` ceasing to export the supervisor, or its
  how-to-mutate. Another repo, another PR. This section of `AGENTS.md` is the
  interface that change will read.
- **Production Go code.** The only planned exception is the temporary fixture of runs
  A and B, which is removed before the merge.
- **Renaming the check.** `Mutation (diff)` is demanded by the `protect-main` ruleset
  by that exact name.