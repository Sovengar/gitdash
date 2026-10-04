# Expected behaviour of "git-sim pull preview".
# Single source of behaviour, for the user's verification.
# It is NOT Cucumber: no step definitions, no runner. Keywords in English.
# Descriptions in the repo's language (English).
#
# Key convention: the keys are the default config's ones.
#   v = visual (arming), p/m/r = variants of the visual selector.
#   p is still pull and r is still rescan: the visual variants
#   are consumed BEFORE the normal routing while the selector is armed.

Feature: Visual pull/merge/rebase preview with git-sim

  # --- Trigger and arming -------------------------------------------------

  Scenario: The visual key arms the selector without running anything
    Given the cursor's row is a git repo with an upstream
    When the user presses `v`
    Then the visual selector is armed with that row's path and upstream
    And no process is launched
    And the keybinds section shows the visual selector's warning
    And the hints are replaced by that warning

  Scenario: With no row under the cursor it does not arm
    Given the cursor is on a group header (or the table is empty)
    When the user presses `v`
    Then a "no git repo — nothing to do" toast appears
    And the visual selector is not armed

  Scenario: On a row with no repo it does not arm
    Given the cursor's row is a project with no git repo
    When the user presses `v`
    Then a "no git repo — nothing to do" toast appears
    And the visual selector is not armed

  Scenario: A key that is not a variant cancels the arming and carries on
    Given the visual selector is armed
    When the user presses a key that is not `p`, `m`, `r` nor `esc`
    Then the selector is disarmed
    And that key is processed like any other dashboard key

  Scenario: The arming keeps the right row even if the cursor moves
    Given the visual selector is armed on repo A
    When the variant comes to be resolved
    Then the gateway points at the path and upstream captured when arming (repo A),
      not at whatever row was under the cursor afterwards

  # --- Variants -----------------------------------------------------------

  Scenario: The p variant launches git-sim pull with no positional arguments
    Given the visual selector is armed on a repo
    When the user presses `p`
    Then the `git-sim` handoff is launched with argv
      ["git-sim", "--media-dir", <cache>/gitdash/git-sim, "pull"]
    And the selector is disarmed

  Scenario: The m variant launches git-sim merge with the branch's upstream
    Given the visual selector is armed on a repo with upstream `origin/main`
    When the user presses `m`
    Then the `git-sim` handoff is launched with argv
      ["git-sim", "--media-dir", <cache>/gitdash/git-sim, "merge", "origin/main"]
    And the selector is disarmed

  Scenario: The r variant launches git-sim rebase with the branch's upstream
    Given the visual selector is armed on a repo with upstream `origin/main`
    When the user presses `r`
    Then the `git-sim` handoff is launched with argv
      ["git-sim", "--media-dir", <cache>/gitdash/git-sim, "rebase", "origin/main"]
    And the selector is disarmed

  Scenario: With no upstream, merge and rebase launch nothing
    Given the visual selector is armed on a repo with NO branch upstream
    When the user presses `m` or `r`
    Then a "no upstream" toast appears
    And no handoff is launched

  Scenario: With no upstream, the p variant is allowed
    Given the visual selector is armed on a repo with NO branch upstream
    When the user presses `p`
    Then the `git-sim pull` handoff is launched
    # git-sim pull simulates the operation; it does not require an explicit ref like merge/rebase.

  # --- Output and flags ---------------------------------------------------

  Scenario: The media-dir always points at gitdash's cache, never the repo
    Given any visual variant
    Then the argv includes `--media-dir <gitdash's XDG cache>/git-sim`
    And the directory is created if it does not exist
    And the repo is NOT left with `git-sim_media/` inside (it is not marked dirty)

  Scenario: Auto-open is not disabled and nothing is animated
    Given any visual variant
    Then the argv does NOT include `-d` (git-sim's auto-open stays active)
    And the argv does NOT include `--animate`

  # --- Handoff and command log ---------------------------------------------

  Scenario: With the binary missing from PATH there is only a toast
    Given the visual selector is armed and `git-sim` is not in PATH
    When the user picks a variant
    Then a "git-sim not installed" toast appears
    And there is no terminal handoff

  Scenario: If the media-dir cannot be created, nothing is launched
    Given git-sim's cache directory cannot be created
    When the user picks a variant
    Then an error toast appears
    And the handoff is not launched (launching it would dirty the repo)

  Scenario: On return from the handoff the exec is recorded and it re-collects
    Given a `git-sim` handoff was launched
    When the child finishes and the terminal returns to gitdash
    Then the command log records an exec with the REAL resolved argv
      (including `--media-dir` and the ref) and Dur=0
    And the intent (key + variant + repo) was recorded when it was chosen
    And the repo's state is re-collected

  Scenario: The visual selector neither consults nor blocks on a rebase in progress
    Given the repo has a mid-rebase
    When the user picks a visual variant
    Then the handoff is launched all the same
    # git-sim does not mutate the real repo: previewing a rebase is exactly what is useful there.

  # --- Config and hints ---------------------------------------------------

  Scenario: The visual action is configurable and shows up in the hints
    Given the default config
    Then `visual` exists in the default keybindings with the key `v`
    And the hints include "v visual" (label without the key inside)
    And an old config using a nonexistent action still warns

  Scenario: The selector's warning is painted through keybinds' single point
    Given the visual selector is armed
    Then promptLine() returns the visual selector's warning
    And keybindsLines() is 1 (the warning replaces the hints)