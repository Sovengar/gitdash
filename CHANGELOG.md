# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Launch group behind the `g` prefix: `g g` opens `lazygit`, `g j` opens `lazyjj` (refused with a toast when the repo has no `.jj`), and `g b` opens `but tui` (GitButler CLI); the prefix key and the sub-keys are remappable (`keybindings.git_tool`, `keybindings.lazyjj`, `keybindings.gitbutler`) and each launcher is recorded in the command log.
- Open the PR/MR form (`O`) as a floating modal over the still-visible dashboard, with base and head editable through type-to-filter branch pickers fed by a bounded on-demand ref read.
- Sync the current branch with the repo's *sync branch* with `s`: an explicit `git fetch origin`, then the configured `[commands] sync` base with `origin <sync>` appended; the result reports as a toast, not a card action block.

### Changed
- `g` no longer launches `lazygit` directly: it arms the launch prefix and the second key picks the tool, the same shape as the pull selector.
- The visual preview renders HTML (`--img-format html --open-in local`) and opens the page in `terminal-browser` beside the dashboard, falling back to `xdg-open` when it is absent. Rebase is offered only with git-sim 0.4.0 or newer; 0.3.5 crashes on divergent rebases.
- In the PR/MR form, `enter` in a branch picker commits the highlighted branch (or the typed text) and advances focus to the next field in tab order; with nothing to commit it is a no-op and the focus stays.
- The visual preview (`v`) no longer lends the terminal to git-sim: the render runs in the background (captured with `--output-only-path`, 60s timeout, the outcome as a toast) behind a centered loading overlay (spinner + variant, `esc` closes the overlay only), and the finished image opens with the desktop viewer; the image cache keeps the last 20 renders.
- Show the commits of the row under the cursor in a panel beside the repos table.
- Split the detail card into a fields column and a worktrees/files column.
- Drop the ACTIVITY and FETCH table columns; the fetch glyph now sits left of the repo name, and the last-commit age is a card field.
