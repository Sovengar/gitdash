# Restore `s sync`: update the current branch with the repo's sync branch

## Why

The dashboard's SYNC column already reports the gap between the current branch
and the repo's *sync branch* (marker `sync_branch` over the global default), but
there is no key that closes that gap — the column is read-only. README's roadmap
carried exactly this gap: "the SYNC column already reports the gap, but there is
no key that fixes it".

A `sync` action on `s` existed before and was removed in `6c4ba58` (#10). The old
one ran `git pull --rebase --autostash` with **no ref**, so it reconciled the
current branch with its own upstream and never used the sync branch. Restoring
the key with **sync-branch semantics** is what makes the SYNC column actionable.

The feature was built on the prototype track (`feat/sync-action`, commits
`8104578..aa2fb2c`) and reviewed; the user declared it final and asked to
integrate it. This issue integrates that work as-is, plus its documented
iterations. The prototype brief is the approved source and its "confirmed
decisions" supersede the open questions:
`docs/planning/0002-feature-sync-action/prototype-brief.md`.

## Scope

On a repo row (`HasRepo`), `s` updates the current branch with the repo's
resolved sync branch. Confirmed decisions:

1. **Ref is `origin/<sync>`.** The remote is `origin`; the branch is the repo's
   resolved sync branch (marker `sync_branch` wins over `cfg.SyncBranch`, default
   `main`) via the existing `gitstatus.SyncFor`.
2. **Refuse when the current branch already IS the sync branch.** Clear toast,
   nothing executed (no fetch, no pull, no exec entry in the command log). The
   intent is still recorded, like the other pre-exec guards.
3. **Argv base is configurable.** New `[commands] sync` entry, default
   `pull --rebase --autostash`. The builder appends `origin <resolved sync>` per
   repo, so the policy is configurable but the ref is not: a static string cannot
   carry a per-repo branch.
4. **Explicit `git fetch origin` first**, then the configured pull. A fetch
   failure short-circuits: the pull never runs.
5. **Both commands are audited** in the command log as separate action-class
   execs, in order, under a single `sync` intent.
6. **Result is a toast, never the card's action block.** The toast is verdict-only
   on success (`sync ok <repo> — <classified outcome>`), where the outcome comes
   from the existing `gitstatus.Classify` (`rebase`, `rebase+autostash`, `merge`,
   `fast-forward`, `up-to-date`); it never prints the argv. On failure the toast
   keeps git's real reason (and the mid-rebase warning when applicable) and no
   outcome is appended.
7. **A finished sync clears any pre-existing card action block for that repo**, on
   success and on failure.
8. **`l` keeps the audit**: the argv and the classified result remain visible in the
   command log panel.

Mechanics that follow the existing patterns: `s` executes directly (no selector,
unlike `p`); the running-lock is respected; `IsPullKind`'s rebase/mid-rebase
handling is shared through a new `isRebaseKind` that also covers `sync` (spinner,
mid-rebase detection, conflict warning); the intent is left by the generic
`handleKey` routing (`commandActions` + `rowActions`); the hint bar shows
`s sync`.

## Non-goals

- No push, publication or integration of any kind.
- git-sync's conflict path (abort rebase → squash → force-push) is out of scope.
- No speculative features beyond the decisions above.

## Constraints

- Repo conventions apply: English comments (WHY only), no references to specs or
  requirement/scenario IDs, no SDD artefacts. `docs/planning/<NNNN>-.../` is the
  planning scaffold, not an SDD artefact.
- Direct model tests (no teatest), diff coverage 100%, mutation gate green.
- The prototype commits are the starting point, not scratch work: reuse them and
  keep the tests that already cover the behaviour
  (`internal/tui/sync_action_test.go`, `internal/gitstatus/sync_test.go`,
  `internal/config/*_test.go`).
