# Prototype brief — restore `s sync` (feat/sync-action)

## Goal

Bring back a `sync` action on key `s`: **update the current branch with the repo's sync branch** — fetch its remote ref and rebase the current branch onto it, autostashed.

## Origin (user request, 2026-10-07)

- `sync` (s) was removed in commit `6c4ba58` (#10). The old action ran `git pull --rebase --autostash` with **no ref** (the current branch's upstream) — it never used the sync branch. The user wants the action back with sync-branch semantics.
- User's phrasing: "the 's sync' action must be in the TUI and must update the current branch with the sync branch" (whether it runs git-sync or a git pull is an implementation choice).
- Already in README Roadmap: "Sync the branch with the *sync branch* (`git pull --rebase origin <sync>`): the SYNC column already reports the gap, but there is no key that fixes it".

## Verified facts (do not re-litigate)

- `git pull --rebase --autostash <sync>` fails when `<sync>` is not a remote name (`fatal: 'main' does not appear to be a git repository`). The command needs a remote: `git pull --rebase --autostash origin <sync>`.
- That equals the happy path of the user's `git-sync` script (`~/.local/scripts/git-sync`: `git fetch origin` + `git pull --rebase --autostash origin <branch>`, no push). git-sync's conflict path (abort rebase → squash → force-push with lease) is **out of scope** here.
- Sync ref resolution today: per-repo marker `.gitdash.toml` `sync_branch` (`discovery.Project.SyncBranch`) overrides global `cfg.SyncBranch` (default `main`); `gitstatus.SyncFor(p, defaultSync)` resolves; the SYNC column measures the **local** ref (`gitstatus.syncBehind` = `rev-list --count HEAD..<sync>`).
- Key `s` is free in `DefaultKeybindings` (`internal/config/config.go`).
- The repo requires: diff coverage 100%, mutation gate, direct model tests (no teatest), English WHY-only comments, no spec IDs.

## Expected behavior (prototype scope)

- On a repo row (`HasRepo`): `s` syncs the current branch with the repo's resolved sync branch.
- Recorded in the command log (intent + exec) like the other command actions; failures surface through the existing action-result path; the running-lock is respected; `RebaseInProgress` guard like the git pulls.
- Hint bar shows `s sync` (`hintLabels` / `HintBarLines`).
- No publication, no push, no integration: prototype on branch `feat/sync-action`, user iterates on it.

## Open decisions (pick one, flag the choice in your report)

- Ref: `origin/<sync>` (fresh, needs a fetch) vs local `<sync>` (what the column measures). Default proposal: `git pull --rebase --autostash origin <sync>`, error surfaced by the existing failure path when remote/branch is missing.
- Behavior when the current branch IS the sync branch.
- Argv source: a new `[commands]` entry vs a single-source builder (precedent: `RemoveWorktreeArgv`).

## Confirmed decisions (user, 2026-10-07 — supersede the open questions)

1. Ref: `origin/<sync>` — confirmed.
2. `s` is **blocked when the current branch IS the sync branch**: refuse before executing with a clear toast (branch == sync branch → nothing to sync).
3. Argv base is configurable: new `[commands] sync` entry (default `pull --rebase --autostash`); the builder appends `origin <resolved sync>` per repo.
4. Run an explicit `git fetch origin` **before** the pull; a fetch failure stops before the pull; both commands visible in the command log.

## Constraints for trying it

- Iterate and verify inside the working environment (`go build`, `go test`, smoke via tmux if useful).
- Do **not** `make install`: that writes the binary outside this working environment. Make it runnable from within (e.g. `go build -o bin/gitdash ./cmd/gitdash`) and report the exact command; build artefacts stay untracked/ignored.
- Advice for the user's try-out: an isolated config via `XDG_CONFIG_HOME=$(mktemp -d)` and/or their real config; the dashboard must show the SYNC column and the `s sync` hint.

## Post-brief iterations (user-approved during the prototype; supersede the section above)

- `7f7c0cf` — sync's result no longer renders in the card's generic `last <kind>` block; it reports as a toast (`l` keeps the full argv+output audit).
- `2f519db` — the success toast is verdict-only (`sync ok <repo>`), and a finished sync clears any pre-existing action block in the card for that repo (also on failure).
- `aa2fb2c` — the success toast appends the classified outcome of the reconciling pull (`sync ok <repo> — rebase|fast-forward|up-to-date|rebase+autostash|merge`), never the argv.
- Verified live on a real repo (dbx): `s` ran `git fetch origin` + `git pull --rebase --autostash origin main`, integrated a pushed probe commit (fast-forward), toast + log as expected.
- Prototype declared final by the user and promoted to the pipeline (2026-10-07).
