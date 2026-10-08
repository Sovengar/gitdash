# Dev shortcuts. After any change: `make install` (the binary lives in $(PREFIX)/bin).

SHELL := /bin/bash

PREFIX      ?= $(HOME)/.local
BINDIR      := $(PREFIX)/bin
BIN         := bin/gitdash
PKG         := ./cmd/gitdash

# Same version dbx uses; run with `go run`, no global binary.
GOLANGCI_LINT_VERSION := v2.13.2
MUTATE_BASE ?= main

.DEFAULT_GOAL := help
.PHONY: help build install uninstall fmt fmt-check vet lint test test-race check all run print fixtures smoke tidy clean mutate-all mutate-all-diff mutate-dry coverage coverage-check

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

# .worktrees is pruned on purpose: those are other branches' checkouts (often dirty WIP), and gofmt over them re-checks the same code twice at best and blocks main's gate with someone's unfinished file at worst.
GO_SRC = find . -path ./.worktrees -prune -o -name '*.go' -print

fmt: ## Runs gofmt over the tree
	gofmt -w $$($(GO_SRC))

fmt-check: ## Checks gofmt formatting without writing (fails if anything is pending)
	@out="$$(gofmt -l $$($(GO_SRC)))"; \
	if [ -n "$$out" ]; then echo "gofmt pending in:"; echo "$$out"; exit 1; fi

vet: ## go vet
	go vet ./...

# lint: fmt-check + golangci-lint; govet runs inside it, the `vet` target above is manual.
lint: fmt-check ## gofmt + golangci-lint (version pinned, always via go run)
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

test: ## Runs the test suite with -race (same class as CI)
	go test -race -count=1 ./...

test-race: test ## Alias of test: the suite already runs with -race

# Coverage: the deduplicated `go test -coverprofile` output plus main()'s subprocess profile.
COVER_PROFILE ?= coverage.out
COVER_MAIN ?= .covmain/main.txt

# .covmain is deleted, not reused: a previous build's counters point at ranges today's code no longer has.
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

# Mutation testing (gremlins). Everything lives in scripts/mutate.sh: warm-up, denominator,
# coefficient, forbidden flags, supervisor and verdict. These targets are the local loop;
# the required check calls the script directly. Budgets default from the script's scope table,
# MUTATE_* overrides them (mandatory under --ci), and MUTATE_BASE is shared with coverage-check.
mutate-all: ## Mutation testing locally, with the budget tuned to this machine (~10min)
	@scripts/mutate.sh --run

mutate-all-diff: ## Like mutate-all but only the diff vs main (daily loop, <1min)
	@scripts/mutate.sh --diff

mutate-dry: ## Warm up and enumerate the mutants without mutating any (the budget that would apply)
	@scripts/mutate.sh --diff --dry

clean: ## Deletes the artefacts in bin/
	rm -rf bin
