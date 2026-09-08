# Proposal: sync branch, worktrees plegados, grupos y renombrado del marcador

Cambio: `0002-feature-sync-worktrees-groups`
Base: evolución del dashboard 0001 (4 features acopladas al contrato de la tabla)

## Problema

1. **Desviación vs rama de referencia**: el ↑↓ actual solo mide contra el
   upstream de la rama actual. El caso de uso real del usuario es CI-first:
   "¿mi rama de desarrollo (`feat/ABC`) tiene los últimos cambios de la rama
   más actualizada (release/main)?" Eso requiere comparar contra una rama
   elegida (sync branch), no contra el upstream.
2. **Worktrees contaminan la tabla**: cada worktree con marcador es una fila
   más; lo normal es querer la vista del repo principal y, solo si interesa,
   los worktrees.
3. **Grupos sin jerarquía visual**: `group` del marcador es una columna más;
   no permite plegar/desplegar ni tener una visión por grupo (patrón ya
   probado en vroom R24).
4. **Nombre del marcador**: `.gitdash.toml` es genérico y colisionable; `.gitdash.toml`
   identifica la herramienta. Cambio duro, sin fallback.

## Propuesta

- Columna nueva adicional `SYNC` (se mantiene ↑↓ vs upstream): behind de la
  rama actual respecto a la sync branch (`git rev-list --count
  <rama>..<sync>`). Sync branch: `sync_branch` global en config.toml +
  override por repo en el marcador. Solo behind (la métrica del caso de uso).
- Worktrees: `git worktree list --porcelain` en la recolección; filas de
  worktree ocultas; indicador `(N wt)` en el repo principal; sección
  `worktrees (N)` extendida en el detalle (Enter sigue abriendo detalle).
  Fallback: worktree cuyo repo principal no está descubierto sigue visible.
- Grupos estilo vroom (R24): siempre agrupado si hay groups, bloques en la
  posición del primer miembro tras el sort attention-first, headers
  navegables y plegables, sección "(ungrouped)" plegable al final.
- Marcador renombrado a `.gitdash.toml` (default actualizable vía `marker=`).

## Fuera de alcance

- Ahead vs sync (solo behind), fetch de la sync branch (la frescura depende
  de la ref local), persistencia del estado plegado entre sesiones,
  acciones grupales (fetch/pull de todo un grupo).

## Alternativas descartadas

- Reemplazar ↑↓ por la sync column: pierde la métrica del upstream (S8).
- Detección de worktrees solo por marcador: omite worktrees fuera de los
  roots (worktrunk crea directorios hermanos) y exige marcador en cada wt.
- Modo de agrupación togglable: vroom ya validó "siempre agrupado si hay"
  y evita duplicar el orden de la tabla.
