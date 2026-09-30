# TODO — cerrar los mutantes que sobreviven

Worktree `wt/mutants`. Rama `wt/mutants`. Objetivo: bajar el NOT COVERED de
gremlins hasta que solo queden identidades demostradas.

## LEE ESTO ANTES DE PICAR UNA LÍNEA (medición defectuosa)

`NOT COVERED` de gremlins **miente**, y miente en la dirección que hace parecer
que hay más trabajo del que hay. Comprobado en esta tanda:

- El perfil real (`go test -coverprofile`) da `count 1` en `internal/tui/table.go`
  líneas 59, 61, 604, 740, 758, 760, 762, 764 y 766. Gremlins las reporta
  NOT COVERED. **Es un fallo de mapeo**: la cobertura de Go se emite por
  BLOQUES, y un bloque que empieza en la columna 54 no cubre (a los ojos de
  gremlins) un mutante de la columna 22 de la misma línea. Todo mutante sentado
  en la parte de una línea que precede al `return`/cuerpo de su bloque paga
  este peaje.
- Por eso el `NOT COVERED` de un fichero NO es una cola de trabajo: es una
  lista de *sospechosos*. Cada línea se verifica **a mano** (mutando el fichero
  y corriendo la suite) antes de escribir un test. Si el mutante ya muere,
  no hay nada que hacer y se marca `KILLED` aquí.
- En sentido inverso, gremlins también marca `LIVED` mutaciones **idénticas**
  (mismo tipo, línea, columna y código resultante) como `KILLED` en el mismo
  run: son las que hacen que un bucle recorra ~1e9 iteraciones y dependen de si
  el proceso mutantado le gana al timeout. Ya allowlistadas como ruido.
- `cmd/gitdash/print.go` sale entero NOT COVERED porque **gremlins no ejecuta el
  paquete `main` de `cmd/`**. Sus 52 no son trabajo: es artefacto.

Conclusión: las únicas dos señales que valen son **LIVED verificado a mano** y
**NOT COVERED verificado a mano**. El resto es ruido que se limpia con un test
mejor escrito, no con más tests.

## Reglas de esta tanda (aprendidas a base de vacíos)

1. **Nada de tests vacíos.** Un test cuya rama nunca se ejecuta da confianza
   falsa: ya me hizo "verificar" una muerte que no ocurría. Si una rama del test
   depende de una fixture, la fixture tiene que traer lo que la rama necesita.
2. **Los bordes se prueban a ambos lados.** Un caso a 3 h no distingue un umbral
   de 24 h de uno de 23 h. Cada umbral lleva un caso por debajo y otro por
   encima.
3. **Un contrato que el código no tiene no se afirma.** `sort.Slice` no es
   estable: afirmar el orden de dos filas empatadas ata el test a una propiedad
   inexistente.
4. **Identidad ≠ hueco.** Si la mutación no cambia nada por construcción (un
   `+= 0`, un `>` dentro de un `!=` que lo implica), ningún test puede matarla:
   se allowlista con la demostración escrita, o se quita el sitio mutado.
5. **Nunca editar ficheros mientras corre gremlins**: muta los ficheros reales
   in-place.

## Cola

Estado por fichero. `NOT COVERED` es el número que reporta gremlins (con el
caveat de arriba); `líneas` son las sospechosas concretas.

| # | fichero | NC | líneas | estado |
|---|---------|----|--------|--------|
| 1 | `internal/tui/bordered/bordered.go` | 3 | 74, 75 | ✅ hecho: las 3 mutaciones de la alineación del título, con el hueco impar (3 de sobrante → 1 izquierda / 2 derecha) y las dos líneas de borde alineadas por separado |
| 2 | `internal/state/state.go` | 3 | 33, 37, 46 | ✅ hecho: `DefaultBaseDir` y `NewStore` **no tenían ni un test** (los 7 tests usaban `NewStoreAt`). Ahora: XDG manda sobre HOME, XDG vacío cuenta como no puesto, sin HOME error, y `NewStore` propaga el error en vez de devolver un store inservible |
| 3 | `internal/config/config.go` | 3 | 112, 188, 221 | ✅ hecho: `Path()` y `Load()` —la puerta de entrada real— no tenían test (todos pasaban un path a mano). Y el guard de `[commands]`: un valor vacío es una casilla sin rellenar, no una orden; y no se registra una acción inexistente con argv vacío |
| 4 | `internal/tui/app.go` | 4 | 454, 458, 464, 853 | ⬜ media |
| 5 | `internal/tui/toast.go` | 6 | 25, 188, 190 | ⬜ media |
| 6 | `internal/tui/proverlay.go` | 6 | 46, 66, 376, 379 | ⬜ media (376/379 ya los mata un test → verificar) |
| 7 | `internal/tui/sections.go` | 11 | — | ⬜ |
| 8 | `internal/tui/update.go` | 11 | — | ⬜ |
| 9 | `internal/gitstatus/parse.go` | 17 | — | ⬜ parser puro, buen blanco |
| 10 | `internal/tui/detail.go` | 24 | — | ⬜ grande |
| 11 | `internal/tui/cmdlogpanel.go` | 24 | — | ⬜ grande |
| 12 | `internal/tui/table.go` | 31 | 59, 61, 604, 652-656, 740-766 | ✅ ya cubiertos (verificado a mano), falta re-verificar |
| 13 | `internal/testutil/testutil.go` | 24 | — | ⬜ infrastructure de tests |
| 14 | `cmd/gitdash/print.go` | 52 | — | ❌ artefacto: gremlins no corre el paquete `main` |

## Allowlist que hay que sincronizar

`CONDITIONALS_BOUNDARY internal/tui/prcreate.go:156` sobrevive y es identidad:
está dentro de `if strings.HasPrefix(reason, "git ")`, así que la llave `]: `
**no puede** estar en el índice 0 — el motivo empieza por `git `. Con `i > 0`
el resultado es el mismo. Allowlistar con esa demostración.

`ARITHMETIC_BASE cmd/gitdash/print.go:111` está en el allowlist y el informe ya
no lo reporta (porque el paquete no se mide). Se queda, pero la entrada se
sostiene con verificación a mano.
