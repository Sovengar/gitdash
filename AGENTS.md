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

## New feature → docs/FEATURES.md

Any **new feature** — and any user-visible change to an existing one — must be
documented in `docs/FEATURES.md` **in the same change** (create the file if it does
not exist yet): add or update its entry with what it does and how it is
triggered (key, flag or command). A feature that is not in `docs/FEATURES.md` does
not exist for the next reader. Keep it a concise inventory, not a tutorial: the
details live in `README.md` and `docs/adr/`.

## CI and `main`'s protection

Two workflows, three required checks: `Lint`, `Test` and `Mutation`. Neither
workflow uses `paths` filters.

### `CI` (`.github/workflows/ci.yml`) — every PR, and pushes to `main`

The gate. `Mutation` is a job of this file.

### `CI fast` (`.github/workflows/ci-fast.yml`) — every commit on a branch

Build + unit tests, no lint, no mutation. Not required: a red never blocks a
merge and a green never authorises one. Integration tests, when they arrive, go
in `ci.yml` (dbx already does it behind `DBX_SKIP_DOCKER=1`).

Two jobs, not three: `Lint` ∥ `Build`+`Test`, so a run asks for two runners
instead of three.

- **`Lint`**: `make lint` → fmt-check (gofmt) + golangci-lint **v2.13.2** (pinned
  in the Makefile; no `.golangci.yml`, so the default set: errcheck, govet,
  ineffassign, staticcheck, unused). `make vet` stays as a manual target. Longest
  job (~112s) and therefore the one that sets the wall clock.
- **`Test`**: four steps.
  1. `go build ./...` — the compile gate.
  2. `go test -race -count=1 -coverpkg ./... -coverprofile=coverage.out ./...`
     (the whole suite, no `-short`). Fixtures are built under `t.TempDir()` with
     `internal/testutil`, so CI needs no `make fixtures`.
  3. `Coverage of the subprocess`: `go build -cover` + `GOCOVERDIR` for `func
     main()`, which calls `os.Exit` and cannot run inside the test binary.
     `-coverpkg ./...` is what puts the cross packages in the profile (97.30%
     without it, 98.17% with it).
  4. `Coverage gate` → `scripts/diff-coverage.sh`: the PR's diff at 100% and the
     total against `scripts/coverage-floor` (only goes up). The base is the
     explicit merge-base, hence `fetch-depth: 0`.

### `Mutation` — a job of `ci.yml`, non-draft PRs only

Runs on non-draft PRs. The job always reports — no `needs:`, no measurement `if:`,
no `continue-on-error` — which is what makes it a required check, and its name is
exactly `Mutation` because the `protect-main` ruleset demands that text.

Mutation policy: make mutate-all

The gate lives in `scripts/mutate.sh` (measure **and** decide in one step); the
workflow only brings paths, refs and budget. Green means the mutation was measured
and no new survivor is left untested: *"could not measure"* is red, and so is an
expired mutant that `.mutation-timeouts` does not record. The reason of a red is in
the step summary.

Budget — mandatory under `--ci`, asserted at startup and again against the clock:

```
2 * CAP < STALL < CEILING        CEILING + SETUP_RESERVE < JOB_CEILING
    60s      180s     300s                   300s + 150s       480s
```

Who owns what:

- **`scripts/mutate.sh`**: scope precheck, warm-up, coefficient, forbidden flags,
  denominator, supervisor call, run log, exit code.
- **`scripts/watchdog.sh`** + **`scripts/watchdog_test.sh`** (vendored from chezmoi):
  cutting on a stall or on the ceiling, and nothing else. Frozen copy — a divergence
  is a PR in the other repo; both are exempt from the comment convention and nothing
  checks their bytes (a golden hash would make an upstream sync a chore).
- **`.mutation-allowlist`**: compared **by line** — an entry naming the file at
  another line does not cover the mutant that appeared.
- **`.mutation-timeouts`**: `<file> <ceiling>` for hangs the engine cannot finish; a
  green that runs over one says so with `count/ceiling` per file.
- **`scripts/mutate_test.sh`** and **`scripts/watchdog_test.sh`**: run as the `Shell
  suites` step. The verdict is a pure function of paths, so the red paths are
  fabricated in a `mktemp -d` in under a second, with no engine and no network.

Scope, counts and inputs:

- The scope is the **committed** diff against the merge-base; a `git diff` that
  cannot be computed is an error, not an empty scope.
- Two counts, and they are different numbers: `WATCH_LINES` (every mutant considered
  = the supervisor's denominator) and `EXPECTED_MEASURED` (in-scope only = what the
  verdict compares against the report).
- "Nothing to mutate" comes from the prior count, never from a missing report; a
  `_test.go`-only diff still produces `report.json`, with total 0.
- The coefficient is `ceil(CAP / elapsed)`, `elapsed` measured on the dry-run;
  gremlins has no absolute per-mutant cap flag.
- The run log is written and never deleted, carries the supervisor's stderr, and is
  cross-checked against itself (aggregate vs lines) before `.mutation-timeouts`
  judges the expiries.
- `jq` is a precondition and the report is parsed before it is read: empty output
  read as "zero new survivors" is the green this check forbids.
- Local loops: `make mutate-all` (whole module) and `make mutate-all-diff` (diff).

What the gate does NOT cover: only the PR's diff (the whole module is the manual
loop), it cannot say *what* expired a mutant, and there is no local equivalent of
CI's diff profile.

`main`'s rules (ruleset **`protect-main`**, reproducible with
`scripts/setup-repo-protection.sh`, idempotent and with `--dry-run`):

- Merge only via PR with `Lint`, `Test` and `Mutation` green; force-push and
  deleting `main` are blocked.
- There is an admin bypass and it is deliberate: the working intention is always
  the PR path.
- `delete_branch_on_merge=true`: GitHub deletes the remote branch on merge.

### Waiting for CI

To follow a PR's checks, wait with `gh run watch <run-id> --exit-status` (or
`gh pr checks <n> --watch`). Never `sleep` + `gh pr checks`: runs go stale after
a force-push and the id has to be asked for again.

## Architecture (data flow)

```
config → discovery (marker walk) → gitstatus (subprocess per repo, pool)
       → events over a channel → tui (Update/View) → render
```

| Package | Role |
|---|---|
| `internal/config` | XDG TOML. `Load()` never fails: defaults + a warning string |
| `internal/discovery` | `Project{Path,Name,Group,SyncBranch,HasRepo,IsWorktree,MainRepo,MarkerErr}`. The marker's folder IS the repo (there is no search for `.git` upwards). Prunes hidden dirs + `exclude`. `MainRepo` links worktree→main repo |
| `internal/gitstatus` | `parse.go` pure (ParsePorcelain, ParseWorktrees, Derive, Score) + `status.go` (Collect, StreamPool, Run, Fetch, RemoteURL, RebaseInProgress, RemoveWorktree) + `outcome.go` (Classify: what git really did). The `Snapshot` carries `Err` embedded and also the deviation vs the sync branch (`SyncBehind`), its commits (`SyncCommits`) and its worktrees; it never fails hard. `runGit`/`runGitCombined` are the **only** place a git subprocess leaves from, and both leave an entry in the command log |
| `internal/forge` | Pure (no I/O): `RepoRef` + `ParseRemoteURL` (remote → forge/host/project, with the subfolder prefix), `WebURL`, `ForgeForHost`/`PublicHosts` (public hosts) and `BuildCreateArgv`/`CreateBin`/`PromptEnv` (the argv of `gh pr create` / `glab mr create`). The execution is NOT here: it is `internal/forge/tool` (Runner with a 30 s deadline and an `Error` carrying the exit code) |
| `internal/cache` | `repos.json` to paint instantly on startup; validated by the marker's existence; corrupt = silent |
| `internal/cmdlog` | Bounded in-memory ring (500) of what ran: `intent` entries (key) and `exec` entries (process with argv, exit, duration and result). Global with a no-op default; only the TUI installs it (`tui.New`) |
| `internal/tui` | `app.go` (model + background pipelines), `update.go` (Update/View/keys), `table.go` (rows/order/cells/grouping), `layout.go` + `sections.go` (the single height+width budget and the pane composition), `detail.go` (split card, no commits), `proverlay.go` (PR form) + `prcreate.go` (its execution), `cmdlogpanel.go` (the log panel), `styles.go` |
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
  is not drawn. Below 22 lines (default config) the dashboard is exactly what it
  was before the feature. That floor comes out of `detailHeadLines`, so
  `minPanelHeight` (test) derives it instead of begging for the number.
- **The share is measured over the FREE height**, not over the terminal: against
  the total, a 30-line window kept 12 for the card and 3 for the table.
- **The card's height budget is its own lines**, not the terminal's: `renderDetail(r,
  rows, width, split)` reserves `detailHeadLines` for the state header; the collapsed
  branch then shares the remaining height between the lists with `listBudget`, which
  reserves the `… N more` warning line when a list does not fit whole. `rows` is
  `lay.previewLines`. Without that, the lists are counted as if they fit and then the
  box crops them without warning.
- **The card is two columns when the inner width is at least 61** (terminal width
  >= 63): `m.layout()` sets `cardSplit` and `renderDetail` consumes it, so the
  decision has a single source. The columns are NOT fixed: `cardColumns(inner)`
  gives the lists 2/5 of the inner width with `cardRightWidth` (24) as floor and
  the fields the rest with `cardLeftWidth` (36) as floor. A fixed 36-cell left
  clipped `path`/`upstream` while the unused rest of a wide card stayed empty. The
  right column owns the worktrees and files lists, each with its own budget so an
  oversized worktrees list does not starve the files list of its `… N more`.
  `renderDetail` pads both columns to the same height and joins them with a dim
  separator before the box styles anything, or ANSI breaks the width. With the
  right column empty (no worktrees, no files) the separator is NOT painted — a
  lone gray line with nothing at its right — and the fields take the whole inner
  width; with it non-empty the separator runs down to the card's bottom (or to
  the full-width tail), not only to the last list item. `activity` (the
  last-commit age) is what raised the head from 5 to `detailHeadLines` = 6 lines.
- **The commits block left the card** for the top-right panel (see below). The
  card never paints a commits list, at any width.
- **The diagnostics and the action/command tails span the full card width**,
  appended below the two columns: the argv and the command output need more than
  the fields column to be readable.
- **The path is a field, not a loose line**: `path` goes in the same key/value
  column as `branch`/`upstream`/`state`/`sync`/`activity`, and its value is dimmed
  (it is context, not state).
- **The card does not repeat the row's keys**: `g lazygit · ! cmd` are already in
  the keybinds section, so the card has no footer (`fichaTail` only adds the `!`
  input). Duplicating them cost a line of useful height and two sources that
  could diverge on a rebind.
- **The `!` input goes at the END of the card and is always visible**
  (`fichaTail`): if the card filled the box, the card is cropped from the top.
  Typing a command without seeing the prompt is typing blind. It is joined after
  the columns so it spans the full card width.
- **The box is filled** (`fitLines`): the height comes from the layout, not from
  the card. Without the fill, a short card would push the keybinds up and the
  view would not fill the terminal.

With the cursor on a **group header** the panel has no card to show: it shows the
group's aggregate (`groupStats`, over `rows()` and before folding, which is what
its header counts). States at zero are not painted.

## Design gotcha: the COMMITS panel (top right)

To the right of the table there is a panel with the commits of the row under the
cursor. Decisions that are not evident:

- **It is additive on WIDTH, not height.** `max(30, min(width/3, width-minTableWidth()-gap))`
  cells plus a 1-cell gap: the 30 floor keeps the old `119` boundary, the share
  buys the commit subjects a readable width, and the cap at the table's minimum
  keeps the fit gate a safety net instead of a dead zone where the share would
  drop the panel. It shares the table's box height and gives back nothing, so
  terminal lines stay conserved at every height.
- **It is dropped, never fallback.** Below the width boundary there is no commits
  panel and the card does not recover the commits block: the behavior says the
  commits "are not shown anywhere", so a fallback would be a second source of
  truth.
- **The title carries the "commits · " prefix** because the card is already
  titled with the repo/group/worktree name; without the prefix two boxes shared
  the same title.
- **Dispatch by row kind**: repo → `Snap.Commits` and, when the repo declares a
  DIFFERENT sync ref with commits, `Snap.SyncCommits` as a second group; worktree
  subrow → its own snapshot's commits (and its own sync group) when the path is
  discovered and live, a dim placeholder otherwise (never the parent's); group
  header → the aggregate text shared with the card (`groupSummaryText`, without
  `cardTail`); no row → the same empty hint as the table and card.
- **Two labelled groups share the height**: `<branch> (current)` on top and
  `<sync> (sync)` below, each with `(rows - headers)/groups` lines and at least
  one commit, so the second group is not clipped away by the first. The sync
  commits come from a second `git log -5` on the SAME ref the divergence counts
  (`SyncFor`/fallback resolved, never a `git config` probe), so what is compared
  and what is shown cannot disagree; the group is skipped when the sync IS the
  current branch (the lists would duplicate) or when the log failed (no ref).
  `Collect` pays that read only where a sync branch is declared.
- **Rendered with the same `sha / age / subject` line as the old card block**,
  newest first, subject truncated to the panel's inner width (never wrapped).

## Design gotcha: the table's fetch slot

`ACTIVITY` left the table (it is now the card's `activity` field) and `FETCH` is
not a column: the transient glyph lives in a fixed 2-cell slot right after the
2-cell cursor prefix, reserved on every row kind (repo, worktree, header). Names
never shift while a fetch runs, and the worktree/header left edge matches the
repo rows. `headerColumns` and the row render slice the columns with
`fitColumns(width - 6)`: the 6 is the 2 borders plus the 4-cell prefix, and it is
what makes the panel's 119 boundary exact.

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

## Design gotcha: the sync action (`s`)

`s` updates the current branch with the repo's sync branch: an explicit
`git fetch origin`, then the configured `[commands] sync` base with `origin
<resolved sync>` appended. It reports through a toast, never the card's action
block. Decisions that are not evident:

- **The fetch is explicit and first.** `s` reconciles against a fresh
  remote-tracking ref, so it runs `git fetch origin` (class action, deliberately
  not `commands.fetch`) before the pull; a failed fetch short-circuits, which is
  why the same action can leave one exec or two in the log.
- **The base is configurable, the ref is not.** `gitstatus.SyncArgv` copies
  `[commands] sync` and appends `origin <sync>`: the base is the user's pull
  policy, the ref comes from the repo (`SyncFor`, marker `sync_branch` over the
  global default). A static command string could not carry the per-repo ref.
- **The refusal reads the snapshot, not a live ref.** On the sync branch there is
  nothing to catch up to, so `s` refuses before any process leaves — but only
  when the last snapshot's `Branch` equals the resolved sync; an uncollected,
  stale or detached branch does not refuse and git's own result governs. The
  intent is still recorded, so a refusal leaves an exec-less log that still
  explains the keypress.
- **Toast-only, verdict classified.** `toastOnlyActions` is the single source of
  the sync policy: it drops the card block, drops the argv from the note and
  appends the classified outcome on success only. The outcome comes from
  `Classify` on git's own output, never from probing `git config`.
- **`isRebaseKind` splits selector membership from rebase semantics.** `IsPullKind`
  is the `p` selector; `isRebaseKind` = pull kinds plus `sync`, so the sync gets
  the spinner and the mid-rebase warning after a conflicted rebase without
  joining the selector.
- **A worktree sub-row uses the global default unless it is itself discovered.**
  An undiscovered worktree path is not in `m.projects`, so `syncOf` falls back to
  `cfg.SyncBranch` (the parent's marker is not consulted) and, having no snapshot
  of its own, the refusal cannot fire; a worktree path with its own marker
  resolves to that project and its snapshot, so the refusal can.

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