Feature: PR/MR creation as a floating modal with searchable branch pickers

  # The `O` form stops replacing the dashboard: it floats on top of it, the
  # dashboard stays visible and inert behind it, and both branches become
  # editable through type-to-filter pickers fed by a bounded on-demand read.

  Scenario: Opening the form as a floating modal
    Given the dashboard is showing repos with their panels
    When the user presses the PR key on a repo row
    Then a bordered PR form appears centered over the still-visible dashboard
    And the repos table, commits panel, detail band and keybinds stay painted behind it
    And the keybinds section shows the form's legend

  Scenario: The modal owns the keyboard
    Given the PR form is open with the focus on the title
    When the user types "p"
    Then the character goes into the title field
    And the pull selector is not armed

  Scenario: Closing the form returns to the untouched dashboard
    Given the PR form is open
    When the user presses esc
    Then the form disappears
    And the dashboard is exactly what it was before opening it
    And no process was executed

  Scenario: The form refuses to open in a terminal that cannot hold it
    Given a terminal below the form's minimum size
    When the user presses the PR key
    Then a toast says the terminal is too small
    And no form opens

  Scenario: Defaults name the row's own branch and its sync branch
    Given the cursor is on a repo row whose branch is "feat/x" and whose sync branch is "main"
    When the form opens
    Then "head" starts as "feat/x"
    And "base" starts as "main"

  Scenario: The base picker lists local and remote-tracking branches
    Given the form is open with the focus on "base"
    When the branch list has loaded
    Then a list pane appears under the field
    And it lists local branches and remote-tracking branches (e.g. "origin/main")
    And the entry matching the current base value is highlighted
    And remote HEAD symbolic refs are not listed

  Scenario: Type-to-filter narrows the branch list
    Given the focus is on a branch field with the branch list loaded
    When the user types "dev"
    Then the pane lists only branches whose name contains "dev"
    And the highlight moves to the first match

  Scenario: Selecting a branch with the keyboard
    Given a filtered list with matches
    When the user moves the highlight with the arrow keys and presses enter
    Then the field takes the highlighted branch's name
    And the filter is cleared and the pane shows the full list again

  Scenario: The highlight is clamped, never out of the list
    Given a filtered list
    When the user presses down past the last entry or up past the first
    Then the highlight stays on a valid entry

  Scenario: A typed ref that matches nothing is used verbatim
    Given the focus is on a branch field with a non-empty filter and no matches
    When the user presses enter
    Then the field takes the typed text as its value
    And this is the escape hatch for refs outside the list, such as a detached sha or a fork's owner:branch

  Scenario: The head picker never offers remote-tracking names
    Given the form is open with the focus on "head"
    Then the pane lists local branches only
    # "origin/feat" is not a valid PR head for gh/glab, so offering it would build a wrong submission.

  Scenario: The branch list loads on demand and is auditable
    Given the dashboard is running its periodic scan
    Then the periodic scan does not list branches
    When the user opens the PR form
    Then a single bounded ref read runs for that repo, and only for that repo
    And it appears in the command log panel as a read entry
    When the form is closed and opened again
    Then a fresh ref read starts

  Scenario: The branch list is being loaded
    Given the form was just opened
    When the focus is on a branch field before the read returns
    Then the pane says the branches are loading
    And the form stays usable

  Scenario: The branch list cannot be read
    Given the ref read fails for the repo
    When the focus is on a branch field
    Then the pane says the branches could not be listed
    And the field keeps its value
    And typing a ref by hand and pressing enter still works

  Scenario: Tab moves through the five fields and wraps
    Given the form is open
    When the user presses tab or shift+tab repeatedly
    Then the focus visits title, base, head, draft, body and wraps around
    And leaving a branch field clears its transient filter

  Scenario: The body is a comfortable editor with an optional skeleton
    Given the focus is on the body
    When the user presses the template key
    Then a Summary/Test-plan skeleton is inserted at the cursor
    And text already written is preserved
    And the body keeps reaching the forge verbatim, newlines included

  Scenario: Submitting sends the chosen branches to the forge
    Given title, base and head are filled
    When the user presses ctrl+s
    Then the submission is accepted and the form closes
    And the forge command runs with the base and head flags carrying the chosen values
    And the execution appears in the command log with its argv

  Scenario: Resizing while open keeps the form when it still fits
    Given the form is open with text typed
    When the terminal is resized but still holds the form
    Then the form stays open and centered, with its text untouched
    When the terminal shrinks below the form's minimum
    Then the form closes with a warning
