# gitdash

Panel de estados git en TUI, estilo GitHub Desktop: todos tus repos marcados
con `.repo.toml` en un dashboard, con branch, cambios pendientes
(dirty) y **↑ahead / ↓behind** (qué hay que subir y bajar de un vistazo),
más acciones rápidas.

Inspirado en [bircni/git-statuses](https://github.com/bircni/git-statuses)
(Rust, CLI one-shot) — no es un fork: la idea del escaneo y el modelo de
datos se reimplementaron en Go + Bubbletea v2 como TUI interactiva, con
descubrimiento por **fichero marcador** en lugar de buscar `.git` suelto y
**fetch automático en batches**.

## Instalación

```bash
go install gitdash/cmd/gitdash@latest   # o
go build -o ~/.local/bin/gitdash ./cmd/gitdash
```

Requiere el binario `git` en el PATH (subprocess; sin cgo ni librerías git).

## Uso

```bash
gitdash            # TUI
gitdash --print    # tabla one-shot en stdout (útil para scripts/debug)
```

Marcador `.repo.toml` en la raíz de cada proyecto (campos todos opcionales):

```toml
name = "api"        # nombre mostrado (default: nombre del directorio)
group = "vsocial"   # agrupación visual (default: "-")
```

### Config

`~/.config/gitdash/config.toml` (todo opcional, con estos defaults):

```toml
marker  = ".repo.toml"
roots   = ["~/dev"]
exclude = ["node_modules", "target", "vendor", "dist", "build", "out",
           "coverage", ".venv", "__pycache__", ".gradle", ".terraform"]
editor  = "vi"        # $EDITOR si está definida

[fetch]
auto        = true    # fetch automático tras cada scan/rescan
concurrency = 4       # fetches en paralelo (batches)
timeout     = "30s"   # timeout por fetch
```

## Teclas

| Tecla | Acción |
|---|---|
| `j/k`, `↑↓` | mover cursor (`g`/`G` extremos) |
| `n` | alternar solo repos con cambios pendientes |
| `/` | filtrar por nombre/grupo (en vivo; `enter` confirma, `esc` limpia) |
| `r` | rescan completo (discovery + estados + fetch auto) |
| `R` | re-coleccionar el repo del cursor |
| `f` / `F` | fetch del repo / fetch de todos |
| `p` | pull (`--ff-only`; si divergió, falla visible con hint) |
| `P` | push |
| `e` | abrir `$EDITOR` en el directorio del repo |
| `enter` | detalle: ficheros cambiados, commits, última acción |
| `q` | salir |

## Cómo descubre repos

Recorre los `roots` en profundidad ilimitada (podando ocultos y
`exclude`), buscando el marcador. La carpeta del marcador ES el repo:
`.git` directorio = repo normal, `.git` fichero = worktree (etiquetado
`[wt]`), sin `.git` = visible como `no repo`. Los estados derivados:
`clean`, `dirty`, `ahead`, `behind`, `diverged`, `no-upstream`,
`detached`, `no repo`, `error` — ordenados atención-primero.

## Desarrollo

```bash
go build ./... && go vet ./... && go test ./...
./scripts/gen-fixtures.sh    # genera testdata/playground (repos fixture)
XDG_CONFIG_HOME=$(mktemp -d) bin/gitdash --print   # prueba headless
```

Arquitectura: `internal/config` (TOML XDG), `internal/discovery` (walk por
marcador), `internal/gitstatus` (subprocess git + parsing `porcelain=v2`),
`internal/cache` (pintura instantánea al arrancar), `internal/tui`
(dashboard Bubbletea v2). La spec completa con escenarios está en
`docs/planning/0001-feature-gitdash-tui/`.

## Roadmap

- Agrupaciones colapsables (vsocial-backend, vsocial-frontend, ...)
- Integración profunda de worktrees (listar/mutar)
- Acciones extra: git update, stash, PRs
- Fetch programado en background

MIT
