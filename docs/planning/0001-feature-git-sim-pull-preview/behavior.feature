# Comportamiento esperado de "git-sim pull preview".
# Fuente única de comportamiento, para verificación del usuario.
# NO es Cucumber: sin step definitions, sin runner. Keywords en inglés.
# Descripciones en el idioma del repo (español).
#
# Convención de teclas: las teclas son las de la config por defecto.
#   v = visual (armado), p/m/r = variantes del selector visual.
#   p sigue siendo pull, r sigue siendo rescan: las variantes visuales
#   se consumen ANTES del enrutado normal mientras el selector está armado.

Feature: Preview visual de pull/merge/rebase con git-sim

  # --- Disparo y armado ---------------------------------------------------

  Scenario: La tecla visual arma el selector sin ejecutar nada
    Given la fila del cursor es un repo git con upstream
    When el usuario pulsa `v`
    Then se arma el selector visual con el path y el upstream de esa fila
    And no se lanza ningún proceso
    And la sección de keybinds muestra el aviso del selector visual
    And las hints se sustituyen por ese aviso

  Scenario: Sin fila bajo el cursor no se arma
    Given el cursor está sobre un header de grupo (o la tabla vacía)
    When el usuario pulsa `v`
    Then aparece un toast de "no git repo — nothing to do"
    And no se arma el selector visual

  Scenario: Sobre una fila sin repo no se arma
    Given la fila del cursor es un proyecto sin repo git
    When el usuario pulsa `v`
    Then aparece un toast de "no git repo — nothing to do"
    And no se arma el selector visual

  Scenario: Una tecla que no es variante cancela el armado y sigue su curso
    Given el selector visual está armado
    When el usuario pulsa una tecla que no es `p`, `m`, `r` ni `esc`
    Then el selector se desarma
    And esa tecla se procesa como cualquier otra tecla del dashboard

  Scenario: El armado sobrevive a la fila correcta aunque el cursor se mueva
    Given el selector visual está armado sobre el repo A
    When llegara a resolverse la variante
    Then el gateway apunta al path y upstream capturados al armar (repo A),
      no a la fila que estuviera bajo el cursor después

  # --- Variantes ----------------------------------------------------------

  Scenario: La variante p lanza git-sim pull sin argumentos posicionales
    Given el selector visual está armado sobre un repo
    When el usuario pulsa `p`
    Then se lanza el handoff de `git-sim` con argv
      ["git-sim", "--media-dir", <caché>/gitdash/git-sim, "pull"]
    And el selector se desarma

  Scenario: La variante m lanza git-sim merge con el upstream de la rama
    Given el selector visual está armado sobre un repo con upstream `origin/main`
    When el usuario pulsa `m`
    Then se lanza el handoff de `git-sim` con argv
      ["git-sim", "--media-dir", <caché>/gitdash/git-sim, "merge", "origin/main"]
    And el selector se desarma

  Scenario: La variante r lanza git-sim rebase con el upstream de la rama
    Given el selector visual está armado sobre un repo con upstream `origin/main`
    When el usuario pulsa `r`
    Then se lanza el handoff de `git-sim` con argv
      ["git-sim", "--media-dir", <caché>/gitdash/git-sim, "rebase", "origin/main"]
    And el selector se desarma

  Scenario: Sin upstream, merge y rebase no lanzan nada
    Given el selector visual está armado sobre un repo SIN upstream de su rama
    When el usuario pulsa `m` o `r`
    Then aparece un toast de "no upstream"
    And no se lanza ningún handoff

  Scenario: Sin upstream, la variante p sí se permite
    Given el selector visual está armado sobre un repo SIN upstream de su rama
    When el usuario pulsa `p`
    Then se lanza el handoff de `git-sim pull`
    # git-sim pull simula la operación; no exige un ref explícito como merge/rebase.

  # --- Salida y flags -----------------------------------------------------

  Scenario: El media-dir siempre apunta a la caché de gitdash, nunca al repo
    Given cualquier variante visual
    Then el argv incluye `--media-dir <caché XDG de gitdash>/git-sim`
    And el directorio se crea si no existe
    And el repo NO queda con `git-sim_media/` dentro (no se marca dirty)

  Scenario: No se desactiva el auto-open ni se anima
    Given cualquier variante visual
    Then el argv NO incluye `-d` (el auto-open de git-sim queda activo)
    And el argv NO incluye `--animate`

  # --- Handoff y command log ---------------------------------------------

  Scenario: Sin el binario en PATH solo hay toast
    Given el selector visual está armado y `git-sim` no está en PATH
    When el usuario elige una variante
    Then aparece un toast de "git-sim not installed"
    And no hay handoff de terminal

  Scenario: Si el media-dir no se puede crear, no se lanza nada
    Given el directorio de caché de git-sim no se puede crear
    When el usuario elige una variante
    Then aparece un toast de error
    And no se lanza el handoff (lanzarlo ensuciaría el repo)

  Scenario: Al volver del handoff se registra el exec y se re-colecta
    Given se lanzó un handoff de `git-sim`
    When el hijo termina y la terminal vuelve a gitdash
    Then el command log registra un exec con el argv REAL resuelto
      (incluye `--media-dir` y el ref) y Dur=0
    And la intención (tecla + variante + repo) quedó registrada al elegir
    And se re-colecta el estado del repo

  Scenario: El selector visual no consulta ni bloquea por rebase en curso
    Given el repo tiene un rebase a medias
    When el usuario elige una variante visual
    Then el handoff se lanza igualmente
    # git-sim no muta el repo real: previsualizar un rebase es justo lo útil ahí.

  # --- Config y hints -----------------------------------------------------

  Scenario: La acción visual es configurable y aparece en las hints
    Given la config por defecto
    Then `visual` existe en los keybindings por defecto con tecla `v`
    And las hints incluyen "v visual" (etiqueta sin la tecla dentro)
    And una config vieja que use una acción inexistente sigue avisando

  Scenario: El aviso del selector se pinta por el punto único de keybinds
    Given el selector visual está armado
    Then promptLine() devuelve el aviso del selector visual
    And keybindsLines() es 1 (el aviso sustituye a las hints)
