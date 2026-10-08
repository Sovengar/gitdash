# ADR 0002 — Modal overlay composition

- Status: accepted
- Date: 2026-10-09

## Context

The PR/MR form (`O`) started as another section of the dashboard: `m.layout()`
carried a `formMin` parameter, `computeLayout` skipped its preview-panel search
while a form was open, and `renderDashboard` replaced the whole body with the
form. That worked while the form was the only thing on screen, but it coupled the
form's minimum to the dashboard's height budget and made the form fight the
preview panel and the commits panel for the same lines: opening the form blanked
the dashboard, and a form that did not fit the derived `bodyLines` could not be
drawn at all.

The feature asks for something different: `O` opens a modal that floats over a
still-visible, inert dashboard, sized by its own content and centred, with the
dashboard painting normally behind it and the keybinds section showing the form's
legend as its footer. A branch picker also needs a fixed-height list pane whose
height does not change per keystroke.

## Decision

The modal is **not a section**: it leaves the layout budget entirely and is
composited over the dashboard by a single ANSI-safe splice.

- `computeLayout`/`m.layout()` lose the `formMin` parameter and the `m.pr`
  special-cases; the height of the table, preview and stats behind it is the same
  as with no form open. The one intended difference is the keybinds band: the
  modal publishes its legend through `promptLine()`, so with the form open
  keybinds is 1 line (the legend) instead of `min(defaultHintLines, hints)`. That
  is the footer the behavior asks for, not a leak of the form into the budget.
- `spliceModal(base, modal, width, height)` is the one compositor: the dashboard
  is rendered normally, the modal is centred, and each of its lines replaces the
  cells it covers (`ansi.Truncate` + block + `ansi.TruncateLeft`), which preserves
  the terminal's exact width. It generalises the primitive `overlayToasts` already
  used for toasts; toasts still paint last, on top of the modal.
- The modal is content-sized: width is the terminal minus a margin clamped to the
  terminal and to the fields' derived minimum; height is a preferred content
  height, shrunk to the terminal but never below its derived minimum. Opening
  below the minimum refuses with a toast; a resize below it closes with a warning.
- The picker pane reserves a fixed number of lines inside the modal (filter line +
  window), so tabbing between fields and filtering never resize the box.
- Every modal minimum is a derived **function** of its parts (`prFixedLines`,
  `prPaneLines`, `prBodyMinLines`, `prMinWidth`), never a bare constant: Go does
  not instrument constant expressions, so a constant would leave its arithmetic
  mutant permanently uncovered.

## Alternatives discarded

- **lipgloss Canvas** as the compositor: its render trims the trailing spaces the
  bordered boxes pad every line with, which breaks the exact-width contract the
  dashboard relies on. The splice never trims.
- **A second layout pass** that treats the modal as a section with its own budget:
  it is what the form did before, and it forces the dashboard to shrink (or the
  form to be clipped) instead of letting the modal float.
- **A popup/layer widget library**: a new dependency and its own render pipeline
  for a single overlay; the splice reuses the toast primitive already proven
  ANSI-safe.
- **Dimming the dashboard**: deliberately cut — it is another full-screen layer
  with its own colour accounting and adds nothing to the form's usability.

## Consequences

- `renderDashboard` is the dashboard and only the dashboard; `View` is the single
  place that knows the composition order (modal, then toasts).
- The modal's fit check lives with the modal (`prFits`, derived minima vs.
  `m.height`/`m.width`), not with the table budget; `sections.go`'s freed-lines
  branch stays for the log panel only.
- The exact-width contract is testable at the seam: the splice gets width/height
  edge tests (wider, taller, exact) and the modal's lines are exactly
  `prModalWidth` wide, so the spliced view keeps `m.width` on every line.
- A new full-screen overlay must decide at this seam; adding a `m.<thing> != nil`
  branch back into `computeLayout` would re-introduce the coupling this ADR
  removes.
