# AGENTS.md — gitdash

A guide for agents with no prior context on this project.

## What it is

A TUI (Go + Bubbletea v2) that shows the state of every git repo of the user,
discovered by a **marker file** `.gitdash.toml`: branch, pending changes (dirty)
and **↑ahead/↓behind**, with automatic batched fetch and quick actions
(pull/push/editor). Inspired by
[bircni/git-statuses](https://github.com/bircni/git-statuses) — it is not a fork:
it only shares the idea.

## Stack

- Go 1.26+, module `gitdash`
- `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`
  (import paths `charm.land`, NOT github.com/charmbracelet)
- `github.com/BurntSushi/toml`
- No cgo: git via subprocess (`git status --porcelain=v2 --branch`)

## Commands

```bash
go build ./... && go vet ./... && go test ./...   # build + lint + tests
go build -o bin/gitdash ./cmd/gitdash              # binary (a build artefact)
make install                                       # installs bin/gitdash into ~/.local/bin (PREFIX/DESTDIR)
go mod tidy                                        # after adding deps
```

```bash
make coverage-check                                # coverage profile + gate (diff at 100%, total with a floor)
make mutate-all                                    # mutation testing of the whole module, tuned to this machine
bash scripts/mutate_test.sh                        # the gate's red paths (<1s)
bash scripts/watchdog_test.sh                      # the mutation supervisor (also a CI step)
./scripts/gen-fixtures.sh                          # regenerates testdata/playground
bin/gitdash --print                                # one-shot table mode
# TUI smoke test (see Gotcha 3):
tmux new-session -d -s gd 'XDG_CONFIG_HOME=<tmp> bin/gitdash' && sleep 3 && tmux capture-pane -t gd -p
```

**RULE**: when you finish any code change, INSTALL the binary
(`make install`). The user runs the one in `~/.local/bin`: a stale binary with
changes already made causes false symptoms (e.g. "it finds no repos" because of
the marker's rename).

## CI and `main`'s protection

There are **two workflows** and **three required checks**: `Lint`, `Test`
and `Mutation`. (`Build` stopped being a required check when it was
folded into `Test`.) Both run without `paths` filters on purpose: a filtered
workflow is skipped, and a skipped required check stays *pending* forever and
blocks every PR that does not touch the filtered paths.

### `CI` (`.github/workflows/ci.yml`) — every PR, and pushes to `main`

This is the gate. `Mutation` is a **job of this file**, not a second
workflow: it was `mutation.yml`, and the header comments explaining its gate moved
with it.

### `CI fast` (`.github/workflows/ci-fast.yml`) — every commit on a branch

Push to any branch other than `main`: build + unit tests, no lint, no mutation.
Not a required check — a red `CI fast` never blocks a merge and a green one never
authorises one.

The two-workflow split is deliberate even though this suite is entirely unit today
(no testcontainers, every fixture under `t.TempDir()`, ~46 s end to end): putting it
in place before integration tests arrive is free, and retrofitting "which of these
are the fast ones" after they exist is the part that never happens. When that day
comes, the gate for them belongs in `ci-fast.yml`'s test step and the full suite
stays in `ci.yml` — dbx already works that way, behind `DBX_SKIP_DOCKER=1`.

Two jobs, not three. The split is `Lint` ∥ `Build`+`Test`, chosen so the run asks for
**two** runners instead of three: each job is an independent runner acquisition, and in a
degraded window that is what makes a 112 s run take 10+ minutes (the first job to get a
runner starts immediately, the others wait on capacity nobody controls). `Build`+`Test`
measure 82 s against `Lint`'s 112 s, so the wall clock is unchanged.

- **`Lint`**: `make lint` → fmt-check (gofmt) + golangci-lint
  **v2.13.2** (pinned in the `Makefile`; there is no `.golangci.yml`, so it runs
  the default linter set: errcheck, **govet**, ineffassign, staticcheck, unused).
  The standalone `make vet` target still exists for a fast local run, it is just
  no longer part of `lint`. This is the LONGEST job (112 s) and therefore the one
  that sets the wall clock.
- **`Test`**: four steps.
  0. `go build ./...` — the compile gate, kept from the old `Build` job. It writes
     no binary and never links (`go build -x ./...` never invokes `link`), so its
     claim is exactly "the packages compile". `go vet` is **not** here: it runs
     once, as golangci-lint's `govet`, in `Lint`.
  1. `go test -race -count=1 -coverpkg ./... -coverprofile=coverage.out ./...`
     (the whole suite, no `-short`). The suite is **self-contained**: every git
     fixture is created under `t.TempDir()` with `internal/testutil`, so CI does
     **not** need `make fixtures` nor `testdata/playground`.
  2. `Coverage of the subprocess`: `go build -cover` + `GOCOVERDIR` to measure
     `func main()`, which calls `os.Exit` and cannot run in the test binary.
     **`-coverpkg ./...` is not optional**: without it the profile does not
     include the cross packages and the helpers used from others
     (`internal/testutil`) come out uncovered. Measured: 97.30% without it,
     98.17% with it.
  3. `Coverage gate` → `scripts/diff-coverage.sh`: the **PR's diff at 100%** and
     the **total with a floor** (`scripts/coverage-floor`, committed and only
     goes up). The diff's base is the **explicit merge-base**, not the branch
     name: on the runner `git diff main...HEAD` comes out empty and the gate
     would approve in silence. That is why the checkout uses `fetch-depth: 0`.

### `Mutation` — a job of `ci.yml`, non-draft PRs only

It used to live in `.github/workflows/mutation.yml`; the job, its gate and its
explanatory comments are now inside `ci.yml`. The `if:` on it is an **event** gate
(a draft pays for nothing) and never a measurement one: the fail-vs-skip decision is
still `scripts/mutate.sh`, which is why the rule below still holds. `ready_for_review`
is in the workflow's `pull_request` types because it is not in the default set —
without it, flipping a draft to ready would report no check at all, and being
required that would block the merge.

The marker below is what `swe how-to-mutate` reads; the rest of this section is its
human version. The named command is the local loop — the diff gate is CI-only.

Mutation policy: make mutate-all

**The policy, short:** the required check goes **green only if the mutation was
really measured** and **no new survivor was left untested**. *"Could not measure"*
is a **red** verdict, never a green one. A mutant the engine let expire was not
measured either, so it is red too — unless `.mutation-timeouts` records that hang
with a ceiling, and then the summary says `count/ceiling` instead of hiding it. And since not every green is the same
green, **a green that measured nothing has to say so**, or the team reads it as a
measurement. The step summary is where the reason of a red is read.

The whole gate lives in **`scripts/mutate.sh`** (measure **and** decide, in a
single step), and that script is **not** decorative: the workflow only brings
paths and refs. The workflow carries **no** `paths:`, no `needs:`/`if:` that could
skip it, no `continue-on-error`, and the job stays at `timeout-minutes: 8` with the
supervisor's ceiling (5m) well below it. The check's name is exactly
`Mutation` because the `protect-main` ruleset demands that text.

**The budget is a chain, not four numbers**, and each link exists for a reason:

```
2 * CAP < STALL < CEILING        CEILING + SETUP_RESERVE < JOB_CEILING
    120s     180s     300s                   300s + 150s       480s
```

- **`2 * CAP < STALL`** is the one that is easy to get wrong. A mutant emits no
  progress while it runs, so the stall detector and the engine's own per-mutant
  timeout **race**. They used to be tied at 120s, so the supervisor's cut won, the
  run arrived as a `124` with the log truncated, and the mutant that expired was
  never reported: **the verdict's `TIMED OUT` branch became unreachable and an
  honest signal was destroyed by the safety net.** A whole cap of slack under the
  stall is what keeps both reachable. This is why the cap is 60s and not the roomier
  120s — 120s would need a stall above 4m, which does not fit under an 8m job with a
  150s reserve.
- **`STALL < CEILING`**: a stalled run is cut by the supervisor, with its reason.
- **`CEILING + SETUP_RESERVE < JOB_CEILING`**: the ceiling has to leave room for
  everything that runs before it, or the platform cancels the job instead.

**The budget invariant is NOT "the supervisor's ceiling fits in the job"**, which
is what it looks like and what it was: it is **"everything that runs before the
supervisor, plus its ceiling, fits in the job"**. Measured on the runner, what runs
first is 98s of checkout + setup-go + the shell suites, plus ~28.5s of warm and
dry-run; with the supervisor's ceiling at 4m the worst case came to **366.5s against
the 300s** of a `timeout-minutes: 5`, which means the platform cancelled the job
instead of letting the supervisor cut it — and a cancelled job is left with no
reason, no report and no log. 8m leaves margin to spare; the number to raise if a
run ever gets close is this one, not the ceiling.

It is checked with **two** assertions, because each one sees the other one's blind
spot:

- **The declared one**, at startup, in the script: `ceiling + setup_reserve <
  job_ceiling`, with `MUTATE_SETUP_RESERVE` (150s) as the declared floor of what the
  setup may take, plus `2 * cap < stall < ceiling`. It catches a `timeout-minutes`
  that is too short, and a budget whose own links contradict each other, before a
  run is spent.
- **The measured one**, right when the supervisor starts: `job_ceiling - (now -
  MUTATE_JOB_START) > ceiling`, with the real clock. It catches what the declared
  one cannot see, which is a setup that went past its reserve. **The clock is
  marked in the job's FIRST step, before the checkout**: marked later it would
  measure only the warm + dry-run and report the invariant satisfied while it was
  not, which is the same class of lie this check exists to stop. It fires on the
  path where a measurement is about to start, so a diff with nothing to mutate
  reaches its verdict without it — there is no supervisor to feed in that case.

Neither passes on its own with the invariant broken, and that is the point: the
declared one is defeated by a reserve declared too small, the measured one by a
clock that has not moved. The verdict also prints **the budget it ran with**,
because its coupling with `timeout-minutes` is otherwise invisible.

Who owns what, because these are responsibilities that overlap:

- **`scripts/mutate.sh`**: scope precheck, warm-up, `elapsed` measurement,
  coefficient, forbidden flags, denominator, the supervisor call, the run log and
  the exit code.
- **`scripts/watchdog.sh`** and **`scripts/watchdog_test.sh`** (vendored): cutting
  on a stall or on the ceiling, and nothing else. A copy of chezmoi's with no
  features added on top, and it differs from the origin in exactly four documented
  ways: a vendored header, one extra sentence about the exit-code contract, the
  dual-mode guard removed (inside this repo nothing consumes the function), and a
  dropped trailing newline. Diverging further is a PR in the other repo, and
  `watchdog_test.sh` asserts the behaviour of *this* copy. **Nothing checks the
  bytes, on purpose**: a golden hash would turn the legitimate arrival of an upstream
  chezmoi change into a constant to edit by hand, which is the rubber-stamping this
  section exists to prevent. What is checked is that the files are still there.
  **Deliberate exception to the comment convention**: these two files do NOT follow
  the "English, one line, only the WHY" rule. They come that way from chezmoi and
  reformatting them would make them diverge from their origin for no gain, which is
  exactly what being a copy forbids. The three files in `scripts/` that **are**
  ours (`mutate.sh`, `mutate_test.sh`, and the workflow's wiring) do follow it, and
  the design that does not fit in a comment lives here.
- **`shellcheck` is not wired into any gate**, and saying "shellcheck clean" without
  a severity is the kind of claim this repo keeps paying for. The real state:
  `mutate.sh` and `mutate_test.sh` report **no warning or error** (`-S warning`, zero
  findings) and **2 notes at default severity**, both intentional and both left in
  place — SC2086 on the unquoted `$ENGINE`, whose word splitting is the documented
  behaviour the forbidden-flag guard mirrors, and SC2016 on a `$PWD` inside a
  generated stub script, which has to reach the generated file unexpanded. The
  vendored pair carries 1 warning and 1 note, and both are pre-existing and untouched.
- **`.mutation-allowlist`**: the gate's reference. The gate is **not** the owner of
  the allowlist nor of the `comm -23`; it only compares **by line**, so an entry
  naming the file at another line does **not** cover the mutant that appeared.
- **`.mutation-timeouts`**: the same contract for what was NEVER measured. An
  expired mutant is not a kill, not a survivor, and `report.json` leaves it out of
  the total, so it cannot become green by silence: it is red unless this file
  records the hang as a **ceiling per file** (`<file> <ceiling>`), and a green that
  ran over one says so in the summary with `count/ceiling` per file. The ceiling
  and not an exact set of lines because expiry is partly contention — the same
  suite reported 9 hangs one run and 7 the next. Clearing an entry honestly means
  a test that fails fast and kills the mutant; raising the ceiling alone only moves
  the number.

Decisions that are not readable in the code:

- **The scope is the COMMITTED diff**, not the index: a file that is only *staged*
  is invisible to the gate, so a smoke fixture has to be committed and pushed to
  prove anything. Both sides use merge-base, so the check's scope and the
  `git diff --merge-base` the engine runs describe the same set. The base is
  **verified and diffed with the SAME ref**: they used to be two (`origin/main` and
  `main`) and on a runner, where the base only exists as a remote-tracking ref, the
  diff failed and its empty output read as "nothing to mutate". A `git diff` that
  cannot be computed is an **error**, not an empty scope: it prints the same bytes
  as a diff with no `.go`.
- **There are two counts, and they are not the same number.** The dry-run reports
  **every mutant the engine considered**, and with `--diff` the ones outside the
  scope come out as `SKIPPED`: 1090 of them for a two-file diff in this repo. The
  dry-run yields two counts, `WATCH_LINES` (all of them, the **supervisor's
  denominator**: the real run prints one line each, SKIPPED included) and
  `EXPECTED_MEASURED` (only the in-scope ones, **what the verdict compares** with
  the report). Using the first for both purposes is exactly how a diff that
  measured nothing could come out "measured and clean".
- **"Nothing to mutate" comes from the prior count**, never from the absence of a
  report. Re-deriving it from the absence does not fix the lie, it moves it. A PR
  with no `.go` and a PR touching only `_test.go` files are the two honest greens,
  and both say that nothing was measured. Watch the shape of each: a `_test.go`-only
  diff **still produces `report.json`**, because the `SKIPPED` mutants are results
  too, so it arrives through the report branch with total 0 and not through "No
  results to report".
- **A prior count > 0 with a report that measured none of them is RED**, in both
  directions, because the numerator and the denominator came out of different runs.
- **The scope's integrity is `measured ⊆ announced`, not equality.** Measured with
  the mutations the engine **reported**, excluding the `SKIPPED` ones: `files[]`
  also lists the files it merely looked at, and the log's progress lines carry one
  per considered mutant. Equality would be a guaranteed false positive, because an
  announced file that produced no mutant (a `_test.go`, a file with nothing mutable)
  is legitimate.
- **The coefficient is `ceil(CAP/elapsed)`**, with `elapsed` measured on the
  **dry-run** and not on the warm: the warm with `-run '^$'` runs no suites, so its
  duration is build time and `ceil(CAP/build_time)` is a ceiling nobody agreed to.
  There is no absolute per-mutant cap flag in gremlins — `--timeout-coefficient`
  multiplies the duration of the coverage pass, and that is the only lever.
  Measurement absent or zero = **hard error**, never a silent 0.
- **The run log is always written and never deleted.** The verdict receives it as a
  positional argument, never by convention, and an empty path is a hard error: it
  is the only place `TIMED OUT` lives, and `report.json` excludes it from the total
  and from the efficacy. The log is cross-checked against itself (aggregate total
  against lines), because a count derived from truncated text gives a number that
  looks fine and is not. The expiries that survive that check are then judged
  against `.mutation-timeouts`, so "we did not measure these" is either recorded
  with a ceiling or a red.
- **The run log carries the supervisor's stderr too**, folded in with `2>&1` rather
  than kept in a second file. The supervisor's reasons (the stall, the ceiling, the
  `alive` heartbeats) go to its stderr, and a stdout-only `tee` dropped the one line
  that explains a `124`: the verdict printed literally `reason not found in the log`
  on the red that most needs it. One file also keeps "the log the verdict reads is
  the log the run produced" true. It cannot disturb the `TIMED OUT` cross-check
  because the supervisor prefixes every line with its own name and neither counted
  shape is line-anchored to anything it emits.
- **A cut's reason is asserted end-to-end, not fabricated.** The verdict's tests used
  to *invent* a log already containing the supervisor's line, which is the shape a
  real run never produced and exactly why the hole survived 169 green assertions. The
  case that matters drives the real supervisor over a wedging engine, so the line has
  to arrive on its own.
- **The budget is explicit in CI.** Under `--ci` the budget knobs are
  **mandatory**: "the environment wins" only fails in one way, a forgotten variable
  landing on the local row, i.e. CI loosening its budget with no diff to show.
  `MUTATE_FORBIDDEN` is in that class for the same reason, and **the guard is
  derived from the list** instead of repeating it: a guard that can diverge from the
  list it claims to apply is a comment passing itself off as code.
- **`jq` is a declared precondition, and the report is parsed before it is read**:
  a tool that fails leaves empty output, and an empty output read as "zero new
  survivors" is exactly the green-without-measurement this check forbids. Presence is
  not parseability.
- **The `runGit` convention is for Go, not for scripts**: every git exec goes
  through `gitstatus` because the binary's subprocesses are auditable from the log
  panel. `scripts/mutate.sh` is a CI script whose output goes to the step summary,
  so it neither competes with that rule nor should.

**How the gate is tested**, because the repo's trap ("a new gate is not tested
until it has actually passed") is worse here: this change touches no `.go`, so its
own scope would say "nothing to mutate" and measure nothing.

- `scripts/mutate_test.sh` is the only place the red paths are reached: the
  verdict is a pure function of paths, so `report.json`, the log and the allowlist
  are fabricated in a `mktemp -d` in under a second, with no engine, no Go and no
  network. Exercising those reds inside the workflow would need a synthetic
  Actions run, which is a hope and not a test. For what does need the engine (the
  run phase: warm, dry-run, the two counts, the budget) it uses a **fake engine**
  with the shape of both real outputs and the real supervisor: without that, the
  budget's measured assertion would be unreachable locally.
- **Both suites run as a workflow step** (`Shell suites`), not just the
  supervisor's: the supervisor's because it is the only thing that proves the
  supervisor works *on the runner* (a broken gate is loud, an absent supervisor was
  green), and the gate's because it costs ~1s and it is the only thing that proves
  the red paths are still alive **on the runner**, which is where the gate runs.

**What the gate does NOT guarantee** (knowing it avoids trusting it too much):

- It does not cover the whole module: only the PR's diff against its base. The
  whole module is the manual local loop.
- It does not measure timed-out mutants as such; it counts them from the log and
  blocks on that, but it cannot say *what* expired them.
- There is no verified local diff profile equivalent to CI's: the local loop is
  `make mutate-all`.

`main`'s rules (ruleset **`protect-main`**, reproducible with
`scripts/setup-repo-protection.sh`, idempotent and with `--dry-run`):

- Merge **only via PR**, with all **four** checks green; force-push and deleting
  `main` are blocked.
- There is an **admin bypass** and it is **deliberate** (approved by the user): an
  admin *could* push straight to main, but the working intention is always the PR
  path. No non-admin actor can do it.
- `delete_branch_on_merge=true`: GitHub deletes the remote branch on merge.

### The workflows' traps (both have been paid for)

- **A duplicated `runs-on` makes GitHub reject the whole workflow**, with
  `conclusion: failure` and **0 jobs**. PyYAML does not complain: in a map a
  repeated key is silently overwritten, so the YAML "validates" and the workflow
  does not exist for Actions. When touching these files, check for duplicate
  keys.
- **A run's logs are read from the ZIP**, not with `gh run view --log`: the `gh`
  API truncates them to ~300 lines and the failing step is usually after that.
  To see a specific step, `curl` with a token to `/actions/runs/<id>/logs` and
  unzip.
- **A new gate is untested until it has actually passed.** The coverage one
  failed twice in PR #19, and both times because of the workflow, not the code:
  `-coverpkg` was missing and the diff's base was a non-comparable ref.
- **Wait for a run with `gh run watch`, never with `sleep N; gh pr checks`.**
  There is no websocket nor SSE for job status: `gh run watch <id> --interval 5
  --exit-status` is the channel of record (it blocks with internal polling and
  exits with the run's code). Polling by hand costs minutes per run and also
  leaves you on *stale* runs: after a force-push you have to ask for the id
  again, because the previous one no longer describes the commit. With `watch`
  you ask for the id once and block.

On a merge: verify that main's `push` workflow went green and that the README
badge reports `passing` (the badge caches for a few seconds).

`make smoke` (tmux + pty, see Gotcha 3) stays **manual and out of CI**: it needs
an interactive terminal that the runner does not guarantee.

**Smoke trap**: it isolates `XDG_CONFIG_HOME`, and git reads its global config
from `$XDG_CONFIG_HOME/git/config`. With the user's config hidden, a `git pull`
on a diverged repo **fails** ("divergent branches") where in your terminal it
rebases, and it looks like a gitdash bug. To exercise the user's real policy,
link git's config into the isolated directory
(`ln -s ~/.config/git/config "$tmp/git/config"`).

## Architecture (data flow)

```
config → discovery (marker walk) → gitstatus (subprocess per repo, pool)
       → events over a channel → tui (Update/View) → render
```

| Package | Role |
|---|---|
| `internal/config` | XDG TOML. `Load()` never fails: defaults + a warning string |
| `internal/discovery` | `Project{Path,Name,Group,SyncBranch,HasRepo,IsWorktree,MainRepo,MarkerErr}`. The marker's folder IS the repo (there is no search for `.git` upwards). Prunes hidden dirs + `exclude`. `MainRepo` links worktree→main repo |
| `internal/gitstatus` | `parse.go` pure (ParsePorcelain, ParseWorktrees, Derive, Score) + `status.go` (Collect, StreamPool, Run, Fetch, RemoteURL, RebaseInProgress, RemoveWorktree) + `outcome.go` (Classify: what git really did). The `Snapshot` carries `Err` embedded and also the deviation vs the sync branch (`SyncBehind`) and its worktrees; it never fails hard. `runGit`/`runGitCombined` are the **only** place a git subprocess leaves from, and both leave an entry in the command log |
| `internal/forge` | Pure (no I/O): `RepoRef` + `ParseRemoteURL` (remote → forge/host/project, with the subfolder prefix), `WebURL`, `ForgeForHost`/`PublicHosts` (public hosts) and `BuildCreateArgv`/`CreateBin`/`PromptEnv` (the argv of `gh pr create` / `glab mr create`). The execution is NOT here: it is `internal/forge/tool` (Runner with a 30 s deadline and an `Error` carrying the exit code) |
| `internal/cache` | `repos.json` to paint instantly on startup; validated by the marker's existence; corrupt = silent |
| `internal/cmdlog` | Bounded in-memory ring (500) of what ran: `intent` entries (key) and `exec` entries (process with argv, exit, duration and result). Global with a no-op default; only the TUI installs it (`tui.New`) |
| `internal/tui` | `app.go` (model + background pipelines), `update.go` (Update/View/keys), `table.go` (rows/order/cells/grouping), `detail.go`, `proverlay.go` (PR form) + `prcreate.go` (its execution), `cmdlogpanel.go` (the log panel), `styles.go` |
| `internal/group` | Arrangement of the 2-level grouped view (vroom style): `Arrange` + `IsPrimaryHeader`/`IsSecondaryHeader` |
| `internal/testutil` | helpers to create real git fixture repos in `t.TempDir()` (bare origin, upstream push, worktrees, branches) |
| `cmd/gitdash` | `main.go` (TUI) + `print.go` (`--print` mode, tabwriter, same ordering) |

## Conventions

- Code comments in **English**, and only the ones that justify the **WHY** (a
  decision that is not readable in the code), never the HOW nor a godoc that
  repeats the name. The **intent** is one line each: if a comment needs more, the
  design goes in the sections below (or in `docs/`), not in the code.
  **What is enforced is a floor, not that intent** — `mutate_test.sh` section 22
  fails the build if a comment in `scripts/mutate.sh` or `scripts/mutate_test.sh`
  exceeds 170 characters, or if a contiguous run of comment lines is longer than 6
  (the header, the budget table and the section banners are what the 6 is for). So
  one line per comment is on the author and a wall of prose is a build failure; the
  machine does not count lines because a header and a table legitimately need more
  than one. A convention nobody checks is a comment. The two vendored files are the
  documented exception and the test asserts that exception rather than assuming it.
- **No references to specs or requirement/scenario IDs**: the code is the source
  of truth. There are no SDD artefacts in the repo and none are created
  (`proposal.md`, `spec.md`, specs with requirement/scenario IDs, `R#n SHALL`,
  `S#n.#`).
- `docs/planning/<NNNN>-<type>-<slug>/` is NOT an SDD artefact: it is the package
  the planning pipeline deposits as its scaffolding, so it is valid in the repo
  and is not removed.
- **Direct** model tests (build the Model, send msgs with Update, inspect state)
  — no teatest. Pattern: `internal/tui/app_test.go`.
- Derived states with precedence: `diverged > dirty > ahead > behind >
  detached > no-upstream > clean`. `State.Score()` (gitstatus) gives the
  attention-first ordering shared by the TUI and `--print`.
- Table cells return `(text, style)`: the render does `pad(text)` BEFORE
  applying the style (ANSI breaks the width calculation).

## Critical gotchas

1. **Event pump**: every `tea.Cmd` reads ONE event from the channel. ALWAYS
   rearm `waitForEvent` (the `withPump` helper) in Update after consuming an
   event from the channel. Without this only the first message arrives and
   states never paint.
2. **ahead/behind require a fetch**: remote-tracking refs are only updated with
   `git fetch`. Tests that simulate behind/diverged must call
   `testutil.FetchLocal` after pushing to the origin.
3. **TUI smoke tests**: use **tmux** (`capture-pane`). `script` does NOT work:
   bubbletea v2 blocks the first render waiting for the answers to the kitty
   capability queries of the dumb pty (symptom: blank alt-screen, live process,
   no stderr).
4. **porcelain v2**: `1 ` lines → 7 fields before the path; `2 ` (renames) → 8
   and it emits `<new>\t<old>`; `u ` → 9. `# branch.oid` gives the sha for
   detached. The path is EVERYTHING that is left (it can contain spaces).
5. **Fixtures**: the marker must be committed in the base commit, otherwise it
   shows up as untracked and dirties the state of every repo.
6. **Coverage CANNOT depend on the machine**: two traps that have already been
   paid for, and both make local and CI measure different things.
   - **Permissions**: a test with `0o000` measures one thing locally and another
     in CI, because root reads a `0o000` (and root runs on some runners, not on
     others). For "this file cannot be read" use **EISDIR**: a *directory* named
     after the marker. It fails the same and fails always.
   - **Installed tools**: a `t.Skip("lazygit not installed")` skips the whole
     path and its statements never reach the profile. For an external tool use a
     **stub in the PATH** (like `forgeStub` for the forge CLIs), not a skip.
   That is why `scripts/coverage-floor`'s floor is 100.00% and holds in both: it
   is measured with a PATH without lazygit, not assumed.
7. **textinput v2 with synthetic keys**: `tea.KeyPressMsg` needs `Code` AND
   `Text` — `Code` alone does not insert runes into the input.
8. **The pull policy is the user's, not ours**: `commands.pull` goes **without
   flags** on purpose. Flags on the command line override the gitconfig, so a
   hardcoded `--ff-only` was cancelling the user's `pull.rebase=true` (checked:
   the same diverged repo rebases with plain `git pull` and does not with
   `git pull --ff-only`). The variants with flags exist only for the `p`
   selector, which offers the explicit policy. **Do not reintroduce flags in the
   default.**
9. **A clashing `pull --rebase` is not a clean failure**: it leaves the rebase
   half-done (`rebase-merge`/`rebase-apply` in the worktree's dir). That is why
   `gitstatus.RebaseInProgress` exists and the warning has priority over the
   divergence/upstream hints: saying "failed" invites you to retry on top of an
   unresolved rebase. It is resolved with `git rev-parse --git-path`, not by
   looking at `.git/rebase-*` directly, because in a worktree `.git` is a file.
10. **The resolved argv travels in `actionMsg`/`actionResult`**: with the policy
    delegated to the gitconfig, the kind no longer implies the flags. The detail
    shows it; without that the UI lies about what it reconciled.
11. **Rows are ordered attention-first**: the cursor's position is NOT the
    fixture's. Tests that need a specific row locate it by path (`cursorOn`),
    not by index.

## Design gotcha: the pull selector

`p` does not run: it arms `pullArmed` with the captured path, and the **next**
key picks the variant (`p`/`r`/`f`/`m` in `PullKinds`, plus `a` for the AI
variant, which is not a git pull and is resolved separately). Two rules:

- The variant keys **clash with real actions** of the table (`p`=pull, `r`=rescan,
  `f`=fetch), so the armed state has to consume the key **before** the normal
  routing in `handleKey`.
- Any other key **cancels and carries on with its normal course** (it is not
  consumed): that is what stops the app from being stuck waiting for a second
  press. It is the prefix-key pattern, not a blocking mode.

The prompt is painted in the **keybinds section, replacing the hints**, not in a
toast: toasts expire after 3 s and the selector lives until the next key.
Keybinds is its place because it shares a function with the hints ("what do I do
now"), and the stats banner stays for the summary and the activity in progress.
The same applies to `removePrompt`: both warnings come out of `armedPrompt()`,
which returns one or the other, and `keybindsLines()` derives the height budget
from there (1 line with a warning armed, `defaultHintLines` without it) so the box
never measures more than its content. With a warning armed, `computeLayout`
degrades **stats before keybinds** (`keepKeybinds`): if the warning's box were to
fall, the app would be waiting for a key without saying which ones.

## Design gotcha: the preview panel

Below the table there is a card for the repo under the cursor (prdash style),
between the list and the keybinds. **It is the only detail view**: there is no
`enter` detail, no `detailSection`, no `detailOpen`. Decisions that are not
evident:

- **The title goes only on the border.** `detailTitle`/`worktreeTitle` compose
  the box's title and `renderDetail` does NOT repeat it as the first line:
  painted in both places, the same text came out duplicated right under the
  border.
- **The panel is additive.** `computeLayout` looks for the largest panel height
  that (a) leaves `minBodyLines` table rows and (b) does not force cropping the
  hints nor hiding stats/keybinds (`mismaChromeQue`). If there is none, the panel
  is not drawn. Below ~21 lines (default config) the dashboard is exactly what it
  was before the feature. That floor comes out of `detailHeadLines`, so
  `minPanelHeight` (test) derives it instead of begging for the number.
- **The share is measured over the FREE height**, not over the terminal: against
  the total, a 30-line window kept 12 for the card and 3 for the table.
- **The card's budget is its own lines**, not the terminal's: `renderDetail(r,
  rows)` reserves `detailHeadLines` for the state header and splits the rest with
  `listBudget`, which reserves the `… N more` warning line when the list does not
  fit whole. `rows` is `lay.previewLines`. Without that, the lists are counted as
  if they fit and then the box crops them without warning.
- **The path is a field, not a loose line**: `path` goes in the same key/value
  column as `branch`/`upstream`/`state`/`sync`, and its value is dimmed (it is
  context, not state). On its own line, with the gap that separated it, it took a
  height the lists need; as a field the header is `detailHeadLines` = 5 lines.
- **The card does not repeat the row's keys**: `g lazygit · ! cmd` are already in
  the keybinds section, so the card has no footer (`fichaTail` only adds the `!`
  input). Duplicating them cost a line of useful height and two sources that
  could diverge on a rebind.
- **The `!` input goes at the END of the card and is always visible**
  (`fichaTail`): if the card filled the box, the card is cropped from the top.
  Typing a command without seeing the prompt is typing blind.
- **The box is filled** (`fitLines`): the height comes from the layout, not from
  the card. Without the fill, a short card would push the keybinds up and the
  view would not fill the terminal.

With the cursor on a **group header** the panel has no card to show: it shows the
group's aggregate (`groupStats`, over `rows()` and before folding, which is what
its header counts). States at zero are not painted.

## Design gotcha: `enter` is the only folding key

`enter` (the `fold` action) folds **whatever is under the cursor**, and each row
has something different under it: a header folds its block, a repo row folds its
worktree subrows, and a worktree subrow has nothing to fold (no-op, and on
purpose: folding the parent's group from the subrow would be a surprise). There
is no detail view to open, so `enter` was free for this; `tab` (fold) and `space`
(expand) were removed as redundant.

Both states still persist in the same `collapsed.json` (worktrees under their
prefix), so folding survives across sessions.

The hints carry the action WITHOUT the key inside (`hintLabels`): `HintBarLines`
prepends the key. If the label carried it, a rebind would produce hints like
`w enter fold`. And `config.LoadFrom` warns (toast + stderr) about
`[keybindings]` actions that no longer exist: without that warning, a
`detail = "enter"` from an old config leaves `enter` dead and it looks like a TUI
bug.

## Design gotcha: the command log (`l`)

The argv **does not** say which pull policy git applied. `commands.pull` goes
without flags on purpose, so `p` `p` runs `git pull` and with `pull.rebase=true`
in the user's gitconfig that integrated with rebase. The log answers "what really
happened" with two pieces:

- **`gitstatus.Classify(args, out, exit)`** deduces the result from the output
  git already printed (`Successfully rebased and updated` → `rebase`,
  `Applied autostash` → `rebase+autostash`, `Merge made by` → `merge`,
  `Fast-forward`, `up to date`, `Not possible to fast-forward` → `diverged`,
  `could not apply`/`CONFLICT` → `conflict`…). Extra subprocesses: **zero**.
- The **intents** (key + action + repo) are recorded by `handleKey`, because the
  argv does not tell "I pressed p and picked rebase" from "the gitconfig decided
  for me".

**Do not probe `git config` to deduce the policy**: `branch.<name>.rebase`
overrides the global `pull.rebase`, that precedence changes across git versions
(`branch.<name>.rebase` is deprecated in favour of `branch.<name>.pullrebase`)
and gitdash would return a plausible and wrong answer. What git DID is in its
output, and with `LC_ALL=C` forced in `gitEnv` the messages are not localised.

**The reflog was discarded as a source** (checked with real git, not from
memory): plain `git pull` leaves `pull (start)/(pick)/(finish)` when it rebases,
`pull: Merge made by the 'ort' strategy.` when it merges and `pull: Fast-forward`
when it ff's, but it **writes no entry at all** when it was already up to date —
exactly the case where you ask "what happened with the pp?" — and it does not
tell a gitdash pull from a manual one in your terminal, nor cover
push/fetch/`!`/worktree remove. If it is ever wanted as a forensic mode, it is a
`git reflog show --date=iso` **on demand** (one call when opening a repo's
detail), not per action.

Panel rules (`internal/tui/cmdlogpanel.go`):

- It is a **view mode**, not an overlay: `logOpen` takes over the body and shares
  the detail's chrome. Its keys are consulted **before** the normal routing
  (like the armed states) because `j`/`k` clash with navigation; every other key
  carries on normally, so the app is not trapped.
- `promptLine()` (formerly `armedPrompt`) is the **only** place keybinds paints a
  warning: the two armed ones and the panel's legend. `keybindsLines()` and
  `keepKeybinds` derive from there, so adding a new warning is adding a `case`
  and nothing else.
- Opening the panel **drops the armed states**: the warning makes no sense
  outside its view, and leaving it armed would force you to guess the next key
  from a panel that is no longer there.
- `launchesCommand` / `actionNeedsRow` decide which actions leave an intent. The
  pure navigation ones (filter, folding, detail, the panel itself) do not: the
  log is about commands, not keys. Neither does having no row under the cursor (a
  `key p` with no repo and no `exec` is confusing).
- The offset counts **from the tail** (0 = the most recent at the end): in a log
  you look at the latest, and new entries do not move you if you were scrolled
  up. It is clamped against the visible lines and never leaves gaps.
- **The argv is untrusted text** (it carries the marker's prompt in `pull_ai`):
  `logLine` runs it through `sanitizeLogText` before painting it. Without that, an
  injected OSC/CSI sequence renders as is, a format character (bidi/zero-width)
  reorders the line and a multiline prompt breaks the panel's height (one entry =
  one line). The sanitising removes control (C0/C1/DEL), Cf (bidi, zero-width)
  and U+2028/U+2029, plus the ESC sequences, and it is **painting only**: the
  executed and the recorded argv are untouched.
- `computeLogColumns` degrades columns by value (verdict → result → repo → kind)
  and gives the argv whatever is left: below `logMinArgv` (20, what fits
  `git pull --ff-only`) the command is read half-way, which is exactly what the
  panel exists to avoid.

## Design gotcha: the visual preview's no-op guard (`v m` / `v r`)

git-sim **aborts with code 1** when the ref you pass is already contained in
HEAD: `merge.py` and `rebase.py` print `Branch 'origin/main' is already included
in the history of active branch 'main'` and exit. That is not a gitdash bug, it
is the correct answer, but reproducing it on screen costs a full terminal handoff
to read an error the snapshot already anticipated: `Status.Behind == 0` **is**
the condition git-sim checks (`git branch --contains <ref>`).

That is why the selector blocks with a toast before handing over the terminal, and
why `armedVisual` captures `behind` **when arming**, along with the path and the
upstream: the guard has to be decided on the chosen row, not on whatever is under
the cursor when the second key arrives.

- **The guard belongs to the variants with a ref (`merge`/`rebase`), not to the
  selector.** `pull` takes no positional argument and git-sim's `pull` really
  clones and simulates, without that check: with `behind == 0` it launches
  anyway. Guarding it would be inventing a restriction the tool does not have.
- **`ahead` does not loosen the block**: a repo at `↑2 ↓0` still has its upstream
  contained in HEAD, so git-sim would fail just the same.
- **The warning names the fetch key with `cfg.KeyFor("fetch")`**, never a
  hardcoded `f`: `behind` comes from the last fetch, so if the remote-tracking is
  stale the blocked simulation did have content and the warning has to say how to
  fix it. The dashboard's hints are painted the same way (label without key,
  `HintBarLines` prepends it); a toast does not go through there, and it is the
  only place where the key is written by hand.

## Design gotcha: opening a PR/MR (`O`)

`O` opens an overlay that collects title, body, base and draft (`proverlay.go`),
and `ctrl+s` sends it to `gh pr create` / `glab mr create` (`prcreate.go`).
Decisions that are not evident:

- **It is NOT a terminal handoff.** gh and glab are non-interactive with all the
  given flags, and `BuildCreateArgv` guarantees that (the body is always emitted,
  and glab carries its `-y`). So there is no TTY to hand over: the output is
  captured with `forge/tool.Runner`, like the `!` command. That, and only that, is
  why this action **does measure duration** in the command log: handoffs go with
  `Dur = 0` because measuring their process would require storing the start in
  the model.
- **The chain is long and every step cuts before executing**: remote →
  `ParseRemoteURL` → `BuildCreateArgv` → `LookPath` → execution. A PR created
  against the wrong repo does not fail visibly (gh would deduce the destination),
  so "I don't know where this comes from" is a toast that says what to do and
  NOTHING executed. The four rejections release `m.running[path]`, with no exec in
  the log and no recollect: nothing happened.
- **The remote is read on demand** (`gitstatus.RemoteURL`, `ClassRead`), not in
  `Collect`: adding it to the scan would be a `git remote get-url` per repo and
  per cycle for data almost nobody looks at. It goes through `runGit` like
  everything else, so it is auditable from the panel with `a` (show all).
- **The forge comes from the config, not from a heuristic**: `forge.ForgeForHost`
  only knows `github.com` and `gitlab.com`, and guessing the provider of an
  unknown host produces links that open 404 without anything failing.
  `Config.ForgeHosts()` and `Config.ForgePrefixes()` (`internal/config/forge.go`)
  resolve the two maps that `ParseRemoteURL` consumes; the public hosts come from
  `forge.PublicHosts()` so the list cannot be duplicated, and the self-managed
  ones are declared:

  ```toml
  [forge.gitlab]
  api_base = "https://git.example.com/git/api/v4/"
  hosts = ["git.example.com"]
  ```

  An **absolute `api_base` names the host it applies to**, and its path yields the
  subfolder prefix (`/git/api/v4/` → `git`, with `forge.PrefixFromAPIBase`): that
  is what makes a GitLab at `/git/` resolve `group/sub/widget` and not
  `git/group/sub/widget`. A relative `api_base` names no host and applies to all
  of that provider's; the hosts it does not name keep the provider's default. A
  provider we do not support warns on load instead of being accepted silently.
- **The argv is recorded RAW and sanitised by whoever paints it.** A person wrote
  the title and body and they end up in the log panel, so they go through
  `sanitizeLogText` (see the command log section). The record keeps what ran, as
  is: a "cleaned" log can lie.
- **Accepting and executing are two steps.** The overlay publishes the submit in
  `m.prPending` and returns a `tea.Cmd` that emits `prStartMsg`; `prCreateCmd`
  consumes it. The seam exists so a test can see the submit accepted with no
  process having gone out, which is the half a handoff does not have.
- **The intent is left by the generic routing**, like every `commandActions`
  action: `key O pr` in the log, and below it the exec with the argv. With the log
  panel open the key is in the guard that prevents opening overlays (otherwise it
  would leave an intent for something that did not happen).
- **The open key is resolved by the config** (`cfg.KeyFor("pr")`) down to the
  panel's warning: a fixed text would leave the user being lied to after a
  rebind. The submit key (`ctrl+s`) does NOT come from the config because it is
  not a rebindable action.

## Design gotcha: the AI pull (`p a`)

The `p` selector has a fifth variant, `a`, which hands over to the configured AI
command. `p a` **launches straight away** (no preview and no confirmation: the
second key is the decision). Decisions that are not evident:

- **The trust boundary is the axis of the feature**: the executable/argv comes
  ONLY from the global config (`[ai.pull] command` in
  `~/.config/gitdash/config.toml`); the committed marker (`.gitdash.toml`,
  untrusted input) contributes ONLY the prompt text. That text goes in as **a
  single argv element** (`config.BuildAIArgv`), never interpolated into an
  `sh -c`. If you duplicate that substitution elsewhere you break the boundary.
- **`pull_ai` is NOT a `PullKind`**: `PullKinds` feeds
  `startActionCmd → cfg.CmdArgs → gitstatus.Run` and the `RebaseInProgress`
  guard, which are git paths. `pull_ai` is resolved as an explicit `case` for
  `a` inside the `pullArmed` block of `update.go` (the armed state consumes the
  key before the normal routing, like p/r/f/m).
- **The prompt is re-read on demand** with `discovery.MarkerPrompt` and is **not**
  stored in `discovery.Project`: that way it does not bloat `repos.json`, does not
  go stale after editing the marker and is resolved on the press, not per frame.
- **`pullOptions` is the single source of variants**: `PullKinds`, `pullPrompt()`
  and `pullVariantLabel()` derive from it. The latent bug it fixes is real: the
  prompt had `[]string{"p","r","f","m"}` hardcoded and a new variant did not
  appear.
- **A half-done rebase does not block the AI variant**: with no preview there is
  nothing to surface, and resolving the rebase may be exactly the prompt's
  intention. `RebaseInProgress` is not consulted (unlike the git pulls).
- **Handoff without timeout nor capture**, like lazygit: the terminal belongs to
  the child and on return `execDoneMsg` records the exec in the command log
  (`Dur=0`, argv with the whole prompt) and re-collects the state. With no
  prompt, no command or no binary (`exec.LookPath`) there is only a toast.

## Wiring gotchas

- **Every git exec goes through `runGit`/`runGitCombined`** (`gitstatus`), which
  measure and record. If you add a new git verb outside them, it does not appear
  in the log. The 6 `exec.Command`s of `tui/app.go` (editor, lazygit, `pull_ai`,
  visual, `!`, shell) are outside: they are recorded by hand, in `execDoneMsg`
  (handoffs, on return) and in `openCmdCmd` (the `!`, which does measure
  duration). Handoffs go with `Dur = 0`: measuring them would require storing the
  start in the model. `gh`/`glab` are not git: they leave through
  `forge/tool.Runner` and `prCreateCmd` records them, also by hand (and it DOES
  measure).
- **`gitstatus.Fetch` takes the `cmdlog.Class` from the caller**: `git fetch
  --prune` is the same command the automatic scan and the `f` key launch, and
  only the origin separates them. It is the only exec whose class is not deduced
  from the argv.
- `RemoveWorktreeArgv` exists so the log, the detail and `RemoveWorktree` cannot
  disagree. If you build the argv somewhere else, the log can lie about what ran.
- `forge.BuildCreateArgv` is the single source of the creation argv (and
  `forge.CreateBin`/`PromptEnv` the ones of the door and the host): if you build
  them elsewhere, the log can show a command that is not the one that ran.

## Trying it

```bash
# the user's real config (default roots: ~/dev)
bin/gitdash
```

```bash
# isolated against the fixtures
XDG_CONFIG_HOME=$(mktemp -d) bin/gitdash --print
# with a config: create <tmp>/gitdash/config.toml with roots=["<repo>/testdata/playground"]
```