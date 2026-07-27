# Goop Packages and Registry (1.0)

## Doctrine for 1.0

**Goop packages are Go modules.** There is no separate Goop package registry
in 1.0.

| Concern | 1.0 answer |
|---------|------------|
| Where do packages live? | Anywhere a Go module path resolves (`github.com/…`, etc.) |
| How do you publish? | Push a Git repo with Goop sources; use a normal Go module path |
| How do you install? | `goop get <module-path>[@version]` |
| How are versions pinned? | `goop.toml` `[dependencies]` + `goop.lock` |
| Separate Goop registry? | **No** — deferred past 1.0 |

Publishing a Goop library is the same workflow as publishing a Go module:
host the tree under a canonical import path, tag releases, and let consumers
`goop get` that path. Goop does not invent a parallel index, mirror, or
`goop.dev/pkg/…` registry for third-party code.

Go packages used from Goop (`import go "…"`) continue to resolve through the
normal Go toolchain (`go.mod` / the module proxy). That path is independent of
`goop get`.

## `goop get`

Fetch a remote Goop module and pin it in the project:

```bash
goop get github.com/acme/lib
goop get github.com/acme/lib@v1.2.3
```

This:

1. Requires a project-root `goop.toml` (creates/updates `[dependencies]`).
2. Clones `https://<module-path>` into `$GOOP_HOME/pkg/mod/<module-path>`
   (`git clone --depth 1 --branch <version>`; falls back to the default branch
   if the branch/tag clone fails). Default version is `main` when `@version`
   is omitted.
3. Upserts a pin in `goop.lock`.
4. Writes `"<module-path>" = "<version>"` under `[dependencies]` in `goop.toml`.

`$GOOP_HOME` defaults to `~/.cache/goop` (override with the `GOOP_HOME`
environment variable). See [20-cli-artifacts.md](20-cli-artifacts.md).

## `$GOOP_HOME` layout

| Path | Purpose |
|------|---------|
| `$GOOP_HOME/pkg/mod` | Source cache for `goop get` |
| `$GOOP_HOME/build` | Compile/build/test sandboxes ([20-cli-artifacts.md](20-cli-artifacts.md)) |
| `$GOOP_HOME/build/go-sigs` | Generated Go `.gosig` stubs ([23-go-sig-resolution.md](23-go-sig-resolution.md)) |

## `goop.lock`

Pinned modules at the project root:

```toml
[[module]]
path = "github.com/acme/lib"
version = "v1.2.3"
source = "github.com/acme/lib"
```

| Field | Meaning |
|-------|---------|
| `path` | Canonical import path (key used by the resolver) |
| `version` | Git tag, branch, or other pin string from `goop get` |
| `source` | Clone identity under `$GOOP_HOME/pkg/mod` (usually same as `path`) |

Commit `goop.lock` so CI and teammates resolve the same pins. The compiler
prefers lock pins over floating `goop.toml` mappings when both exist.

`goop get` is the writer; there is no separate `goop lock` / `goop tidy`
command in 1.0.

## `goop.toml` dependencies

```toml
module_root = "github.com/you/yourapp"

[mappings]
"std.io" = "github.com/Macho0x/Goop/std/io"

[dependencies]
"github.com/acme/lib" = "v1.2.3"
```

- `[mappings]` — logical Goop paths (`"std.io"`) → Go import paths.
- `[dependencies]` — declared pins; kept in sync by `goop get`.
- `module_root` — this project's own Go module path prefix.

## Module entry convention

Remote Goop modules are discovered at:

`<module>/<last-segment>/<last-segment>.goop`

Example: `github.com/Macho0x/Goop/std/io/io.goop`.

Resolution order for `import goop "path"` (simplified):

1. Local project tree under `module_root` / known repo layout.
2. `goop.lock` pin → `$GOOP_HOME/pkg/mod/<source>/…`.
3. Cache path for the import string itself.
4. `[mappings]` / built-in defaults for logical names like `"std.io"`.

## Imports

```goop
module Main

import (
  go "fmt"                              (* Go package — go.mod / Go toolchain *)
  goop "std.io"                         (* logical path via mappings *)
  orderbook goop "github.com/you/app/orderbook"  (* canonical Go module path *)
)

import goop . "std.list"                (* dot import: unqualified exports *)
```

| Form | Resolved by |
|------|-------------|
| `import go "path"` | Go module graph (`go.mod`, module proxy) |
| `import goop "path"` | Goop resolver + optional `goop.lock` / cache |
| `import goop . "path"` | Same as `import goop`, then open exports |

Full import syntax: [05-modules-and-packages.md](05-modules-and-packages.md).

## Authoring a publishable package

1. Put `.goop` sources in a Git repo whose path is the intended import path
   (or document the mapping).
2. Follow the entry-file convention above.
3. Tag releases (`v1.2.3`, …).
4. Consumers run `goop get your.module/path@v1.2.3` and
   `import goop "your.module/path"`.

No signup, upload, or Goop-specific index is required for 1.0.

## `goop resolve`

Print import resolution and the transitive `import goop` graph:

```bash
goop resolve main.goop
```

## Out of scope for 1.0

- A Goop-native package registry or mirror
- Checksums / `go.sum`-style integrity in `goop.lock`
- Automatic transitive `goop get` of dependencies declared only inside a
  fetched module (consumers still `goop get` what they import)
- Replacing `import go` with Goop-registry metadata

If a dedicated registry is ever added, it will sit *beside* Go module paths,
not replace them as the primary identity for packages.
