# Dev shortcuts; the binary the user runs lives in $(PREFIX)/bin (default
# ~/.local/bin, see AGENTS.md), so after any change it must be reinstalled with `make install`.

SHELL := /bin/bash

PREFIX      ?= $(HOME)/.local
BINDIR      := $(PREFIX)/bin
BIN         := bin/gitdash
PKG         := ./cmd/gitdash

# Same version dbx uses; run with `go run`, no global binary.
GOLANGCI_LINT_VERSION := v2.13.2
MUTATE_BASE ?= main

.DEFAULT_GOAL := help
.PHONY: help build install uninstall fmt fmt-check vet lint test test-race check all run print fixtures smoke tidy clean mutate mutate-diff mutate-all mutate-all-diff coverage coverage-check _mutate_check _mutate_total _mutate_total_diff

help: ## Shows this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-11s\033[0m %s\n", $$1, $$2}'

build: ## Builds the binary at bin/gitdash
	go build -o $(BIN) $(PKG)

install: build ## Installs bin/gitdash into PREFIX/bin (default ~/.local/bin; mandatory after changes)
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(BIN) $(DESTDIR)$(BINDIR)/gitdash

uninstall: ## Deletes the installed binary from PREFIX/bin (default ~/.local/bin)
	rm -f $(DESTDIR)$(BINDIR)/gitdash

fmt: ## Runs gofmt over the tree
	gofmt -w .

fmt-check: ## Checks gofmt formatting without writing (fails if anything is pending)
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt pending in:"; echo "$$out"; exit 1; fi

vet: ## go vet
	go vet ./...

lint: vet fmt-check ## go vet + gofmt + golangci-lint (version pinned, always via go run)
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

test: ## Runs the test suite with -race (same class as CI)
	go test -race -count=1 ./...

test-race: test ## Alias of test: the suite already runs with -race

# Coverage is measured on the deduplicated `go test -coverprofile` output (one
# block per test binary) plus the profile of the subprocess running func main(),
# which that test cannot collect.
COVER_PROFILE ?= coverage.out
COVER_MAIN ?= .covmain/main.txt

# .covmain is deleted, not reused: it holds the instrumented binary's counters and a previous build's refer to ranges today's code no longer has, so merging them drags the total down (measured: 80.24% with 7 stale files, 99.74% clean).
# The `rm` sits in the recipe and this comment OUTSIDE: inside a recipe it is passed to the shell, which runs it as another command and breaks the target.
coverage: ## Deduplicated coverage profile + the one of main()'s subprocess
	@go test -count=1 -coverpkg ./... -coverprofile=$(COVER_PROFILE) ./... > /dev/null
	@rm -rf .covmain
	@mkdir -p .covmain
	@go build -cover -o .covmain/gitdash $(PKG)
	@XDG_CONFIG_HOME="$$(mktemp -d)" GOCOVERDIR="$$PWD/.covmain" \
		./.covmain/gitdash --print >/dev/null 2>&1 || true
	@go tool covdata textfmt -i=.covmain -o=$(COVER_MAIN)
	@echo "profiles: $(COVER_PROFILE) $(COVER_MAIN)"

coverage-check: coverage ## Gate: this change's DIFF at 100%, and the total with a floor
	@extra=""; [ -f "$(COVER_MAIN)" ] && extra="-a $$PWD/$(COVER_MAIN)"; \
		scripts/diff-coverage.sh "$(COVER_PROFILE)" "$(MUTATE_BASE)" $$extra

check: build lint test ## build + lint + test (equivalent to CI's gate)
	@echo "check OK"

all: check test-race ## check + test-race

run: ## Starts the TUI against your real config
	go run $(PKG)

print: ## One-shot table mode (--print)
	go run $(PKG) --print

fixtures: ## Regenerates testdata/playground (git repo fixtures)
	./scripts/gen-fixtures.sh

smoke: build ## Smoke tests the TUI in tmux with an isolated config and the fixtures
	@test -d testdata/playground || { echo "testdata/playground is missing — run 'make fixtures'"; exit 1; }
	@tmp="$$(mktemp -d)"; mkdir -p "$$tmp/gitdash"; \
	printf 'roots = ["%s/testdata/playground"]\n' "$(CURDIR)" > "$$tmp/gitdash/config.toml"; \
	tmux kill-session -t gitdash-smoke 2>/dev/null || true; \
	tmux new-session -d -s gitdash-smoke \
		"XDG_CONFIG_HOME=$$tmp XDG_STATE_HOME=$$tmp/state XDG_CACHE_HOME=$$tmp/cache $(CURDIR)/$(BIN)"; \
	sleep 3; tmux capture-pane -t gitdash-smoke -p; \
	tmux kill-session -t gitdash-smoke 2>/dev/null || true; \
	rm -rf "$$tmp"

tidy: ## go mod tidy
	go mod tidy

# MUTATE_EXCLUDE keeps out what is NOT module code: without it gremlins walks .worktrees/ (Worktrunk) and measures a full COPY of the repo in another commit, duplicating the report, inflating "not covered" and mixing old code; the pattern is anchored on the path, not the directory name.
# internal/testutil is excluded for a different reason: it is test scaffolding exercised by EVERY suite in the module (which is why --coverpkg counts it as covered), but gremlins mutates package by package running only that package's tests, so against testutil only its two tests would run, giving 11 survivors that are the contradiction of measuring with all packages and executing with one.
MUTATE_EXCLUDE ?= '(\.worktrees/|internal/testutil/)'

# MUTATE_COVERPKG: without it gremlins measures only the package under test, which lies here (other packages' tests call internal/testutil, so its lines come out "not covered" while being live code).
MUTATE_COVERPKG ?= ./...

# The per-mutant timeout is (measured suite duration) x this coefficient, and it is applied by GREMLINS, not by the watchdog: a context.WithTimeout around each mutant's `go test` plus its own -timeout, and when it fires the mutant lands as "TIMED OUT" while the run CONTINUES.
# TIMED OUT is what cannot happen: it does not go into mutants_total and the CI gate only looks at LIVED, so 15 untested mutants produce a run reporting 100% efficacy.
# The coefficient is COUPLED to the workers (measured over the diff scope: 16 workers with coef 2 was worse than 4 with coef 2, and 16 with coef 30 was the best), and these defaults are CI's (2 workers, coef 2); for local use `make mutate-all`, which measures the machine and picks the coefficient.
MUTATE_WORKERS ?= 2
MUTATE_TIMEOUT_COEFFICIENT ?= 2

MUTATE_FLAGS = --exclude-files $(MUTATE_EXCLUDE) --coverpkg $(MUTATE_COVERPKG) \
	--workers $(MUTATE_WORKERS) --timeout-coefficient $(MUTATE_TIMEOUT_COEFFICIENT)

# This bounds the WHOLE RUN and cuts it if it goes STALL (no progress during MUTATE_STALL_LIMIT), which is what really hurts: an absolute 45m ceiling does not shorten a 2m hang.
# Nothing has to be restored on the cut because gremlins does NOT mutate the repo: it copies the source tree to a temp dir per package and mutates the copy (measured: SIGKILL in five windows never changed the source md5); what a hard cut does leave behind are orphan /tmp/gremlins-* dirs, deliberately not cleaned here since they may belong to another run in flight.
MUTATE_HARD_LIMIT ?= 45m
MUTATE_STALL_LIMIT ?= 5m

# The supervisor lives in swe (~/.local/lib/swe, managed by chezmoi) and is NOT a verb: how-to-mutate keeps the engine and the invocation in the caller and the watchdog only supervises, which is why the caller computes the denominator and forbids the flags that break it.
WATCHDOG ?= $(HOME)/.local/lib/swe/lib/watchdog.sh

# One gremlins mutant line: report.go prints "%s%s %s at %s\n", so the line ends in `at <file>:<line>:<col>` with the colors in the status, and the trailing `[^[:space:]]` tolerates the CR a terminal's line discipline adds so this does not depend on the output being redirected.
# This is the ONLY place defining what "one unit of progress" is: the watchdog receives it and numerator and denominator come from the SAME expression.
MUTATE_LINE_RE = at [^[:space:]]+:[[:digit:]]+:[[:digit:]]+[^[:space:]]*$$

# Numerator and denominator both count MUTATE_LINE_RE lines, which is why the dry-run log is NOT counted with `wc -l`: that file also carries Starting..., Gathering coverage..., done in ... and the footer (measured: 13 lines for 5 mutants), and confusing the two makes the watchdog "reach the total" early and stop watching progress.
# And `mutants_total` in report.json is NOT the denominator: it is lived+killed+notViable, excluding not-covered and timed-out, while the dry-run prints those.
# The dry-run pre-pass is not free (coverage.Run() executes the whole `go test -coverpkg` before the engine exists, paying an instrumented suite plus the tree copy per worker, and runs zero suites per mutant), and it is the only way: MutantsTotal is only computed when the report is written, never during the run, so there is no total to read mid-flight.
MUTATE_ENUM_FLAGS = $(MUTATE_FLAGS)

# These flags BREAK the "exactly one line per mutant" contract, and so the denominator; they are checked here and not in the watchdog because they are a restriction on the INVOCATION, which belongs to the caller (how-to-mutate).
# -S/--output-statuses filters the lines by status, so with `-S k` the numerator stops meaning "mutants processed" (measured: 0 lines out of 5); -s/--silent suppresses every log.Infof, i.e. all mutant lines, and being a persistent root flag it counts anywhere (measured: 39 bytes, 0 lines).
# With either one the dry-run denominator is 0 or the real run freezes the progress: the two failures the watchdog exists to see.
MUTATE_FORBIDDEN = -S --output-statuses -s --silent

# `make mutate` uses CI's defaults (2 workers, coef 2), which is why on a many-core machine it takes ~25min and with high workers produces TIMED OUT by accident; this target is the one to use locally because it measures the machine and goes to 16 workers.
mutate-all: ## Mutation testing locally, with workers and timeout tuned to this machine (~10min)
	@scripts/mutate-all.sh

mutate-all-diff: ## Like mutate-all but only the diff vs main (daily loop, <1min)
	@scripts/mutate-all.sh --diff

mutate: ## Mutation testing (gremlins) on the whole module — advisory, never blocks CI
	@$(MAKE) --no-print-directory _mutate_check
	$(WATCHDOG) $(MUTATE_STALL_LIMIT) $(MUTATE_HARD_LIMIT) "$$($(MAKE) --no-print-directory _mutate_total)" '$(MUTATE_LINE_RE)' -- \
		go tool gremlins unleash $(MUTATE_FLAGS) --output report.json

# gremlins silently falls back to the whole module when the diff is empty (base == HEAD),
# so fail fast instead of running a full-module run that looks diff-scoped.
mutate-diff: ## Mutation testing (gremlins) restricted to the diff vs main — advisory
	@if git diff --name-only $(MUTATE_BASE)...HEAD | grep -q '\.go$$'; then \
		$(MAKE) --no-print-directory _mutate_check; \
		$(WATCHDOG) $(MUTATE_STALL_LIMIT) $(MUTATE_HARD_LIMIT) \
			"$$($(MAKE) --no-print-directory _mutate_total_diff)" '$(MUTATE_LINE_RE)' -- \
			go tool gremlins unleash --diff $(MUTATE_BASE) $(MUTATE_FLAGS) --output report.json; \
	else \
		echo "no .go changes vs $(MUTATE_BASE) - nothing to mutate"; \
	fi

# The check compares by token and not by substring, and covers the three ways a forbidden flag can slip in: exact, with `=value`, and grouped with another shorthand (`-Sk`); the forbidden shorthands live in two places because bash cannot derive a character class from a list, so adding one to MUTATE_FORBIDDEN means adding it to the second case's pattern too.
_mutate_check:
	@for p in $(MUTATE_FORBIDDEN); do \
		for f in $(MUTATE_FLAGS); do \
			case "$$f" in \
				"$$p"|"$$p"=*) \
					echo "mutate: '$$f' breaks the one-line-per-mutant contract (ver MUTATE_FORBIDDEN)" >&2; \
					exit 2 ;; \
			esac; \
		done; \
	done; \
	for f in $(MUTATE_FLAGS); do \
		case "$$f" in \
			--*) ;; \
			-?*) case "$${f#-}" in \
				s*|S*) \
					echo "mutate: '$$f' carries a grouped forbidden shorthand (ver MUTATE_FORBIDDEN)" >&2; \
					exit 2 ;; \
			esac ;; \
		esac; \
	done

# The denominator is a dry-run pre-pass with the SAME scope flags as the real run, supervised with the same watchdog but with no total (TOTAL=0) since the total is what that pre-pass computes; during the coverage phase gremlins emits no line at all, so only the ceiling can be watched there.
# stderr is NOT discarded: if the pre-pass fails, the watchdog's diagnostic is the only thing saying why, and a total of 0 degrades to line mode, which is correct.
_mutate_total:
	@$(WATCHDOG) $(MUTATE_STALL_LIMIT) $(MUTATE_HARD_LIMIT) 0 '' -- \
		go tool gremlins unleash --dry-run $(MUTATE_ENUM_FLAGS) \
		| grep -cE '$(MUTATE_LINE_RE)'

_mutate_total_diff:
	@$(WATCHDOG) $(MUTATE_STALL_LIMIT) $(MUTATE_HARD_LIMIT) 0 '' -- \
		go tool gremlins unleash --dry-run --diff $(MUTATE_BASE) $(MUTATE_ENUM_FLAGS) \
		| grep -cE '$(MUTATE_LINE_RE)'

clean: ## Deletes the artefacts in bin/
	rm -rf bin
