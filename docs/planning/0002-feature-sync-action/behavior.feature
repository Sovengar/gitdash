# `s sync` — update the current branch with the repo's sync branch
#
# For user verification. Gherkin, but NOT wired to a runner: the executor turns
# these scenarios into the real Go tests.
#
# Key mechanics: `s` runs an explicit `git fetch origin` and then the configured
# `[commands] sync` base (default `pull --rebase --autostash`) with `origin` and the
# repo's resolved sync branch appended. The resolved branch is the marker's
# `sync_branch` over the global default — NOT the SYNC column's legacy-`master`
# fallback probe (that probe is display-only). The result reports as a toast; the
# preview card's action block is unused and cleared; `l` keeps the full argv+output
# audit. Nothing is ever pushed.

Feature: `s sync` updates the current branch with the repo's sync branch

  Background:
    Given a dashboard with a repo whose branch is "feature/paint" and whose resolved sync branch is "main"
    And the repo's remote "origin" exists
    And the cursor is on that repo's row

  Scenario: Sync brings the current branch up to date with origin/<sync>
    When the user presses "s"
    Then "git fetch origin" runs first
    And then the sync pull runs as "git pull --rebase --autostash origin main"
    And the repo's state is re-collected
    And a success toast says "sync ok <repo> — <outcome>", with the outcome classified from the pull's output: rebase, rebase+autostash, merge, fast-forward or up-to-date
    And the toast never shows the argv

  Scenario: A custom sync base keeps only the ref out of reach
    Given the user configured `[commands] sync = "pull --ff-only"`
    When the user presses "s"
    Then the sync pull runs as "git pull --ff-only origin main"

  Scenario: A finished sync leaves no action block in the preview card
    Given the repo's card is showing a previous action block
    When a sync of that repo finishes
    Then the card's action block for that repo is gone, on success and on failure
    And the command log keeps the audit: one "sync" intent plus both execs, in order, with their argv and output

  Scenario: An unclassifiable success has no dangling separator
    Given the pull's output matches no known outcome
    When the sync finishes
    Then the toast is exactly "sync ok <repo>", with no " — " suffix

  Scenario: While it runs, the activity indicator lists the sync
    Given a sync of the repo is in flight
    Then the stats banner shows the running action as "sync <repo>"

  Scenario: Refuse when the current branch already is the sync branch
    Given the repo's snapshot says its current branch is "main"
    When the user presses "s"
    Then an info toast says "main is the sync branch — nothing to sync"
    And nothing executes: no fetch, no pull and no exec entry in the command log
    And the intent is still recorded

  Scenario: A branch the snapshot cannot pin down does not trigger the refusal
    Given the repo's snapshot does not say the current branch is the sync branch (not collected yet, stale, or detached HEAD)
    When the user presses "s"
    Then the sync runs and git's own result governs: a success toast with the classified outcome (e.g. up-to-date) or a failure toast with git's reason

  Scenario: A failed fetch stops before the pull
    Given "git fetch origin" fails
    When the user presses "s"
    Then the toast reports "sync failed <repo>: <git's reason>"
    And the pull never runs
    And the command log shows only the failed fetch exec

  Scenario: A failed pull reports git's reason and the mid-rebase warning
    Given the fetch succeeds
    And the pull fails leaving a rebase half-done
    When the user presses "s"
    Then the toast reports "sync failed <repo>: <git's reason>" plus the mid-rebase warning
    And no classified outcome is appended

  Scenario: The sync branch does not exist on origin
    Given the resolved sync branch "main" does not exist on "origin"
    When the user presses "s"
    Then the fetch succeeds and the pull fails with git's "couldn't find remote ref" reason
    And the failure is reported as a toast; no fallback ref is invented
    # the SYNC column may have shown the legacy "master" fallback: that probe is display-only

  Scenario: No sync branch resolves
    Given no sync branch resolves for the repo
    When the user presses "s"
    Then a warning toast says "no sync branch configured"
    And nothing executes

  Scenario: The sync command resolves to no arguments
    Given `[commands] sync` resolves to no arguments
    When the user presses "s"
    Then a warning toast says "empty sync command"
    And nothing executes

  Scenario: The repo already has an action running
    Given the repo has another action in flight
    When the user presses "s"
    Then a warning toast says that action is already running in the repo
    And nothing executes

  Scenario: The row has no git repo
    Given the row's project has no git repo
    When the user presses "s"
    Then an info toast says "no git repo — nothing to do"
    And nothing executes
    And the intent is still recorded

  Scenario: The key is discoverable and rebindable
    Given the dashboard is painted
    Then the hint bar announces the sync action with its configured key ("s" by default)
    And rebinding `keybindings.sync` moves the action with its label and intent

  Scenario: Sync also runs with the command-log panel open
    Given the command log panel is open
    When the user presses "s" on the repo row
    Then the sync runs and its execs show up in the panel

  Scenario: A worktree sub-row resolves against the global default
    Given the cursor is on a worktree sub-row whose path is not itself a discovered project
    When the user presses "s"
    Then the sync runs in the worktree directory
    And the sync branch is the global default: the parent repo's marker override is not consulted
    And the same-branch refusal cannot fire, because a worktree sub-row has no snapshot of its own
    # current prototype behavior; flagged at the behavior checkpoint as the one
    # point to accept consciously or change in a follow-up
