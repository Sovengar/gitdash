#!/usr/bin/env bash
# mutate-local.sh — la corrida de mutation testing como se debe hacer en local.
#
# Por que existe y por que no es solo un `make mutate` con otros numeros:
#
# Los DOS knobs de esta corrida estan acoplados, y medirlos por separado da
# un numero que miente. `testExecutionTime = (duracion de la cobertura) x
# --timeout-coefficient`, y ese valor se pasa a `go test -timeout` de CADA
# mutante (executor.go:101 y 227). Con la cache de `go test` caliente la
# cobertura mide ~2s, asi que coef 2 da un techo de 6s por mutante. La suite de
# internal/tui tarda 3s sola. En cuanto hay contencion, los mutantes expiran:
#
#   workers  coef   wall  killed  TIMED OUT  killed/s
#        4     2     30s      46           0      1.53
#       10     2     20s      29          17      1.45   <- 26 mutantes
#       16     2     20s      20          26      1.00      desperdiciados
#       10    30     25s      46           0      1.84
#       16    30     20s      46           0      2.30   <- el que hay que usar
#
# El coeficiente no es un dial: es un interruptor. Y `TIMED OUT` es lo peor que
# puede pasar, porque NO va a `mutants_total` (report.go:178 lo excluye) y el gate
# solo mira `LIVED`: el run reporta 100% de eficacia sin haber testeado 15
# mutantes.
#
# Que el coeficiente tenga que seguir a la temperatura de la cache es lo unico
# que este script NO puede hacer solo: se mide la cobertura y se elige. Con
# cache fria (~23s de cobertura) coef 30 daria un techo de 690s por mutante y un
# mutante realmente colgado tardaria 11 minutos en caer en vez de 46s.
#
# Uso:
#   scripts/mutate-local.sh             # modulo completo
#   scripts/mutate-local.sh --diff      # solo el diff vs main (bucle diario)
#   scripts/mutate-local.sh --diff-dry  # el denominador del diff, sin mutar
#   scripts/mutate-local.sh --dry       # el denominador del modulo, sin mutar
#
# Override: WORKERS=8 COEF=10 scripts/mutate-local.sh

set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 2

WD=${WATCHDOG:-$HOME/.local/lib/swe/lib/watchdog.sh}
MUTATE_EXCLUDE='(\.worktrees/|internal/testutil/)'
MUTATE_COVERPKG=./...
STALL=${STALL:-20m}
MAX=${MAX:-60m}

# Una linea de mutante de gremlins. `Mutant()` imprime "%s%s %s at %s\n"
# (report.go:272-281), asi que la cola es `at <file>:<linea>:<col>` y los colores
# van en el status, no al final: el patron aguanta aunque el destino sea un TTY.
# El `[^[:space:]]` final tolera el CR que mete la disciplina de linea de un
# terminal, para no depender de que la salida este redirigida.
PROGRESS_RE='at [^[:space:]]+:[[:digit:]]+:[[:digit:]]+[^[:space:]]*$'

[[ -x $WD ]] || {
	echo "mutate-local: falta el supervisor en $WD" >&2
	echo "  chezmoi source: ~/.local/share/chezmoi/home/dot_local/lib/swe/lib/executable_watchdog.sh" >&2
	echo "  aplica con: chezmoi apply ~/.local/lib/swe/lib/watchdog.sh" >&2
	exit 2
}

SCOPE=()
SOLO_DRY=0
case ${1:-} in
--diff)
	SCOPE=(--diff "${MUTATE_BASE:-main}")
	;;
--dry)
	SCOPE=(--dry-run)
	;;
--diff-dry)
	SCOPE=(--diff "${MUTATE_BASE:-main}" --dry-run)
	SOLO_DRY=1
	;;
'') ;;
*)
	echo "uso: ${0##*/} [--diff|--diff-dry|--dry]" >&2
	exit 2
	;;
esac

# La pre-pasada de dry-run es la que mide la cobertura, y su DURACION es lo que
# decide el coeficiente. Sale de aqui el número, no de una constante inventada.
echo "mutate-local: midiendo la cobertura para elegir el timeout por mutante…" >&2
# El formato real es "done in 8.858216525s" (log.Infof("done in %s\n"), con
# %s sobre un time.Duration). El `sed` saca los NUMEROS y descarta la 's' que
# dura el valor; un `s|.*done in (.*)|\1|` se comia el resto de la linea, que en
# espanol con coma decimal aparece como "8,858216525s".
cov_secs=$(go tool gremlins unleash --dry-run "${SCOPE[@]}" \
	--exclude-files "$MUTATE_EXCLUDE" --coverpkg "$MUTATE_COVERPKG" 2>&1 |
	sed -n 's/.*done in \([0-9,.]*\)s.*/\1/p' | tail -1 |
	tr ',' '.')
[[ -n $cov_secs ]] || cov_secs=0

# El techo por mutante es `cov_secs * COEF` + 2s (el +2 es lo que gremlins pasa
# a `go test -timeout` por encima del contexto, executor.go:227).
#
# El techo tiene que ser HOLGADO para que la contencion no se coma mutantes
# sanos, pero no tan holgado que un mutante realmente colgado tarde minutos en
# caer. Con la cache caliente la cobertura mide ~2s y hace falta coef 30 para que
# internal/tui (3s) quepa; con la cache FRIA mide ~9-25s y coef 30 daría un techo
# de 5-12 minutos por mutante, que es demasiado: un cuelgue real se quedaría
# colgado media hora.
#
# El corte esta en 12s de cobertura, medido: es el punto donde `cov * 2` deja de
# dar un techo comfortable (24s, holgado para una suite que en cache caliente
# tarda 3s) y empieza a dar uno que se come mutantes lentos de verdad. Con la
# cache caliente la cobertura mide 1-3s y cae en coef 30; con la cache fria mide
# 10-25s y cae en coef 2.
COEF=${COEF:-}
if [[ -z $COEF ]]; then
	COEF=30
	awk -v c="$cov_secs" 'BEGIN{exit !(c+0 > 12)}' && COEF=2
fi
WORKERS=${WORKERS:-8}
timeout_secs=$(awk -v c="$cov_secs" -v k="$COEF" 'BEGIN{printf "%d", c*k+2}')

# Aviso si el techo por mutante se va de las manos: un mutante realmente colgado
# pasaria minutos sin que nadie lo mate, y el run entero se iria al techo de 60m.
# Medido: con la cache a medio calentar la cobertura salia en 12s y coef 30 daba
# 363s por mutante.
if [[ $timeout_secs -gt 120 ]]; then
	echo "mutate-local: AVISO — el techo por mutante sale en ${timeout_secs}s." >&2
	echo "  Un mutante colgado pasaria ${timeout_secs}s sin que nadie lo mate." >&2
	echo "  Si la cache se esta calentando, espera a que lo este y repite; o forzalo:" >&2
	echo "    COEF=2 $0 ${1:+--diff}" >&2
	timeout_secs=120
	COEF=2
fi

# El techo tiene que dar ABRIGO para la contencion, y la contencion es
# brutal: medido sobre internal/gitstatus (la suite mas lenta, ~8s sola), el
# go test mas lento de una tanda de N en paralelo tarda:
#
#     N= 1   8.2s     (referencia)
#     N= 8  29.0s     (3.5x)
#     N=16  74.2s     (9.0x)
#
# Con el techo por mutante en ~75s, 16 workers dejan al mas lento a 1s del
# limite: por eso salian 13 TIMED OUT que no son mutantes lentos sino contencion.
# 8 workers va al 40% del techo, con un margen que aguanta una cache friendo o una
# suite que se alargue dos segundos. Perder un par de workers cuesta wall clock;
# perder 13 mutantes cuesta un falso 100% de eficacia, porque TIMED OUT no entra
# en mutants_total ni lo ve el gate.
#
# Override: WORKERS=16 para apurar un run y aceptar el riesgo.

# Sin denominador, el supervisor degrada a modo lineas. Con denominador, vigila
# "procesados/total" y cambia a solo-techo al llegar al total.
TOTAL=$(go tool gremlins unleash "${SCOPE[@]}" --dry-run \
	--exclude-files "$MUTATE_EXCLUDE" --coverpkg "$MUTATE_COVERPKG" 2>/dev/null |
	grep -cE "$PROGRESS_RE")

echo "mutate-local: cobertura ${cov_secs}s -> coef $COEF, techo por mutante ${timeout_secs}s" >&2
echo "mutate-local: ${WORKERS} workers, ${TOTAL} mutantes esperados" >&2

if [[ ${1:-} == --dry || $SOLO_DRY -eq 1 ]]; then
	echo "mutate-local: solo el denominador; nada que mutar"
	exit 0
fi

exec "$WD" "$STALL" "$MAX" "$TOTAL" "$PROGRESS_RE" -- \
	go tool gremlins unleash "${SCOPE[@]}" \
	--exclude-files "$MUTATE_EXCLUDE" --coverpkg "$MUTATE_COVERPKG" \
	--workers "$WORKERS" --timeout-coefficient "$COEF" --output report.json
