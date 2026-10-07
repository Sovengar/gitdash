---
feature: 0002-feature-sync-action
freshness: aa2fb2cb1c00292f5d95bfa2f9e10aa4b033de59
codegraph: ready
generated_by: codebase-researcher
---

# Context: `s sync` — update the current branch with the repo's sync branch

## Scope
- In: integrate the prototype (commits `8104578..aa2fb2c`, already on the branch) — behavior verification pass against `behavior.feature`, the `AGENTS.md` "Design gotcha: sync" docs gap, gates, install.
- Out: no code redesign, no push/publication, no ADR (`adr_required: false`), no CHANGELOG in this branch (hand-back item).

## Files to Touch

| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| AGENTS.md "Design gotcha: sync" section | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-sync-action/AGENTS.md` | insert at 524 (between AI-pull section ending 523 and "Wiring gotchas" 525) | The docs gap. Follow the existing design-gotcha pattern: Why-only English, one-line intents, no spec IDs. Content: fetch-before-pull, refuse-on-branch, configurable base with non-configurable ref, toast-only + classified outcome, `isRebaseKind` split. |
| sync tests (only unpinned scenarios) | `/home/buble/dev/projects/gitdash/.worktrees/gitdash.feat-sync-action/internal/tui/sync_action_test.go` | 404 (16 tests) | Add tests ONLY for the gaps below. Do NOT duplicate the 12 already pinned. |

### Implementation map (already shipped — read-only for the executor)

| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `DefaultKeybindings` "sync":"s" | `internal/config/config.go` | 189, 196 | Keybinding exists. |
| `DefaultCommands` "sync" base | `internal/config/config.go` | 214, 223 | `[commands] sync` default `pull --rebase --autostash`. |
| `hintLabels` / `hintActions` / `HintBarLines` | `internal/config/config.go` | 292, 299 / 316, 317 / 323, 338 | Hint bar shows `s sync` (row 2). |
| `SyncArgv` | `internal/gitstatus/status.go` | 208 | Single argv source; copies base, appends `origin <sync>`. |
| `SyncFor` / `SyncForAllowsFallback` | `internal/gitstatus/status.go` | 148 / 156 | Marker `sync_branch` over global default. |
| `actionMsg.outcome` | `internal/tui/app.go` | 52, 56 | Classified verdict field. |
| `startSyncCmd` | `internal/tui/app.go` | 453 | Resolves branch, refuses (empty/same-branch/empty-cmd), returns `startActionArgs` with fetch+pull. |
| `startActionArgs` | `internal/tui/app.go` | 473 | Shared executor: lock, short-circuit, `Classify` on success, `RebaseInProgress` if `isRebaseKind`, recollect. |
| `isRebaseKind` | `internal/tui/app.go` | 523 | `IsPullKind(kind) \|\| kind == "sync"`. |
| `logIntent` | `internal/tui/app.go` | 825 | Generic intent recording; header rows (no row) record nothing. |
| `syncOf` | `internal/tui/app.go` | 856 | Resolves sync branch for a path. |
| `actionMsg` case (toast-only) | `internal/tui/update.go` | 78 | Drops `lastAction`, appends ` — <outcome>` on success. |
| `actionNote` | `internal/tui/update.go` | 203 | Drops argv for toast-only; mid-rebase warning via `isRebaseKind`. |
| `case "sync"` | `internal/tui/update.go` | 469 | Executes directly (no selector); no-repo guard. |
| `commandActions` / `rowActions` / `toastOnlyActions` | `internal/tui/update.go` | 545 / 555 / 563 | `sync` in all three maps. |
| `actionForKey` | `internal/tui/update.go` | 569 | Key→action resolution (rebindable). |
| log-panel guard (pull/visual/pr only) | `internal/tui/update.go` | 391–396 | `s` is NOT blocked with the panel open. |
| `runningActions` | `internal/tui/sections.go` | 116, 119 | Uses `isRebaseKind` so sync shows in the spinner. |
| `Classify` | `internal/gitstatus/outcome.go` | 21 | Deduces outcome from output (no git config probing). |
| README sync docs | `README.md` | 65–75, 147 | Already documents `s` (key table + `[commands] sync`). |

## Contracts
- `SyncArgv(base, remote, sync)` is the single argv source — copy the base, never mutate it (`internal/gitstatus/status.go:208`).
- Every git exec leaves through `runGit`/`runGitCombined` (`internal/gitstatus/status.go:254,273`) so the cmdlog records it.
- Intent is recorded by generic routing (`launchesCommand` → `logIntent`) BEFORE the guards (`internal/tui/update.go:421–423`, `internal/tui/app.go:825`); refusals still record the intent, header rows record nothing.
- `toastOnlyActions` (`internal/tui/update.go:563`) is the single source for card-drop + argv-drop + outcome-append.
- `IsPullKind` = selector membership; `isRebaseKind` = rebase-family semantics (spinner, mid-rebase detection, conflict warning) (`internal/tui/app.go:118,523`).
- `Classify(args, output, exitCode)` is deduced from git's output, never from `git config` (`internal/gitstatus/outcome.go:21`).

## Pattern to Follow
- **Adding/routing an action** → `actionForKey` (`update.go:569`), `commandActions`/`rowActions` (`update.go:545,555`), `handleKey` case (`update.go:469`), `hintLabels`/`hintActions`/`HintBarLines` (`config.go:292–343`).
- **Action reporting** → `toastCmd` (`app.go:575`) vs the `lastAction` card; the `actionMsg` case (`update.go:78`) is where toast-only diverges.
- **Test model + key driving** → `newTestModel` (`app_test.go:26`), `press` (`app_test.go:123`), `cursorOn` by path (`pull_selector_test.go:15`), `snapOnBranch` (`sync_action_test.go:15`), `collectActionMsg`/`collectAction`/`execArgvs` (`sync_action_test.go:22,39,56`).
- **AGENTS.md design-gotcha section** → model after the AI-pull section (`AGENTS.md:493–523`): a `## Design gotcha: <name>` heading, then a short intro paragraph, then `**bold lead**: explanation` bullets. Why-only, one line per intent, no spec IDs.

## Tests
- Existing affected: `internal/tui/sync_action_test.go` (16 tests), `internal/config/config_test.go:601`, `internal/config/pull_policy_test.go:51,63`, `internal/gitstatus/sync_test.go:238`.
- Framework / runner: `go build ./... && go vet ./... && go test ./...`
- Coverage gate: `make coverage-check` (diff at 100%, total floor in `scripts/coverage-floor`).
- Mutation gate: `make mutate-all` / `make mutate-all-diff` (`scripts/mutate.sh`; `.mutation-allowlist` compared by line; `.mutation-timeouts`).
- Lint: `make lint` (gofmt + golangci-lint v2.13.2).
- Install rule: `make install` after any code change (the user runs `~/.local/bin/gitdash`).
- Planning-docs-only changes (AGENTS.md) need no test changes.

### Scenario ↔ test coverage (17 scenarios in `behavior.feature`)

| # | Scenario (behavior.feature) | Covered by |
|---|------------------------------|-----------|
| 1 | Sync brings branch up to date | ✅ `sync_action_test.go:66` |
| 2 | Custom sync base keeps ref out of reach | ✅ `sync_action_test.go:127` + `pull_policy_test.go:63` |
| 3 | Finished sync leaves no action block | ✅ `sync_action_test.go:320,359` |
| 4 | Unclassifiable success no dangling separator | ✅ `sync_action_test.go:348` |
| 5 | Activity indicator lists sync | ✅ `sync_action_test.go:290` |
| 6 | Refuse when current branch is sync branch | ✅ `sync_action_test.go:182` |
| 7 | **Unknown branch does not trigger refusal** | ❌ **UNPINNED** — no test with empty/detached branch pressing `s` |
| 8 | Failed fetch stops before pull | ✅ `sync_action_test.go:243` |
| 9 | Failed pull reports reason + mid-rebase | ✅ `sync_action_test.go:302,359` |
| 10 | **Sync branch does not exist on origin** | ❌ **UNPINNED** — no test where fetch succeeds but pull fails with "couldn't find remote ref" |
| 11 | No sync branch resolves | ✅ `sync_action_test.go:162` |
| 12 | Sync command resolves to no arguments | ✅ `sync_action_test.go:204` |
| 13 | Repo already has action running | ✅ `sync_action_test.go:223` |
| 14 | Row has no git repo | ✅ `sync_action_test.go:145` |
| 15 | Key discoverable **and rebindable** | ⚠️ **PARTIAL** — discoverable ✅ `config_test.go:601`; rebindable ❌ no test rebinds `keybindings.sync` and presses the new key |
| 16 | **Sync runs with command-log panel open** | ❌ **UNPINNED** — no test with `logOpen=true` pressing `s` |
| 17 | **Worktree sub-row resolves against global default** | ❌ **UNPINNED** — no test with a worktree sub-row pressing `s` |

**Suggested targets:** add the missing tests to `internal/tui/sync_action_test.go` (scenarios 7, 10, 16, 17) and a rebind test (scenario 15) — either in `sync_action_test.go` or `config_test.go` following the `TestKeyByActionInvertsTheMap` pattern (`config_test.go:497`).

## Conventions & Boundaries
- Code comments in English, WHY-only, one line each; no godoc that repeats the name.
- No references to specs or requirement/scenario IDs; no SDD artefacts. `docs/planning/<NNNN>-.../` is the planning scaffold (valid in the repo).
- Direct model tests (build the Model, send msgs with `Update`, inspect state) — no teatest.
- Event pump: ALWAYS rearm `waitForEvent` (the `withPump` helper) in `Update` after consuming a channel event.
- Rows are ordered attention-first; locate the cursor by path (`cursorOn`), never by index.
- Table cells return `(text, style)`; the render pads BEFORE applying the style.

## Integration Points (non-obvious)
- `s` is NOT blocked with the command-log panel open (only `pull`/`visual`/`pr` are guarded at `update.go:391–396`); sync executes directly, no selector, no pre-exec `RebaseInProgress` guard (the mid-rebase check runs after a failure, like the pulls).
- `logIntent` runs before the guards: refusals (same branch, no repo, busy, empty base) still record the intent; header rows (no row) record nothing.
- sync's fetch is plain `git fetch origin` (class action), deliberately NOT `commands.fetch`; a fetch failure short-circuits the pull.
- The same-branch guard reads the last snapshot (`m.states[path].Status.Branch`), not a live `symbolic-ref`.
- Worktree sub-rows: global default sync branch (parent marker not consulted), refusal cannot fire (no snapshot) — accepted current behavior per plan.
- Legacy-`master` repos: the action resolves via `SyncFor` (marker/global), NOT the SYNC column's fallback probe; failure is loud ("couldn't find remote ref") — per plan.
- `origin/main` advanced to `8964917` since the fork (UI restructure + ADR + CHANGELOG); merge simulation is clean; reconciliation + a CHANGELOG `Added` entry are hand-back items, NOT branch work. This branch has no `CHANGELOG.md` or `docs/adr/`.

## Risks / Assumptions
- Gate strictness is the real risk surface: diff coverage at 100% and the mutation gate are strict. New tests must actually exercise the new paths.
- `AGENTS.md` sync section will land next to main's new design gotchas at merge time (possible textual conflict) — hand-back reconciliation.
- The prototype commits are the starting point, not scratch work: reuse them and keep the tests that already cover the behaviour.
