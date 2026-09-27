# Issue — Preview visual de pull con `git-sim`

## Problema

gitdash decide acciones de reconciliación (pull/merge/rebase) sin que el usuario
pueda **ver antes** qué pasaría. Un `git pull` sobre una rama divergida puede
integrar con rebase o merge según el gitconfig, y un rebase a medias deja el repo
en un estado que invita a reintentar sobre algo sin resolver. No hay forma de
previsualizar la operación sin ejecutarla de verdad.

`git-sim` (initialcommit-com/git-sim, PyPI 0.3.5) dibuja un diagrama de lo que
haría un comando git —`pull`, `merge`, `rebase`— **sin mutar el repo real** (las
operaciones de red corren en un clon temporal bajo `/tmp/git_sim/<repo>`). Es
puramente visual y efímero: genera una imagen y la abre con `xdg-open`.

## Propuesta

Añadir a gitdash un **preview visual** invocado como *handoff* de terminal:

- Tecla nueva `v` → acción configurable `visual`.
- `v` **no ejecuta**: arma un selector prefix-key (hermano de `pullArmed`) con el
  path del repo bajo el cursor y su upstream capturados.
- La siguiente tecla elige variante:
  - `p` → `git-sim pull` (sin argumentos; espejo del `p` pelado de gitdash).
  - `m` → `git-sim merge <upstream-ref>`
  - `r` → `git-sim rebase <upstream-ref>`
  - Cualquier otra tecla cancela y sigue su curso normal.
- Siempre se pasa `--media-dir <caché XDG de gitdash>/git-sim` para que git-sim
  NO escriba `git-sim_media/` dentro del repo (gitdash usa `git status` real: lo
  marcaría dirty tras cada preview).
- `git-sim` se resuelve por PATH (`exec.LookPath`); si no está, solo toast. El
  handoff cede la terminal al hijo, como lazygit/editor/`p a`, y al volver se
  registra el exec en el command log (`Dur=0`) y se re-colecta el estado.

## Fuera de alcance

- La variante `s`/squash de git-sim: DESCARTADA.
- No se toca la máquina de estados de `p`/`pullArmed`, ni `PullKinds`, ni
  `commands.pull*`, ni `[ai.pull]`.
- No se parsea ni se persigue la ruta de la imagen generada (timestamp no
  determinista) ni el clon `/tmp/git_sim/<repo>`.
- `git-sim` NO es git: no pasa por `runGit`/`runGitCombined`.

## Por qué ahora

El repo ya tiene el patrón exacto que hace falta (selector prefix-key de `p`,
handoff de terminal de lazygit/`p a`, command log con intención + argv) y
`promptLine()` como punto único de avisos en keybinds. La feature es aditiva: un
camino nuevo que reutiliza esos raíles sin modificar los existentes.
