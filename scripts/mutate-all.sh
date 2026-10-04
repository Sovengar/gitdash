#!/usr/bin/env bash
# mutate-all.sh — the mutation testing run as it should be done locally.
# Why it exists and is not just `make mutate` with different numbers: its two knobs are COUPLED and measuring them apart gives a lying number (`testExecutionTime = coverage duration x --timeout-coefficient`, passed as `go test -timeout` to each mutant), so the coefficient is a switch and not a dial, and TIMED OUT is the worst outcome because it does not go into `mutants_total` and the gate only looks at `LIVED`, i.e. a run reporting 100% efficacy without having tested 15 mutants.
# What this script cannot do alone is follow the cache temperature, so it measures coverage and picks: with a warm cache (~2s) coef 30 gives a ~6s ceiling while internal/tui alone takes 3s, and a cold cache (~23s) with coef 30 would make a truly hung mutant take 11 minutes to fall.
# Usage:
#   scripts/mutate-all.sh             # whole module
#   scripts/mutate-all.sh --diff      # only the diff vs main (daily loop)
#   scripts/mutate-all.sh --diff-dry  # the diff's denominator, without mutating
#   scripts/mutate-all.sh --dry       # the module's denominator, without mutating
# Override: WORKERS=8 COEF=10 scripts/mutate-all.sh

set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 2

WD=${WATCHDOG:-$HOME/.local/lib/swe/lib/watchdog.sh}
MUTATE_EXCLUDE='(\.worktrees/|internal/testutil/)'
MUTATE_COVERPKG=./...
STALL=${STALL:-20m}
MAX=${MAX:-60m}

# One gremlins mutant line: `Mutant()` prints "%s%s %s at %s\n" (report.go:272-281), so the line ends in `at <file>:<line>:<col>` with the colors in the status, and the trailing `[^[:space:]]` tolerates the CR a terminal's line discipline adds so this does not depend on the output being redirected.
PROGRESS_RE='at [^[:space:]]+:[[:digit:]]+:[[:digit:]]+[^[:space:]]*$'

[[ -x $WD ]] || {
	echo "mutate-all: the supervisor is missing at $WD" >&2
	echo "  chezmoi source: ~/.local/share/chezmoi/home/dot_local/lib/swe/lib/executable_watchdog.sh" >&2
	echo "  apply it with: chezmoi apply ~/.local/lib/swe/lib/watchdog.sh" >&2
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
	echo "usage: ${0##*/} [--diff|--diff-dry|--dry]" >&2
	exit 2
	;;
esac

# The dry-run pre-pass is what measures coverage, and its DURATION is what decides the coefficient, so the number comes from here and not from an invented constant.
echo "mutate-all: measuring coverage to choose the per-mutant timeout…" >&2
# The real format is "done in 8.858216525s" (log.Infof("done in %s\n") with %s over a time.Duration), so the `sed` pulls the NUMBERS and drops the trailing 's'; a `s|.*done in (.*)|\1|` ate the rest of the line, which in a Spanish locale with a decimal comma shows up as "8,858216525s".
cov_secs=$(go tool gremlins unleash --dry-run "${SCOPE[@]}" \
	--exclude-files "$MUTATE_EXCLUDE" --coverpkg "$MUTATE_COVERPKG" 2>&1 |
	sed -n 's/.*done in \([0-9,.]*\)s.*/\1/p' | tail -1 |
	tr ',' '.')
[[ -n $cov_secs ]] || cov_secs=0

# The per-mutant ceiling is `cov_secs * COEF` + 2s (the +2 is what gremlins passes to `go test -timeout` on top of the context, executor.go:227).
# The ceiling has to be GENEROUS so contention does not eat healthy mutants, but not so generous that a truly hung one takes minutes to fall: a warm cache measures ~2s and needs coef 30 for internal/tui (3s), while a COLD cache measuring ~9-25s with coef 30 would give a 5-12 minute ceiling.
# The cutoff sits at 12s of coverage, measured: that is where `cov * 2` stops giving a comfortable ceiling (24s, generous for a suite that takes 3s warm) and starts giving one that eats genuinely slow mutants (warm cache 1-3s lands on coef 30, cold 10-25s on coef 2).
COEF=${COEF:-}
if [[ -z $COEF ]]; then
	COEF=30
	awk -v c="$cov_secs" 'BEGIN{exit !(c+0 > 12)}' && COEF=2
fi
WORKERS=${WORKERS:-8}
timeout_secs=$(awk -v c="$cov_secs" -v k="$COEF" 'BEGIN{printf "%d", c*k+2}')

# Per-mutant ceiling, with an override. Measured: 120s is INSUFFICIENT with the
# internal/tui suite at ~12s and several workers in parallel — two runs in a row
# with that ceiling ate 560 and 520 mutants (TIMED OUT), which enter neither
# mutants_total nor the gate's view: the run reported 99.78% efficacy without
# having tested half the module.
#
# The default rises to 300s because the ceiling has to leave ROOM for contention,
# and contention is brutal (measured on internal/gitstatus, the slowest suite):
#
#     N= 1   8.2s     (reference)
#     N= 8  29.0s     (3.5x)
#     N=16  74.2s     (9.0x)
#
# A TRULY hung mutant falls at 300s, not at the 5 minutes coef 30 gives with a
# warm cache (300s * 8 mutants in flight / 8 workers = the run's ceiling stays
# dominated by useful work, not by hangs).
CEIL_MAX=${CEIL_MAX:-300}
if [[ $timeout_secs -gt $CEIL_MAX ]]; then
	echo "mutate-all: WARNING — the per-mutant ceiling comes out at ${timeout_secs}s, clipping it to ${CEIL_MAX}s." >&2
	echo "  A hung mutant would sit for ${timeout_secs}s with nobody killing it." >&2
	echo "  If the cache is thrashing, raise the ceiling:" >&2
	echo "    CEIL_MAX=${timeout_secs} WORKERS=4 $0 ${1:+--diff}" >&2
	echo "  And lowering workers cuts contention, which is the real cause." >&2
	timeout_secs=$CEIL_MAX
fi

# The ceiling has to give ROOM to contention, and contention is brutal: measured on internal/gitstatus (the slowest suite, ~8s alone) the slowest `go test` of a batch of N in parallel takes 8.2s (N=1), 29.0s (N=8) and 74.2s (N=16), so with a ~75s ceiling 16 workers leave the slowest 1s from the limit (13 TIMED OUTs that are contention, not slow mutants) while 8 workers sit at 40% of it.
# Losing a couple of workers costs wall clock; losing 13 mutants costs a false 100% efficacy, because TIMED OUT enters neither mutants_total nor the gate's view.
# Override: WORKERS=16 to push a run and accept the risk.

# With no denominator the supervisor degrades to line mode; with one it watches "processed/total" and switches to ceiling-only on arrival.
TOTAL=$(go tool gremlins unleash "${SCOPE[@]}" --dry-run \
	--exclude-files "$MUTATE_EXCLUDE" --coverpkg "$MUTATE_COVERPKG" 2>/dev/null |
	grep -cE "$PROGRESS_RE")

echo "mutate-all: coverage ${cov_secs}s -> coef $COEF, per-mutant ceiling ${timeout_secs}s" >&2
echo "mutate-all: ${WORKERS} workers, ${TOTAL} mutants expected" >&2

if [[ ${1:-} == --dry || $SOLO_DRY -eq 1 ]]; then
	echo "mutate-all: only the denominator; nothing to mutate"
	exit 0
fi

exec "$WD" "$STALL" "$MAX" "$TOTAL" "$PROGRESS_RE" -- \
	go tool gremlins unleash "${SCOPE[@]}" \
	--exclude-files "$MUTATE_EXCLUDE" --coverpkg "$MUTATE_COVERPKG" \
	--workers "$WORKERS" --timeout-coefficient "$COEF" --output report.json
