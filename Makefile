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
MUTATE_BASE ?= main

.DEFAULT_GOAL := help
.PHONY: help build install uninstall fmt fmt-check vet lint test test-race check all run print fixtures smoke tidy clean mutate mutate-diff

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

# MUTATE_EXCLUDE deja fuera lo que NO es codigo del modulo. Sin esto, gremlins
# recorre .worktrees/ (Worktrunk) y mide una COPIA completa del repo en otro
# commit: duplica el informe, infla el "not covered" y mezcla codigo viejo. El
# patron se ancla al path, no al nombre del directorio.
#
# internal/testutil queda fuera por un motivo distinto: es andamiaje de tests.
# Su correccion la ejercitan TODOS los suites del modulo (por eso --coverpkg lo
# da por cubierto), pero gremlins muta paquete a paquete y ejecuta solo los tests
# de ESE paquete: contra testutil solo correrian sus dos tests, que unicamente
# llaman a Marker. Medirlo asi produce 11 supervivientes que no son riesgos, son
# la contradiccion entre medir con todos los paquetes y ejecutar con uno solo.
MUTATE_EXCLUDE ?= '(\.worktrees/|internal/testutil/)'

# MUTATE_COVERPKG: sin esto gremlins mide SOLO el paquete bajo test, y aqui eso
# miente: internal/testutil lo llaman los tests de otros paquetes, asi que sus
# lineas salen "not covered" siendo codigo vivo. Con ./... el perfil mide todo
# el modulo, que es lo que el gate necesita para ser cierto.
MUTATE_COVERPKG ?= ./...

# El timeout por mutante es (duracion medida de la suite) x este coeficiente, no
# el timeout por defecto de go test. La suite tarda ~23s, asi que 2 deja 46s: de
# sobra para un test lento de verdad yjusto para que un mutante que no termina
# cuelgue el run entero. Con 1 (23s) un runner de CI cargado daria timeout falso,
# y los TIMED OUT no van a report.json: se convertiran en huecos invisibles.
MUTATE_TIMEOUT_COEFFICIENT ?= 2

MUTATE_FLAGS = --exclude-files $(MUTATE_EXCLUDE) --coverpkg $(MUTATE_COVERPKG) \
	--workers 4 --timeout-coefficient $(MUTATE_TIMEOUT_COEFFICIENT)

mutate: ## Mutation testing (gremlins) on the whole module — advisory, never blocks CI
	go tool gremlins unleash $(MUTATE_FLAGS) --output report.json

# gremlins silently falls back to the whole module when the diff is empty (base == HEAD),
# so fail fast instead of running a full-module run that looks diff-scoped.
mutate-diff: ## Mutation testing (gremlins) restricted to the diff vs main — advisory
	@if git diff --name-only $(MUTATE_BASE)...HEAD | grep -q '\.go$$'; then \
		go tool gremlins unleash --diff $(MUTATE_BASE) $(MUTATE_FLAGS) --output report.json; \
	else \
		echo "no .go changes vs $(MUTATE_BASE) - nothing to mutate"; \
	fi

clean: ## Borra los artefactos de bin/
	rm -rf bin
