---
name: gitdash
description: "Trigger: gitdash, Git dashboard, multi-repo status, batched Git actions, .gitdash.toml, or gitdash config. Operates and configures gitdash."
---

## Activation Contract

Load for gitdash dashboards, multi-repository status, batched Git actions, or configuration.

## Hard Rules

- Inspect repository status before changing it.
- Keep repository-specific failures visible in batched actions.
- Do not discard, reset, or force-push changes without explicit authorization.

## Execution Steps

1. List target repositories and inspect status.
2. Select the smallest gitdash command or configuration change.
3. Review each repository result.
4. Report successes, failures, and uncommitted changes.

## Batch Rule

Use a preview or status command before a batched mutation. Report every repository separately; never infer that one successful repository proves the others are safe.

