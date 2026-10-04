#!/usr/bin/env bash
# Generates testdata/playground with deterministic git fixture repos; the bare "origin" remotes live under .origin/ (hidden, so the scanner prunes them).
set -euo pipefail

BASE="$(cd "$(dirname "$0")/.." && pwd)/testdata/playground"
ORIGIN="$BASE/.origin"
rm -rf "$BASE"
mkdir -p "$ORIGIN"

G() { git -c user.email=gitdash@local -c user.name=gitdash "$@"; }

marker() { # marker <dir> [name] [primary] [secondary]
  local dir="$1" name="${2:-}" primary="${3:-}" secondary="${4:-}" c=""
  [ -n "$name" ] && c+="name = \"$name\""$'\n'
  [ -n "$primary" ] && c+="primary_group = \"$primary\""$'\n'
  [ -n "$secondary" ] && c+="secondary_group = \"$secondary\""$'\n'
  printf '%s' "$c" > "$dir/.gitdash.toml"
}

new_repo() { # new_repo <name> <marker-name> <primary> <secondary>
  local dir="$BASE/$1"
  git init -q -b main "$dir"
  git -C "$dir" config user.email gitdash@local
  git -C "$dir" config user.name gitdash
  echo base > "$dir/base.txt"
  marker "$dir" "$2" "$3" "${4:-}"  # in the base commit: it does not dirty the state
  G -C "$dir" add -A && G -C "$dir" commit -qm base
  echo "$dir"
}

with_origin() { # stdout: dir; wires a bare origin and tracks main
  local dir="$1" origin="$ORIGIN/$(basename "$1").git"
  git init -q --bare -b main "$origin"
  G -C "$dir" remote add origin "$origin"
  G -C "$dir" push -qu origin main
  G -C "$dir" branch -qu --set-upstream-to=origin/main main 2>/dev/null || true
  echo "$origin"
}

push_upstream_commits() { # push_upstream_commits <origin> <n> <prefix>
  local origin="$1" n="$2" prefix="$3" clone
  clone="$(mktemp -d)"
  G clone -q "$origin" "$clone"
  git -C "$clone" config user.email gitdash@local
  git -C "$clone" config user.name gitdash
  for i in $(seq 1 "$n"); do
    echo up > "$clone/${prefix}$i.txt"
    G -C "$clone" add -A && G -C "$clone" commit -qm "$prefix $i"
  done
  G -C "$clone" push -q origin main
  rm -rf "$clone"
}

dir=$(new_repo clean-go clean-go gitdash playground); origin=$(with_origin "$dir")

dir=$(new_repo dirty-java dirty-java gitdash playground); with_origin "$dir" >/dev/null
echo changed > "$dir/base.txt"; echo x > "$dir/un1.txt"; echo y > "$dir/un2.txt"

dir=$(new_repo ahead-rust ahead-rust gitdash playground); origin=$(with_origin "$dir")
echo a > "$dir/a.txt"; G -C "$dir" add -A; G -C "$dir" commit -qm "local 1"
echo b > "$dir/b.txt"; G -C "$dir" add -A; G -C "$dir" commit -qm "local 2"

dir=$(new_repo behind-python behind-python gitdash playground); origin=$(with_origin "$dir")
push_upstream_commits "$origin" 3 behind-
G -C "$dir" fetch -q origin

dir=$(new_repo diverged-node diverged-node gitdash playground); origin=$(with_origin "$dir")
echo a > "$dir/a.txt"; G -C "$dir" add -A; G -C "$dir" commit -qm "local 1"
echo b > "$dir/b.txt"; G -C "$dir" add -A; G -C "$dir" commit -qm "local 2"
push_upstream_commits "$origin" 3 div-
G -C "$dir" fetch -q origin

new_repo no-upstream-cpp no-upstream-cpp gitdash playground >/dev/null

dir=$(new_repo detached-shell detached-shell gitdash playground); origin=$(with_origin "$dir")
G -C "$dir" checkout -q --detach HEAD

main=$(new_repo worktree-main worktree-main gitdash playground)
G -C "$main" worktree add -q "$BASE/worktree-wt" -b wt-branch
marker "$BASE/worktree-wt" worktree-wt gitdash playground
G -C "$BASE/worktree-wt" add -A && G -C "$BASE/worktree-wt" commit -qm marker
G -C "$main" worktree add -q "$BASE/worktree-nomarker" -b nomarker

dir=$(new_repo sync-behind sync-behind gitdash playground)
G -C "$dir" checkout -q -b feat
echo f > "$dir/f.txt"; G -C "$dir" add -A; G -C "$dir" commit -qm "feat 1"
wt=$(mktemp -d)
G -C "$dir" worktree add -q "$wt" main
echo m1 > "$wt/m1.txt"; G -C "$wt" add -A; G -C "$wt" commit -qm "sync 1"
echo m2 > "$wt/m2.txt"; G -C "$wt" add -A; G -C "$wt" commit -qm "sync 2"
G -C "$dir" worktree remove --force "$wt"

mkdir -p "$BASE/no-repo-plain"
marker "$BASE/no-repo-plain" no-repo-plain gitdash playground

dir="$BASE/bad-marker-toml"
git init -q -b main "$dir"
git -C "$dir" config user.email gitdash@local; git -C "$dir" config user.name gitdash
printf 'name = [roto\n' > "$dir/.gitdash.toml"
echo base > "$dir/base.txt"
G -C "$dir" add -A && G -C "$dir" commit -qm base

dir=$(new_repo nested mono gitdash playground)
sub="$dir/sub"; git init -q -b main "$sub"
git -C "$sub" config user.email gitdash@local; git -C "$sub" config user.name gitdash
echo base > "$sub/base.txt"; G -C "$sub" add -A; G -C "$sub" commit -qm base
marker "$sub" sub-nested gitdash playground

echo "playground ready at $BASE"
