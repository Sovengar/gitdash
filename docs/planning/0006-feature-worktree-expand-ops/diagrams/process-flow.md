# Process flow — planning 0006-feature-worktree-expand-ops

Flujo de planificación con las decisiones reales de ESTE cambio.

```mermaid
flowchart TD
    S0[worktree setup<br/>wt switch --create feat/0006-worktree-expand-ops] --> S1[index check<br/>no existía → codebase-explorer]
    S1 --> S1b[Engram codebase-index/gitdash obs 2202<br/>codegraph init+sync = ready]
    S1b --> S2[state recovery<br/>obs #2200: pedido + decisiones cerradas]
    S2 --> S3[idea-refiner → issue.md]
    S3 --> C1{{CHECKPOINT issue<br/>APROBADA sin cambios}}
    C1 --> S4[brainstormer + architect en paralelo]
    S4 --> S4b[decisiones: kindWorktree · dedupe por path canónico<br/>expanded map namespace wt: · render dedicado<br/>space configurable · búsqueda transitoria]
    S4b --> S5[behavior.feature R30–R39]
    S5 --> C2{{CHECKPOINT behavior<br/>aprobado + añadido búsqueda / en v1}}
    C2 --> S5b[R40 + S40.# + issue.md IN item 9]
    S5b --> S6[plan.md<br/>adr_required: TRUE<br/>adr-0006-worktree-expansion-state-and-row-model]
    S6 --> S7[codegraph sync<br/>codebase-researcher → context.md<br/>freshness a681588d]
    S7 --> S8[diagramas<br/>feature-flow + process-flow]
    S8 --> C3{{CHECKPOINT plan<br/>pendiente}}
    C3 -- aprobado --> S9[commit<br/>chore: add 0006 plan]
    C3 -- feedback --> S6
```

Decisiones clave registradas para este cambio:
- `adr_required: true` (modelo de datos de persistencia + modelo de filas con alternativas descartadas).
- Scope cuts: sin estado git por worktree; sin tocar discovery/gitstatus/group/print; sin nuevas acciones.
- Riesgo principal: render dedicado de sub-fila (snapshot vacío mentiría).
