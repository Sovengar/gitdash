# TODOLIST — coverage to 100%

Goal: no statement of the module left unexecuted, and a CI gate that prevents it.

## How to measure (do not trust bare `go test -cover`)

`go test -coverpkg ./...` writes **each block once per test binary** (13 packages
here, and in practice 22 copies). Adding them raw gives ~10% and means nothing.
You have to deduplicate by range and keep the maximum `count`:

```bash
go clean -testcache && go test -coverpkg ./... -coverprofile=/tmp/cov.out ./...
# -> deduplicate by block, max(count); covered/total
```

State measured when this document was opened: **96.67% (2697/2790), 85 blocks
uncovered**. Progress: **98.35% (2750/2796), 41 uncovered**.

Target: 2790/2790.

## Rule of this todolist

- A box is marked `[x]` **only** when the block has `count>0` in a deduplicated
  profile, not when the test passes.
- A passing test does not prove coverage: `TestToastEmptyNotIsQueues` failed
  first because of a misunderstood case. Measure after writing.
- If a block turns out to be unreachable (see §unreachable), it is documented here
  with the why and explicitly excluded from the gate. It is neither deleted nor
  glossed over.

---

## [x] Starting point

- [x] Measure the real coverage with the deduplicated denominator (it was 94.9%, not 100)
- [x] Classify the 132 blocks: real `count=0` / gremlins bug / no block

## [x] Package `const` — Go does not instrument them (8 mutants)

- [x] `actionTimeout`, `commandTimeout`, `toastDuration` → one-line functions
- [x] `tool.DefaultTimeout` → function
- [x] `prMinBodyLines`, `prMinWidth` → functions
- [x] `TestDeadlinesInUnits` asserts in units, not against the source

## [x] The handoff seam (the TUI's ceiling)

- [x] `handoff handoffFunc` field in `Model`, default `tea.ExecProcess`
- [x] `handoffSpy` that does not give up the terminal and exposes `exit(err)`
- [x] The 5 handoffs executed whole: editor, lazygit, shell, `p a`, visual

## [x] Branches that existed and nobody looked at

- [x] `renderGroupSummary`: `errors` and `wt` (never enabled)
- [x] `activityIndicator`: `case m.scanning` (`New` leaves `scanning=true`)
- [x] `renderGroupSummary` in worktree mode

---

## Pending per file

### `internal/tui/update.go` — 13 → 4 left

- [x] `31.23,34.16` — `spinner.TickMsg` (it has to be sent by hand: the spinner emits it)
- [x] `221.2` — `return m, nil` for an unknown msg
- [x] `421,432` — `!HasRepo` and no row in the `!` command + `enter`
- [x] `592,596` — `busyActionCmd`: busy and free
- [x] `635` — `toggleFold` with no row (no-op, cursor still)
- [x] `770,783` — `visualPrompt`/`removePrompt`/`promptLine` with nothing armed
- [ ] `66.74,68.5` — `statusMsg.err` / `rebaseInProgress`
- [ ] `538.22` — `sendEvent` of a collect that did finish
- [ ] `711` — `vars["branch"]` of a worktree

### `internal/tui/app.go` — 22 → 15 left

- [x] `200` — `visualOptionForKey` with an unknown key
- [x] `412` — `NotifyConfig` (the warning has to be visible, not go to stderr)
- [x] `616` — `removeWorktreeCmd` with the parent busy
- [x] `647` — `recollectCmd` with the repo busy
- [x] `701` — `openLazygitCmd` with the repo busy (order of the two guards)
- [x] `930` — `startVisualCmd` with the repo busy
- [x] `998` — `execExit` with a real `*exec.ExitError` (exit 3)
- [x] `1013` — `logIntent` with the recorder off
- [x] `1066` — `saveCollapsed` with a nil store
- [ ] `417` — `Init()` (starts the scan + the 1s ticker)
- [ ] `430` — `startScanCmd` with an event that is not `scanMsg`
- [x] `446` — `tickCmd` (its `tea.Cmd` is invoked: real 1s, and it emits the tick)
- [ ] `468` — `ctx.Err() != nil` in the collector
- [ ] `477` — `sendEvent(collectDoneMsg)`
- [ ] `487` — `!p.HasRepo` in the automatic fetch
- [ ] `494` — repo already in `fetchStates["fetching"]`
- [ ] `766` — worktree not found
- [ ] `789` — unreadable marker in `p a`
- [ ] `821` — `openPullAICmd`, happy path with the seam
- [ ] `835` — `pullAIArgv` with an error
- [ ] `879` — `startVisualCmd`, happy path with git-sim in PATH

### `internal/tui/table.go` — 0 · `detail.go` — 0 · `sections.go` — 2

- [x] `table.go` — whole `styleFor`, header glyph, `worktreeHidden` in
      `summary`, `repoExpanded` induced by search, `(detached)` with no sha
- [x] `detail.go` — detached with a branch, empty branch, no upstream, worktree in another root
- [ ] `sections.go 128` — `onlyDirty` in the stats header
- [ ] `sections.go 289` — `[worktree]` in the card's title

### `internal/tui/proverlay.go` — 0

- [x] `193` — `openPR` with `logOpen` (the guard's second net)
- [x] `408` — `prParams()` with `m.pr == nil`
- [x] `492` — `draftLabel` with draft true
- [x] `507` — `prPrompt()` with `m.pr == nil`

### `internal/tui/toast.go` — 3

- [ ] `183` — `head == ""` in the width cut (progress safeguard)
- [ ] `206` — `len(lines) == 0` after wrapping
- [ ] `248` — zero-height block in the stacking

### `internal/tui/prcreate.go` — 2 · `cmdlogpanel.go` — 1 · `bordered.go` — 0

- [ ] `prcreate.go 62` — `sub == nil` when resolving the base flag
- [ ] `prcreate.go 101` — `bin == "" || len(argv) == 0`
- [ ] `cmdlogpanel 176` — empty `cmdline` → `-`
- [x] `bordered 90,93` — empty `leftChar`/`rightChar`

### `cmd/gitdash/` — 11 → 4 left

- [x] `main.go 52,57,58` — `depsProd`: the five functions are not nil and `load` really reads
- [x] `main.go 65` — `run(printMode, eout)` with real deps
- [x] `print.go 51,57,62,65,69` — duplicated worktree, `[wt]` suffix, detached,
      empty branch, worktree counter (`printRowOf` was extracted so they could be
      tested over fabricated rows: a real HEAD cannot be left detached)
- [x] `main.go 21` — `func main()`, in a SUBPROCESS (`go build -cover` + exec +
      `GOCOVERDIR` + `covdata textfmt`, and the test FAILS if the profile does not
      carry the block). **Careful**: exercised for real, but its counter does not
      reach the combined profile of `-coverpkg ./...` that the gate uses, so it
      reads 0 in the global metric. A subprocess's counters have to be merged by
      hand.
- [x] `main.go 57,58` — the two `depsProd` closures
- [ ] `print.go 29` — `discovery.Scan` error to stderr

### `internal/discovery/scan.go` — 7 → 5 left

- [x] `47,57` — nonexistent roots and ones that are a file (aggregated error)
- [x] `85` — `WalkDir` over a directory without permission (it skips, it does not abort)
- [x] `173` — `MarkerPrompt` with a nonexistent marker (empty, not an error)
- [x] `187` — `MarkerPrompt` with a malformed marker (yes, error)
- [x] `222` — `.git` unreadable due to permissions
- [ ] `100` — the walk that ABORTS (returns an error, not just skips)

Note: the permission tests are skipped if the process runs as root, which cannot
be prevented from reading a 0o000. Here it is not root, but in a CI container it
can happen: if they get skipped, the block is uncovered again with nobody noticing.

### `internal/gitstatus/status.go` — 2 · `state.go` — 2 · `cache.go` — 1

- [x] `status.go 140` — `syncBehind` on a nonexistent ref → `known=false`
- [x] `status.go 176` — `StreamPool` with the context already cancelled
- [x] `status.go 333` — `FailureReason` with no output / hints only
- [ ] `status.go 263` — `rev-parse` returning empty
- [ ] `state.go 70` — `json.Marshal` failing (unserialisable map)
- [ ] `state.go 78` — `os.WriteFile` failing
- [ ] `cache.go 106` — `json.MarshalIndent` failing

### `internal/config/config.go` — 1 · `internal/forge/parse.go` — 1

- [ ] `config.go 417` — `label = key` **done with `t.Cleanup`, re-verify**
- [ ] `parse.go 70` — `RepoRef` with a known `Host` and no segments

---

## Unreachable (decide before the gate)

No test covers these without changing the code or the metric:

**Nothing here ended up unreachable.** The three cases that had been written off
as lost come out, and the lesson is that "it cannot be tested" and "I never
thought about it" look a lot alike:

1. ~~`internal/testutil` — 8 `t.Fatal` blocks.~~ **Resolved, 8 → 1.** The helpers
   take a `TB` (an interface with `Helper`/`Fatal`/`Fatalf`/`TempDir`) instead of
   `*testing.T`, and `MockTB` records the failure instead of killing the process.
   `*testing.T` satisfies the interface, so the repo's 160 calls do not change.
   The failure is provoked FOR REAL: a file where the directory should be gives
   ENOTDIR.
2. ~~`func main()`.~~ **Resolved** (see `cmd/main.go` in the todolist).
3. ~~`tea.Tick`.~~ **Resolved**: `tea.Tick` returns a `tea.Cmd` that can be
   invoked directly. It costs 1s per test and runs the closure.

What **does** stay outside the goal, and not because it is unreachable:

4. **The 83 mutation NOT COVERED.** They are not coverage: 36 are a bug in
   gremlins' lookup (see `WATCHDOG-PLAN.md` §0) and 47 are `case` conditions,
   which Go instruments from the body's column. The `switch`→`if` refactor was
   tried and **does not bring them down**.

---

## The gate — DONE

Two layers, because they cover different things and one without the other leaves
a hole. Green and verified with 6 cases.

- [x] `scripts/diff-coverage.sh` — this change's diff at 100% (whoever touches the
      code tests it; the old code does not block a PR)
- [x] `scripts/coverage-floor` — the total's floor, committed, only goes up
- [x] `make coverage` / `make coverage-check` — `-coverpkg ./...` + the profile of
      `main()`'s subprocess
- [x] CI step, with `fetch-depth: 0` (without history the diff is a no-op and the
      gate approves in silence)

State: **diff 100% (368/368) · total 98.17% · floor 98.17%**

The failures it had and how they were detected, because they were all **silent
passes**:

| | |
|---|---|
| wrong module prefix (`gitdash//`) | no path matched → "no lines touched" → passed |
| comments counted as statements | the diff came out 55% instead of 99% |
| a nonexistent extra profile was ignored | the gate passed without `main()`'s profile |
| CI without history | `git diff main...HEAD` empty → passed |
| floor compared as a raw float | it failed by 0.001 |

The first two were caught by looking at the output; the other three by proving
that the gate **fails** when it should (with `--min 101`, with the floor at 99.99,
with no profile, and by adding a function with no tests).

### What is left of this document

There is still coverage to raise (`app.go`'s event pump above all), but it
**does not block the gate**: none of it is in any PR's diff. The diff's 100% is
reachable by construction, because it only looks at what is touched.