# Feature flow — 0006 worktrees expandibles y operables

Flujo de comportamiento derivado de `behavior.feature` (R30–R40).

```mermaid
flowchart TD
    A[Tabla principal<br/>repos descubiertos] --> B{El repo del cursor<br/>tiene worktrees?<br/>len Snapshot.Worktrees &gt; 0}
    B -- No --> B1[space = no-op<br/>sin glyph, sin contador]
    B -- Sí --> C[NAME: ▸ (N wt)<br/>plegado por defecto]

    C -- space --> D[NAME: ▾ (N wt)<br/>expandido]
    D --> E[Sub-filas: TODOS los worktrees<br/>de worktree list --porcelain]
    E --> E1[sin marcador]
    E --> E2[fuera de roots]
    E --> E3[descubierto con marcador<br/>dedupe: 1 sola vez]

    D --> F[Cursor j/k sobre la sub-fila]
    F --> G[Operaciones por path del worktree<br/>fetch · pull · sync · push<br/>lazygit · update · editor<br/>recollect · ! · shell]
    F --> H[enter → detalle mínimo<br/>path · rama/detached · head]
    G --> I[Notificación con basename]
    H --> I

    D --> J[Persistencia<br/>collapsed.json namespace wt:&lt;path&gt;]
    J --> K[Reinicio → sigue expandido]

    C --> L[space de nuevo]
    L --> M[plegado; cursor clamp dentro de rango]

    A --> N{/ búsqueda}
    N -- matchea rama o basename de un worktree --> O[padre visible + expandido<br/>solo sub-filas que matchean<br/>TRANSITORIO: no persiste]
    N -- limpiar búsqueda --> C
    N -- sin match --> P[sin filas]

    Q[filtro n / grupo plegado] --> R[padre oculto → sus sub-filas también]
```

Notas:
- Las sub-filas nunca pasan por el render de filas de repo (evita estado falso).
- El huérfano (principal no descubierto) sigue top-level con `[wt]` y **no** es expandible.
- `tab` sobre una sub-fila es no-op; `space` fuera de un repo con worktrees es no-op.
