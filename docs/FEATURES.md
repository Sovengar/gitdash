# Features

Concise feature inventory of gitdash. Details live in `../README.md` and `adr/`.

---

## Feature inventory

| Feature | What it does | Trigger |
|---|---|---|
| [Dashboard & table](#dashboard--table) | Attention-first repo table with branch, dirty, ahead/behind, sync | TUI |
| [Discovery & marker file](#discovery--marker-file) | Walk roots for `.gitdash.toml`; the folder is the repo | `roots`, `marker` config |
| [Groups & worktrees](#groups--worktrees) | Two-level foldable groups; worktrees as navigable subrows | `enter`, marker `primary_group`/`secondary_group` |
| [Pull selector](#pull-selector) | Armed prefix key; next key picks the policy variant | `p` then `p`/`r`/`f`/`m`/`a` |
| [Switch branch](#switch-branch) | Armed prefix key; picker changes the current or the sync branch | `b` then `c`/`s` |
| [Push](#push) | Push the cursor's repo | `P` |
| [Sync](#sync) | Fetch + pull --rebase --autostash against the sync branch | `s` |
| [AI pull](#ai-pull) | Hand off to a configured AI command with the marker's prompt | `p` then `a` |
| [PR/MR creation](#prmr-creation) | Overlay form → `gh pr create` / `glab mr create` | `O`, `ctrl+s` |
| [Visual preview](#visual-preview) | git-sim pull/merge/rebase simulation | `v` then `p`/`m`/`r` |
| [Fetch](#fetch) | Fetch one repo or all (batched, concurrent) | `f` / `F` |
| [Editor & lazygit](#editor--lazygit) | Open `$EDITOR` or `lazygit` in the repo | `e` / `g` |
| [Command input](#command-input) | Run `$SHELL -c` in the repo; empty = interactive shell | `!` |
| [Command log](#command-log) | Session timeline of intents and execs | `l`, `a` |
| [Detail card](#detail-card) | Detail box (fields, worktrees, tails) + peer files box below the table | automatic |
| [Commits panel](#commits-panel) | Top-right commits of the cursor's row | automatic (width-gated) |
| [Filter & dirty toggle](#filter--dirty-toggle) | Live filter by name/group/worktree; show only dirty | `/`, `d` |
| [Config & keybindings](#config--keybindings) | XDG TOML; remappable actions; command templates | `~/.config/gitdash/config.toml` |
| [CLI `--print`](#cli---print) | One-shot table on stdout | `gitdash --print` |
| [Cache & state](#cache--state) | Instant paint from `repos.json`; persisted folds in `collapsed.json` | automatic |
| [Degradation rules](#degradation-rules) | Additive panels; stats before keybinds; quiet table | automatic |

---

## Dashboard & table

- **Attention-first ordering** — rows sorted by state score (`diverged > dirty > ahead > behind > detached > no-upstream > clean`), then last-commit recency, then name.
- **Columns** — NAME (with worktree count glyph), BRANCH, Work Tree (`N ?M` / `∅` / `⚠`), ↑↓up (`↑N↓N` / `no-up`), SYNC (`<branch> ↓N` / `—`).
- **Fetch slot** — 2-cell transient glyph (`⟳`/`✗`) right after the cursor prefix; names never shift.
- **Stats bar** — `N repos · N dirty · N ahead · N behind` plus a spinner for scan/fetch/running actions.

## Discovery & marker file

- **Marker** — `.gitdash.toml` at the repo root; the folder IS the repo (no upward `.git` search).
- **Fields** — `name`, `primary_group`, `secondary_group`, `sync_branch`, `[ai.pull].prompt`.
- **Classification** — `.git` dir = normal repo; `.git` file = worktree (tagged `[wt]`); no `.git` = `no repo`.
- **Roots** — walked with unlimited depth; hidden dirs and `exclude` pruned (default excludes include `testdata`).

## Groups & worktrees

- **Two-level grouping** — `primary_group` (level 1) and `secondary_group` (level 2); both foldable via `enter`; state persists in `collapsed.json`.
- **Ungrouped** — repos without a primary go to a `(ungrouped)` section at the end.
- **Worktree subrows** — expanded with `enter` on a repo row; navigable and operable (all repo actions run against the worktree path); branch shown, state cells left empty.
- **Orphan worktrees** — visible as `[wt]` rows only when the main repo is not discovered.
- **Worktree removal** — `D` on a subrow arms a confirmation; second `D` runs `git worktree remove`; dirty worktree arms a `--force` level.

## Pull selector

- **Armed prefix** — `p` captures the cursor's path; the next key picks the variant. Any other key cancels and runs its normal action.
- **Variants** — `p` default (no flags, gitconfig policy wins), `r` rebase+autostash, `f` ff-only, `m` merge, `a` AI pull.
- **Mid-rebase guard** — a clashing `pull --rebase` is detected; the warning says how to continue/abort, not just "failed".

## Push

- Runs `git push` (configurable via `[commands] push`); records the resolved argv in the command log.

## Switch branch

- **Armed prefix** — `b` captures the cursor's path; the next key picks the variant. Any other key cancels and runs its normal action.
- **Variants** — `c` checkout (change the checked-out branch), `s` sync ref (change the repo's sync branch).
- **Branch picker** — the variant opens a floating centered modal over the dashboard (the table and stats stay visible); the list is read on demand.
- **Scope** — local branches (upstream and no-upstream flagged), plus origin's remote-tracking branches (`origin/*`); a remote entry checks out its local name.
- **Navigation** — `↑`/`↓` move (and `j`/`k` while the filter is empty); a text input filters as you type; `enter` selects, `esc` cancels.
- **Current branch** — selecting the branch already checked out is a no-op toast; no process runs.
- **Sync ref persistence** — `bs` rewrites only the `sync_branch` line of the repo's `.gitdash.toml` (comments and the `[ai]` prompt stay byte-identical) and refreshes the project and snapshot.
- **Command log** — both variants leave an intent (`key bc` / `key bs`); `checkout` runs through the shared git executor.

## Sync

- **Two-step** — explicit `git fetch origin`, then `[commands] sync` base with `origin <sync_branch>` appended.
- **Ref resolution** — marker `sync_branch` overrides the global `sync_branch`; a leading `-` is refused; on the sync branch itself it refuses (nothing to catch up to).
- **Toast-only** — reports via toast with the classified verdict; no card action block.

## AI pull

- **Trust boundary** — the executable comes only from `[ai.pull] command` in the global config; the marker contributes only the prompt text as a single argv element.
- **Placeholders** — `{prompt}`, `{branch}`, `{upstream}`, `{state}`, `{ahead}`, `{behind}`, `{sync}`.
- **Guards** — no prompt, no command, or no binary → warning only; never launches blind.
- **Handoff** — terminal belongs to the child; on return the state is re-collected.

## PR/MR creation

- **Overlay form** — `O` opens a form (title, body, base, draft); `tab`/`shift+tab` cycle fields; `space`/`enter` toggles draft; `ctrl+s` submits; `esc` closes.
- **Forge resolution** — remote URL → `ParseRemoteURL` → `BuildCreateArgv` → `gh pr create` / `glab mr create`; unknown hosts are rejected before executing.
- **Forge config** — `[forge.gitlab]` with `api_base`, `hosts`, `clone_base`, `enabled`; public hosts (github.com, gitlab.com) built-in.
- **Not a handoff** — output is captured (non-interactive with all flags); duration is measured.

## Visual preview

- **git-sim integration** — `v` arms a selector; next key picks `p` (pull), `m` (merge), or `r` (rebase).
- **No-op guard** — merge/rebase variants are blocked when `behind == 0` (git-sim would abort); the warning names the fetch key.
- **Media dir** — `--media-dir` is mandatory (git-sim writes `git-sim_media/` into the repo otherwise).

## Fetch

- **Auto fetch** — after every scan/rescan if `[fetch] auto = true`; batched with `[fetch] concurrency` and `[fetch] timeout`.
- **Manual** — `f` fetches the cursor's repo; `F` fetches all repos with an upstream.

## Editor & lazygit

- `e` opens `$EDITOR` (configurable) in the repo directory; `g` opens `lazygit` (warns if not installed). Both are terminal handoffs.

## Command input

- `!` opens an input at the end of the detail box; `enter` runs `$SHELL -c <cmd>` in the repo; empty `enter` opens an interactive shell; `esc` cancels.

## Command log

- **View mode** — `l` opens a panel (replaces the table); `j`/`k` scroll; `a` toggles show-all (includes scan reads and auto-fetch); `esc` or `l` closes.
- **Entries** — `key` intents (what you pressed) and `exec` processes (argv, exit, duration, result); 500-entry in-memory ring; no file.
- **Result classification** — deduced from git's own output (`rebase`, `merge`, `fast-forward`, `diverged`, `conflict`, …); never from probing `git config`.
- **Sanitising** — argv is untrusted (carries the marker prompt); control/format characters stripped for painting only.

## Detail card

- **Detail box** — path, branch, upstream, state, sync, activity, worktrees list, diagnostics/last action (with resolved argv), last `!` command.
- **Files box** — a peer bordered box titled `files (N)` next to the detail box; one `  <code> <path>` line per file and `… N more` on overflow.
- **Shared divider** — the detail/files boxes and the repos/commits boxes above share one width split, so the files box and the commits box are the same width.
- **Empty-files collapse** — with no files on the row there is no files box; the detail box takes the whole band.
- **Group aggregate** — cursor on a header shows the group's stats (repos, dirty, ahead, behind, errors, worktrees) in the single box.
- **Worktree card** — minimal (path, branch, head) when discovered; no invented git state.
- **Narrow collapse** — below the split floor (terminal width < 63) the band is one box and fields, worktrees and files stack inside it.

## Commits panel

- **Top-right band** — shows the cursor's row's commits (current branch + sync branch as labelled groups); same height as the table.
- **Up to 15 commits per side** — both lists are fetched at 15 and the panel paints only what fits in the shared height.
- **Colours** — commits only the sync branch has paint blue (↓) and only the current branch green (↑), the table's palette; shared commits stay neutral. Decided per commit (same sha in both), so merges cannot mislead it.
- **Untrusted subjects** — the subject text is sanitised before painting (control/escape sequences stripped), like the command log.
- **Additive on width** — the lists column's share of the card plus the files box's borders, capped so the table keeps its 5 columns; dropped entirely below the boundary (the table takes the full width and the detail/files band keeps its share).

## Filter & dirty toggle

- `/` opens a live filter by name, group, or worktree branch/basename; `enter` confirms; `esc` clears.
- `d` toggles showing only repos with pending changes (dirty/ahead/behind/diverged).

## Config & keybindings

- **XDG TOML** — `~/.config/gitdash/config.toml`; `Load()` never fails (defaults + warning).
- **Remappable actions** — `[keybindings]` maps action → key; unknown actions warned on load; navigation/universal keys not configurable.
- **Command templates** — `[commands]` for `pull`, `pull_rebase`, `pull_ff`, `pull_merge`, `push`, `fetch`, `sync`.
- **Fetch config** — `[fetch]` `auto`, `concurrency`, `timeout`.
- **AI config** — `[ai.pull] command` (global only; marker contributes only the prompt).
- **Forge config** — `[forge.<provider>]` with `enabled`, `host`, `api_base`, `clone_base`.

## CLI `--print`

- `gitdash --print` prints a flat table on stdout (tabwriter) with the same ordering as the TUI; includes a GROUP column; handy for scripts/debugging.

## Cache & state

- **`repos.json`** — discovery cache in the cache dir; validated by marker existence; corrupt/unknown-version = silent empty; enables instant paint on startup.
- **`collapsed.json`** — persisted group folds and worktree expansions in the state dir; atomic write (tmp + rename).

## Degradation rules

- **Additive panels** — the detail card and commits panel only enter if they cost nothing to the table/stats/keybinds; below the threshold they are not drawn.
- **Stats before keybinds** — when a warning is armed, the layout degrades the stats banner first so the keybinds (which show the warning) are never hidden.
- **Quiet table** — cells stay empty when there is nothing to report; only what demands attention is painted.
- **Config resilience** — a broken config falls back to defaults with a toast + stderr warning; the TUI never aborts on config errors.
