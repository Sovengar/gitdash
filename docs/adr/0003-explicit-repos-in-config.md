# ADR 0003 — Explicit repos in the global config

- Status: accepted
- Date: 2026-11-11

## Context

Discovery was marker-only: gitdash walks `roots` and every project needs a
`.gitdash.toml` at its root. That is a good default — the marker travels with
the repo, the folder is the repo, and the metadata (name, groups, sync branch,
AI prompt) lives next to the code it describes.

It is a poor fit for two cases:

- a repo outside every configured root;
- a repo the user does not want to add a file to, or cannot (a checkout owned
  by someone else, a read-only clone).

The open question was whether to replace the marker with a global list of
repos. A full replacement would move per-repo metadata into one XDG file and
break the "the marker travels with the repo" property, and it forces a
migration of every existing `.gitdash.toml`.

## Decision

Keep the marker as the discovery default and add an **optional, additive**
`repos` list to `~/.config/gitdash/config.toml`.

```toml
repos = ["~/dev/api", "/srv/checkouts/web"]
```

- `repos` is additive to the `roots` walk, never instead of it.
- A listed repo needs **no marker**: the user already said it is a repo. When
  the marker exists it still supplies name, groups and sync branch, so it keeps
  precedence over the defaults.
- A path found by both the walk and `repos` is listed once; the walk result and
  its marker metadata win.
- An unreadable entry is reported as `unreadable repo: <path>` and never aborts
  the scan, same as an unreadable root.
- Paths expand `~` like `roots`.

## Rejected alternatives

- **Marker removed, everything in config**: duplicates metadata in a shared
  file, loses the repo-local prompt, and makes every repo a manual entry.
- **A second file (`~/.config/gitdash/repos`)**: two schemas for the same
  concept; one `config.toml` key is enough.
- **Inline metadata per listed repo**: not needed for the two cases above; the
  marker already covers them when it exists.

## Consequences

- Existing setups keep working with no migration.
- The cache (`repos.json`) still validates entries by marker, so a listed repo
  without a marker can miss the first instant paint; the scan that follows
  adds it. Fixing the cache is a separate change.
