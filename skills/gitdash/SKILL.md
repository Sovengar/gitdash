---
name: gitdash
description: "Trigger: gitdash, Git dashboard, Worktrunk wt, branch checkout, worktree, lazygit/lazyjj/GitButler launchers, .gitdash.toml, or gitdash config. Operates and configures gitdash."
---

## Activation Contract

Load for gitdash dashboards, multi-repository status, branch checkout, Worktrunk worktrees, batched Git actions, or configuration.

## Hard Rules

- Inspect repository status before changing it.
- Keep repository-specific failures visible in batched actions.
- Do not discard, reset, or force-push changes without explicit authorization.
- Do not add a gitdash push action. Use the `g` launchers (`g g` lazygit, `g b` GitButler) for staging, commits, and pushes.
- Keep switch, worktree, sync, and config keys aligned with the TUI, docs, and key hints.

## Execution Steps

1. List target repositories and inspect status.
2. Select the smallest gitdash command or configuration change.
3. Review each repository result.
4. Report successes, failures, and uncommitted changes.

## Branch and worktree keys

| Keys | Action |
| --- | --- |
| `s` | Switch to a selected branch. With no match, Enter creates it. |
| `S` | Sync with the configured sync branch. |
| `w` `c` / `w` `d` / `w` `D` | Create a Worktrunk worktree / detach the selected worktree keeping its branch / detach it and delete the branch (`git branch -d`, `-D` when unmerged). |
| `c` `s` | Persist the selected sync ref in `.gitdash.toml`. |
| `c` `n` | Rename the repo in `.gitdash.toml` (`name`); empty removes the override. |

## Launch keys

| Keys | Action |
| --- | --- |
| `g` `g` | Open `lazygit` in the cursor repo. |
| `g` `j` | Open `lazyjj` in the cursor repo; requires a `.jj` folder, otherwise it only warns (`no jj repo — nothing to do`). |
| `g` `b` | Open `but tui` (GitButler CLI) in the cursor repo. |

The first `g` only arms the prefix (a prompt in the keybinds line, `esc` cancels); the repo is captured when it arms. All three are terminal handoffs recorded in the command log (`l`) as `key` intents plus the `exec`, and they warn instead of launching when the binary is missing (`lazygit`/`lazyjj`/`but not installed`) or the repo already has an action running. The sub-keys are config actions: `keybindings.git_tool` (the prefix), `keybindings.lazyjj`, `keybindings.gitbutler`.

`w c` needs the `wt` binary. It refreshes the parent repo after Worktrunk exits. Remote `origin/X` selections use `X` so Worktrunk creates or uses local tracking.

`w d` and `w D` are plain git (`git worktree remove` + `git branch -d/-D`), so they work without `wt`. The same key twice confirms; another key only re-arms. Arming checks the worktree with `git status --porcelain`, so a dirty worktree asks first (`d to delete anyway, esc to cancel`) and the confirmation runs with `--force`; an unmerged branch asks the same way before `-D`. `D` on a detached worktree only warns.

## Batch Rule

Use a preview or status command before a batched mutation. Report every repository separately; never infer that one successful repository proves the others are safe.

