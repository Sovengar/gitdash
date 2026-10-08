# ADR 0001 — Layout budget ownership

- Status: accepted
- Date: 2026-10-07

## Context

The dashboard is a stack of bordered sections whose additivity contract is a
single height budget assembled by `computeLayout`/`m.layout()`. The restructure
introduces side-by-side panes: the repos table with a commits panel to its right,
and a detail card split into a fields column and a lists column. A side-by-side
split needs widths, and widths were never part of the budget: each renderer read
`m.width` on its own (`m.section`, `table.go`, `detail.go`).

Two width truths are the failure mode: if the layout decides a pane is 30 cells
wide and the pane still reads the terminal's width to clip its columns, the pane
overflows its own border. The same class of bug already exists on the height
axis, which is why there is exactly one `layout` value today.

## Decision

`layout` is the single owner of the dashboard budget, both height and width.
`computeLayout` stays height-only and keeps the additive-degradation search;
`m.layout()` assembles the width split on top of it (`showPanel`, `tableWidth`,
`panelWidth`, `cardWidth`, `cardSplit`). Every pane renderer receives its width
as an argument and never reads `m.width` for the panes it composes:

- the table and the commits panel get `lay.tableWidth` / `lay.panelWidth`;
- the card gets `lay.cardWidth` and decides split vs. stacked from `lay.cardSplit`;
- full-width sections (stats, filter, keybinds and the log) keep
  `m.width`, because they are never split.

The PR form is no longer one of those full-width sections: ADR 0002 moved it out
of the budget and onto a compositor over the dashboard, so it does not appear in
`layout` at all.

`--print` is out of scope and keeps its own columns and ordering; no column
model is shared with the TUI.

## Alternatives discarded

- **A new composition layer** between `layout` and the renderers: it would still
  need the same width fields and adds a hop, with two places able to compute the
  same split.
- **Self-sizing sections** (each pane measuring itself and telling the caller):
  the split is a joint decision — the commits panel only enters if the table
  keeps its columns — so a pane cannot decide alone. It also gives no single
  place where width-driven degradation can be asserted.
- **Widths in the model** (`m.tableWidth` set during update): the pane widths are
  a pure function of `m.width` and the constants; storing them duplicates state
  that can go stale on resize.

## Consequences

- One value to test: the width boundaries (`119` for the panel, `63` for the card
  split) are asserted against `m.layout()` and the render can only agree with it.
- New pane renderers must take a width; a renderer that reads `m.width` to clip a
  sub-pane is a bug by construction.
- The height contract is untouched: the commits panel shares the table's box
  height and gives back nothing, so terminal lines stay conserved.
