#!/usr/bin/env bash
# mutate.sh — measure the mutation of a scope AND decide its verdict, in one process.
#
# It replaces the mutation wiring that used to live in three places at once (the
# Makefile recipes, scripts/mutate-all.sh, and an inline gate in the workflow).
# One place knows how to warm, how to measure the denominator, how to pick the
# per-mutant ceiling, how to call the supervisor, and what to do with the result.
#
# The rule that shapes the whole design: "no measurement" is a RED verdict, never
# a green one. The old gate printed "no report.json -> no result, gate passes"
# and exited 0, so a supervisor missing from the runner turned the required check
# green while measuring nothing.
#
# Split: the run phase MEASURES (gremlins + watchdog) and the verdict phase
# DECIDES from files only — no gremlins, no Go, no git, no clock, no network.
# That is what makes every red path testable locally in under a second, with
# fixtures fabricated in a temp dir; see scripts/mutate_test.sh.
#
# Usage:
#   scripts/mutate.sh                 # measure the whole module
#   scripts/mutate.sh --diff          # measure only the diff vs $MUTATE_BASE
#   scripts/mutate.sh --diff --ci     # same, as the required check does it
#   scripts/mutate.sh --diff --dry    # warm + denominator only, mutate nothing
#   scripts/mutate.sh --verdict-only <report> <run-log> <allowlist> \
#       --expected-total N --engine-rc N [--announced P] [--budget P] [--summary P]
#
# Exit: 0 green, 1 red. 2 is misuse (bad flags, unresolvable base, missing budget
# knob) and never means anything about mutation.

set -uo pipefail

# One definition of "one unit of progress". The numerator (the watchdog's poll)
# and the denominator (the dry-run count) both come from this expression; define
# it twice and the supervisor declares a healthy run finished early.
# The format comes from report.go: Mutant prints "%s%s %s at %s\n", so a line
# ends in "at <file>:<line>:<col>". The trailing [^[:space:]] tolerates the CR a
# terminal line discipline adds, so this does not depend on stdout being piped.
PROGRESS_RE='at [^[:space:]]+:[[:digit:]]+:[[:digit:]]+[^[:space:]]*$'

# Flags that break the "exactly one line per mutant" contract, and with it the
# denominator. -S filters lines by status, so the numerator stops meaning
# "mutants processed"; -s suppresses every log.Infof, so the engine prints
# nothing while still exiting 0 WITH a report present — the silent shape this
# gate exists to refuse. The check lives in the caller because it restricts the
# INVOCATION, and the invocation belongs to the caller, not to the supervisor.
MUTATE_FORBIDDEN='-S --output-statuses -s --silent'

# The local budget, one row per scope, so the full-module local run and the CI
# diff run read the same formula instead of each keeping its own copy.
#
# cap      absolute ceiling per mutant. gremlins has no per-mutant absolute
#          timeout: --timeout-coefficient is the only lever, and it multiplies the
#          coverage-pass duration, so an absolute cap has to be expressed as
#          coefficient = ceil(cap / elapsed).
# workers  parallelism. Contention is what expires mutants, not slowness.
# stall    seconds without a new progress line before the run counts as wedged.
# ceiling  absolute ceiling for the whole run.
#
# CI does NOT inherit these: under --ci the budget knobs are mandatory, because
# "the environment wins" alone fails by a forgotten variable landing on this
# table, which is CI quietly loosening its own budget with no diff to show.
#
# scope  cap  workers  stall  ceiling
# run    180s 8       20m   60m
# diff   120s 4       2m    4m

# --- output -----------------------------------------------------------------
# Every reason line goes to stdout AND, when --summary was given, to that file:
# the run phase uses stdout, the workflow's verdict-only call uses the file.
OUT_LINES=()

_out() {
	printf '%s\n' "$*"
	OUT_LINES+=("$*")
}

_flush_summary() { # _flush_summary <path> <title>
	[[ -n ${1:-} ]] || return 0
	mkdir -p "$(dirname "$1")"
	{
		printf '### %s\n\n' "$2"
		printf '%s\n' "${OUT_LINES[@]}"
	} >"$1"
}

die() { # die <code> <message...>
	local code=$1
	shift
	printf 'mutate: %s\n' "$*" >&2
	exit "$code"
}

# Seconds, or a number with a 30s/5m/2h suffix.
_to_secs() {
	case $1 in
	*s) echo $((${1%s})) ;;
	*m) echo $((${1%m} * 60)) ;;
	*h) echo $((${1%h} * 3600)) ;;
	*) echo "$1" ;;
	esac
}

# ============================================================================
# VERDICT — a pure function of paths.
# ============================================================================
# Reads files and arguments, prints the reason, returns the colour. It never
# invokes gremlins, go, git or the clock, so every branch is reachable from a
# temp dir in under a second. Branch order is "least ambiguous first": a missing
# tool cannot judge anything, an unresolvable scope is not the set we announced,
# and a cancelled run has no verdict at all.
_verdict() { # <report> <run_log> <allowlist> <expected_total> <engine_rc> [announced] [budget]
	local report=$1 run_log=$2 allowlist=$3 expected=$4 engine_rc=$5
	local announced=${6:-} budget=${7:-}
	local tmp reason unannounced total new

	if ! command -v jq >/dev/null 2>&1; then
		_out "- **no measurement**: jq is not available, so the report cannot be read and no verdict can be reached."
		_out "- Install jq: an extractor that fails leaves an empty output, and an empty output read as 'zero new survivors' is the green-without-measurement this check forbids."
		return 1
	fi

	# Without an allowlist every survivor would look new, so the gate would be
	# comparing against nothing. Red with instructions, never a skip: a skipped
	# required check stays pending forever and blocks every PR.
	if [[ ! -f $allowlist ]]; then
		_out "- **no measurement**: $allowlist is missing, so the gate has nothing to compare survivors against."
		_out "- Seed it with \`make mutate-all\`, then commit $allowlist."
		return 1
	fi

	# An empty run log is the shape --silent leaves behind: the engine exits 0
	# with a report present and prints nothing, so there is no mutant line, no
	# TIMED OUT line, nothing to count. Judging that green is the hole itself.
	if [[ ! -s $run_log ]]; then
		_out "- **no measurement**: the run log is empty, so the engine produced no progress lines at all."
		_out "- An empty log is exactly what a silent run leaves behind while exiting 0; nothing was measured."
		return 1
	fi

	# 124 is the supervisor's own code for "I cut it", and the reason is the last
	# thing it printed on stderr, which is inside the run log. Stall and ceiling
	# demand opposite responses, so they are told apart, not lumped together.
	if [[ $engine_rc -eq 124 ]]; then
		reason=$(grep -oE 'no progress for [0-9]+s|reached .* did not exit within the [0-9]+s ceiling|[0-9]+s ceiling' "$run_log" | tail -1)
		_out "- **no measurement**: the supervisor cut the run — ${reason:-reason not found in the log}."
		_out "- A stall means the engine stopped making progress; the ceiling means it outlived its budget. Neither is a verdict about the tests."
		_out ''
		_log_tail "$run_log"
		return 1
	fi

	# 130/143 mean the SUPERVISOR was signalled, which on a runner is a
	# cancellation. No layer may translate them into survivors: they are not a
	# mutation result, and calling them one would be a lie.
	if [[ $engine_rc -eq 130 || $engine_rc -eq 143 ]]; then
		_out "- **no measurement**: the run was cancelled (the supervisor exited $engine_rc), so there is no result to judge."
		_out "- 130 and 143 pass through untranslated on purpose: they mean the runner cancelled the job, not that mutation produced a verdict."
		return 1
	fi

	if [[ $engine_rc -ne 0 ]]; then
		_out "- **no measurement**: the mutation engine exited $engine_rc, so the run failed."
		_out "- An exit that is not 124/130/143 is the engine's own error, not a cut by the supervisor."
		_out ''
		_log_tail "$run_log"
		return 1
	fi

	# TIMED OUT mutants enter neither report.json's totals (report.go:178 sums
	# lived + killed + notViable) nor the efficacy, so a run that expired 15 of
	# them reports a perfect score for 15 mutants nobody tested. The count comes
	# from the aggregate footer because a per-line count dies with a truncated
	# log, and a number derived from partial text looks right while being wrong.
	local timed_out timed_out_lines
	timed_out=$(sed -n 's/^Timed out: \([0-9][0-9]*\),.*/\1/p' "$run_log" | tail -1)
	timed_out=${timed_out:-0}
	timed_out_lines=$(grep -cE '^[[:space:]]*TIMED OUT [^ ]+ at [^[:space:]]+:[[:digit:]]+:[[:digit:]]+' "$run_log" || true)
	if [[ $timed_out -ne $timed_out_lines ]]; then
		_out "- **no measurement**: the log claims $timed_out timed-out mutants but carries $timed_out_lines timed-out lines."
		_out "- One of the two counts is truncated or partial, so neither can be trusted and the run cannot be judged."
		return 1
	fi
	if [[ $timed_out -gt 0 ]]; then
		_out "- **no measurement**: $timed_out mutants expired on the per-mutant timeout and were never tested."
		_out "- The report does not count them, so its efficacy covers fewer mutants than the run generated."
		_out "- Raise the per-mutant cap or lower the worker count: contention, not slowness, is what expires them."
		return 1
	fi

	if [[ ! -f $report ]]; then
		# A missing report is "nothing to mutate" ONLY when the run said it had
		# nothing to report AND the pre-count agrees. Deriving it from the
		# absence alone would move the lie instead of fixing it.
		if [[ $expected -eq 0 ]] && grep -qF 'No results to report.' "$run_log"; then
			_out "- **nothing to mutate**: the diff has Go files but no mutable statements, so zero mutants were generated."
			_out "- **no mutation was measured, and that is a result, not a failure.**"
			_out "- Source of this verdict: the pre-count of expected mutants was 0, not the missing report."
			_budget_lines "$budget"
			return 0
		fi
		_out "- **no measurement**: no report was produced, so there is nothing to judge."
		_out "- The pre-count expected $expected mutant(s), so the scope was not empty of mutable code."
		_out ''
		_log_tail "$run_log"
		return 1
	fi

	# Parsed before it is read, because an extractor that fails leaves an empty
	# output and an empty output read as "zero new survivors" is the
	# green-without-measurement this check forbids. Presence is not parseability.
	jq -e . "$report" >/dev/null 2>&1 || {
		_out "- **no measurement**: $report exists but cannot be parsed, so it cannot be judged."
		_out "- A failing extractor leaves empty output, and empty output read as 'zero new survivors' is the green this check exists to refuse."
		_out ''
		_log_tail "$run_log"
		return 1
	}

	_out '- measured:'
	_out "  - total: $(jq -r '.mutants_total' "$report")"
	_out "  - killed: $(jq -r '.mutants_killed' "$report")"
	_out "  - survived: $(jq -r '.mutants_lived' "$report")"
	_out "  - not viable: $(jq -r '.mutants_not_viable' "$report")"
	_out "  - not covered: $(jq -r '.mutants_not_covered' "$report")"
	_out "  - test efficacy: $(jq -r '.test_efficacy' "$report")%"
	_out "  - mutator coverage: $(jq -r '.mutations_coverage' "$report")%"

	# A verdict about a set different from the one that was announced is a verdict
	# about something else. The measured set comes from report.json counting the
	# mutations the engine actually REPORTED, and SKIPPED ones are excluded: with
	# --diff the engine walks the whole module, emits a SKIPPED line for all 1090
	# mutants outside the diff and counts none of them, so a set taken from
	# progress lines or from files[] is the whole module either way.
	if [[ -n $announced && -f $announced ]]; then
		unannounced=$(jq -r '.files[] | .file_name as $f | .mutations[] | select(.status != "SKIPPED") | $f' "$report" |
			sort -u | comm -23 - <(grep -vE '^[[:space:]]*$' "$announced" | sort -u))
		if [[ -n $unannounced ]]; then
			_out '- **no measurement**: the run reported mutants for files outside the scope that was announced:'
			local f
			while IFS= read -r f; do [[ -n $f ]] && _out "  - $f"; done <<<"$unannounced"
			return 1
		fi
	fi

	# A pre-count of 0 against a report full of mutants means the numerator and
	# the denominator described different runs, and the supervisor was given the
	# wrong one: the total it was told to watch for was never reachable.
	if [[ $expected -eq 0 ]] && [[ $(jq -r '.mutants_total' "$report") != 0 ]]; then
		_out "- **no measurement**: the pre-count expected 0 mutants but the run measured $(jq -r '.mutants_total' "$report")."
		_out "- The denominator and the numerator came out of different runs, so the verdict would be about a scope that was never the one announced."
		return 1
	fi

	tmp=$(mktemp -d "${TMPDIR:-/tmp}/gitdash-mutate-verdict.XXXXXX") || return 1
	jq -r '.files[] | .file_name as $f | .mutations[] | select(.status=="LIVED") | "\(.type) \($f):\(.line)"' "$report" |
		sort -u >"$tmp/lived.txt"
	total=$(wc -l <"$tmp/lived.txt" | tr -d ' ')

	# Compared BY LINE, not by substring: an entry naming the same file at a
	# different line does not cover the mutant that appeared.
	grep -vE '^[[:space:]]*(#|$)' "$allowlist" | sed 's/[[:space:]]*$//' | sort -u >"$tmp/allow.txt"
	comm -23 "$tmp/lived.txt" "$tmp/allow.txt" >"$tmp/new.txt"
	new=$(wc -l <"$tmp/new.txt" | tr -d ' ')

	_out "- surviving in the measured scope: $total (allowlisted: $((total - new)), new: $new)"
	_budget_lines "$budget"

	if [[ $new -gt 0 ]]; then
		_out ''
		_out "- **$new surviving mutant(s) are not in $allowlist**:"
		local survivor
		while IFS= read -r survivor; do _out "  - $survivor"; done <"$tmp/new.txt"
		_out ''
		_out "Add a test that kills them, or — only if provably equivalent — add the line to $allowlist with a comment saying why."
		rm -rf "$tmp"
		return 1
	fi
	rm -rf "$tmp"

	if [[ $expected -eq 0 ]]; then
		_out '- **nothing to mutate**: the pre-count found no mutants, so no mutation was measured.'
	else
		_out '- **measured and clean**: every surviving mutant is allowlisted, so this change introduced no new gap.'
	fi
	return 0
}

_log_tail() { # _log_tail <run_log>
	_out 'Tail of the run log:'
	_out '```'
	local line
	while IFS= read -r line; do _out "  $line"; done < <(tail -n 25 "$1")
	_out '```'
}

# The budget the run used, on every verdict: these numbers only mean something
# relative to the job ceiling, and that coupling is invisible from the workflow.
_budget_lines() { # _budget_lines <budget_file>
	local line
	[[ -n ${1:-} && -f $1 ]] || return 0
	_out '- budget this run was measured with:'
	while IFS= read -r line; do _out "  - $line"; done <"$1"
}

# ============================================================================
# RUN — warm, enumerate, supervise.
# ============================================================================

REPORT=${MUTATE_REPORT:-report.json}
RUN_LOG=${MUTATE_RUN_LOG:-.mutation-run.log}
SCOPE_FILE=${MUTATE_SCOPE_FILE:-.mutation-scope.txt}
BUDGET_FILE=${MUTATE_BUDGET_FILE:-.mutation-budget.txt}
ALLOWLIST=${MUTATE_ALLOWLIST:-.mutation-allowlist}
ENGINE=${MUTATE_ENGINE:-go tool gremlins unleash}
WATCHDOG=${MUTATE_WATCHDOG:-scripts/watchdog.sh}
EXCLUDE=${MUTATE_EXCLUDE:-'(\.worktrees/|internal/testutil/)'}
COVERPKG=${MUTATE_COVERPKG:-./...}
BASE=${MUTATE_BASE:-main}

MODE=run
CI=0
DRY=0
VERDICT_ONLY=0
POSITIONAL=()
EXPECTED_TOTAL=${MUTATE_EXPECTED_TOTAL:-}
ENGINE_RC=${MUTATE_ENGINE_RC:-}
ANNOUNCED=${MUTATE_ANNOUNCED:-}
BUDGET=${MUTATE_BUDGET:-}
SUMMARY=${MUTATE_SUMMARY:-}

while [[ $# -gt 0 ]]; do
	case $1 in
	--diff)
		MODE=diff
		;;
	--run)
		MODE=run
		;;
	--ci)
		CI=1
		;;
	--dry)
		DRY=1
		;;
	--verdict-only)
		VERDICT_ONLY=1
		;;
	--expected-total)
		EXPECTED_TOTAL=${2:-}
		shift
		;;
	--engine-rc)
		ENGINE_RC=${2:-}
		shift
		;;
	--announced)
		ANNOUNCED=${2:-}
		shift
		;;
	--budget)
		BUDGET=${2:-}
		shift
		;;
	--summary)
		SUMMARY=${2:-}
		shift
		;;
	-h | --help)
		sed -n '3,24p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	-*)
		die 2 "unknown flag '$1' (try --help)"
		;;
	*)
		POSITIONAL+=("$1")
		;;
	esac
	shift
done

# --- verdict-only: the decision half, with no measurement machinery at all ----
if [[ $VERDICT_ONLY -eq 1 ]]; then
	[[ ${#POSITIONAL[@]} -eq 3 ]] ||
		die 2 "--verdict-only needs exactly three paths: <report> <run-log> <allowlist>"
	[[ -n $EXPECTED_TOTAL ]] || die 2 "--verdict-only needs --expected-total"
	[[ -n $ENGINE_RC ]] || die 2 "--verdict-only needs --engine-rc"
	[[ $EXPECTED_TOTAL =~ ^[0-9]+$ ]] || die 2 "--expected-total must be a non-negative integer"
	[[ $ENGINE_RC =~ ^[0-9]+$ ]] || die 2 "--engine-rc must be a non-negative integer"
	# The run log arrives as a positional argument, never by convention: a path
	# that resolved to nothing and read as "no log, so no problems" is the very
	# failure this gate exists to close.
	[[ -n ${POSITIONAL[1]} ]] || die 2 "--verdict-only was given an empty run-log path"

	_verdict "${POSITIONAL[0]}" "${POSITIONAL[1]}" "${POSITIONAL[2]}" \
		"$EXPECTED_TOTAL" "$ENGINE_RC" "$ANNOUNCED" "$BUDGET"
	rc=$?
	_flush_summary "$SUMMARY" 'Mutation testing'
	exit "$rc"
fi

# --- budget table -----------------------------------------------------------
case $MODE in
run)
	CAP_DEFAULT=180s WORKERS_DEFAULT=8 STALL_DEFAULT=20m CEILING_DEFAULT=60m
	;;
diff)
	CAP_DEFAULT=120s WORKERS_DEFAULT=4 STALL_DEFAULT=2m CEILING_DEFAULT=4m
	;;
esac

for knob in CAP WORKERS STALL CEILING JOB_CEILING; do
	var=MUTATE_$knob
	if [[ $CI -eq 1 && -z ${!var:-} ]]; then
		die 2 "--ci requires $var: the budget is mandatory here so that a missing one cannot fall back to the local row"
	fi
done

CAP=${MUTATE_CAP:-$CAP_DEFAULT}
WORKERS=${MUTATE_WORKERS:-$WORKERS_DEFAULT}
STALL=${MUTATE_STALL:-$STALL_DEFAULT}
CEILING=${MUTATE_CEILING:-$CEILING_DEFAULT}
JOB_CEILING=${MUTATE_JOB_CEILING:-}

CAP_SECS=$(_to_secs "$CAP")
CEILING_SECS=$(_to_secs "$CEILING")
JOB_CEILING_SECS=$([[ -n $JOB_CEILING ]] && _to_secs "$JOB_CEILING" || echo 0)
STALL_SECS=$(_to_secs "$STALL")

# Invariant assertions instead of magic numbers. What has to hold is that the
# supervisor dies before the platform does, and that one mutant cannot outlive the
# run it belongs to. A typo in any limit is caught here rather than by a job that
# the platform cancels with no reason, no report and no log.
[[ $STALL_SECS -gt 0 ]] || die 2 "the stall limit must be positive, got '$STALL'"
[[ $STALL_SECS -lt $CEILING_SECS ]] || die 2 "the stall limit ($STALL) must be below the run ceiling ($CEILING)"
[[ $CAP_SECS -le $CEILING_SECS ]] || die 2 "the per-mutant cap ($CAP) must not exceed the run ceiling ($CEILING)"
if [[ $JOB_CEILING_SECS -gt 0 ]]; then
	[[ $CEILING_SECS -lt $JOB_CEILING_SECS ]] ||
		die 2 "the run ceiling ($CEILING) must be below the job ceiling ($JOB_CEILING), or the platform cancels the job before the supervisor can say why"
fi

# --- forbidden flags --------------------------------------------------------
# Rejected before anything runs, so a forbidden flag is a refusal and not a
# measurement. Both spellings matter: the flag alone or with =value, and the
# grouped shorthand (-Sk) that slips through a loop matching whole flags.
reject_forbidden() { # reject_forbidden <flag>...
	local f
	for f in "$@"; do
		case $f in
		-S | --output-statuses | -s | --silent | -S=* | --output-statuses=* | -s=* | --silent=*)
			die 2 "'$f' breaks the one-line-per-mutant contract and would make the denominator and this gate meaningless (see MUTATE_FORBIDDEN)"
			;;
		-[!-]?*)
			# A grouped shorthand: bash cannot derive a character class from a
			# list, so s and S are spelled out.
			case ${f#-} in
			*s* | *S*) die 2 "'$f' carries a forbidden shorthand grouped with other flags (see MUTATE_FORBIDDEN)" ;;
			esac
			;;
		esac
	done
}

ENGINE_FLAGS=(--exclude-files "$EXCLUDE" --coverpkg "$COVERPKG")
EXTRA_FLAGS=()
if [[ -n ${MUTATE_EXTRA_FLAGS:-} ]]; then
	# shellcheck disable=SC2206
	EXTRA_FLAGS=($MUTATE_EXTRA_FLAGS)
fi
reject_forbidden "${ENGINE_FLAGS[@]}" ${EXTRA_FLAGS[@]+"${EXTRA_FLAGS[@]}"}

# --- scope precheck ---------------------------------------------------------
# gremlins silently falls back to the whole module when the diff is empty, so a
# PR with no Go code would launch a ten-minute full-module measurement inside a
# four-minute job and get cut by the platform. The scope is the COMMITTED diff,
# not the index: a file that is only staged is invisible here and invisible to
# the check, which is what makes a staged-only fixture prove nothing.
SCOPE_ARGS=()
if [[ $MODE == diff ]]; then
	# ONE string, verified and then diffed. The two used to differ: the check
	# resolved `origin/$BASE` while the diff asked for `$BASE`, and on a runner
	# there is no local `main`, so the diff failed and its empty output was read
	# as "nothing to mutate". A green from an unresolvable base is the exact hole
	# this check exists to close.
	if ! git rev-parse --verify --quiet "$BASE" >/dev/null 2>&1; then
		_out "- **no measurement**: the base ref \`$BASE\` could not be resolved, so the set of files to mutate is unknown."
		_out "- An empty diff from an unresolved base is indistinguishable from a PR with no changes, and treating them alike is the hole this check closes."
		_out "- Pass the SAME ref you fetched, e.g. \`MUTATE_BASE=origin/main\` on a runner where the base is only a remote-tracking ref."
		_flush_summary "$SUMMARY" 'Mutation testing'
		die 1 "base ref $BASE could not be resolved"
	fi
	SCOPE_ARGS=(--diff "$BASE")
	# A diff that cannot be computed is an ERROR, not an empty scope: `git diff`
	# printing nothing on failure is the same bytes as a diff with no Go files.
	if ! git diff --name-only "$BASE...HEAD" >"$SCOPE_FILE.all"; then
		rm -f "$SCOPE_FILE.all"
		_out "- **no measurement**: the diff against \`$BASE\` could not be computed, so the scope is unknown."
		_out "- A failed \`git diff\` prints nothing, and nothing is indistinguishable from 'this PR touches no Go files'."
		_flush_summary "$SUMMARY" 'Mutation testing'
		die 1 "git diff $BASE...HEAD failed"
	fi
	grep '\.go$' "$SCOPE_FILE.all" >"$SCOPE_FILE" || true
	rm -f "$SCOPE_FILE.all"
else
	git ls-files '*.go' | grep -vE "$EXCLUDE" >"$SCOPE_FILE" || true
fi
ANNOUNCED_COUNT=$(grep -cvE '^[[:space:]]*$' "$SCOPE_FILE" || true)

# Published before any early exit, so the artifact upload knows the paths even
# when the run stops at the precheck. A red with no log attached is a red nobody
# can act on.
if [[ -n ${GITHUB_OUTPUT:-} ]]; then
	{
		printf 'report=%s\n' "$REPORT"
		printf 'run_log=%s\n' "$RUN_LOG"
		printf 'scope=%s\n' "$MODE"
		printf 'expected_total=%s\n' 0
	} >>"$GITHUB_OUTPUT"
fi

write_budget_file() {
	[[ -n $BUDGET_FILE ]] || return 0
	{
		printf 'scope: %s\n' "$MODE"
		printf 'per-mutant cap: %s\n' "$CAP"
		printf 'workers: %s\n' "$WORKERS"
		printf 'stall limit: %s\n' "$STALL"
		printf 'run ceiling: %s\n' "$CEILING"
		printf 'job ceiling: %s\n' "${JOB_CEILING:-unset}"
		printf 'files in scope: %s\n' "$ANNOUNCED_COUNT"
	} >"$BUDGET_FILE"
}
write_budget_file

# A PR with no Go files at all is a green that says out loud that nothing was
# measured. It is decided by the PRE-CHECK and never rederived from a missing
# report: "the diff has nothing to mutate" deduced from "no report" is the same
# lie in a different place.
if [[ $ANNOUNCED_COUNT -eq 0 ]]; then
	_out "- **nothing to mutate**: the scope against \`$BASE\` contains no .go files, so no mutation run was launched."
	_out "- **no mutation was measured.** This is a result, not a failure."
	_out "- Source of this verdict: the scope precheck found 0 files, not an absent report."
	_budget_lines "$BUDGET_FILE"
	_flush_summary "$SUMMARY" 'Mutation testing'
	exit 0
fi

# --- prerequisites ----------------------------------------------------------
command -v jq >/dev/null 2>&1 ||
	die 2 "jq is required to read the report and compare it against $ALLOWLIST"
[[ -x $WATCHDOG ]] ||
	die 2 "the supervisor is missing at $WATCHDOG (it is vendored in this repo; chezmoi is not involved)"
[[ -f $ALLOWLIST ]] ||
	die 2 "no $ALLOWLIST, so the gate has nothing to compare survivors against; seed it with: make mutate-all"

# A report from an earlier run must never be read as this run's. The working tree
# survives between local runs, so this is not hypothetical.
rm -f "$REPORT" "$RUN_LOG"

# --- warm -------------------------------------------------------------------
# Cache primer only, and deliberately NOT the source of the denominator: -run '^$'
# builds the instrumented packages and runs zero suites, so its duration is build
# time, and ceil(cap/build_time) is a cap nobody agreed to. The dry-run below is
# where elapsed is measured, because it runs the same coverage pass before the
# engine exists. go already parallelises the build, so this is not parallelised.
echo "mutate: warming the build cache…" >&2
go test -coverpkg "$COVERPKG" -run '^$' ./... >/dev/null 2>&1 || true

# --- enumerate and measure elapsed, in ONE dry-run ---------------------------
DRY_OUT=$(mktemp "${TMPDIR:-/tmp}/gitdash-mutate-dry.XXXXXX") || die 2 "cannot create the dry-run scratch file"
# shellcheck disable=SC2064
trap "rm -f '$DRY_OUT'" EXIT

$ENGINE "${SCOPE_ARGS[@]}" --dry-run "${ENGINE_FLAGS[@]}" ${EXTRA_FLAGS[@]+"${EXTRA_FLAGS[@]}"} >"$DRY_OUT" 2>&1
DRY_RC=$?

# The footer says "done in 8.858216525s" (coverage.go, log.Infof over a
# time.Duration) and a duration prints as "8,858216525s" under a Spanish locale,
# so only the digits are taken and the decimal comma is normalised.
cov_secs=$(sed -n 's/.*done in \([0-9,.]*\)s.*/\1/p' "$DRY_OUT" | tail -1 | tr ',' '.')
if [[ -z $cov_secs ]] || ! awk -v c="$cov_secs" 'BEGIN{exit !(c+0 > 0)}'; then
	tail -n 25 "$DRY_OUT" >&2
	die 2 "the dry-run reported no measurable coverage time, so the per-mutant cap cannot be derived (engine exit $DRY_RC)"
fi

# Counted with the same expression the supervisor counts with: wc -l on that log
# would also count "Starting...", "done in ..." and the footer, which would make
# the supervisor declare a healthy run finished early.
TOTAL=$(grep -cE "$PROGRESS_RE" "$DRY_OUT" || true)

# ceil(cap / elapsed) as an integer, because an absolute per-mutant timeout does
# not exist in gremlins: this multiplier over the coverage duration is the only
# lever there is.
COEF=$(awk -v cap="$CAP_SECS" -v el="$cov_secs" 'BEGIN{printf "%d", (cap/el==int(cap/el)) ? cap/el : int(cap/el)+1}')
PER_MUTANT=$(awk -v el="$cov_secs" -v k="$COEF" 'BEGIN{printf "%d", el*k+2}')

echo "mutate: coverage ${cov_secs}s -> coefficient $COEF (per-mutant ceiling ~${PER_MUTANT}s, cap $CAP)" >&2
echo "mutate: $WORKERS workers, $TOTAL mutants expected, $ANNOUNCED_COUNT files in scope" >&2

if [[ $DRY -eq 1 ]]; then
	echo "mutate: --dry: warmed and enumerated, nothing mutated" >&2
	exit 0
fi

# --- supervise --------------------------------------------------------------
# No exec: this script always has a verdict left to write, and exec would hand
# the process to the supervisor and take with it the ability to tell 124 from a
# failed mutation. Dropping exec does not weaken the group kill — being the
# direct parent and giving the engine its own group are properties of
# watchdog.sh, which creates its own pgid and refuses to run if it could not.
echo "mutate: supervising (stall $STALL, ceiling $CEILING, $TOTAL expected progress lines)" >&2
"$WATCHDOG" "$STALL" "$CEILING" "$TOTAL" "$PROGRESS_RE" -- \
	$ENGINE "${SCOPE_ARGS[@]}" "${ENGINE_FLAGS[@]}" \
	--workers "$WORKERS" --timeout-coefficient "$COEF" --output "$REPORT" --silent \
	${EXTRA_FLAGS[@]+"${EXTRA_FLAGS[@]}"} |
	tee "$RUN_LOG"

# Taken immediately, because under pipefail a pipeline's status is "the last
# non-zero": a tee that failed on ENOSPC would read as a failed mutation run.
ENGINE_RC=${PIPESTATUS[0]}

echo "mutate: engine exit $ENGINE_RC, log $RUN_LOG, report $REPORT" >&2

# One path for producer, verdict and upload: they cannot disagree if they are
# told the same one. The scope step already published these; only the count is
# final now.
if [[ -n ${GITHUB_OUTPUT:-} ]]; then
	printf 'expected_total=%s\n' "$TOTAL" >>"$GITHUB_OUTPUT"
fi

_verdict "$REPORT" "$RUN_LOG" "$ALLOWLIST" "$TOTAL" "$ENGINE_RC" "$SCOPE_FILE" "$BUDGET_FILE"
rc=$?
_flush_summary "$SUMMARY" 'Mutation testing'
exit "$rc"