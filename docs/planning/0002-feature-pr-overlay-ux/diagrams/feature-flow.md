# Feature flow — PR/MR form as a floating modal with searchable branch pickers

Derived from `behavior.feature`. The dashboard never stops painting: the form is a
modal composited on top, and the branch read happens once per open.

```mermaid
flowchart TD
  A["Dashboard · cursor on a repo row"] -->|"O"| B{"Modal fits terminal?"}
  B -->|"no"| B1["Toast: terminal too small · nothing executes"]
  B -->|"yes"| C["Floating modal centered over the still-visible dashboard · keyboard captured"]
  C --> D["Async read: for-each-ref refs/heads refs/remotes · ClassRead · one per open · path-guarded"]
  D --> E["Focus: title → base → head → draft → body · tab / shift+tab wrap"]
  E -->|"branch field focused"| F["Pane: loading → filtered list · no-match / read-error states"]
  F -->|"type"| G["Filter narrows (case-insensitive) · highlight → first match"]
  G -->|"up / down"| G2["Highlight moves, clamped · scroll window follows"]
  G2 -->|"enter"| H{"Match highlighted?"}
  H -->|"yes"| I["Field = highlighted ref · filter cleared · full list back"]
  H -->|"no · filter non-empty"| J["Field = typed text verbatim · detached sha · owner:branch"]
  I --> E
  J --> E
  E -->|"ctrl+t on body"| K["Summary/Test-plan skeleton inserted at cursor · text preserved"]
  E -->|"esc"| L["Modal closes · dashboard untouched · nothing executed"]
  E -->|"ctrl+s"| M{"title and base set?"}
  M -->|"no"| M1["Inline error · form stays"]
  M -->|"yes"| N["prStartMsg → forge exec · argv carries base/head · command log · form closes"]
  C -->|"resize"| P{"Still fits?"}
  P -->|"yes"| P1["Re-fit / re-center · text preserved"]
  P -->|"no"| P2["Close + warning toast"]
```

Key rules the scenarios pin:

- head defaults to the row's branch (worktree label when there is no snapshot);
  base defaults to the sync branch.
- The head picker lists local branches only (`origin/foo` is not a valid `-H`);
  the base picker lists locals + remote-tracking refs.
- The verbatim enter (no match) is the escape hatch: read failures and exotic
  refs never block submission.
- Submit path unchanged: `forge.Params` / `BuildCreateArgv` already carry base/head.
