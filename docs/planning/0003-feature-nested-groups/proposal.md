# Proposal: agrupación jerárquica primary_group/secondary_group

Cambio: `0003-feature-nested-groups`
Base: evolución de 0002 (amplía R16; reemplaza la clave `group` del marcador)

## Problema

1. **Un solo nivel de agrupación**: el `group` actual es plano. El caso de uso
   real es tener un proyecto (p. ej. `vsocial`) y dentro agrupar por área
   (backend, frontend, infra). Hoy eso obliga a elegir un único eje o a
   fragmentar en grupos sueltos sin jerarquía.
2. **Clave ambigua**: `group` no comunica si es el nivel superior o el único;
   al introducir el segundo nivel conviene nombrar explícitamente ambos ejes.

## Propuesta

- Marcador: `group` se **reemplaza** (cambio duro, sin fallback) por
  `primary_group` (nivel superior, p. ej. proyecto) y `secondary_group`
  (nivel interno, p. ej. backend/frontend/infra).
- TUI: la vista agrupada pasa a dos niveles de headers plegables con el
  patrón vroom ya validado (bloque contiguo en la posición del primer
  miembro tras el sort attention-first):
  - header primario `▾ <primary> (n)` en columna 0,
  - header secundario `▾ <secondary> (n)` indentado dentro del primario,
  - ambos plegables con `tab`/`enter`; plegar un primario oculta también
    sus headers secundarios.
- Migración: los `.gitdash.toml` existentes del usuario (14 repos vsocial)
  pasan a `primary_group = "vsocial"` + `secondary_group` por área. El
  fixture generator (`gen-fixtures.sh`) y tests migran a las claves nuevas.
- Cache: `repos.json` versión 3 con `primary_group`/`secondary_group`
  (el cache v2 existente se ignora silenciosamente, ya cubierto por el
  check de versión).

## Fuera de alcance

- Más de dos niveles de anidación, acciones grupales (fetch/pull por grupo),
  persistencia del estado plegado entre sesiones, orden custom de grupos
  (sigue "posición del primer miembro").

## Alternativas descartadas

- Mantener `group` como alias de `primary_group`: dos claves para lo mismo
  confunde el spec y alarga la deprecación; el usuario prefirió reemplazo
  duro y migrar todos los marcadores (incluye fixtures/tests).
- `secondary_group` sin `primary` asciende a nivel superior: rompe la
  semántica "secundario solo existe dentro de un primario"; se ignora y
  cae en `(ungrouped)`.
- Pseudo-header `(general)` para repos con primario pero sin secundario:
  añade ruido visual; esos repos se muestran directamente bajo el primario.
