---
name: gitdash
description: "Trigger: gitdash, Git dashboard, Worktrunk wt, branch checkout, worktree, .gitdash.toml, or gitdash config. Operates and configures gitdash."
---

## Activation Contract

Load for gitdash dashboards, multi-repository status, branch checkout, Worktrunk worktrees, batched Git actions, or configuration.

## Hard Rules

- Inspect repository status before changing it.
- Keep repository-specific failures visible in batched actions.
- Do not discard, reset, or force-push changes without explicit authorization.
- Do not add a gitdash push action. Use lazygit for staging, commits, and pushes.
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
| `w` `c` / `w` `d` | Create a Worktrunk worktree / remove the selected worktree. |
| `c` `s` | Persist the selected sync ref in `.gitdash.toml`. |
| `c` `n` | Rename the repo in `.gitdash.toml` (`name`); empty removes the override. |

`w c` needs the `wt` binary. It refreshes the parent repo after Worktrunk exits. Remote `origin/X` selections use `X` so Worktrunk creates or uses local tracking.

## Batch Rule

Use a preview or status command before a batched mutation. Report every repository separately; never infer that one successful repository proves the others are safe.

