#!/usr/bin/env bash
#
# setup-repo-protection.sh — configura de forma idempotente el ruleset de
# protección (nombre `protect-<rama>`, p. ej. `protect-main`), el ajuste de
# merge del repo y las labels que referencia dependabot.yml.
#
# Política (ver el plan / descripción del PR para el razonamiento):
#   - bloquea el borrado de la rama      (`deletion`)
#   - bloquea los force-push             (`non_fast_forward`)
#   - exige PR para mergear              (`pull_request`, 0 approvals -> dev solo)
#   - exige los checks de CI             (`required_status_checks`, strict = false)
#
# `strict_required_status_checks_policy` es deliberadamente false: exigir la
# rama al día antes de mergear forzaría un rebase en cada PR concurrente. No
# queremos rebases forzados.
#
# Los contexts de los checks requeridos se DERIVAN DE LA REALIDAD: se leen de
# los check runs del head del PR actualizado más recientemente, de modo que el
# gate nunca se configura contra nombres de check que no existen.
#
# BYPASS — consecuencia aceptada, deliberada; NO "arreglar": el rol de admin del
# repo mantiene `bypass_mode: always`. El admin puede por tanto mergear PRs en
# rojo y pushear o force-pushear `main`, saltándose todas las reglas de arriba.
# El gate es absoluto solo para actores no-admin. Modo estricto (sin válvula de
# escape) = quitar `bypass_actors` y desactivar el ruleset temporalmente para
# hotfixes.
#
# Labels: dependabot.yml referencia `dependencies` y `ci`. GitHub descarta en
# silencio las labels no definidas, así que este script las crea si faltan
# (idempotente; una label renombrada se recrea con el nombre nuevo).
#
# Requiere: gh (autenticado, admin del repo) y jq.
#
# Uso:
#   scripts/setup-repo-protection.sh [--dry-run] [--contexts Build,Lint,Test] [--sha <commit>]
#
# Variables de entorno:
#   RULESET_NAME=protect-<rama>   BRANCH=<default del repo>   GH_ACTIONS_APP_ID=15368

set -euo pipefail

# GitHub Actions es la integración que reporta nuestros check runs de CI.
GH_ACTIONS_APP_ID="${GH_ACTIONS_APP_ID:-15368}"
REQUIRED_DEFAULT=(Build Lint Test)

# Labels referenciadas por .github/dependabot.yml, como "nombre|color|descripción".
LABELS=(
  "dependencies|0366d6|Dependency updates"
  "ci|0e8a16|CI / build pipeline"
)

DRY_RUN=0
OPT_CONTEXTS=""
OPT_SHA=""

usage() {
  # Imprime el bloque de comentario inicial (todo tras el shebang).
  awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
  exit 0
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --contexts) OPT_CONTEXTS="${2:-}"; shift 2 ;;
    --sha) OPT_SHA="${2:-}"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "argumento desconocido: $1" >&2; exit 2 ;;
  esac
done

for bin in gh jq; do
  command -v "$bin" >/dev/null 2>&1 || { echo "ERROR: se requiere '$bin' y no está instalado." >&2; exit 1; }
done

REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
[ -n "$REPO" ] || { echo "ERROR: no se pudo resolver owner/repo (ejecuta dentro del repositorio)." >&2; exit 1; }

# La rama por defecto se deriva del repo; BRANCH (entorno) la sobreescribe.
BRANCH="${BRANCH:-$(gh repo view --json defaultBranchRef -q .defaultBranchRef.name)}"
[ -n "$BRANCH" ] && [ "$BRANCH" != "null" ] || { echo "ERROR: no se pudo resolver la rama por defecto de ${REPO}." >&2; exit 1; }

# El nombre del ruleset se deriva de la rama (protect-<rama>), de modo que el
# script sirva igual para repos cuya rama por defecto no sea `main`.
RULESET_NAME="${RULESET_NAME:-protect-${BRANCH}}"

# find_ruleset_id imprime el id del ruleset llamado RULESET_NAME, o nada.
# La lista es paginada (per_page=100) para que una colección grande no haga
# perder el ruleset existente y crear un duplicado. Un fallo de gh es fatal: un
# error transitorio jamás debe leerse como "no existe ruleset". `jq -s` aglutina
# los arrays por página que emite `gh api --paginate`.
find_ruleset_id() {
  local json
  if ! json="$(gh api "repos/${REPO}/rulesets?per_page=100" --paginate)"; then
    echo "ERROR: no se pudieron listar los rulesets de ${REPO}." >&2
    exit 1
  fi
  printf '%s' "$json" | jq -s -r --arg name "$RULESET_NAME" \
    '[.[][] | select(.name == $name) | .id] | first // empty'
}

# ensure_label NOMBRE COLOR DESCRIPCIÓN — crea la label solo si falta, de modo
# que los renombrados se detectan y las existentes quedan intactas.
ensure_label() {
  local name="$1" color="$2" desc="$3" existing=""
  if existing="$(gh api "repos/${REPO}/labels/${name}" --jq '.name' 2>/dev/null)" && [ -n "$existing" ]; then
    echo "==> La label '${name}' ya existe"
    return 0
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "--- haría POST repos/${REPO}/labels { name: ${name}, color: ${color} } ---"
    return 0
  fi
  gh api -X POST "repos/${REPO}/labels" \
    -f "name=${name}" -f "color=${color}" -f "description=${desc}" >/dev/null
  echo "==> Label '${name}' creada"
}

# ensure_labels aplica la tabla LABELS.
ensure_labels() {
  local spec name color desc
  for spec in "${LABELS[@]}"; do
    IFS='|' read -r name color desc <<< "$spec"
    ensure_label "$name" "$color" "$desc"
  done
}

echo "==> Repositorio: ${REPO}"
echo "==> Ruleset:     ${RULESET_NAME} (target=refs/heads/${BRANCH}, enforcement=active)"

# ---------------------------------------------------------------------------
# 1. Verifica el app id de GitHub Actions en vez de confiar en el valor fijo.
# ---------------------------------------------------------------------------
actual_app_id="$(gh api /apps/github-actions --jq '.id')"
if [ "$actual_app_id" != "$GH_ACTIONS_APP_ID" ]; then
  echo "ERROR: el app id de GitHub Actions es '${actual_app_id}', se esperaba '${GH_ACTIONS_APP_ID}'." >&2
  echo "       Actualiza GH_ACTIONS_APP_ID antes de continuar." >&2
  exit 1
fi
echo "==> App id de GitHub Actions verificado: ${GH_ACTIONS_APP_ID}"

# ---------------------------------------------------------------------------
# 2. Resuelve los contexts de los checks requeridos.
#    Por defecto: se derivan de los check runs reales del head del último PR.
# ---------------------------------------------------------------------------
contexts=()
if [ -n "$OPT_CONTEXTS" ]; then
  IFS=',' read -r -a names <<< "$OPT_CONTEXTS"
  for name in "${names[@]}"; do
    name="$(printf '%s' "$name" | xargs)"
    [ -n "$name" ] && contexts+=("$name")
  done
  echo "==> Contexts de check (override explícito): ${contexts[*]}"
else
  sha="${OPT_SHA}"
  if [ -z "$sha" ]; then
    sha="$(gh api "repos/${REPO}/pulls?state=all&sort=updated&direction=desc&per_page=1" --jq '.[0].head.sha' 2>/dev/null || true)"
  fi
  if [ -z "$sha" ] || [ "$sha" = "null" ]; then
    echo "ERROR: no hay SHA de head de PR para derivar los nombres de check requeridos." >&2
    echo "       Abre un PR cuya CI haya corrido, o pasa --sha <commit> / --contexts Build,Lint,Test." >&2
    exit 1
  fi
  echo "==> Derivando contexts de los check runs de ${sha}"
  observed="$(gh api "repos/${REPO}/commits/${sha}/check-runs?per_page=100" --jq '.check_runs[].name' 2>/dev/null || true)"
  if [ -z "$observed" ]; then
    echo "ERROR: no se encontraron check runs en ${sha}. Puede que la CI aún no haya corrido." >&2
    exit 1
  fi
  for req in "${REQUIRED_DEFAULT[@]}"; do
    match="$(printf '%s\n' "$observed" | grep -Fx "$req" | head -n1 || true)"
    if [ -z "$match" ]; then
      echo "ERROR: no se encontró el check '${req}' en ${sha}." >&2
      echo "       Check runs observados:" >&2
      while IFS= read -r line; do
        [ -n "$line" ] && printf '         %s\n' "$line" >&2
      done <<< "$observed"
      echo "       Se rechaza configurar un ruleset contra checks inexistentes." >&2
      exit 1
    fi
    contexts+=("$match")
  done
  echo "==> Contexts de check (derivados de runs observados): ${contexts[*]}"
fi

if [ "${#contexts[@]}" -eq 0 ]; then
  echo "ERROR: no se resolvió ningún context de status check." >&2
  exit 1
fi

# Construye el array required_status_checks, fijando cada context a la
# integración de GitHub Actions para que solo los checks reportados por Actions
# satisfagan el gate.
checks_json="["
first=1
for ctx in "${contexts[@]}"; do
  if [ "$first" -eq 0 ]; then checks_json+=","; fi
  first=0
  checks_json+="{\"context\":\"${ctx}\",\"integration_id\":${GH_ACTIONS_APP_ID}}"
done
checks_json+="]"

# ---------------------------------------------------------------------------
# 3. Construye el payload del ruleset.
#
# NOTA sobre `bypass_actors`: el rol de admin del repo (actor_id 5) mantiene
# `bypass_mode: "always"` a propósito — es la válvula de escape aprobada por el
# dueño. Consecuencia aceptada (deliberada): un admin puede mergear PRs en rojo
# y pushear o force-pushear `main`, así que el ruleset es absoluto solo para
# actores no-admin. Aplicación estricta (sin válvula) = quitar `bypass_actors` y
# desactivar el ruleset explícitamente durante un hotfix.
# ---------------------------------------------------------------------------
payload="$(cat <<JSON
{
  "name": "${RULESET_NAME}",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": {
      "include": ["refs/heads/${BRANCH}"],
      "exclude": []
    }
  },
  "bypass_actors": [
    {
      "actor_id": 5,
      "actor_type": "RepositoryRole",
      "bypass_mode": "always"
    }
  ],
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false,
        "require_extra_approval_for_unattributed_changes": true,
        "allowed_merge_methods": ["merge", "squash", "rebase"]
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": false,
        "do_not_enforce_on_create": false,
        "required_status_checks": ${checks_json}
      }
    }
  ]
}
JSON
)"

# Falla rápido si el JSON generado está malformado.
printf '%s' "$payload" | jq -e . >/dev/null

# ---------------------------------------------------------------------------
# 4. Busca un ruleset existente con este nombre (idempotencia).
# ---------------------------------------------------------------------------
existing_id="$(find_ruleset_id)"

if [ "$DRY_RUN" -eq 1 ]; then
  echo "==> DRY RUN — no se realizará ninguna mutación."
  if [ -n "$existing_id" ]; then
    echo "--- haría PUT repos/${REPO}/rulesets/${existing_id} ---"
  else
    echo "--- haría POST repos/${REPO}/rulesets ---"
  fi
  printf '%s\n' "$payload"
  echo "--- haría PATCH repos/${REPO} { \"delete_branch_on_merge\": true } ---"
  ensure_labels
  exit 0
fi

# ---------------------------------------------------------------------------
# 5. Crea o actualiza el ruleset (JSON por stdin).
# ---------------------------------------------------------------------------
if [ -n "$existing_id" ]; then
  echo "==> Actualizando ruleset existente id=${existing_id}"
  printf '%s' "$payload" | gh api -X PUT "repos/${REPO}/rulesets/${existing_id}" --input - >/dev/null
else
  echo "==> Creando ruleset"
  printf '%s' "$payload" | gh api -X POST "repos/${REPO}/rulesets" --input - >/dev/null
fi

ruleset_id="$(find_ruleset_id)"
if [ -z "$ruleset_id" ]; then
  echo "ERROR: ruleset '${RULESET_NAME}' no encontrado tras la escritura." >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# 6. Ajuste del repo: borra la rama head al mergear un PR.
# ---------------------------------------------------------------------------
gh api -X PATCH "repos/${REPO}" -f delete_branch_on_merge=true >/dev/null
echo "==> Ajuste de repo delete_branch_on_merge=true aplicado"

# ---------------------------------------------------------------------------
# 6b. Labels de dependabot — creadas solo si faltan (idempotente).
# ---------------------------------------------------------------------------
ensure_labels

# ---------------------------------------------------------------------------
# 7. Resumen de auditoría — relee el ruleset desde la API.
# ---------------------------------------------------------------------------
readback="$(gh api "repos/${REPO}/rulesets/${ruleset_id}")"
echo
echo "================= RULESET AUDIT ================="
printf '%s' "$readback" | jq -r '
  "id:          \(.id)",
  "name:        \(.name)",
  "target:      \(.target)",
  "enforcement: \(.enforcement)",
  "include:     \(.conditions.ref_name.include | join(", "))",
  "",
  "rules:"
'
printf '%s' "$readback" | jq -r '
  .rules[] |
  if .type == "required_status_checks" then
    "  - \(.type) (strict=\(.parameters.strict_required_status_checks_policy)): " +
    ([.parameters.required_status_checks[] | .context] | join(", "))
  elif .type == "pull_request" then
    "  - \(.type) (required_approving_review_count=\(.parameters.required_approving_review_count))"
  else
    "  - \(.type)"
  end
'
echo
echo "bypass actors:"
printf '%s' "$readback" | jq -r '.bypass_actors[]? | "  - \(.actor_type) actor_id=\(.actor_id) mode=\(.bypass_mode)"'
echo
echo "labels:"
for spec in "${LABELS[@]}"; do
  name="${spec%%|*}"
  if ! gh api "repos/${REPO}/labels/${name}" --jq '"  - \(.name) #\(.color) — \(.description)"' 2>/dev/null; then
    echo "  - ${name} MISSING"
  fi
done
echo "================================================="
echo "==> Hecho. ruleset_id=${ruleset_id}"
