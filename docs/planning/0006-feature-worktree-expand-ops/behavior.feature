# behavior.feature — 0006-feature-worktree-expand-ops
#
# Fuente ÚNICA del comportamiento esperado, para VERIFICACIÓN DEL USUARIO.
# NO es Cucumber: no hay step definitions ni runner Gherkin. El executor deriva
# tests reales (unit/integration) de cada escenario.
#
# Numeración: continúa el SDD del repo (0004 terminó en R29). 0005 está
# referenciado en código (lazygit / comando `!`) pero no tiene spec propia; este
# cambio usa R30+. Mapeo con el SDD recortado del repo:
#   behavior.feature ≈ spec.md (R#n SHALL + S#n.# GIVEN/WHEN/THEN)
#   issue.md         ≈ proposal.md (por qué + alcance)

Feature: Worktrees expandibles y operables desde el menú principal

  Los repos con worktrees son toggleables con `space`. Al expandir, cada worktree
  de `git worktree list --porcelain` aparece como sub-fila navegable y operable,
  con la misma superficie de acciones que una fila de repo. El estado
  expandido/plegado persiste entre sesiones.

  # R30: El toggle de expansión SHALL alternar las sub-filas de worktree del repo
  # bajo el cursor. SHALL ser no-op (sin cambio de estado) sobre un header de grupo,
  # sobre una sub-fila de worktree, y sobre un repo sin worktrees. La tecla SHALL
  # ser configurable (acción `expand`), con default `space`.
  @R30
  Scenario: S30.1 — repo con worktrees, plegado por defecto
    Given un repo descubierto con 2 worktrees y sin estado de expansión persistido
    When se renderiza la tabla
    Then su celda NAME muestra el glyph de plegado `▸` y el contador `(2 wt)`
    And no se muestran sub-filas de worktree

  @R30
  Scenario: S30.2 — space expande
    Given el repo del cursor tiene worktrees y está plegado
    When se pulsa `space`
    Then su celda NAME muestra el glyph `▾`
    And bajo su fila aparecen las sub-filas de sus worktrees

  @R30
  Scenario: S30.3 — space vuelve a plegar
    Given el repo del cursor está expandido
    When se pulsa `space`
    Then sus sub-filas desaparecen y el glyph vuelve a `▸`

  @R30
  Scenario: S30.4 — space sobre un repo sin worktrees
    Given el repo del cursor no tiene worktrees
    When se pulsa `space`
    Then no se muestran sub-filas y no cambia ningún estado de expansión

  @R30
  Scenario: S30.5 — space sobre un header de grupo
    Given el cursor está sobre un header primario o secundario
    When se pulsa `space`
    Then el plegado de grupos no cambia (solo `tab`/`enter` pliegan grupos)

  @R30
  Scenario: S30.6 — space sobre una sub-fila de worktree
    Given el cursor está sobre una sub-fila de worktree
    When se pulsa `space`
    Then no cambia el estado de expansión del repo padre

  # R31: La expansión SHALL listar TODOS los worktrees que reporta
  # `git worktree list --porcelain` para el repo, incluidos los que NO tienen
  # marcador y los que están fuera de los roots de discovery. El repo principal
  # SHALL NO aparecer como sub-fila. Un worktree descubierto con marcador SHALL
  # aparecer una sola vez (como sub-fila), nunca además como fila de repo
  # top-level. El fallback de huérfanos (0002 S15.3) SHALL preservarse.
  @R31
  Scenario: S31.1 — cobertura total
    Given un repo con 3 worktrees: uno con marcador, uno sin marcador dentro de
      un root, y uno sin marcador fuera de todos los roots
    When se expande el repo
    Then aparecen 3 sub-filas de worktree

  @R31
  Scenario: S31.2 — el principal no es sub-fila
    Given un repo con worktrees
    When se expande el repo
    Then el propio repo principal no aparece entre las sub-filas

  @R31
  Scenario: S31.3 — dedupe de worktree descubierto
    Given un worktree que además fue descubierto con su propio marcador
    When se expande su repo principal
    Then ese worktree aparece exactamente una vez, como sub-fila bajo el principal
    And no aparece como fila de repo top-level

  @R31
  Scenario: S31.4 — huérfano sigue visible y no es expandible
    Given un worktree descubierto cuyo repo principal NO está descubierto
    When se renderiza la tabla
    Then el worktree sigue visible como fila de repo top-level con el tag `[wt]`
    And `space` sobre él no expande nada (no tiene padre en la tabla)

  @R31
  Scenario: S31.5 — repo sin worktrees
    Given un repo descubierto sin worktrees
    When se renderiza la tabla
    Then no muestra glyph de expansión ni contador `(N wt)`

  @R31
  Scenario: S31.6 — snapshot aún sin recolectar
    Given un repo cuyo snapshot todavía no tiene worktrees recolectados
    When se renderiza la tabla
    Then no muestra glyph de expansión y `space` no expande nada

  # R32: Las sub-filas de worktree SHALL ser navegables con el cursor como
  # cualquier otra fila (j/k, home/end) y SHALL participar del scroll. Al plegar
  # con el cursor sobre una sub-fila, el cursor SHALL reubicarse dentro de rango.
  @R32
  Scenario: S32.1 — el cursor alcanza la sub-fila
    Given un repo expandido con 2 worktrees
    When el cursor baja hasta la primera sub-fila
    Then la sub-fila queda seleccionada y el resto de acciones operan sobre ella

  @R32
  Scenario: S32.2 — scroll sigue a la sub-fila
    Given una tabla con más filas que la ventana visible y un repo expandido
    When el cursor se mueve a una sub-fila fuera de la ventana
    Then la tabla hace scroll para mantener la sub-fila visible

  @R32
  Scenario: S32.3 — plegar con el cursor en la sub-fila
    Given el cursor está sobre una sub-fila del repo expandido
    When se pliega el repo con `space`
    Then el cursor queda dentro del rango de filas visibles (sin quedar colgado)

  # R33: Sobre una sub-fila de worktree, TODAS las operaciones a nivel de repo
  # SHALL ejecutarse contra el path del worktree concreto, con los mismos guards
  # y notificaciones que una fila de repo. El detalle (enter) SHALL mostrar los
  # datos disponibles del worktree (path, rama, head) sin inventar estado git.
  @R33
  Scenario: S33.1 — fetch sobre el worktree
    Given el cursor sobre la sub-fila de un worktree con upstream
    When se pulsa la tecla de fetch
    Then `git fetch` corre en el path de ese worktree

  @R33
  Scenario: S33.2 — pull, sync y push sobre el worktree
    Given el cursor sobre la sub-fila de un worktree
    When se pulsa pull, sync o push
    Then el comando correspondiente corre en el path de ese worktree

  @R33
  Scenario: S33.3 — lazygit, update y editor sobre el worktree
    Given el cursor sobre la sub-fila de un worktree
    When se pulsa lazygit, update o editor
    Then el proceso hace handoff de terminal con el directorio de trabajo
      puesto en el path de ese worktree

  @R33
  Scenario: S33.4 — recollect sobre el worktree
    Given el cursor sobre la sub-fila de un worktree
    When se pulsa recollect
    Then se re-colecta el estado de ese path

  @R33
  Scenario: S33.5 — comando `!` y shell sobre el worktree
    Given el cursor sobre la sub-fila de un worktree y el detalle abierto
    When se ejecuta un comando `!` (o shell interactiva con input vacío)
    Then el comando corre con el directorio de trabajo puesto en el path del worktree

  @R33
  Scenario: S33.6 — detalle de un worktree
    Given el cursor sobre la sub-fila de un worktree
    When se pulsa enter
    Then el detalle muestra el path, la rama (o `(detached)`) y el head del worktree
    And no muestra estado git derivado (dirty/ahead/behind/sync) para ese worktree

  @R33
  Scenario: S33.7 — notificaciones con nombre legible
    Given una acción ejecutada sobre la sub-fila de un worktree
    When termina la acción
    Then la notificación identifica el worktree por su nombre de directorio (basename),
      no por su ruta absoluta completa

  # R34: La sub-fila SHALL distinguirse visualmente de una fila de repo y de un
  # header de grupo (indentación + glyph propio) y SHALL mostrar la rama del
  # worktree (o `(detached)`). Las celdas que dependen de estado git por-worktree
  # SHALL quedar vacías/dim: la UI NO SHALL prometer dirty/ahead/behind/sync/actividad
  # por worktree. El NAME del repo principal SHALL mostrar `▾`/`▸` junto a `(N wt)`.
  @R34
  Scenario: S34.1 — sub-fila distinguible
    Given un repo expandido con un header de grupo secundario y sub-filas
    When se renderiza la tabla
    Then la sub-fila de worktree es visualmente distinta de la fila de repo y del
      header de grupo

  @R34
  Scenario: S34.2 — rama del worktree visible
    Given un worktree en la rama `feat/x`
    When se expande su repo
    Then la sub-fila muestra `feat/x` en la columna BRANCH

  @R34
  Scenario: S34.3 — worktree detached
    Given un worktree con HEAD detached (sin rama)
    When se expande su repo
    Then la sub-fila muestra `(detached)` en BRANCH y el head corto como dato disponible

  @R34
  Scenario: S34.4 — sin estado por-worktree
    Given un repo expandido
    When se renderiza la tabla
    Then las celdas de estado por-worktree (Work Tree, ↑↓up, SYNC, ACTIVITY) de la
      sub-fila están vacías o dim (no se inventa estado)

  @R34
  Scenario: S34.5 — glyph y contador en el principal
    Given un repo con worktrees, plegado y luego expandido
    When se renderiza la tabla
    Then su NAME muestra `▸ (N wt)` plegado y `▾ (N wt)` expandido

  # R35: El estado expandido/plegado de worktrees SHALL persistir entre sesiones
  # reutilizando el mismo store que el plegado de grupos (`state.Store` /
  # `collapsed.json`), sin fichero nuevo y sin colisionar con las claves de grupo.
  # Ausencia de estado o fichero corrupto SHALL degradar a plegado por defecto.
  @R35
  Scenario: S35.1 — la expansión sobrevive al reinicio
    Given un repo con worktrees expandido
    When la app se cierra y se vuelve a abrir
    Then el repo aparece expandido con sus sub-filas

  @R35
  Scenario: S35.2 — default plegado
    Given un repo con worktrees y sin estado persistido para él
    When arranca la app
    Then el repo aparece plegado

  @R35
  Scenario: S35.3 — sin colisión con claves de grupo
    Given un `collapsed.json` que ya contiene claves de grupos plegados
    When se expande un repo con worktrees
    Then las claves de grupo existentes se conservan y la expansión se guarda
      en una clave de namespace propio

  @R35
  Scenario: S35.4 — estado corrupto o ausente
    Given un `collapsed.json` corrupto o inexistente
    When arranca la app
    Then todos los repos aparecen plegados y la app no falla

  # R36: Las sub-filas SHALL seguir la visibilidad de su repo padre bajo el
  # filtro `n` (dirty) y el plegado de grupos: una sub-fila solo es visible si su
  # padre lo es. El filtro `/` de búsqueda se rige por R40. El estado de
  # expansión de worktrees SHALL ser independiente del plegado de grupos.
  @R36
  Scenario: S36.1 — filtro dirty oculta al padre y a sus sub-filas
    Given un repo expandido con worktrees que no pasa el filtro `n` activo
    When se renderiza la tabla
    Then ni el repo ni sus sub-filas de worktree son visibles

  @R36
  Scenario: S36.2 — grupo plegado oculta las sub-filas
    Given un repo expandido dentro de un grupo plegado
    When se renderiza la tabla
    Then las sub-filas de sus worktrees no son visibles

  @R36
  Scenario: S36.3 — expansión y plegado de grupos independientes
    Given un repo expandido con worktrees
    When se pliega y se vuelve a desplegar su grupo
    Then el repo sigue expandido con sus sub-filas

  # R37: La acción de expansión SHALL ser configurable vía `keybindings` en
  # config.toml con default `space`, y SHALL aparecer en la barra de hints.
  @R37
  Scenario: S37.1 — default space
    Given una config sin override de keybindings
    When se pulsa la barra espaciadora sobre un repo con worktrees
    Then el repo se expande

  @R37
  Scenario: S37.2 — rebind
    Given un config.toml con `expand = "w"` (o similar)
    When se pulsa `w` sobre un repo con worktrees
    Then el repo se expande
    And `space` ya no expande

  @R37
  Scenario: S37.3 — hint visible
    When se renderiza la barra de hints
    Then aparece el hint de la acción de expansión con su tecla configurada

  # R38: El modo `--print` SHALL permanecer inalterado: no incorpora sub-filas ni
  # glyph de expansión (es no interactivo).
  @R38
  Scenario: S38.1 — print sin sub-filas
    Given un repo con worktrees
    When se ejecuta `gitdash --print`
    Then la salida no incluye sub-filas de worktree ni glyphs de expansión

  # R39: `tab` (fold) sobre una sub-fila de worktree SHALL ser no-op: no SHALL
  # plegar la sección `(ungrouped)` ni ningún contenedor de grupo por accidente.
  @R39
  Scenario: S39.1 — tab sobre sub-fila no pliega grupos
    Given el cursor sobre una sub-fila de worktree
    When se pulsa `tab`
    Then el plegado de grupos no cambia

  # R40: La búsqueda `/` SHALL matchear, además del nombre y grupos de un repo,
  # el basename/nombre y la rama de sus worktrees. Cuando la búsqueda matchea un
  # worktree, su repo padre SHALL quedar visible y expandido mostrando la sub-fila
  # que matchea, aunque el padre no matchee por sí mismo. Esa expansión inducida
  # por la búsqueda SHALL ser transitoria (de vista): SHALL NO modificar el estado
  # de expansión persistido del repo.
  @R40
  Scenario: S40.1 — match por rama de worktree
    Given una búsqueda activa que coincide con la rama `feat/x` de un worktree de
      un repo que no matchea por sí mismo
    When se renderiza la tabla
    Then el repo padre es visible y expandido
    And se ve la sub-fila del worktree en `feat/x`

  @R40
  Scenario: S40.2 — match por basename de worktree
    Given una búsqueda activa que coincide con el basename del directorio de un worktree
    When se renderiza la tabla
    Then el repo padre es visible y expandido con la sub-fila de ese worktree

  @R40
  Scenario: S40.3 — solo la sub-fila que matchea
    Given un repo con 3 worktrees donde solo uno matchea la búsqueda
    When se renderiza la tabla
    Then el padre es visible y expandido mostrando la sub-fila que matchea
    And no se muestran las sub-filas que no matchean

  @R40
  Scenario: S40.4 — sin match
    Given una búsqueda que no coincide con ningún repo ni worktree
    When se renderiza la tabla
    Then no se muestra ninguna fila

  @R40
  Scenario: S40.5 — expansión inducida por búsqueda es transitoria
    Given un repo plegado cuya búsqueda activa matchea uno de sus worktrees
    When se limpia la búsqueda
    Then el repo vuelve a su estado de expansión persistido (plegado)
