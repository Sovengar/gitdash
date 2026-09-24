# Atajos de desarrollo de gitdash.
# El binario que ejecuta el usuario vive en ~/.local/bin (ver AGENTS.md):
# tras cualquier cambio de código hay que recompilarlo con `make install`.

SHELL := /bin/bash

INSTALL_DIR := $(HOME)/.local/bin
BIN         := bin/gitdash
PKG         := ./cmd/gitdash

# Misma versión que usa dbx; se ejecuta con `go run`, sin binario global.
GOLANGCI_LINT_VERSION := v2.13.2

.DEFAULT_GOAL := help
.PHONY: help build install fmt fmt-check vet lint test test-race check all run print fixtures smoke tidy clean

help: ## Muestra esta ayuda
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-11s\033[0m %s\n", $$1, $$2}'

build: ## Compila el binario en bin/gitdash
	go build -o $(BIN) $(PKG)

install: ## Compila e instala en ~/.local/bin (obligatorio tras cambios)
	go build -o $(INSTALL_DIR)/gitdash $(PKG)

fmt: ## Aplica gofmt sobre el árbol
	gofmt -w .

fmt-check: ## Verifica formato gofmt sin modificar (falla si hay pendientes)
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt pendiente en:"; echo "$$out"; exit 1; fi

vet: ## go vet
	go vet ./...

lint: vet fmt-check ## go vet + gofmt + golangci-lint (binario local o go run)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run; \
	fi

test: ## Ejecuta la suite de tests
	go test ./...

test-race: ## Ejecuta la suite con el detector de carreras
	go test -race ./...

check: build lint test ## build + lint + test (equivalente al gate de CI)
	@echo "check OK"

all: check test-race ## check + test-race

run: ## Arranca la TUI contra tu config real
	go run $(PKG)

print: ## Modo tabla one-shot (--print)
	go run $(PKG) --print

fixtures: ## Regenera testdata/playground (repos git fixture)
	./scripts/gen-fixtures.sh

smoke: build ## Smoke test de la TUI en tmux con config aislada y fixtures
	@test -d testdata/playground || { echo "falta testdata/playground — corre 'make fixtures'"; exit 1; }
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

clean: ## Borra los artefactos de bin/
	rm -rf bin
