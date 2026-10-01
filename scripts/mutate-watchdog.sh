#!/usr/bin/env bash
# Lanza un comando y lo corta si se QUEDA PARADO, no solo si se pasa de tiempo.
#
# Por que no basta `timeout N`: ese es un techo absoluto, asi que un run que se
# atasca a los dos minutos sigue ocupando media hora. Aqui lo que se vigila es
# el PROGRESO: si el comando no escribe nada en su salida durante STALL
# segundos, esta colgado y se le corta.
#
# Por que kill al GRUPO de procesos y no solo al hijo: el comando lanza `go test`
# por debajo, y esos hijos heredan el fichero mutado. Si se mata solo al padre,
# los nietos siguen corriendo y siguen dejando el arbol a medias.
#
# Uso: mutate-watchdog.sh STALL_SEGUNDOS MAX_SEGUNDOS -- comando args...
# Salida: el codigo del comando, o 124 si se corto por atasco o por techo.
set -u

STALL=${1:?falta STALL}
MAX=${2:?falta MAX}
shift 2
[ "${1:-}" = "--" ] && shift
[ $# -gt 0 ] || { echo "uso: $0 STALL MAX -- cmd..." >&2; exit 2; }

# Acepta segundos sueltos o con sufijo (30s, 5m, 2h). Enteros a pelo, que es
# como se lee un techo, y con sufijo, que es como se piensa un margen.
to_secs() {
	case "$1" in
	*s) echo $((${1%s})) ;;
	*m) echo $(( ${1%m} * 60 )) ;;
	*h) echo $(( ${1%h} * 3600 )) ;;
	*) echo "$1" ;;
	esac
}
STALL=$(to_secs "$STALL")
MAX=$(to_secs "$MAX")

out=$(mktemp)
trap 'rm -f "$out"' EXIT

# setsid: el comando pasa a ser lider de su propio grupo, para que el kill
# alcance tambien a los nietos.
setsid "$@" >"$out" 2>&1 &
pid=$!

inicio=$(date +%s)
ultima=$inicio
tamano=0

while kill -0 "$pid" 2>/dev/null; do
	sleep 5
	nuevo=$(date +%s)
	actual=$(wc -c <"$out")
	if [ "$actual" -ne "$tamano" ]; then
		tamano=$actual
		ultima=$nuevo
	fi

	if [ $((nuevo - ultima)) -ge "$STALL" ]; then
		echo "mutate-watchdog: sin salida hace $((nuevo - ultima))s -> atascado, cortando" >&2
		kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null
		sleep 2
		kill -KILL -- "-$pid" 2>/dev/null || true
		cat "$out"
		exit 124
	fi

	if [ $((nuevo - inicio)) -ge "$MAX" ]; then
		echo "mutate-watchdog: $MAXs de techo -> cortando" >&2
		kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null
		sleep 2
		kill -KILL -- "-$pid" 2>/dev/null || true
		cat "$out"
		exit 124
	fi
done

wait "$pid"
rc=$?
cat "$out"
exit $rc
