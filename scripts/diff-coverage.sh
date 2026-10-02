#!/usr/bin/env bash
# diff-coverage.sh — la cobertura de las lineas que este PR TOCA.
#
# Por que el diff y no el total: un umbral sobre el total del proyecto es un
# numero que ningun PR puede mover de forma.local. Anades tres lineas testeadas
# y sube a 99.9%; refactorizas cien lineas sin tests y baja a 97%. El primero es
# casi siempre ruido y el segundo casi siempre un bug, y un gate que no distingue
# entre los dos no sirve para ninguna de las dos cosas.
#
# El diff invierte eso: mira SOLO las lineas del cambio, y exige el 100% de ahi.
# Quien toca el codigo es quien tiene que testearlo, y codigo viejo que no se
# toca no puede bloquear un PR. Es la metrica que Google llama changelist
# coverage (Ivankovic et al., "Code Coverage at Google", FSE 2019: 14M de
# mediciones de changelist sobre un billon de lineas) y la que se muestra en su
# code review al autor y a los revisores.
#
# Uso:  diff-coverage.sh <perfil.out> [base] [--min N] [-a perfil.extra]
#   perfil.out     salida de go test -coverprofile
#   base           ref contra la que se compara (default: MUTATE_BASE, luego main)
#   --min N        porcentaje minimo del diff (default 100)
#   -a perfil      perfil adicional a fusionar (contadores de un subproceso;
#                  tambien se puede con DIFF_COVERAGE_EXTRA, separados por espacio)
#
# Salida: tabla por fichero + resumen; exit 1 si el diff queda por debajo del
# minimo, exit 2 si el calculo no se puede hacer.
set -euo pipefail

PROFILE="${1:?falta el perfil de cobertura}"
shift || true

BASE="${MUTATE_BASE:-main}"
MIN=100
EXTRA_ARGS=()
while [ $# -gt 0 ]; do
  case "$1" in
    --min) MIN="${2:?--min necesita un numero}"; shift 2 ;;
    --min=*) MIN="${1#--min=}"; shift ;;
    -a) EXTRA_ARGS+=("${2:?-a necesita un perfil}"); shift 2 ;;
    -a*) EXTRA_ARGS+=("${1#-a}"); shift ;;
    *) BASE="$1"; shift ;;
  esac
done

if [ ! -f "$PROFILE" ]; then
  echo "diff-coverage: no existe el perfil '$PROFILE'" >&2
  exit 2
fi

# El perfil tiene un bloque por test binary: el mismo rango sale N veces, una con
# count>0 (el binario dueño del paquete) y el resto a 0 (los demas, que
# instrumentan pero no ejercitan). Deduplicar por rango quedandose con el MAX
# del count es lo que convierte el perfil en una medida. Sin esto, aqui mismo se
# leeria 10% de cobertura en un repo al 98%.
python3 - "$PROFILE" "$BASE" "$MIN" "${EXTRA_ARGS[@]}" <<'PY'
import re, subprocess, sys, os
from collections import defaultdict

profile, base, min_pct = sys.argv[1], sys.argv[2], float(sys.argv[3])
# Los perfiles extra llegan tal cual, SIN filtrar por existencia: un perfil que
# no existe tiene que ser un error, no un perfil que se ignora en silencio (eso
# haria que el gate pasara sin el, que es justo lo que el perfil extra existe
# para evitar).
argv_extra = [a for a in sys.argv[4:] if not a.startswith("-")]

# --- perfiles adicionales ---------------------------------------------------
# `go test -coverprofile` NO recoge los contadores de un SUBPROCESO. El unico
# statement con esa forma en este repo es func main(), que se ejecuta de verdad
# en un subproceso (ver cmd/gitdash/main_test.go) pero cuyo contador vive en un
# fichero aparte. Sin este merge, main() sale a 0 en la metrica aunque su test
# verifique que se ejecuto.
#
# Se fusiona por MAX de count sobre el mismo rango, que es la misma regla que
# para los duplicados del perfil principal: dos medidas del mismo bloque, gana la
# mayor. Sumar daria un numero sin sentido.
extra_profiles = argv_extra + os.environ.get("DIFF_COVERAGE_EXTRA", "").split()
all_profiles = [profile] + extra_profiles

# --- perfil -> bloques unicos con el maximo de count -------------------------
# El perfil nombra los ficheros como <modulo>/<ruta>, y el path del repo es
# <ruta>. El prefijo sale de go.mod, no de una constante: si el modulo se
# renombra, un prefijo hardcodeado deja TODAS las rutas sin casar y el script
# informa "sin lineas tocadas", que es un aprobado silencioso de un PR entero.
prefix = ""
try:
    with open("go.mod") as fh:
        for l in fh:
            if l.startswith("module "):
                prefix = l.split(None, 1)[1].strip()
                break
except OSError:
    pass

blocks = defaultdict(dict)   # fichero -> {(sl,sc,el,ec): count}
for prof in all_profiles:
    if not os.path.exists(prof):
        print(f"diff-coverage: perfil adicional no existe: {prof}", file=sys.stderr)
        sys.exit(2)
    with open(prof) as fh:
        for line in fh:
            if line.startswith("mode:"):
                continue
            rng, n, c = line.rsplit(" ", 2)
            m = re.match(r"^(.*):(\d+)\.(\d+),(\d+)\.(\d+)$", rng)
            if not m:
                continue
            f = m.group(1)
            if prefix and f.startswith(prefix + "/"):
                f = f[len(prefix) + 1:]
            key = (int(m.group(2)), int(m.group(3)), int(m.group(4)), int(m.group(5)))
            prev = blocks[f].get(key, -1)
            if int(c) > prev:
                blocks[f][key] = int(c)

total_stmt = sum(len(v) for v in blocks.values())
covered = sum(1 for v in blocks.values() for c in v.values() if c > 0)
pct = 100.0 * covered / total_stmt if total_stmt else 0.0

# --- que lineas ha cambiado el diff ------------------------------------------
# `git diff -U0` da los hunks sin contexto: una linea modificada cuenta, las de
# alrededor no. El segundo `-U0` del formato es para que el hunk no traiga el
# Codigo de la linea anterior.
def changed_lines(path):
    try:
        out = subprocess.run(
            ["git", "diff", "-U0", "--diff-filter=ACMRT", f"{base}...HEAD", "--", path],
            capture_output=True, text=True, check=False).stdout
    except FileNotFoundError:
        return None
    if not out.strip():
        return None
    added = set()
    for hunk in re.finditer(r"^@@ -\S+ \+(\d+)(?:,(\d+))? @@", out, re.M):
        start, count = int(hunk.group(1)), int(hunk.group(2) or 1)
        if count == 0:
            continue          # solo borrados: no existen, no se cubren
        added.update(range(start, start + count))
    return added or None

# --- diff contra los bloques del perfil --------------------------------------
# Una linea tocada esta cubierta si pertenece a un bloque con count>0. Un bloque
# de varias lineas marca todas: en Go un bloque es un grupo de sentencias, y si
# se ejecuto una del grupo se ejecuto el grupo.
#
# Y una linea tocada SOLO cuenta si ademas esta en ALGUN bloque (count>0 o no).
# `git diff` no distingue un statement de un comentario, de un `}` de cierre o de
# una linea de import: son lineas añadidas igual. Contarlas como "sin cubrir"
# haria que un fichero con 60 lineas de comentario y 3 statements saliera al 5%,
# y que el gate fuera inalcanzable sin importar quantos tests escribas.
diff_total = diff_cov = 0
por_fichero = defaultdict(lambda: [0, 0])   # fichero -> [total, cubiertas]
sin_cubrir = []

for f, bs in sorted(blocks.items()):
    touched = changed_lines(f)
    if not touched:
        continue
    # Las lineas con statement segun el propio perfil: la unica fuente de verdad
    # sobre que es ejecutable.
    ejecutable = {ln for (sl, sc, el, ec) in bs for ln in range(sl, el + 1)}
    touched = touched & ejecutable
    if not touched:
        continue
    hit = set()
    for (sl, sc, el, ec), c in bs.items():
        if c <= 0:
            continue
        for ln in touched:
            if sl <= ln <= el:
                hit.add(ln)
    # Cada linea tocada cuenta UNA vez, sin importar cuantos bloques la toquen:
    # sin esto, una linea en el borde de tres bloques cuenta triple y el
    # porcentaje del diff deja de ser un porcentaje de lineas.
    for ln in touched:
        por_fichero[f][0] += 1
    for ln in hit:
        por_fichero[f][1] += 1
    for ln in sorted(touched - hit):
        sin_cubrir.append(f"{f}:{ln}")

diff_total = sum(v[0] for v in por_fichero.values())
diff_cov = sum(v[1] for v in por_fichero.values())

print(f"## Cobertura\n")
print(f"**Total del proyecto: {pct:.2f}%** ({covered}/{total_stmt} statements)")
print(f"**Diff vs {base}: "
      + (f"{(100.0 * diff_cov / diff_total if diff_total else 100.0):.2f}%** ({diff_cov}/{diff_total} lineas)"
         if diff_total else "sin lineas .go tocadas**") + "")
print()

if por_fichero:
    print("| Fichero | Diff |")
    print("| --- | --- |")
    for f, (t, c) in sorted(por_fichero.items()):
        print(f"| `{f}` | {100.0 * c / t:.1f}% ({c}/{t}) |")
    print()

if sin_cubrir:
    print("<details><summary>Lineas del diff sin cubrir "
          f"({len(sin_cubrir)})</summary>")
    print()
    for s in sin_cubrir:
        print(f"- `{s}`")
    print()
    print("</details>")
    print()

# --- veredicto ---------------------------------------------------------------
# Dos gates distintos, y por eso dos numeros distintos:
#
#   1. El DIFF al 100%. Quien toca el codigo tiene que testearlo. No le
#      importa nada lo que pase en el resto del repo, y por eso las lineas
#      viejas sin cubrir no bloquean un PR.
#   2. El TOTAL con suelo. El diff solo mira lo nuevo: un PR puede meter codigo
#      sin tests en un fichero que ya estaba cubierto y el diff no lo ve. El
#      suelo es lo que impide que el conjunto se degrade, y solo puede subir.
#
# El suelo vive en un fichero del repo, no en el workflow: si es una excepcion
# tiene nombre, se commitea y se ve en el diff. Aqui solo se COMPRA el suelo (el
# total nunca baja del que hay) y se AVISA cuando el total sube, para que subir
# el suelo sea una decision.
diff_pct = 100.0 * diff_cov / diff_total if diff_total else 100.0
fallos = []

if diff_pct < min_pct:
    fallos.append(f"el diff se queda en {diff_pct:.2f}%, por debajo del {min_pct:.0f}% pedido")

baseline_file = "scripts/coverage-floor"
suelo = None
if os.path.exists(baseline_file):
    # El fichero lleva comentario explicativo arriba, asi que el numero es la
    # ULTIMA linea no comentada y no vacia. Leer la primera palabra daria "#".
    with open(baseline_file) as fh:
        for l in fh:
            l = l.strip()
            if l and not l.startswith("#"):
                suelo = float(l)
                break
    if suelo is None:
        print(f"diff-coverage: {baseline_file} no tiene numero", file=sys.stderr)
        sys.exit(2)
    # La comparación va con la MISMA precision con la que se imprime el suelo, y
    # con la que el total sale en el resumen. Comparar el float crudo contra un
    # suelo escrito a dos decimales hace fallar el gate por un 0.001: con 1929/1960
    # el total es 98.11800610376399, se imprime "98.12" y el suelo es "98.12", y
    # aun asi 98.118 < 98.12. El suelo es un numero que se lee, no un float.
    if round(pct, 2) < round(suelo, 2):
        fallos.append(f"el total baja a {pct:.2f}%, por debajo del suelo de {suelo:.2f}% "
                      f"(sube test, no bajes el suelo; si el suelo esta equivocado, "
                      f"corrige scripts/coverage-floor en el MISMO commit)")
    elif round(pct, 2) > round(suelo, 2):
        print(f"aviso: el total esta en {pct:.2f}% y el suelo es {suelo:.2f}%. "
              f"Sube scripts/coverage-floor a {pct:.2f} en este commit para que "
              f"el ratchet no se quede viejo.", file=sys.stderr)
else:
    print(f"aviso: no hay {baseline_file}; el suelo del total no se comprueba",
          file=sys.stderr)

for f in fallos:
    print(f"diff-coverage: {f}", file=sys.stderr)

if fallos:
    print(f"diff: {diff_pct:.2f}% (minimo {min_pct:.0f}%) · "
          f"total: {pct:.2f}% (suelo {suelo if suelo is not None else 'ninguno'})")
    sys.exit(1)
print(f"diff-coverage: ok (diff {diff_pct:.2f}%, total {pct:.2f}%)")
PY