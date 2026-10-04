# Issue — Visual pull preview with `git-sim`

## Problem

gitdash decides reconciliation actions (pull/merge/rebase) without the user being
able to **see beforehand** what would happen. A `git pull` on a diverged branch may
integrate with rebase or merge depending on the gitconfig, and a mid-rebase leaves
the repo in a state that invites you to retry on top of something unresolved.
There is no way to preview the operation without really running it.

`git-sim` (initialcommit-com/git-sim, PyPI 0.3.5) draws a diagram of what a git
command —`pull`, `merge`, `rebase`— **would do without mutating the real repo**
(the network operations run in a temporary clone under `/tmp/git_sim/<repo>`). It
is purely visual and ephemeral: it generates an image and opens it with
`xdg-open`.

## Proposal

Add a **visual preview** to gitdash, invoked as a terminal *handoff*:

- New key `v` → configurable `visual` action.
- `v` **does not run**: it arms a prefix-key selector (sibling of `pullArmed`) with
  the repo path under the cursor and its upstream captured.
- The next key picks the variant:
  - `p` → `git-sim pull` (no arguments; mirror of gitdash's plain `p`).
  - `m` → `git-sim merge <upstream-ref>`
  - `r` → `git-sim rebase <upstream-ref>`
  - Any other key cancels and carries on with its normal course.
- `--media-dir <gitdash's XDG cache>/git-sim` is always passed so that git-sim does
  NOT write `git-sim_media/` inside the repo (gitdash uses real `git status`: it
  would mark it dirty after every preview).
- `git-sim` is resolved through PATH (`exec.LookPath`); if it is not there, only a
  toast. The handoff hands the terminal to the child, like lazygit/editor/`p a`,
  and on return it records the exec in the command log (`Dur=0`) and re-collects
  the state.

## Out of scope

- git-sim's `s`/squash variant: DISCARDED.
- `p`/`pullArmed`'s state machine is not touched, nor `PullKinds`, nor
  `commands.pull*`, nor `[ai.pull]`.
- The path of the generated image (non-deterministic timestamp) and the
  `/tmp/git_sim/<repo>` clone are neither parsed nor chased.
- `git-sim` is NOT git: it does not go through `runGit`/`runGitCombined`.

## Why now

The repo already has the exact pattern needed (p's prefix-key selector,
lazygit/`p a`'s terminal handoff, command log with intent + argv) and
`promptLine()` as the single place for warnings in keybinds. The feature is
additive: a new path that reuses those rails without modifying the existing ones.