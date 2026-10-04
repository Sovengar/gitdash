#!/usr/bin/env bash
# mutate_test.sh — the suite for scripts/mutate.sh, and the only harness in this
# repo that can reach every red path of the mutation gate.
#
# The reason it can is the same reason the script is split in two: the verdict is
# a pure function of paths, so report.json, the run log and .mutation-allowlist
# are fabricated in a temp dir and the verdict is asked about them. No gremlins,
# no Go build, no git, no clock. Exercising those reds inside the workflow would
# need a synthetic Actions run, which is a hope, not a test.
#
# The repo's declared trap ("a new gate is not tested until it has actually
# passed") is why these are plain assertions on real invocations rather than a
# table of expected strings.
#
# Run with: bash scripts/mutate_test.sh   (exits non-zero if any case fails)

set -uo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
REPO=$(dirname "$HERE")
MUTATE="$HERE/mutate.sh"

pass=0
fail=0
ok() {
	pass=$((pass + 1))
	printf 'PASS  %s\n' "$1"
}
bad() {
	fail=$((fail + 1))
	printf 'FAIL  %s\n' "$1"
}
check() { # check <label> <expected> <actual>
	if [[ $2 == "$3" ]]; then ok "$1"; else bad "$1 (expected [$2] got [$3])"; fi
}
contains() { # contains <label> <needle> <haystack>
	if [[ $3 == *"$2"* ]]; then ok "$1"; else bad "$1 (missing [$2])"; fi
}
lacks() { # lacks <label> <needle> <haystack>
	if [[ $3 != *"$2"* ]]; then ok "$1"; else bad "$1 (unexpected [$2])"; fi
}

[[ -f $MUTATE ]] || {
	echo "no $MUTATE"
	exit 1
}

tmp=$(mktemp -d "${TMPDIR:-/tmp}/gitdash-mutate-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

# --- fixtures ---------------------------------------------------------------

# A report whose only survivors are the given "<TYPE> <file>:<line>" entries, and
# whose killed count is whatever makes the stated total add up. Written by hand
# rather than through jq so the JSON is exactly what a real report looks like.
make_report() { # make_report <path> <total> <lived_entries_csv...>
	local path=$1 total=$2
	shift 2
	local lived_files='' killed=$((total - $#))
	local type file line entry
	# Over "$@", not over $*: each entry carries a space of its own.
	for entry in "$@"; do
		type=${entry%% *}
		file=${entry#* }
		line=${file##*:}
		file=${file%:*}
		lived_files+="{\"file_name\":\"$file\",\"mutations\":[{\"type\":\"$type\",\"line\":$line,\"column\":9,\"status\":\"LIVED\"}]},"
	done
	printf '{"go_module":"gitdash","mutants_total":%d,"mutants_killed":%d,"mutants_lived":%d,' \
		"$total" "$killed" "$#" >"$path"
	local efficacy
	efficacy=$(awk -v k="$killed" -v t="$total" 'BEGIN{printf "%.2f", t?100*k/t:0}')
	printf '"mutants_not_viable":0,"mutants_not_covered":0,"test_efficacy":%s,"mutations_coverage":%s,' \
		"$efficacy" "$efficacy" >>"$path"
	printf '"elapsed_time":30.5,"mutator_statistics":{},"files":[%s]}' "${lived_files%,}" >>"$path"
}

# A run log that looks like the stdout of a finished gremlins run: the aggregate
# footer, the timed-out total, and one line per mutant in the real format
# ("%s%s %s at %s" with the status padded to twelve columns).
make_log() { # make_log <path> <killed> <timed_out> [extra_line...]
	local path=$1 killed=$2 timed=$3
	shift 3
	{
		printf 'Starting...\n'
		printf 'Gathering coverage...\n'
		printf 'done in 2.1s\n'
		local i
		for ((i = 1; i <= killed; i++)); do
			printf '  %s%s at pkg/f%d.go:%d:9\n' ' ' KILLED "$i" "$i"
		done
		[[ $timed -gt 0 ]] && for ((i = 1; i <= timed; i++)); do
			printf '   TIMED OUT CONDITIONALS_BOUNDARY at pkg/t%d.go:%d:9\n' "i" "i"
		done
		printf '\n'
		printf 'Mutation testing completed in 30s\n'
		printf 'Killed: %d, Lived: 0, Not covered: 0\n' "$killed"
		printf 'Timed out: %d, Not viable: 0, Skipped: 0\n' "$timed"
		printf 'Test efficacy: 100.00%%\n'
		printf 'Mutator coverage: 100.00%%\n'
		local extra
		for extra in "$@"; do printf '%s\n' "$extra"; done
	} >"$path"
}

# The verdict, invoked the way the workflow invokes it: three paths plus the two
# numbers only the run phase knows.
verdict() { # verdict <case_dir> <expected_total> <engine_rc> [extra flags...]
	local d=$1 total=$2 rc=$3
	shift 3
	local out rc_out
	out=$("$MUTATE" --verdict-only "$d/report.json" "$d/run.log" "$d/allowlist" \
		--expected-total "$total" --engine-rc "$rc" "$@" 2>&1)
	rc_out=$?
	printf '%s' "$out" >"$d/verdict.out"
	return "$rc_out"
}

new_case() { # new_case <name> -> echoes the case dir
	local d="$tmp/$1"
	mkdir -p "$d"
	printf '# allowlist fixture\n' >"$d/allowlist"
	printf '%s' "$d"
}

# ============================================================================
echo "=== 1. survivors that are not allowlisted block the merge ==="
# ============================================================================

d=$(new_case new-survivors)
make_report "$d/report.json" 46 'CONDITIONALS_BOUNDARY internal/tui/table.go:292' 'CONDITIONALS_NEGATION internal/tui/app.go:380'
make_log "$d/run.log" 44 0
verdict "$d" 46 0
check "new survivors: red" 1 $?
out=$(cat "$d/verdict.out")
contains "new survivors: lists the first one" "CONDITIONALS_BOUNDARY internal/tui/table.go:292" "$out"
contains "new survivors: lists the second one" "CONDITIONALS_NEGATION internal/tui/app.go:380" "$out"
contains "new survivors: publishes the counts" "allowlisted: 0, new: 2" "$out"
contains "new survivors: publishes the figures" "total: 46" "$out"
contains "new survivors: says how to resolve it" "Add a test that kills them" "$out"

# ============================================================================
echo
echo "=== 2. the allowlist is compared by line, not by substring ==="
# ============================================================================

d=$(new_case allowlist-by-line)
make_report "$d/report.json" 46 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
make_log "$d/run.log" 45 0
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:295\n' >>"$d/allowlist"
verdict "$d" 46 0
check "same file, other line: red" 1 $?
contains "same file, other line: names the survivor" "CONDITIONALS_BOUNDARY internal/tui/table.go:292" "$(cat "$d/verdict.out")"

# The same entry at the right line is the other half of the same scenario.
d=$(new_case allowlist-same-line)
make_report "$d/report.json" 46 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
make_log "$d/run.log" 45 0
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\n' >>"$d/allowlist"
verdict "$d" 46 0
check "same file, same line: green" 0 $?
contains "same file, same line: counts it as allowlisted" "allowlisted: 1, new: 0" "$(cat "$d/verdict.out")"

# ============================================================================
echo
echo "=== 3. known survivors only: green, with the numbers ==="
# ============================================================================

d=$(new_case all-known)
make_report "$d/report.json" 50 'CONDITIONALS_BOUNDARY internal/tui/table.go:292' 'ARITHMETIC_BASE internal/forge/parse.go:45' 'CONDITIONALS_BOUNDARY internal/tui/toast.go:75'
make_log "$d/run.log" 47 0
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\nARITHMETIC_BASE internal/forge/parse.go:45\n' >>"$d/allowlist"
printf 'CONDITIONALS_BOUNDARY internal/tui/toast.go:75\n' >>"$d/allowlist"
verdict "$d" 50 0
check "all known: green" 0 $?
out=$(cat "$d/verdict.out")
contains "all known: survivor count" "surviving in the measured scope: 3" "$out"
contains "all known: allowlisted count" "allowlisted: 3, new: 0" "$out"
contains "all known: killed count" "killed: 47" "$out"
contains "all known: calls it measured and clean" "measured and clean" "$out"

# ============================================================================
echo
echo "=== 4. nothing to mutate is a verdict of its own ==="
# ============================================================================

# A diff with Go files but zero mutable statements: no report, and the engine says
# it had nothing to report. Green, and explicitly not a failed measurement.
d=$(new_case no-mutants)
make_log "$d/run.log" 0 0 'No results to report.'
verdict "$d" 0 0
check "zero mutants: green" 0 $?
out=$(cat "$d/verdict.out")
contains "zero mutants: says nothing to mutate" "nothing to mutate" "$out"
contains "zero mutants: records that nothing was measured" "no mutation was measured" "$out"
lacks "zero mutants: does not claim a failure" "no measurement" "$out"

# The same state with a pre-count that disagrees is the third red, not a green:
# "nothing to mutate" may never be derived from an absent report.
d=$(new_case no-mutants-disagrees)
make_log "$d/run.log" 0 0 'No results to report.'
verdict "$d" 12 0
check "zero mutants but pre-count says 12: red" 1 $?
contains "zero mutants but pre-count says 12: names the count" "expected 12" "$(cat "$d/verdict.out")"

# A report full of mutants against a pre-count of zero means the denominator came
# from a different run than the numerator, so the verdict would be about a scope
# that was never the one announced.
d=$(new_case precount-zero-with-mutants)
make_report "$d/report.json" 8 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
make_log "$d/run.log" 7 0
verdict "$d" 0 0
check "pre-count 0 with 8 measured: red" 1 $?
contains "pre-count 0 with 8 measured: names the contradiction" "measured 8" "$(cat "$d/verdict.out")"

# ============================================================================
echo
echo "=== 5. a PR with no Go files never launches a measurement ==="
# ============================================================================

# The scope precheck runs before the engine, so this is a real git repo with a
# real committed diff and a stub engine that would leave a marker if it ran.
gr=$tmp/repo-no-go
mkdir -p "$gr"
git -C "$gr" init -q -b main
git -C "$gr" config user.email t@example.com
git -C "$gr" config user.name t
printf 'name = "x"\n' >"$gr/config.toml"
mkdir -p "$gr/scripts"
printf 'x\n' >"$gr/Makefile"
printf 'y\n' >"$gr/CONTRIBUTING.md"
cp "$MUTATE" "$gr/scripts/mutate.sh"
chmod +x "$gr/scripts/mutate.sh"
printf '# allowlist\n' >"$gr/.mutation-allowlist"
printf '#!/usr/bin/env bash\necho RAN > "$PWD/engine-ran"\n' >"$gr/scripts/watchdog.sh"
chmod +x "$gr/scripts/watchdog.sh"
git -C "$gr" add -A
git -C "$gr" commit -qm base
git -C "$gr" checkout -qb feature
printf 'z\n' >>"$gr/Makefile"
printf 'w\n' >"$gr/scripts/new.sh"
git -C "$gr" add -A
git -C "$gr" commit -qm change

out=$(cd "$gr" && MUTATE_ENGINE='bash -c "echo RAN > \"$PWD/engine-ran\""' \
	MUTATE_RUN_LOG=run.log MUTATE_SCOPE_FILE=scope.txt \
	MUTATE_BUDGET_FILE=budget.txt MUTATE_SUMMARY=summary.md \
	bash scripts/mutate.sh --diff 2>&1)
rc=$?
check "no .go in the diff: green" 0 "$rc"
contains "no .go in the diff: says nothing to mutate" "nothing to mutate" "$out"
contains "no .go in the diff: records that nothing was measured" "no mutation was measured" "$out"
contains "no .go in the diff: says the scope is the source" "scope precheck found 0 files" "$out"
lacks "no .go in the diff: does not claim a failed measurement" "no measurement" "$out"
if [[ -f $gr/engine-ran ]]; then
	bad "no .go in the diff: the engine was launched anyway"
else
	ok "no .go in the diff: no engine ran"
fi
if [[ -f $gr/run.log ]]; then
	bad "no .go in the diff: a run log was written"
else
	ok "no .go in the diff: no run log written"
fi

# A second repo, this one WITH a Go file in scope, so the flag guard and the
# budget assertions reach the run phase instead of being answered by the precheck.
gr2=$tmp/repo-stale
mkdir -p "$gr2/scripts"
cp "$MUTATE" "$gr2/scripts/mutate.sh"
chmod +x "$gr2/scripts/mutate.sh"
printf 'package x\n' >"$gr2/x.go"
printf '#!/usr/bin/env bash\nexit 0\n' >"$gr2/scripts/watchdog.sh"
chmod +x "$gr2/scripts/watchdog.sh"
printf '# allowlist\n' >"$gr2/.mutation-allowlist"
git -C "$gr2" init -q -b main >/dev/null 2>&1
git -C "$gr2" config user.email t@example.com
git -C "$gr2" config user.name t
git -C "$gr2" add -A
git -C "$gr2" commit -qm base

# ============================================================================
echo
echo "=== 6. the three reasons for a red are told apart ==="
# ============================================================================

# (a) The measurement could not be made at all: no report with a non-zero
# pre-count.
d=$(new_case red-no-report)
make_log "$d/run.log" 0 0 'panic: something went wrong'
verdict "$d" 9 0
check "no report: red" 1 $?
out=$(cat "$d/verdict.out")
contains "no report: says there was no measurement" "no measurement" "$out"
contains "no report: includes the tail of the log" "panic: something went wrong" "$out"
contains "no report: includes the log tail fence" '```' "$out"

# (b) The run was cut in half by the supervisor, for each of its two reasons.
d=$(new_case red-stall)
make_log "$d/run.log" 3 0 'watchdog.sh: no progress for 121s -> cutting process group 4242'
verdict "$d" 20 124
check "stall cut: red" 1 $?
contains "stall cut: names the stall" "no progress for 121s" "$(cat "$d/verdict.out")"

d=$(new_case red-ceiling)
make_log "$d/run.log" 3 0 'watchdog.sh: 240s ceiling -> cutting process group 4242'
verdict "$d" 20 124
check "ceiling cut: red" 1 $?
out=$(cat "$d/verdict.out")
contains "ceiling cut: names the ceiling" "240s ceiling" "$out"
lacks "ceiling cut: not blamed on the stall" "no progress for" "$out"

d=$(new_case red-tail-ceiling)
make_log "$d/run.log" 20 0 'watchdog.sh: reached 20/20 but did not exit within the 240s ceiling'
verdict "$d" 20 124
check "post-total tail cut: red" 1 $?
contains "post-total tail cut: names the tail" "did not exit within the 240s ceiling" "$(cat "$d/verdict.out")"

# (c) The engine came out well but wrote nothing: a silent run.
d=$(new_case red-silent)
: >"$d/run.log"
make_report "$d/report.json" 4 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\n' >>"$d/allowlist"
verdict "$d" 4 0
check "empty log with exit 0: red" 1 $?
out=$(cat "$d/verdict.out")
contains "empty log with exit 0: says nothing was measured" "no measurement" "$out"
contains "empty log with exit 0: names the silent run" "silent run" "$out"

# (d) The engine's own non-zero exit, which is not a cut.
d=$(new_case red-engine-rc)
make_log "$d/run.log" 10 0 'fatal: coverage failed'
make_report "$d/report.json" 12 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\n' >>"$d/allowlist"
verdict "$d" 12 3
check "engine exit 3: red" 1 $?
contains "engine exit 3: names the exit" "exited 3" "$(cat "$d/verdict.out")"

# The cancellation codes pass through untranslated: they are not survivors.
for rc_code in 130 143; do
	d=$(new_case "cancel-$rc_code")
	make_log "$d/run.log" 10 0
	make_report "$d/report.json" 12 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
	printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\n' >>"$d/allowlist"
	verdict "$d" 12 "$rc_code"
	check "cancelled ($rc_code): red" 1 $?
	out=$(cat "$d/verdict.out")
	contains "cancelled ($rc_code): says it was cancelled" "was cancelled" "$out"
	lacks "cancelled ($rc_code): is not called a surviving mutant" "surviving mutant(s)" "$out"
done

# ============================================================================
echo
echo "=== 7. expired mutants block even with a perfect efficacy ==="
# ============================================================================

d=$(new_case timed-out)
make_report "$d/report.json" 46 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\n' >>"$d/allowlist"
make_log "$d/run.log" 43 3
verdict "$d" 46 0
check "expired mutants: red" 1 $?
out=$(cat "$d/verdict.out")
contains "expired mutants: says how many went unchecked" "3 mutants expired" "$out"
contains "expired mutants: says the report does not count them" "does not count them" "$out"

# The count is cross-checked against itself: a log truncated mid-run loses the
# aggregate footer AND keeps the per-mutant lines it already printed, and a
# number derived from partial text looks right while being wrong.
d=$(new_case timed-out-mismatch)
make_report "$d/report.json" 46 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\n' >>"$d/allowlist"
make_log "$d/run.log" 43 3
grep -v '^Timed out:' "$d/run.log" >"$d/run.log.tmp" && mv "$d/run.log.tmp" "$d/run.log"
verdict "$d" 46 0
check "expired count mismatch: red" 1 $?
out=$(cat "$d/verdict.out")
contains "expired count mismatch: reports both counts" "claims 0 timed-out" "$out"
contains "expired count mismatch: reports the other count" "carries 3 timed-out lines" "$out"

# ============================================================================
echo
echo "=== 8. the announced scope and the measured scope are the same one ==="
# ============================================================================

d=$(new_case scope-integrity)
make_report "$d/report.json" 12 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
make_log "$d/run.log" 11 0
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\n' >>"$d/allowlist"
printf 'internal/tui/table.go\n' >"$d/scope.txt"
verdict "$d" 12 0 --announced "$d/scope.txt"
check "measured inside the announced scope: green" 0 $?

d=$(new_case scope-outside)
make_report "$d/report.json" 12 'CONDITIONALS_BOUNDARY internal/config/forge.go:149'
make_log "$d/run.log" 11 0
printf 'CONDITIONALS_BOUNDARY internal/config/forge.go:149\n' >>"$d/allowlist"
printf 'internal/tui/table.go\n' >"$d/scope.txt"
verdict "$d" 12 0 --announced "$d/scope.txt"
check "measured outside the announced scope: red" 1 $?
contains "measured outside the announced scope: names the file" "internal/config/forge.go" "$(cat "$d/verdict.out")"

# ============================================================================
echo
echo "=== 9. a missing tool cannot produce a green ==="
# ============================================================================

d=$(new_case no-jq)
make_report "$d/report.json" 12 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
make_log "$d/run.log" 11 0
printf 'CONDITIONALS_BOUNDARY internal/tui/table.go:292\n' >>"$d/allowlist"
# A PATH with everything the verdict shells out to except jq: an extractor that
# is not there leaves an empty output, and an empty output read as "zero new
# survivors" is exactly the green this check forbids.
mkdir -p "$tmp/nojq-bin"
for b in bash env sed grep tail head comm sort mktemp wc tr cat rm dirname awk; do
	src=$(command -v "$b" 2>/dev/null) && ln -sf "$src" "$tmp/nojq-bin/$b"
done
before=$(cat "$d/report.json")
out=$(PATH="$tmp/nojq-bin" "$MUTATE" --verdict-only "$d/report.json" "$d/run.log" \
	"$d/allowlist" --expected-total 12 --engine-rc 0 2>&1)
rc=$?
check "jq missing: red" 1 $rc
contains "jq missing: names the tool" "jq is not available" "$out"
contains "jq missing: does not read as a clean run" "no measurement" "$out"
check "jq missing: the report is left untouched" "$before" "$(cat "$d/report.json")"

# The other half of the same failure: jq IS installed and returns garbage. An
# unparseable report must not be read as a report with zero survivors.
d=$(new_case broken-report)
printf '{"mutants_total": 12, "mutants_lived": this is not json\n' >"$d/report.json"
make_log "$d/run.log" 11 0
verdict "$d" 12 0
check "unparseable report: red" 1 $?
contains "unparseable report: says it cannot be parsed" "cannot be parsed" "$(cat "$d/verdict.out")"

# ============================================================================
echo
echo "=== 10. no allowlist: red with the command to seed it ==="
# ============================================================================

d=$(new_case no-allowlist)
make_report "$d/report.json" 12 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
make_log "$d/run.log" 11 0
out=$("$MUTATE" --verdict-only "$d/report.json" "$d/run.log" "$d/nope" \
	--expected-total 12 --engine-rc 0 2>&1)
rc=$?
check "no allowlist: red" 1 $rc
contains "no allowlist: says what is missing" "allowlist" "$out"
contains "no allowlist: says why it matters" "nothing to compare" "$out"
contains "no allowlist: says the exact command" "make mutate-all" "$out"

# ============================================================================
echo
echo "=== 11. forbidden flags are refused before anything runs ==="
# ============================================================================

# -s and -Sk are the two that actually slip through: the first suppresses every
# log line while exiting 0 with a report present, the second is the grouped
# shorthand a loop over whole flags never sees.
for forbidden in '-s' '--silent' '-S' '--output-statuses' '-Sk' '-sS' '-xs'; do
	out=$(cd "$gr2" && MUTATE_EXTRA_FLAGS="$forbidden" MUTATE_RUN_LOG=run.log \
		MUTATE_SCOPE_FILE=scope.txt MUTATE_BUDGET_FILE=budget.txt \
		bash scripts/mutate.sh --run 2>&1)
	rc=$?
	check "forbidden '$forbidden': exit 2" 2 $rc
	contains "forbidden '$forbidden': names the flag" "$forbidden" "$out"
	contains "forbidden '$forbidden': points at the contract" "MUTATE_FORBIDDEN" "$out"
done

# A legal flag must get past the same door: the guard matches the forbidden
# shorthands, not everything that starts with a dash.
out=$(cd "$gr2" && MUTATE_EXTRA_FLAGS='--verbose' MUTATE_RUN_LOG=run.log \
	MUTATE_SCOPE_FILE=scope.txt MUTATE_BUDGET_FILE=budget.txt \
	MUTATE_ENGINE=true bash scripts/mutate.sh --run 2>&1)
rc=$?
check "an unrelated long flag is not caught by the guard" 2 $rc
lacks "an unrelated long flag does not claim MUTATE_FORBIDDEN" "MUTATE_FORBIDDEN" "$out"

# ============================================================================
echo
echo "=== 12. an absent measurement is a hard error, never a silent zero ==="
# ============================================================================

# A report from an earlier run, which must not be readable as this run's.
printf '{"mutants_total":999,"mutants_lived":999}\n' >"$gr2/report.json"

# The engine is a no-op, so the dry-run yields no measurable time and the script
# has to say so instead of computing a cap out of a zero.
out=$(cd "$gr2" && MUTATE_ENGINE=true MUTATE_RUN_LOG=run.log \
	MUTATE_SCOPE_FILE=scope.txt MUTATE_BUDGET_FILE=budget.txt \
	bash scripts/mutate.sh --run 2>&1)
rc=$?
check "no measurable coverage time: exit 2" 2 $rc
contains "no measurable coverage time: says what is missing" "no measurable coverage time" "$out"
lacks "no measurable coverage time: announces no ceiling" "per-mutant ceiling" "$out"
if [[ -f $gr2/report.json ]]; then
	bad "stale report: survived the start of a new run"
else
	ok "stale report: deleted before a new run starts"
fi

# ============================================================================
echo
echo "=== 13. the budget is mandatory in CI and its invariants are asserted ==="
# ============================================================================

ci_out=$(cd "$gr2" && MUTATE_ENGINE=true MUTATE_RUN_LOG=run.log MUTATE_SCOPE_FILE=scope.txt \
	MUTATE_BUDGET_FILE=budget.txt bash scripts/mutate.sh --run --ci 2>&1)
check "--ci without a budget: exit 2" 2 $?
contains "--ci without a budget: names the missing knob" "MUTATE_CAP" "$ci_out"

ci_env=(MUTATE_CAP=120s MUTATE_WORKERS=4 MUTATE_STALL=2m MUTATE_CEILING=4m MUTATE_JOB_CEILING=5m)
ci_out=$(cd "$gr2" && env "${ci_env[@]}" MUTATE_ENGINE=true MUTATE_RUN_LOG=run.log \
	MUTATE_SCOPE_FILE=scope.txt MUTATE_BUDGET_FILE=budget.txt \
	bash scripts/mutate.sh --run --ci 2>&1)
check "--ci with a coherent budget: gets past the assertions" 2 $?
lacks "--ci with a coherent budget: no invariant complaint" "must be" "$ci_out"

# The platform must not be able to cancel the job before the supervisor explains.
for bad in 'MUTATE_CEILING=5m MUTATE_JOB_CEILING=5m' 'MUTATE_CEILING=6m MUTATE_JOB_CEILING=5m'; do
	ci_out=$(cd "$gr2" && env "${ci_env[@]}" $bad MUTATE_ENGINE=true MUTATE_RUN_LOG=run.log \
		MUTATE_SCOPE_FILE=scope.txt MUTATE_BUDGET_FILE=budget.txt \
		bash scripts/mutate.sh --run --ci 2>&1)
	check "ceiling at or above the job ceiling: exit 2" 2 $?
	contains "ceiling at or above the job ceiling: says why" "platform cancels the job" "$ci_out"
done

# A cap longer than the whole run cannot bound anything.
ci_out=$(cd "$gr2" && env "${ci_env[@]}" MUTATE_CAP=9m MUTATE_ENGINE=true MUTATE_RUN_LOG=run.log \
	MUTATE_SCOPE_FILE=scope.txt MUTATE_BUDGET_FILE=budget.txt \
	bash scripts/mutate.sh --run --ci 2>&1)
check "per-mutant cap above the run ceiling: exit 2" 2 $?
contains "per-mutant cap above the run ceiling: says why" "must not exceed the run ceiling" "$ci_out"

# A stall longer than the run is a stall limit that never fires.
ci_out=$(cd "$gr2" && env "${ci_env[@]}" MUTATE_STALL=9m MUTATE_ENGINE=true MUTATE_RUN_LOG=run.log \
	MUTATE_SCOPE_FILE=scope.txt MUTATE_BUDGET_FILE=budget.txt \
	bash scripts/mutate.sh --run --ci 2>&1)
check "stall above the run ceiling: exit 2" 2 $?
contains "stall above the run ceiling: says why" "below the run ceiling" "$ci_out"

# ============================================================================
echo
echo "=== 14. the scope is measured against the base ref, or it is not measured ==="
# ============================================================================

out=$(cd "$gr2" && MUTATE_BASE=refs/heads/does-not-exist MUTATE_ENGINE=true \
	MUTATE_RUN_LOG=run.log MUTATE_SCOPE_FILE=scope.txt MUTATE_BUDGET_FILE=budget.txt \
	MUTATE_SUMMARY=summary.md bash scripts/mutate.sh --diff 2>&1)
rc=$?
check "unresolvable base: red" 1 $rc
contains "unresolvable base: says the base could not be resolved" "could not be resolved" "$out"
contains "unresolvable base: explains why that is not an empty diff" "indistinguishable from a PR with no changes" "$out"
if [[ -f $gr2/engine-ran || -f $gr2/run.log ]]; then
	bad "unresolvable base: something was measured anyway"
else
	ok "unresolvable base: nothing was measured"
fi

# ============================================================================
echo
echo "=== 15. the summary the workflow publishes carries the reason ==="
# ============================================================================

d=$(new_case summary-file)
make_report "$d/report.json" 46 'CONDITIONALS_BOUNDARY internal/tui/table.go:292'
make_log "$d/run.log" 45 0
printf 'per-mutant cap: 120s\nrun ceiling: 4m\n' >"$d/budget.txt"
"$MUTATE" --verdict-only "$d/report.json" "$d/run.log" "$d/allowlist" \
	--expected-total 46 --engine-rc 0 --budget "$d/budget.txt" \
	--summary "$d/summary.md" >/dev/null 2>&1
summary=$(cat "$d/summary.md" 2>/dev/null)
contains "summary: has the header" "### Mutation testing" "$summary"
contains "summary: names the survivor" "CONDITIONALS_BOUNDARY internal/tui/table.go:292" "$summary"
contains "summary: publishes the budget it ran with" "budget this run was measured with" "$summary"
contains "summary: publishes the per-mutant cap" "per-mutant cap: 120s" "$summary"

echo
printf '%d/%d passed\n' "$pass" "$((pass + fail))"
[[ $fail -eq 0 ]]