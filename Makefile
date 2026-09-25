# Atajos de desarrollo de gitdash.
# El binario que ejecuta el usuario vive en $(PREFIX)/bin (default ~/.local/bin,
# ver AGENTS.md): tras cualquier cambio de código hay que instalarlo con
# `make install` (compila bin/gitdash y lo copia).

SHELL := /bin/bash

PREFIX      ?= $(HOME)/.local
BINDIR      := $(PREFIX)/bin
BIN         := bin/gitdash
PKG         := ./cmd/gitdash

# Misma versión que usa dbx; se ejecuta con `go run`, sin binario global.
GOLANGCI_LINT_VERSION := v2.13.2

.DEFAULT_GOAL := help
.PHONY: help build install uninstall fmt fmt-check vet lint test test-race check all run print fixtures smoke tidy clean

help: ## Muestra esta ayuda
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-11s\033[0m %s\n", $$1, $$2}'

build: ## Compila el binario en bin/gitdash
	go build -o $(BIN) $(PKG)

install: build ## Instala bin/gitdash en PREFIX/bin (default ~/.local/bin; obligatorio tras cambios)
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(BIN) $(DESTDIR)$(BINDIR)/gitdash

uninstall: ## Borra el binario instalado de PREFIX/bin (default ~/.local/bin)
	rm -f $(DESTDIR)$(BINDIR)/gitdash

fmt: ## Aplica gofmt sobre el árbol
	gofmt -w .

fmt-check: ## Verifica formato gofmt sin modificar (falla si hay pendientes)
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt pendiente en:"; echo "$$out"; exit 1; fi

vet: ## go vet
	go vet ./...

lint: vet fmt-check ## go vet + gofmt + golangci-lint (versión pineada, siempre vía go run)
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

test: ## Ejecuta la suite de tests con -race (misma clase que CI)
	go test -race -count=1 ./...

test-race: test ## Alias de test: la suite ya corre con -race

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
