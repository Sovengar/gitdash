# Feature flow — dashboard restructure

Derived from `behavior.feature`. Two new degradable regions (commits panel, card split)
around the unchanged height contract.

```mermaid
flowchart TD
    A[renderDashboard] --> B[Top band: table + commits panel side by side]
    A --> C[Bottom card]

    B --> B1[repos table, 5 columns<br/>NAME BRANCH Work Tree updown SYNC]
    B1 --> B2[row prefix: 2-cell cursor + 2-cell fetch slot<br/>glyph+space: spinner / failure / blank]

    B --> G{panel costs the table a column?}
    G -- "yes (<=118)" --> G1[panel dropped<br/>commits not shown anywhere]
    G -- "no (>=119)" --> P{row under the cursor}
    P -- repo --> P1[its commits: sha / age / subject<br/>up to body lines + 1, newest first]
    P -- "worktree subrow + live snapshot" --> P2[that worktree's commits]
    P -- "worktree subrow, no snapshot" --> P3[dim placeholder<br/>never the parent's commits]
    P -- group header --> P4[group aggregate<br/>repos / errors / dirty / ahead / behind / wt]
    P -- none --> P5[empty hint<br/>same text as table and card]

    C --> S{inner width >= 61?}
    S -- "yes (>=63)" --> S1[left: path branch upstream state sync activity]
    S -- yes --> S2[right: worktrees + files lists<br/>per-list "... N more"]
    S -- "no (<=62)" --> S3[collapsed stack: fields, worktrees, files]
    C --> T["! input always at the end<br/>(both split and collapsed)"]

    C -.-> F{card fits with 6 head lines?}
    F -- "no (h=21)" --> F1[card not drawn]
    F -- "yes (h=22)" --> C

    L[log open or PR overlay] --> X[body replaced: no table, no panel, no card]
```

Notes:
- The height search (`computeLayout` / `panelCandidates` / `mismaChromeQue`) is unchanged in
  shape; only the card floor moves with `detailHeadLines` 5→6.
- The width gates are independent of each other and of the height gate.
