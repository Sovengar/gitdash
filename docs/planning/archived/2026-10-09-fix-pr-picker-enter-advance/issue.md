# Issue: pr-picker-enter-advance

Status: implemented

`enter` in a PR branch picker commits the selection and advances to the next
field like `tab`; restructured `switch{case}` → `if/else-if` so the branch
conditions sit in covered blocks for the mutation gate.
