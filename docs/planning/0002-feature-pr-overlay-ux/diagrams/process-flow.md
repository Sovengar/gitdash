# Process flow — planning `0002-feature-pr-overlay-ux`

Feature-aware: the real slug, the checkpoints, the forks decided, and the cuts
made for THIS change.

```mermaid
flowchart TD
  A["Branch feat/pr-overlay-ux · worktree gitdash.feat-pr-overlay-ux"] --> B["Index check: stale (aa2fb2c, pre-UI-restructure) · refresh: codegraph init -i @ 359998a + Engram index"]
  B --> C["idea-refiner → issue.md"]
  C --> C1{"Checkpoint 1 · issue — APPROVED"}
  C1 --> D["brainstormer ∥ architect · forks: overlay compositing · picker model · ref read scope/lifetime · body shape · head edge rows"]
  D --> E["behavior.feature · 18 scenarios"]
  E --> E1{"Checkpoint 2 · behavior — APPROVED · body confirmed modest: textarea + ctrl+t"}
  E1 --> F["plan.md · adr_required: true · modal-overlay-composition"]
  F --> F1{"Checkpoint 3 · plan — APPROVED"}
  F1 --> G["codebase-researcher → context.md · freshness 359998a · codegraph ready · files/symbols/lines · test map · 6 risks"]
  G --> H["Diagrams + commit planning package"]

  D -.-> D1["DECIDED: ANSI-safe splice over lipgloss Canvas (Canvas trims the exact-width padding)"]
  D -.-> D2["DECIDED: modal out of the layout budget; dashboard behind renders normally"]
  D -.-> D3["DECIDED: inline fixed-height picker; committed value + transient filter; verbatim enter"]
  D -.-> D4["DECIDED: async ClassRead ref read once per open; head = locals only"]
  D -.-> D5["CUT: dimming · popup picker · per-picker esc · refresh key · output cap · remote-as-head · structured body · markdown preview"]
```

Gate map carried into implementation:

- diff coverage 100% + mutation gate on non-draft PRs (minima as functions, pure
  helpers, conditional-free goroutine, pinned boundaries);
- `docs/FEATURES.md` updated in the same change;
- ADR `0002-modal-overlay-composition` written with the change;
- direct model tests, no teatest.
