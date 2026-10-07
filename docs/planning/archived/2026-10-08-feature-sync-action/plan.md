# Plan — `s sync`: update the current branch with the repo's sync branch

`adr_required: false` — no design decision with discarded alternatives, no public
API break (the new keybinding and `[commands] sync` key are additive; the action is
not exported), no migration and no data-loss risk. The one real tradeoff
(configurable base argv, ref appended from the repo) is documented inline, in README
and in the prototype brief; it does not rise to an ADR (main's `docs/adr/0001` covers
a layout-ownership decision of a different weight).

- Run: promoted prototype (declared final by the user: "doy por finalizado el
  prototipo, integralo"), branch `feat/sync-action`, base `2ad0f32`, prototype
  commits `8104578..aa2fb2c` (HEAD).
- Artifacts: `issue.md`, `behavior.feature`, `diagrams/`, plus the approved
  `prototype-brief.md` in this directory.

## Intended outcome

`s` on a repo row updates the current branch with the repo's sync branch: an explicit
`git fetch origin`, then the configured `[commands] sync` base with `origin <resolved
sync>` appended, refused when the snapshot says the branch already IS the sync branch,
reported as a toast carrying the classified outcome, never as a card action block,
while `l` keeps the argv and classified result. Documented in README and AGENTS.md; gates
green; the branch ready to merge through the usual PR path.

## Approach

The five prototype commits are the implementation — verified live on a real repo — so
integration is closing the loop, not rebuilding. The architect's boundary review found
the seams sound and requires no code change: config owns the base argv, `gitstatus`
owns `SyncArgv` (single source, log and exec cannot drift), tui owns the orchestration
and the toast/card policy, and `Classify` is reused where the argv still is. The shared
`startActionArgs` executor (lock, short-circuit, recollect) and the `isRebaseKind`
split (selector membership vs rebase-family semantics) stay as the prototype drew them.

Work, at a high level:

1. **Behavior verification pass** — walk `behavior.feature` against the shipped code
   and the existing tests; add tests only where a scenario is genuinely unpinned, fix
   code only where a scenario is violated. No refactor, no redesign.
2. **Docs gap** — add the `AGENTS.md` "Design gotcha: sync" section (the non-obvious
   decisions: fetch-before-pull, refuse-on-branch, configurable base with a
   non-configurable ref, toast-only + classified outcome, `isRebaseKind` split). README
   already carries the user-facing docs from the prototype.
3. **Gates** — build/vet, full tests, diff coverage at 100%, mutation gate; then
   install the binary per the repo rule.
4. **Hand back** — the branch goes to the normal PR path; no publication happens from
   the sync action itself.

## Key decisions carried into the plan

- User decisions rule: ref `origin/<sync>` resolved by the existing `SyncFor` (marker
  `sync_branch` over the global default); refusal when the snapshot's current branch
  equals the sync branch (nothing executed, intent still recorded); explicit fetch
  first with short-circuit on failure; both execs audited under one `sync` intent;
  success toast verdict-only with the classified outcome (`rebase`, `rebase+autostash`,
  `merge`, `fast-forward`, `up-to-date`); card block cleared on success and failure.
- Guard granularity is the last snapshot: an unknown branch (not yet collected, stale,
  detached HEAD) does not refuse — git's own result governs. Accepted.
- Legacy-`master` repos fail loudly against `origin/main`: the SYNC column's fallback
  probe is display-only and the action never invents a ref. Accepted.
- A worktree sub-row uses the global default (the parent repo's marker override is not
  consulted) and cannot trigger the refusal. Accepted as current behavior; candidate
  follow-up, out of this run's scope.

## Risks

- No migration, no breaking change, no data loss beyond what the user's own configured
  pull policy does; a conflicting pull leaves git's own half-done rebase, which the
  mid-rebase warning names.
- `[commands] sync` is user-controlled: a base other than a pull changes what `s` does
  (the full argv is audited). Accepted by design.
- The same-branch refusal is only as fresh as the last scan; nothing else depends on it.
- Gates are the real risk surface (diff coverage 100% and the mutation gate are strict),
  not the feature behavior.

## Integration note — main moved after the fork

Since the fork point (`2ad0f32`), `origin/main` advanced to `8964917` (UI restructure,
ADR 0001, `CHANGELOG.md`, archived planning package). A non-destructive merge
simulation of this branch against `origin/main` is **clean** (no textual conflicts),
even though both sides touched `internal/tui/sections.go` and `internal/tui/update.go`;
the symbols the delta inserts into (`handleKey`, `commandActions`, `runningActions`,
`startActionCmd`, `syncOf`, `selected`) all exist on main. At hand-back the usual PR
path reconciles with main and reruns the gates. Two doc-level items belong to that
reconciliation: the `AGENTS.md` sync section will land next to main's new design
gotchas (possible textual conflict), and the user-facing change should add the
`Added` entry to the `CHANGELOG.md` that main introduced.

## Ordering (coarse)

BDD verification → docs (`AGENTS.md`) → gates (tests, coverage, mutation) → install →
hand back for the PR path, which reconciles with the new `main` and its changelog.
