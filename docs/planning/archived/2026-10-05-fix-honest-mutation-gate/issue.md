# Fix the mutation testing gate: it must really measure, or it must fail

## The problem

The required check `Mutation (diff)` today is a **no-op that goes green while
measuring nothing**. The broken chain is this:

1. The `Mutate` step calls `make mutate-diff`, whose recipe invokes the supervisor at
   `$(WATCHDOG)` = `$(HOME)/.local/lib/swe/lib/watchdog.sh` (`Makefile:178`).
2. On a GitHub runner that file **does not exist**. It lives in chezmoi as
   `~/.local/share/chezmoi/home/dot_local/lib/swe/lib/executable_watchdog.sh`, and
   chezmoi deploys it to `~/.local/lib/swe/lib/watchdog.sh`. The runner does not run
   chezmoi. The step dies instantly.
3. The step carries `continue-on-error: true` (`mutation.yml:102`), so the job does
   not stop.
4. The `Gate` step does not find `report.json` and literally runs this
   (`mutation.yml:117-120`):

   ```
   - no report.json (timed out, or gremlins crashed) -> no result, gate passes
   exit 0
   ```

Result: **green, zero mutants measured, zero artefacts**. Confirmed in real runs:
`37069167074`, `37070797637`, `37074649740`, `37075347525`.

The last run that measured anything was `36688020576` (30-Sep, before the watchdog
existed): Killed 125 / Lived 4 / TIMED OUT 1, ~1m45s, artefact `mutation-report-18`.
Everything after that is theatre.

## Why it matters

A required check green while measuring nothing is **worse than a check in red**: a
red says "this has not been verified" and gets fixed; a green says "this is
verified" and the team stops looking. Worse still because the coverage gate has
already paid for this same class of failure twice (a workflow wired wrong, approved
in silence), so the lesson "a new gate is not tested until it has actually passed"
is already written in `AGENTS.md` and this check breaks it on every push.

And the damage is not theoretical. `.mutation-allowlist` is the list of the 17
survivors the team accepts as equivalent or as conscious risk. A gate that compares
nothing means **nobody is watching that list**: any new mutant surviving in a PR's
diff goes clean, and the list never grows nor is ever reviewed.

## The second hole: "no result → green" is dishonest by construction

Verified in the gremlins v0.6.0 source:

- `internal/report/report.go:178` computes
  `MutantsTotal: r.lived + r.killed + r.notViable`. **The `TIMED OUT` mutants are
  left out of the total and out of `report.json`.**
- The current gate only looks at `LIVED`. So a run where 15 mutants expired
  reports **100% test efficacy without having tested those 15**, and the gate has
  no way of seeing it.
- `TIMED OUT` exists only in **stdout**, as a per-mutant line emitted by
  `report.Mutant()` (`report.go:270-281`, format `"%s%s %s at %s\n"`; the status
  comes from `mutator.go:60`). And it goes to stdout because
  `cmd/gremlins/main.go:44` does `log.Init(color.Output, color.Error)`.

Conclusion: **stdout is the only source of truth for `TIMED OUT`, and the gate has
to count it there.**

## Why the local loop cannot be copied as it is

The per-mutant deadline is coupled to the coverage duration, so the coefficient is
not a dial but a switch. Measured in `internal/engine/executor.go`:

- `:101` — `testExecutionTime: elapsed * time.Duration(coefficient)`, where `elapsed`
  is the duration of **gremlins' coverage pass over the whole module**.
- `:194` — `context.WithTimeout(..., m.testExecutionTime)`; on `DeadlineExceeded`
  the mutant is left `TIMED OUT` and **the run continues**.
- `:227` — `go test -timeout` = that deadline **+2s**, plus `-failfast`, and it runs
  **a single package** per mutant.
- **There is no absolute per-mutant cap flag at all.** `--timeout-coefficient` is
  the only lever, so an absolute cap has to be expressed as
  `coefficient = ceil(CAP / elapsed)`.

The table measured in `Makefile:135-148` (hot `go test` cache → ~2s coverage → coef
2 gives a 6s ceiling, while `internal/tui` alone takes 3s):

```
   workers  coef   wall  killed  TIMED OUT  killed/s
        4     2     30s      46           0      1.53
       10     2     20s      29          17      1.45
       16     2     20s      20          26      1.00   <- worse than 4 workers
       16    30     20s      46           0      2.30   <- the one to use
```

That is why the coefficient has to follow the warmth of the machine's cache, and why
`scripts/mutate-all.sh` measures the coverage itself and chooses. The local script
already knows how to do this; what it lacks is an absolute cap.

## Approved scope

### 1. Vendor the supervisor into the repo

- `scripts/watchdog.sh`: the supervisor, copied from chezmoi
  (`lib/executable_watchdog.sh`, 319 lines), **with its comments**.
- `scripts/watchdog_test.sh`: its suite (357 lines, plain bash, grouped cases A–G,
  exits non-zero if any case fails), with the paths adapted. The `deployed || source`
  fallback that resolves the script's path today (`$ROOT/lib/watchdog.sh` →
  `$ROOT/lib/executable_watchdog.sh`) is redundant inside the repo and is removed.
- `scripts/mutate.sh`: **absorbs** `scripts/mutate-all.sh` and the mutation logic
  duplicated today between the `Makefile` recipes and the script. One single place
  that knows the warm-up, the denominator, the coefficient choice, the forbidden flags
  and the supervisor call.
- The `Makefile` keeps **a single local target `mutate-all`**, manual. The targets
  `mutate`, `mutate-diff`, `mutate-all-diff`, `_mutate_check`, `_mutate_total`,
  `_mutate_total_diff` and the `MUTATE_*` variables that only existed for those
  recipes disappear or are reduced to what the script reads.
- The workflow calls **the script directly** in diff mode
  (`gremlins --diff <base>`), without going through `make`.

### 2. Honest gate

- **No `report.json` → RED**, with the log's tail in the message.
- **`TIMED OUT` > 0 → RED**, counted on stdout.
- **gremlins' exit code ≠ 0 → RED.** This includes the supervisor's **124** (the
  exit it uses to cut on a stall or on the ceiling): nobody translates it into a gate
  failure today, so downstream 124 and "gremlins exited 0 with an empty report" are
  indistinguishable.
- `continue-on-error` stops being a path to green.
- **`set -e` in the gate.** Today it runs `set -uo pipefail` **without** `-e`
  (`mutation.yml:112`), so a `jq` that fails leaves a half-written summary instead of
  red. It is the same hole from the tool-failure side: the gate cannot go red even
  when it should.

### 3. Numbers

- Job `timeout-minutes: 5` (today 20).
- Supervisor: stall 2m, run ceiling 4m. **It has to die before the job does**, and
  with a readable message saying why.
- Absolute per-mutant cap: ~120s in CI (a starting value, to be tuned with real data)
  and ~180s locally.
- CI: 4 workers.
- `coefficient = ceil(CAP / elapsed)`, computed **after** the warm-up.

### 4. Warm up before measuring

`go test -cover -coverpkg ./... -run '^$' ./...`: the same instrumented build that
gremlins is about to do, without running tests. **Deliberately not parallelised** —
`go` already parallelises the compilation.

### 5. The watchdog's suite runs as a step of the mutation workflow

It is what proves the vendored supervisor works on the runner, which is exactly what
cannot be checked today.

Warning: the repo has **no shell test infrastructure at all**. Neither `bats`, nor
shellspec, nor a `*_test.sh`, nor `shellcheck` — `make lint` is Go only, so
**nothing lints the bash scripts**. The vendored suite will be the repo's first
shell harness and the only check that exists over that code, which is why it has to
run in CI and not only when somebody remembers.

### 6. `AGENTS.md`: the mutation section becomes policy

Rewrite it declaring: locally = the manual script; on a PR = CI verified with
`gh run watch`; there is no local diff profile; what the gate does and does not
guarantee. That section is the interface the tooling will read, so it has to tell the
truth.

### 7. `MUTATE_FORBIDDEN` survives, and becomes load-bearing

The `MUTATE_FORBIDDEN` flags (`-S --output-statuses -s --silent`) and their check
(`_mutate_check`) have to exist in the new script, or their disappearance has to be
justified. It is no longer hygiene: **`--silent` suppresses every `log.Infof`**, so a
silent run would emit zero stdout lines, the `TIMED OUT` count would read 0 and the
gate would go green without having measured anything. It is the same hole that item 2
fixes, reached by another path.

Secondary reason: `mutate-all.sh` reimplements the protection inline and **lacks the
grouped shorthand case** (e.g. `-Sk`).

## Out of scope

- **The change on the chezmoi/swe side.** `swe` ceasing to be the source of the
  supervisor, or ceasing to export it, is another PR.
- **No Go code under `internal/gitstatus`.** The user has WIP there in another
  checkout and it is not touched. (Also out of scope: touching any **production**
  `.go` — this PR carries no production code, and its diff should not find mutants.
  The `internal/tmpgate` fixture from § Open questions is the only planned exception,
  and it is temporary and removed before the merge.)

## Constraints that are not broken

- The check's name is still exactly **`Mutation (diff)`**: the `protect-main` ruleset
  demands that name.
- **No `paths:`** on the job. **No `needs:`/`if:`** that could skip it: a skipped
  required check stays *pending* forever and blocks every PR.
- The `Calibration` step **stays inside the job**: with no allowlist, RED with
  instructions on how to seed it.
- The allowlist gate is still compared **by line** (`comm -23` over `LIVED`): the
  same mutator at a different line must be seen as new.
- `concurrency` with `cancel-in-progress`.
- Pinned action SHAs.
- `fetch-depth: 0`.
- Report upload, only if the file exists.
- **The coverage gate has to keep passing.** The test fixture (§ Open questions)
  falls inside the diff, so `diff-coverage.sh` sees it: it has to come out at 100% of
  the diff. Coverage and mutation are different things — a live mutant is not an
  uncovered line — but both are measured over the same diff.

## Open questions

**Main one: how does this PR test its own gate?**

Trap declared in `AGENTS.md`: *"a new gate is not tested until it has actually
passed"*. And the case is worse here, because the workflow runs the file **from the
branch** (correct), but if the PR **does not touch any `.go`**, the scope step prints
`no .go changes vs main - nothing to mutate` and measures nothing. This PR is
precisely one of scripts: without a push, its own check keeps being the no-op it
fixes.

**There is already a verified mechanism to force a real scope: `internal/tmpgate`.**
It was built and measured on branch `tmp/verify-gate` (already deleted, so it has to
be rebuilt; its exact shape is recorded here). It was an exported `Clamp` with its
test, and the real run gave:

```
KILLED 2  (CONDITIONALS_NEGATION at gate.go:6 and :9)
LIVED  2  (CONDITIONALS_BOUNDARY at gate.go:6 and :9)
efficacy 50%   — no survivor in .mutation-allowlist  ⇒ the Gate exited 1
```

Two operational facts that validate the fixture and that have to be respected:

- **The scope greps the *committed* diff** (`git diff --name-only
  origin/main...HEAD`). A staged-only file is invisible to the gate: the fixture has
  to be **committed and pushed**, not merely in the index.
- **golangci-lint v2.13.2 does not flag the exported and unused `Clamp`** in an
  `internal` package (0 issues). The dead package does not break the `Lint` check.

The fixture's strong point is that it **discriminates the three cases at once**: if
the intermediate run comes out red *listing the 2 new survivors* in the step summary,
then the supervisor ran, gremlins measured, there was a `report.json` and the
comparison with the allowlist happened — which is exactly the opposite of "no
report.json → passes". The reason of the red is read in `$GITHUB_STEP_SUMMARY`, not
assumed.

Candidates, none decided yet:

1. **`internal/tmpgate` fixture committed in the PR** (the verified mechanism above).
   The protocol would be: push with the fixture → observe the intermediate red →
   remove the fixture → the diff is back to having no `.go` and the PR is green. The
   proof stays in the run's history, and `main` does not inherit dead code. It has
   been attempted before by a different route (PR #21, `7e8a1b`, `2fa81b6`) and was
   reverted for leaving a real change in `main`.
2. A `gh workflow run` with `workflow_dispatch` whose diff exercises the real scope.
   Complementary: it isolates the gate's logic without dirtying the PR, but a
   manually triggered run is not the PR's required check, so it does not block it.
3. An explicit smoke of the gate's path (a dry-run script asserting that an absent
   `report.json` gives RED and that `TIMED OUT > 0` gives RED, with no `.go` needed).

This is decided at the plan checkpoint, not here.

**Secondary:**

- Is the 120s per-mutant cap enough in CI with 4 workers on a 2-4 core runner? The
  `Makefile` table suggests yes with margin (measured contention was N=1 → 8.2s,
  N=8 → 29.0s, N=16 → 74.2s over the slowest suite, so 4 workers against a 120s cap
  is plenty), but the first real run is what decides.
- Does the warm-up (`-run '^$'`) pay for itself? If not, the coefficient measured with
  a cold cache will be wrong and the first mutant will expire.
- Does the watchdog's suite enter as a step before `Mutate` (cheap, proves the
  supervisor) or after (if the gate fails, at least it already measured)?
- Is `report.json` uploaded also when the gate fails on `TIMED OUT` without a report,
  or does the stdout log become an artefact so it can be diagnosed?

**Latent risk, not of this PR but worth not forgetting:**
`.mutation-allowlist` is indexed by **line number**, so any refactor that moves code
invalidates its entries. This PR does not touch `.go`, so it does not trigger it —
but it is the next trap somebody will pay for.