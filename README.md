# gitdash

[![CI](https://github.com/Sovengar/gitdash/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Sovengar/gitdash/actions/workflows/ci.yml)

A git status dashboard in TUI form, GitHub Desktop style: every repo you mark
with `.gitdash.toml` in one dashboard, with branch, pending changes (dirty) and
**↑ahead / ↓behind** (what is to push and to pull at a glance), plus quick
actions.

Inspired by [bircni/git-statuses](https://github.com/bircni/git-statuses)
(Rust, one-shot CLI) — this is not a fork: the scanning idea and the data model
were reimplemented in Go + Bubbletea v2 as an interactive TUI, with discovery by
**marker file** instead of hunting for a loose `.git`, and **automatic batched
fetch**.

## Install

```bash
go install gitdash/cmd/gitdash@latest   # or, from the repo:
make install                            # installs into ~/.local/bin (honours PREFIX/DESTDIR)
```

Requires the `git` binary in PATH (subprocess; no cgo and no git libraries).

## Usage

```bash
gitdash            # TUI
gitdash --print    # one-shot table on stdout (handy for scripts/debugging)
```

Marker file `.gitdash.toml` at the root of each project (every field optional):

```toml
name = "api"                # displayed name (default: the directory's name)
primary_group = "vsocial"   # primary group (level 1, foldable)
secondary_group = "backend" # secondary group (level 2, foldable, inside the primary)
sync_branch = "main"        # reference branch for the SYNC column (overrides the global)

[ai.pull]
prompt = "fix the rebase that was left half-done"   # text your AI command receives
```

### Config

`~/.config/gitdash/config.toml` (all optional, these are the defaults):

```toml
marker      = ".gitdash.toml"
roots       = ["~/dev"]
exclude     = ["node_modules", "target", "vendor", "dist", "build", "out",
               "coverage", ".venv", "__pycache__", ".gradle", ".terraform"]
editor      = "vi"        # $EDITOR when it is set
sync_branch = "main"     # reference branch for the SYNC column

[fetch]
auto        = true    # automatic fetch after every scan/rescan
concurrency = 4       # fetches in parallel (batches)
timeout     = "30s"   # timeout per fetch

[ai.pull]
command = "jcode -run {prompt}"   # executable behind the selector's AI pull variant
```

The AI command is **opt-in and the executable always comes from the global
config** (the marker travels with the repo and is not trustworthy): the
`.gitdash.toml` only contributes the `prompt` text. That text goes in as **a
single argument** of the process, never interpolated into an `sh -c`, so spaces,
quotes or `$` are not interpreted. Optional context placeholders: `{prompt}`,
`{branch}`, `{upstream}`, `{state}`, `{ahead}`, `{behind}`, `{sync}`.

## The panel below the table

Between the repo list and the shortcuts there is a **card for the repo under the
cursor** (prdash style). It follows the cursor, so `j`/`k` read the repos as they
go by. **It is the only detail view**: there is nothing to open.

- It shows the repo's card: path, branch, upstream, state, sync, its worktrees,
  the changed files, the commits, the result of the last action (with the argv
  that really ran) and the one from the last `!`.
- With the cursor on a **group header** there is no repo to describe, so it shows
  the group's aggregate: how many repos it has and how many are dirty, ahead,
  behind or in error (only those that exist: the table stays quiet for the same
  reason).
- With the cursor on a **worktree subrow** it shows its minimal card (path,
  branch, head), without inventing git state.
- `!` opens its input at the end of the card, always visible: if the card fills
  the box, what gets cropped is the card, never the prompt. `enter` runs it
  (`$SHELL -c` in the cursor's repo; empty means an interactive shell) and `esc`
  cancels it.
- Lists that do not fit are announced (`… N more`) instead of being cut in half.

The panel is **additive**: it keeps its share of the free height only if it costs
nothing to what was already there. On a terminal where it does not fit (below
~22 lines with the default config) it is not drawn and the dashboard is the usual
one.

## Table columns

The table is *quiet*: cells stay empty when there is nothing to report and only
what demands attention is painted (the order is still attention-first).

- **BRANCH** current branch; when detached `<sha> (detached)` (the only mention
  of the state in the row).
- **Work Tree** the working tree only: `N ?M` = N tracked files changed + M
  untracked; `∅` with no repo; `⚠` git error. No dot and no arrows: commit
  states live in ↑↓up.
- **↑↓up** derived from commits against the *upstream* of the current branch
  (what is to push/pull): `↑N↓N` diverged, `↑N`/`↓N`, `no-up` = branch with no
  tracked upstream ("no rope to the remote": there is nothing to compare
  against).
- **SYNC** deviation with respect to the *sync branch* (`sync_branch` global,
  overridable per repo in the marker): `<branch> ↓N` = there are N commits on
  the sync branch that your branch does not have — the CI-first question "does my
  `feat/x` have the latest changes from `main`?"; plain `<branch>` = up to date;
  `<branch> —` = the ref does not exist; `—` = no resolved branch. The chosen
  branch is always visible. Freshness depends on the sync branch's local ref
  being up to date; the automatic fetch only refreshes remote-tracking refs.
- **ACTIVITY** last commit in relative time.
- **FETCH** only transient: `⟳ fetch` running, `✗ fetch` failed (persistent
  until the next fetch of that repo); success takes no cell (the bar's
  notification says so).

## Keys

| Key | Action |
|---|---|
| `j/k`, `↑↓` | move the cursor (`g`/`G` for the ends) |
| `n` | toggle only repos with pending changes |
| `/` | filter by name/group or by worktree branch/basename (live; `enter` confirms, `esc` clears) |
| `D` | delete the worktree under the cursor (configurable, `worktree_remove` action; only the worktree, the branch is kept) |
| `r` | full rescan (discovery + states + auto fetch) |
| `R` | re-collect the cursor's repo |
| `f` / `F` | fetch the repo / fetch everything |
| `p` | pull selector (see below) |
| `P` | push |
| `e` | open `$EDITOR` in the repo's directory |
| `enter` | fold/unfold whatever is under the cursor: the repo's worktrees, or a group header's block (configurable, `fold` action) |
| `l` | command log panel: what really ran, with what result and why (see below) |
| `q` | quit |

### Pull: the policy lives in your gitconfig

`p` does not run a pull: it opens a selector and the **next** key picks the
variant.

| Key | Variant | Command |
|---|---|---|
| `p` `p` | default | `git pull` (no flags) |
| `p` `r` | rebase | `git pull --rebase --autostash` |
| `p` `f` | ff-only | `git pull --ff-only` |
| `p` `m` | merge | `git pull --no-rebase` |
| `p` `a` | AI | your `[ai.pull] command` command (launches straight away) |

The **default variant carries no flags on purpose**: the reconciliation policy is
yours (`pull.rebase`, `pull.ff`, …) and command-line flags override it. With
`--ff-only` hardcoded, a `pull.rebase=true` in your config was ignored. The other
three variants exist to override the policy without having to edit gitdash's
config.

The **AI** variant is a terminal handoff: gitdash lends the screen to your
command (which runs in the repo's directory, with the marker's `prompt`) and
re-collects the state on return. With no prompt in the marker, no
`[ai.pull] command` or no installed binary it only warns: it never launches
blind.

Any other key cancels the selector and runs its normal action (`esc` just
cancels). The target repo is captured when you press `p`, so the second key
cannot operate on a different row.

**Each repo's card stores the argv that really ran** (last action, in memory,
only the last one). It is the only way to know what reconciled: with `p` `p` there
are no flags to read, your gitconfig made the decision.

If a `--rebase` pull clashes, the warning does **not** just say "failed": it says
the rebase was left half-done and how to continue or abort. Saying "failed"
invites you to retry on top of an unresolved rebase.

## Command log: what really ran

`l` opens a panel with the session's timeline: which command was launched, in
which repo, with what result and how long it took. By default only the user's
actions are shown; `a` widens the view to the scan's reads (`status`, `log`,
`worktree list`, `rev-list`) and to the automatic fetch, and `j`/`k` scroll.

```
22:23:13.378  key p    diverged-node   pull
22:23:14.381  key r    diverged-node   pull_rebase
22:23:14.529  exec     diverged-node   git pull --rebase --…  rebase+autostash
```

The `key` lines are the key you pressed; the `exec` ones are the process that
finished. Both are there because the argv does **not** say which policy git
applied: `p` `p` runs plain `git pull`, and if your config has
`pull.rebase=true` with `rebase.autostash=true` what got integrated was a rebase
with autostash. The result is deduced from the output git already printed (with
`LC_ALL=C` forced, so the messages are not localised), not from reading the
config: the precedence of `pull.rebase` and `branch.<name>.rebase` changes across
git versions, and what git **did** is in its output.

Results the panel tells apart: `rebase`, `rebase+autostash`, `merge`,
`fast-forward`, `up-to-date`, `diverged`, `conflict`, `no-upstream`, `pushed`,
`rejected`, `failed`.

The log lives **in memory and per session only** (500 entries): it writes no
file. An intent with no `exec` behind it means the action was rejected later (the
repo already had one running, `lazygit` is not installed, or the AI pull lacked a
prompt, command or binary); the reason is in the toast of that same moment.

The **AI pull** (`p` `a`) also leaves its two lines: the `key a` intent and the
`exec` with the resolved argv, whose last element is the whole prompt. The log
is where you audit what the AI was asked for.

## Worktrees and groups

- **Worktrees** are shown folded under their main repo with a `▸ (N wt)`
  indicator and are **expanded/folded with `enter`** on their row. Expanded,
  every worktree of `git worktree list` appears as a **navigable and operable
  subrow** (including the ones with no marker and the ones outside the roots):
  the cursor can sit on it and *all* repo actions (fetch, pull, push, lazygit,
  editor, recollect and `!`) run against that worktree's path, just like the
  card. The subrow shows its branch (or `(detached)`) and leaves the per-worktree
  state cells empty (no dirty/ahead/behind/sync is invented). The expanded/folded
  state persists across sessions (the same `collapsed.json` as the group folding,
  in its own namespace). The `/` filter finds worktrees by branch or basename
  and reveals their parent expanded transiently (without changing what is
  persisted). A worktree stays visible as its own row only when its main repo is
  not discovered (tag `[wt]`).
- With the cursor on a **worktree subrow**, `D` (the `worktree_remove` action,
  configurable) deletes the worktree: folder and registration in
  `.git/worktrees`, **never the branch**. The first `D` arms a persistent
  confirmation (`remove worktree <name>? D to confirm, esc to cancel`); the
  second `D` runs `git worktree remove` from the main repo and, on finishing,
  re-collects the parent so the subrow disappears. If the worktree has
  uncommitted or untracked changes, git fails: the real reason is shown and a
  second level is armed (`D to force`) that retries with `--force`; a failure of
  the forced run reports the error and disarms (no loop). `esc` cancels at any
  point, and any other key disarms. `D` on a repo row, a group header or the
  empty table does nothing (it warns `select a worktree`).
- If any marker defines `primary_group`, the table is **grouped in two levels**
  (the vroom pattern): each group's block starts at its first member's position
  with the header `▾ name (n)`; inside a primary, each `secondary_group` forms a
  sub-block with an indented header. Both levels are foldable (folding a primary
  hides its secondaries). Repos without a primary go to the `(ungrouped)`
  section at the end; a `secondary_group` without `primary_group` is ignored. The
  TUI does not repeat the group in a column (the foldable headers already say
  it); `--print` — flat table, no headers — does show the GROUP column. With no
  groups the table is flat.

## How it discovers repos

It walks the `roots` with unlimited depth (pruning hidden dirs and `exclude`),
looking for the marker. The marker's folder IS the repo: `.git` directory =
normal repo, `.git` file = worktree (tagged `[wt]`), no `.git` = visible as
`no repo`. The derived states: `clean`, `dirty`, `ahead`, `behind`, `diverged`,
`no-upstream`, `detached`, `no repo`, `error` — ordered attention-first.

## Development

```bash
go build ./... && go vet ./... && go test ./...
./scripts/gen-fixtures.sh    # generates testdata/playground (fixture repos)
XDG_CONFIG_HOME=$(mktemp -d) bin/gitdash --print   # headless smoke test
```

Architecture: `internal/config` (XDG TOML), `internal/discovery` (marker walk),
`internal/gitstatus` (git subprocess + `porcelain=v2` parsing), `internal/cache`
(instant paint on startup), `internal/tui` (Bubbletea v2 dashboard).

## Roadmap

- Sync the branch with the *sync branch* (`git pull --rebase origin <sync>`):
  the SYNC column already reports the gap, but there is no key that fixes it
- Fetch the sync branch (refresh the SYNC column without a manual pull)
- Group actions (fetch/pull a whole group)
- Extra actions: stash, PRs
- Scheduled background fetch

MIT