# Plan — PR/MR form as a floating modal with searchable branch pickers

adr_required: true
(One decision has heavy, cross-package weight: the form leaves the layout budget
and is composited over the dashboard by an ANSI-safe splice — lipgloss Canvas is
rejected because its render trims the trailing spaces the bordered boxes pad
every line with, breaking the exact-width contract. ADR: `modal-overlay-composition`,
written as `docs/adr/0002-modal-overlay-composition.md` in the same change. The
picker and ref-read cuts are scoped and reversible and stay documented below.)

## Wanted outcome

`O` opens a floating modal over the still-visible, inert dashboard. Base and head
are both editable through type-to-filter pickers fed by a new bounded on-demand
ref read. The body is comfortable to write in. Submit behavior is unchanged:
`forge.Params` / `BuildCreateArgv` already carry the base/head flags.

## Approach (high level)

### Overlay composition

- A single ANSI-safe splice primitive, generalizing the existing toast overlay,
  is the compositor: render the dashboard normally, splice the centered modal,
  then let toasts paint last on top. No new compositor, no new dependency.
- The modal leaves the height/width budget: the `m.pr` special-cases disappear
  from the layout, so the dashboard behind measures exactly as if no form were
  open. This is the highest-risk edit of the change and needs its own pinning
  tests (no leaking height into the body, no panel state change).
- No dimming: it is explicitly cut, reversible, and would be another layer.

### Modal: size, focus, resize

- Content-sized and centered, with width clamped to the terminal; the minima are
  derived functions from their parts (never constants: Go does not instrument
  constant expressions and mutants would stay uncovered).
- Keyboard capture and field semantics stay as today (the modal is consulted
  before the normal routing; typing does not reach the table).
- Resize re-fits and re-centers when the modal still fits; below the minimum it
  closes with a warning. Opening below the minimum keeps today's refusal toast.

### Branch pickers

- One reusable picker component with two instances (base, head): the field holds
  the committed value; a transient filter and a highlight live beside it; a
  fixed-height list pane appears under the focused branch field, so filtering
  never resizes the modal per keystroke.
- Type-to-filter (case-insensitive contains), arrows move, enter commits the
  highlighted branch — or, with no match, the typed text verbatim. That verbatim
  path is the escape hatch for refs outside the list (detached sha, fork's
  `owner:branch`) and keeps the form usable when the ref read fails.
- Base offers local and remote-tracking branches; head offers local branches
  only, because a remote-tracking name is not a valid PR head for gh/glab and
  offering it would build a wrong submission.
- Pane states: loading, empty/no match, list error — all painted from state,
  owned by small pure helpers; ref names are untrusted text (sanitized,
  truncated by display width, padded before styling).
- `esc` closes the modal (single rule, today's contract); leaving a branch field
  discards the transient filter; tab order becomes the five fields
  (title, base, head, draft, body) with wrap-around.

### Ref read contract

- New bounded read in `gitstatus`: `for-each-ref` over `refs/heads` and
  `refs/remotes`, through the shared git runner with the read class so it is
  auditable in the command log. Pure parse helper strips prefixes, classifies
  local/remote, and drops `remotes/*/HEAD` symrefs. Git's refname sort provides
  deterministic ordering (locals, then remotes, alphabetical); no dedup, no
  output cap.
- Issued asynchronously when the modal opens — one read per form open serves both
  pickers, cached in the draft, never folded into the periodic scan. The result
  arrives as an event (rearm the pump); it is attributed to the repo it was read
  for and dropped when the modal has closed or moved on.

### Body

- Taller textarea in the content-sized modal; `ctrl+t` inserts a non-destructive
  `Summary` / `Test plan` skeleton at the cursor. No structured fields, no
  template file, no commit-based fill. The body reaches the argv verbatim.

### Submit path

- The draft's committed values feed `forge.Params` as today; no forge, config,
  discovery or command-log changes. Picker interactions are form interaction,
  not commands — no intents; the submit still logs intent + exec as today.

### Explicitly cut

Dimming behind the modal, popup/layer picker, per-picker esc, refresh key,
output cap, remote refs as head, `owner:branch` special handling, structured body
sections, markdown preview, generation-token staleness (a path guard suffices).

## Test strategy (gates)

- Everything is testable directly: model tests drive `Update` and inspect state,
  pure helpers get canned inputs (parsed refs, filtering, clamping, scroll,
  head/base resolution), and the splice gets width/height edge tests (wider,
  taller, exact).
- The ref read is pinned through its effect: one subprocess through the shared
  runner, read class, visible in the command log; the goroutine body stays
  conditional-free (call + send), with error folding into the message.
- Modal-fit boundary: exact minimum and one below; resize both branches
  (re-fit preserving text, close with warning); stale-result both drop cases
  (no modal, wrong path) and the accept case.
- Existing `proverlay_test.go` / `prcreate_test.go` are re-pinned where the
  overlay and the five-field order change; the submit seam, argv recording and
  sanitising stay pinned.
- The new-conditionals mutant diet: highlight clamp, filter bounds, scroll
  window, literal-enter fallback, pane state selection, modal clamps. Minima and
  derived sizes stay functions, not constants.

## Docs

- `docs/FEATURES.md`: update the `O` entry in the same change.
- `docs/adr/0002-modal-overlay-composition.md`: the overlay decision with the
  Canvas rejection rationale and the layout-budget ownership.

## Risks

1. **Layout migration** — removing the `m.pr` special-cases changes what the
   dashboard paints behind the modal; pinned by width/height-exact tests.
2. **ANSI splice correctness** — untrusted ref names and exact-width lines;
   sanitize + width truncation + pad-before-style, with boundary tests.
3. **Event pump / stale results** — a dropped rearm stalls the UI; a missing
   path guard paints another repo's branches.
4. **Key precedence** — picker keys (arrows, enter, runes) must not leak to the
   table nor steal the modal-global keys (tab, esc, ctrl+s).

## Coarse ordering

1. Ref read + pure parse (independently testable).
2. Draft state evolution (five fields, picker state) + async message/cmd seam.
3. Splice primitive + layout migration + modal render.
4. Picker pane, keys and states; body template key; sizing/resize.
5. Re-pin existing tests, add the new ones, update `docs/FEATURES.md`, write the ADR.
