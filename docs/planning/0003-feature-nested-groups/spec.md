# Spec: agrupación jerárquica primary_group/secondary_group

Cambio: `0003-feature-nested-groups`
Base: 0002 (reemplaza la clave `group` de R4/R16; conserva el patrón
vroom de bloques contiguos y la sección ungrouped)

## Requirements

### R18: Claves del marcador primary_group/secondary_group

El marcador `.gitdash.toml` SHALL aceptar `primary_group` y `secondary_group`
(string, opcionales). La clave `group` SHALL desaparecer: un marcador que
solo defina `group` SHALL NO agrupar (cambio duro, sin fallback). La
descomposición SHALL ser:

| primary_group | secondary_group | efecto |
|---|---|---|
| definido | definido | dos niveles de headers |
| definido | vacío | un nivel: repos directos bajo el primario |
| vacío | definido | el secondary SHALL ignorarse → fila sin grupo |
| vacío | vacío | sección `(ungrouped)` |

El campo `Project.Group` SHALL renombrarse a `PrimaryGroup` y SHALL añadirse
`SecondaryGroup`. La búsqueda `/` SHALL matchear name, primary y secondary.

#### S18.1: dos niveles definidos

- GIVEN un marcador con `primary_group = "vsocial"` y `secondary_group = "backend"`
- WHEN se descubre
- THEN el proyecto reporta PrimaryGroup=vsocial, SecondaryGroup=backend

#### S18.2: group ya no agrupa

- GIVEN un marcador con solo `group = "backend"` (clave vieja)
- WHEN se descubre y renderiza
- THEN la fila aparece en `(ungrouped)` sin header `backend`

#### S18.3: secondary sin primary se ignora

- GIVEN un marcador con solo `secondary_group = "infra"`
- WHEN se descubre
- THEN SecondaryGroup se descarta y la fila cae en `(ungrouped)`

#### S18.4: primario sin secundario

- GIVEN un marcador con solo `primary_group = "misc"`
- WHEN se agrupa
- THEN la fila va directamente bajo el header `misc`, sin header secundario

#### S18.5: búsqueda matchea ambos ejes

- GIVEN repos con primary=vsocial y secondary=backend/frontend
- WHEN se filtra con `/backend`
- THEN matchean las filas cuyo secondary o primary (o name) contengan "backend"

### R19: Arrangement jerárquico de dos niveles

SI existe algún proyecto con `primary_group` no vacío, la vista SHALL agrupar
siempre en dos niveles (sin modo toggle). Tras el sort attention-first:

- el bloque de un primario SHALL emitirse completo en la posición de su
  primer miembro (patrón vroom 0002 R16);
- dentro de un primario, el bloque de cada secundario SHALL emitirse
  contiguo en la posición de su primer miembro;
- los repos con primario pero sin secundario SHALL conservar su posición
  de sort dentro del primario (sin header propio);
- los proyectos sin primario SHALL formar la sección `(ungrouped)` plegable
  al final, con un solo nivel (sin headers secundarios).

Los grupos sin miembros visibles tras filtrar SHALL desaparecer.

#### S19.1: bloques anidados contiguos

- GIVEN filas ordenadas [A(sec=backend,prim=vsocial), B(sin grupo),
  C(sec=infra,prim=vsocial), D(sec=backend,prim=vsocial)]
- WHEN se agrupa
- THEN el orden es A, D (bloque backend), C (bloque infra), B; B al final
  dentro de `(ungrouped)`

#### S19.2: primario en posición del primer miembro

- GIVEN filas [X(prim=otros), Y(prim=vsocial), Z(prim=otros)]
- WHEN se agrupa
- THEN X y Z quedan contiguos desde la posición de X, Y forma su bloque y
  los bloques conservan el orden de primera aparición (X antes de Y)

#### S19.3: mezcla con y sin secundario

- GIVEN un primario con repos backend/frontend y un repo sin secundario
- WHEN se agrupa
- THEN el repo sin secundario aparece bajo el primario en su posición de
  sort, sin header secundario que lo envuelva

#### S19.4: filtro vacía grupos enteros

- GIVEN agrupación activa y filtro `/x` sin matches en el secundario infra
- WHEN se renderiza
- THEN no existe header `infra` ni su primario si quedó sin miembros

### R20: Headers plegables de dos niveles

Los headers primario y secundario SHALL ser filas navegables: primario
dibujado `▾ <primary> (n)` en columna 0 y secundario `▾ <secondary> (n)`
indentado (prefijo fijo, p. ej. 2 espacios extra). El conteo `n` SHALL ser
el de repos visibles del bloque (tras filtros, antes de plegar); el conteo
del primario SHALL incluir a todos sus secundarios. El plegado SHALL
alternarse con `tab` (header o miembro) y `enter` (header):

- plegar un secundario oculta solo sus repos;
- plegar un primario oculta sus repos, sus headers secundarios y sus
  secciones internas;
- `tab` sobre un repo SHALL plegar el contenedor más interno al que
  pertenece (su secundario si tiene, si no su primario).

El estado plegado SHALL usar claves únicas: primario = su nombre,
secundario = `<primary>/<secondary>` (evita colisión de nombres entre
primarios distintos). Persistencia solo en sesión (map en memoria).

#### S20.1: plegado de secundario

- GIVEN el header secundario `▾ vsocial › backend (6)`
- WHEN se pulsa `tab` sobre él
- THEN sus 6 repos se ocultan, el header pasa a `▸` y el primario sigue abierto

#### S20.2: plegado de primario oculta secundarios

- GIVEN el primario vsocial abierto con backend/frontend/infra visibles
- WHEN se pliega el primario
- THEN desaparecen sus repos Y sus 3 headers secundarios; solo queda
  `▸ vsocial (14)`

#### S20.3: tab sobre repo pliega el contenedor interno

- GIVEN el cursor sobre un repo con secundario backend
- WHEN se pulsa `tab`
- THEN se pliega `vsocial/backend`, no `vsocial`

#### S20.4: conteo del primario suma secundarios

- GIVEN vsocial con backend (6), frontend (4), infra (4)
- WHEN se renderiza el header primario
- THEN muestra `▾ vsocial (14)`

#### S20.5: claves de plegado sin colisión

- GIVEN dos primarios `a` y `b`, ambos con secundario `backend`
- WHEN se pliega `a/backend`
- THEN `b/backend` permanece desplegado

### R21: Propagación a print y detalle

El modo `--print` SHALL mostrar en la columna GROUP el compuesto
`<primary>/<secondary>` cuando exista secundario, o `<primary>` si solo hay
primario (`-` si ninguno). El detalle (enter sobre repo) SHALL mostrar el
mismo compuesto en el título.

#### S21.1: print con compuesto

- GIVEN un repo con primary=vsocial y secondary=backend
- WHEN se ejecuta `gitdash --print`
- THEN su columna GROUP muestra `vsocial/backend`

#### S21.2: detalle con compuesto

- GIVEN el cursor sobre un repo con ambos grupos
- WHEN se abre el detalle
- THEN el título muestra `<name> · vsocial/backend`

### R22: Cache v3

El cache `repos.json` SHALL versionarse a 3 con claves `primary_group` y
`secondary_group` (eliminando `group`). Un cache v2 SHALL ignorarse
silenciosamente (comportamiento ya cubierto por el check de versión) y
repoblarse en el primer rescan.

#### S22.1: cache viejo ignorado

- GIVEN un repos.json con `"version": 2`
- WHEN arranca gitdash
- THEN la tabla arranca vacía (sin snapshot stale) y el rescan la repuebla

#### S22.2: roundtrip de los campos nuevos

- GIVEN proyectos con primary/secondary
- WHEN se guarda y carga el cache
- THEN ambos campos se restauran idénticos

## Revisiones

- **R18**: reemplazo duro de `group` elegido por el usuario (sin alias de
  compatibilidad); implica migrar marcadores del usuario, fixtures de
  `testdata/playground` vía `gen-fixtures.sh` y tests existentes.
- **R18**: `secondary_group` sin `primary` se ignora (decisión del usuario):
  el secundario solo tiene sentido dentro de un primario.
- **R19**: repos con primario y sin secundario conservan posición de sort
  dentro del bloque (sin pseudo-header `(general)`): menos ruido visual.
- **R20**: `tab` sobre repo pliega el contenedor más interno (secundario si
  existe, si no primario): acceso directo al nivel relevante sin navegar
  hasta el header.
- **R20**: clave de plegado `primary/secondary` para evitar colisión de
  nombres de secundarios entre primarios distintos.
- **R21**: en la TUI la columna GROUP muestra el mismo compuesto
  `<primary>/<secondary>` (truncada al ancho de columna).
