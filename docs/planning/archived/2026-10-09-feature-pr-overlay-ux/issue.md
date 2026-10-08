# Issue — PR/MR form as a floating modal with searchable branch pickers

## Problem

The `O` PR/MR form is functional but hostile to a human, in five concrete ways:

1. **It feels like a new window, not an overlay.** `O` replaces the whole body
   (table + preview band + commits panel) with the form; the dashboard the user
   was looking at disappears, so there is no context behind the form and the
   transition reads as leaving the app.
2. **It is hard to use in general** — fields, focus and feedback do not make
   typing a PR comfortable.
3. **The head branch is not editable.** It is captured when the form opens and
   rendered as a dim, read-only label (`prDraft.head`). A user who wants to open
   the PR from a different branch must leave, change branch and reopen.
4. **Branch choice is weak.** The base is a free-text input pre-filled with the
   sync branch: the user types a ref from memory and typos produce a PR against
   the wrong branch, or against none. There is no list, no filter, no
   autocompletion.
5. **The body field UX is bad.** A plain textarea is the whole experience: no
   convenience for the text people actually paste into a PR description.

## Scope

### In

- **Floating modal overlay.** The `O` form becomes a modal composited *on top
  of* the still-visible dashboard, in the same spirit as the toast overlay. The
  dashboard stays painted behind/around it; the form keeps capturing the whole
  keyboard while open (modal focus). How the base is visually de-emphasised
  behind the overlay is a next-phase decision.
- **Editable branch pickers for BOTH base and head**, replacing today's
  free-text base and read-only head. Each must be a **searchable picker with
  type-to-filter autocompletion**: the user filters an enumerated branch list by
  typing and picks a match, rather than typing a ref from memory.
- **A new bounded, on-demand branch enumeration read.** No branch listing exists
  anywhere today. It must be a `gitstatus` read going through
  `runGit`/`runGitCombined` with `ClassRead` (never a raw `exec.Command`), issued
  on demand when the picker needs it, not folded into the periodic `Collect`
  scan. It must cover both local heads and remote-tracking refs, since a PR head
  or base is frequently a remote branch.
- **Body field UX redesign**, matching a real PR-writing workflow.
- **General usability pass** on the form: field navigation, labels, validation
  feedback and sizing so typing a PR is comfortable.

### Out

- **The submit/execution path, beyond what Base/Head editing requires.** The
  recon confirms `forge.Params` already carries `Title/Body/Base/Head/Draft` and
  `BuildCreateArgv` already emits the base/head flags for both `gh` and `glab`,
  so editing base and head is expected to flow through unchanged. Any change here
  is only whatever the pickers strictly force, and must be stated explicitly if
  it happens.
- **Terminal handoff / no-capture semantics.** The creation stays a captured,
  measured, non-interactive exec recorded in the command log.
- New forge capabilities (labels, reviewers, assignees, templates, milestone).
- Changes to discovery, config schema (beyond what the feature strictly needs) or
  the `Collect` scan.
- The exact layout maths of the modal and the picker interaction model — decided
  in the next phase, not prescribed here.

## Constraints

- **Coverage:** the PR's diff must reach 100% (the repo gate). The branch read
  and the picker logic are new code that must be exercised directly.
- **Mutation gate:** runs on non-draft PRs; a surviving mutant fails. New
  conditionals (filtering, ref selection, bounds) need tests that kill them.
- **Tests:** direct model tests (build the `Model`, send msgs through `Update`,
  inspect state) — no `teatest`. Existing `proverlay_test.go` /
  `prcreate_test.go` pin today's behavior and will need updating where the
  overlay and the pickers change it; the submit seam
  (`prPending` → `prStartMsg` → `prCreateCmd`), argv recording and sanitising
  must stay pinned.
- **Wiring:** every git exec goes through `gitstatus.runGit`/`runGitCombined`;
  the branch read leaves through them and appears in the command log.
- **Comments:** English, WHY-only, one line where possible.
- **No SDD artefacts / no requirement or scenario IDs** anywhere in the code.
- **`docs/FEATURES.md`** must be updated in the same change: this is a
  user-visible change to the `O` PR/MR entry.

## Open questions (resolve in the next phase — do not prescribe now)

- **Overlay compositing mechanism:** lipgloss v2.0.6 ships `Canvas`/`Layer`
  (`NewCanvas`/`NewLayer`/`Compose`/`Render`), but the repo already has an
  ANSI-safe manual splice precedent (`overlayToasts`). Which to use, how the
  dashboard behind is painted/dimmed, and how the modal coexists with toasts.
- **Picker interaction model:** inline dropdown vs a separate popup layer; how
  filtering, selection and cancel map onto the current tab/wrap field order; what
  happens with no matches or an empty repo; ordering and de-duplication of local
  vs remote-tracking refs.
- **Branch read scope and lifetime:** which refs exactly (local + remote-tracking,
  and whether the default-branch remote is included), error handling when the
  read fails, and whether it is read once on open or refreshed per interaction.
- **Body UX shape:** what replaces the bare textarea and how it fits the modal.
- **Modal sizing/placement:** content-sized vs fixed, minimum-terminal behavior,
  and what happens on resize while open (today it closes the form).
- **Head semantics on edge rows:** detached HEAD, a worktree subrow with no
  snapshot, or a repo whose current branch is not among the enumerated refs.
