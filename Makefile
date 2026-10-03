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
.PHONY: help build install uninstall fmt fmt-check vet lint test test-race check all run print fixtures smoke tidy clean mutate mutate-diff mutate-all mutate-all-diff coverage coverage-check _mutate_check _mutate_total _mutate_total_diff

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

# La cobertura se mide sobre el perfil deduplicado que produce
# `go test -coverprofile` (un bloque por test binary), mas el perfil del
# subproceso que ejecuta func main(), que ese test no recoge.
COVER_PROFILE ?= coverage.out
COVER_MAIN ?= .covmain/main.txt

# .covmain se BORRA, no se reutiliza: guarda los contadores del binario
# instrumentado, y los de una build anterior tienen rangos que el codigo de hoy
# ya no tiene. Fusionarlos suma bloques que ya no existen al perfil y el total
# baja solo (medido: 80.24% con 7 ficheros viejos, 99.74% limpio). El `rm` va en
# la recipe, y este comentario FUERA: dentro se lo pasa al shell, que lo ejecuta
# como un comando mas y rompe el target.
coverage: ## Perfil de cobertura deduplicado + el del subproceso de main()
	@go test -count=1 -coverpkg ./... -coverprofile=$(COVER_PROFILE) ./... > /dev/null
	@rm -rf .covmain
	@mkdir -p .covmain
	@go build -cover -o .covmain/gitdash $(PKG)
	@XDG_CONFIG_HOME="$$(mktemp -d)" GOCOVERDIR="$$PWD/.covmain" \
		./.covmain/gitdash --print >/dev/null 2>&1 || true
	@go tool covdata textfmt -i=.covmain -o=$(COVER_MAIN)
	@echo "perfiles: $(COVER_PROFILE) $(COVER_MAIN)"

coverage-check: coverage ## Gate: el DIFF de este cambio al 100%, y el total con suelo
	@extra=""; [ -f "$(COVER_MAIN)" ] && extra="-a $$PWD/$(COVER_MAIN)"; \
		scripts/diff-coverage.sh "$(COVER_PROFILE)" "$(MUTATE_BASE)" $$extra

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
# el timeout por defecto de go test.
#
# OJO: este timeout lo aplica GREMLINS, no el watchdog. Es un
# context.WithTimeout alrededor del go test de cada mutante (executor.go:194), mas
# un -timeout propio pasado a go test (executor.go:227); si se dispara, el mutante
# queda con status "TIMED OUT" y el run CONTINUA. Asi que un mutante colgado ya
# estaba parado antes de que existiera el watchdog, y el watchdog no aporta nada
# en ese caso: solo ve "el run entero no ha emitido nada en STALL".
#
# Y el coeficiente esta ACOPLADO a los workers, medido. Con la cache de `go test`
# caliente la cobertura mide ~2s, asi que coef 2 da un techo de 6s por mutante
# mientras la suite de internal/tui tarda 3s sola: en cuanto hay contencion, los
# mutantes expiran. Medido sobre el scope del diff (48 mutantes):
#
#   workers  coef   wall  killed  TIMED OUT  killed/s
#        4     2     30s      46           0      1.53
#       10     2     20s      29          17      1.45
#       16     2     20s      20          26      1.00   <- peor que 4 workers
#       16    30     20s      46           0      2.30   <- el que hay que usar
#
# TIMED OUT es lo que no puede pasar: NO va a `mutants_total` (report.go:178 lo
# excluye) y el gate de CI solo mira `LIVED`, asi que 15 mutantes sin testear
# producen un run con "100% de eficacia".
#
# Estos defaults son los de CI (2 workers, coef 2, runner de 2-4 nucleos). Para
# local NO son los buenos: usa `make mutate-all`, que mide la cobertura de la
# maquina y elige el coeficiente que corresponde. O overridea:
#   make mutate MUTATE_WORKERS=16 MUTATE_TIMEOUT_COEFFICIENT=30
MUTATE_WORKERS ?= 2
MUTATE_TIMEOUT_COEFFICIENT ?= 2

MUTATE_FLAGS = --exclude-files $(MUTATE_EXCLUDE) --coverpkg $(MUTATE_COVERPKG) \
	--workers $(MUTATE_WORKERS) --timeout-coefficient $(MUTATE_TIMEOUT_COEFFICIENT)

# El coeficiente acota CADA mutante; esto acota el RUN entero, y ademas lo
# corta si se QUEDA PARADO (sin progreso durante MUTATE_STALL_LIMIT), que es lo
# que de verdad duele: un techo absoluto de 45m no acorta un atasco de 2m.
#
# NO hace falta restaurar nada al cortar. gremlins NO muta el repo: copia el
# arbol fuente a un temporal por paquete (workdir.CachedDealer) y muta la copia
# (TokenMutator.SetWorkdir). Medido: matando el proceso con SIGKILL en cinco
# ventanas distintas, el md5 del fuente no cambia ni una vez. Lo que SI deja un
# corte duro son temporales huerfanos en /tmp/gremlins-*, porque Clean() es un
# defer y SIGKILL no lo ejecuta; no se limpian aqui a proposito, que podrian
# ser de otro run de gremlins en marcha.
MUTATE_HARD_LIMIT ?= 45m
MUTATE_STALL_LIMIT ?= 5m

# El supervisor vive en swe (~/.local/lib/swe, gestionado por chezmoi) y NO es un
# verbo: how-to-mutate deja el motor y la invocacion en el llamador, y el watchdog
# solo supervisa. Por eso el llamador es quien calcula el denominador y quien
# prohibe los flags que lo rompen (§ MUTATE_FLAGS).
WATCHDOG ?= $(HOME)/.local/lib/swe/lib/watchdog.sh

# Una linea de mutante de gremlins. El formato sale de report.go (Mutant imprime
# "%s%s %s at %s\n"), asi que la cola de la linea es `at <file>:<linea>:<col>` y
# los codigos de color van en el status, no al final: el patron aguanta aunque el
# destino sea un TTY. El `[^[:space:]]` final tolera el CR que mete la disciplina
# de linea de un terminal, para no depender de que la salida este redirigida.
#
# Este es el UNICO sitio donde se define que es "una unidad de progreso": el
# watchdog la recibe y el numerador y el denominador salen de la MISMA expresion.
MUTATE_LINE_RE = at [^[:space:]]+:[[:digit:]]+:[[:digit:]]+[^[:space:]]*$$

# El numerador cuenta lineas de MUTATE_LINE_RE; el denominador tambien, y por eso
# el log del dry-run NO se cuenta con `wc -l`: ese fichero trae ademas Starting...,
# Gathering coverage..., done in ..., y el pie (Runnable: N, ... / Killed: N, ...).
# Medido: 13 lineas para 5 mutantes. Confundir los dos numeros hace que el
# watchdog "alcanze el total" antes de tiempo y deje de vigilar el progreso.
#
# OJO: `mutants_total` de report.json NO es el denominador. Es
# lived+killed+notViable (report.go), que excluye not-covered y timed-out; el
# dry-run si los imprime. Son dos numeros distintos.
#
# La pre-pasada en dry-run no es gratis: coverage.Run() corre `go test -coverpkg`
# entero ANTES de que el motor exista (y el motor es donde se mira dryRun), asi
# que paga una suite instrumentada + la copia del arbol por trabajador, y corre
# cero suites por mutante. Medido en este repo: segundos, contra las ~1027 suites
# del run real. Y es la unica via: MutantsTotal solo se calcula al escribir el
# informe, nunca durante el run (report.go:178), asi que no hay total en banda
# queantagear.
MUTATE_ENUM_FLAGS = $(MUTATE_FLAGS)

# Flags que ROMPEN el contrato "exactamente una linea por mutante", y por tanto
# el denominador. Se comprueban aqui y no en el watchdog porque son una restriccion
# sobre la INVOCACION, y la invocacion es del llamador (how-to-mutate). Meter
# flags de gremlins en el helper seria justo el acoplamiento que swe evita.
#
#   -S / --output-statuses : filtra las lineas por status (report/logger.go), asi
#                            que con `-S k` el numerador deja de ser "mutantes
#                            procesados". Medido: 0 lineas de 5 mutantes.
#   -s / --silent          : suprime todo log.Infof (internal/log), o sea todas
#                            las lineas de mutante. Ademas es flag PERSISTENT de
#                            la raiz, asi que vale en cualquier posicion.
#                            Medido: 39 bytes, 0 lineas.
# Con cualquiera de los dos, el dry-run daria denominador 0 o el run real
# congelaria el progreso: los dos fallos que el watchdog existe para ver.
MUTATE_FORBIDDEN = -S --output-statuses -s --silent

# El bucle de local. `make mutate` usa los defaults de CI (2 workers, coef 2) y
# por eso en una maquina con nucleos tarda ~25min y con workers altos produce
# TIMED OUT sin querer. Este target es el que hay que usar aqui: mide la
# cobertura de la maquina, elige el coeficiente que corresponde y va a 16 workers.
# Ver scripts/mutate-all.sh y el comentario de MUTATE_TIMEOUT_COEFFICIENT.
mutate-all: ## Mutation testing en local, con workers y timeout ajustados a esta maquina (~10min)
	@scripts/mutate-all.sh

mutate-all-diff: ## Como mutate-all pero solo el diff vs main (bucle diario, <1min)
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

# El check compara por token y no por subcadena, y cubre las tres formas en que
# un flag prohibido puede colarse: exacto, con `=valor`, y agrupado con otro
# shorthand (`-Sk`). Los shorthands prohibidos estan en dos sitios porque bash no
# puede derivar una clase de caracteres de una lista; si anades uno a
# MUTATE_FORBIDDEN, anadelo tambien al patron de la segunda case.
_mutate_check:
	@for p in $(MUTATE_FORBIDDEN); do \
		for f in $(MUTATE_FLAGS); do \
			case "$$f" in \
				"$$p"|"$$p"=*) \
					echo "mutate: '$$f' rompe el contrato una-linea-por-mutante (ver MUTATE_FORBIDDEN)" >&2; \
					exit 2 ;; \
			esac; \
		done; \
	done; \
	for f in $(MUTATE_FLAGS); do \
		case "$$f" in \
			--*) ;; \
			-?*) case "$${f#-}" in \
				s*|S*) \
					echo "mutate: '$$f' lleva un shorthand prohibido agrupado (ver MUTATE_FORBIDDEN)" >&2; \
					exit 2 ;; \
			esac ;; \
		esac; \
	done

# El denominador: una pre-pasada en dry-run con LOS MISMOS flags de scope que el
# run real. Se supervisea con el mismo watchdog pero sin total (TOTAL=0), porque
# el total es justo lo que esta pre-pasada calcula; durante la fase de cobertura
# gremlins no emite ninguna linea, asi que ahi solo puede vigilante el techo.
# stderr NO se descarta: si la pre-pasada falla, el diagnostico del watchdog es lo
# unico que dice por que. Un total de 0 degrada a modo lineas, que es lo correcto.
_mutate_total:
	@$(WATCHDOG) $(MUTATE_STALL_LIMIT) $(MUTATE_HARD_LIMIT) 0 '' -- \
		go tool gremlins unleash --dry-run $(MUTATE_ENUM_FLAGS) \
		| grep -cE '$(MUTATE_LINE_RE)'

_mutate_total_diff:
	@$(WATCHDOG) $(MUTATE_STALL_LIMIT) $(MUTATE_HARD_LIMIT) 0 '' -- \
		go tool gremlins unleash --dry-run --diff $(MUTATE_BASE) $(MUTATE_ENUM_FLAGS) \
		| grep -cE '$(MUTATE_LINE_RE)'

clean: ## Borra los artefactos de bin/
	rm -rf bin
