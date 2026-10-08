# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Sync the current branch with the repo's *sync branch* with `s`: an explicit `git fetch origin`, then the configured `[commands] sync` base with `origin <sync>` appended; the result reports as a toast, not a card action block.

### Changed
- The visual preview (`v`) no longer lends the terminal to git-sim: the render runs in the background (captured with `--output-only-path`, 60s timeout, the outcome as a toast) behind a centered loading overlay (spinner + variant, `esc` closes the overlay only), and the finished image opens with the desktop viewer; the image cache keeps the last 20 renders.
- Show the commits of the row under the cursor in a panel beside the repos table.
- Split the detail card into a fields column and a worktrees/files column.
- Drop the ACTIVITY and FETCH table columns; the fetch glyph now sits left of the repo name, and the last-commit age is a card field.
