<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/goop-banner.png">
    <source media="(prefers-color-scheme: light)" srcset="assets/goop-banner-whitebg.jpg">
    <img alt="Goop banner" src="assets/goop-banner-whitebg.jpg" width="680">
  </picture>
</p>

<p align="center">
  <a href="https://github.com/Macho0x/Goop/actions/workflows/ci.yml">
    <img alt="CI status" src="https://github.com/Macho0x/Goop/workflows/CI/badge.svg">
  </a>
  <a href="https://github.com/Macho0x/Goop/releases">
    <img alt="Latest release" src="https://img.shields.io/github/v/release/Macho0x/Goop?display_name=tag&label=release">
  </a>
  <img alt="License" src="https://img.shields.io/badge/license-MIT%2FApache--2.0-blue">
</p>

<h1 align="center">Goop</h1>

<p align="center">
  <strong>OCaml-style safety on Go’s runtime.</strong>
</p>

Exhaustive ADTs · branded IDs · maps · Go interop · same binaries and stdlib

Editors: [VS Code](editors/vscode/) · [Zed](editors/zed/) · [Neovim](editors/neovim/) · [Helix](editors/helix/)

## Safe

Go lets these slip through. Goop stops them at compile time.

**Forgot a match case**

```goop
type Severity = | Low | High | Critical

let should_alert (s: Severity) : bool =
  match s with
  | Low -> false
  | High -> true
```

```
✕ EXHAUST003: non-exhaustive match: missing constructor(s): Critical
╭─[example.goop:4:3]
  3 │ let should_alert (s: Severity) : bool =
> 4 │   match s with
  ·   ╰── EXHAUST003: non-exhaustive match: missing constructor(s): Critical
  5 │   | Low -> false
╰────
```

**Forgot `None`**

```goop
let greet (name: string option) : string =
  match name with
  | Some n -> "hi, " ^ n
```

```
✕ EXHAUST003: non-exhaustive match: missing constructor(s): None
╭─[example.goop:3:3]
  2 │ let greet (name: string option) : string =
> 3 │   match name with
  ·   ╰── EXHAUST003: non-exhaustive match: missing constructor(s): None
  4 │   | Some n -> "hi, " ^ n
╰────
```

## Getting started

```bash
cd src && go build -o ../goop ./cmd/goop
../goop check docs/examples/hello.goop
../goop build docs/examples/hello.goop   # → ./goop-out
./goop-out
../goop test tests/
```

Generated Go stays under `$GOOP_HOME/build` by default.
[Tutorial](docs/tutorial/README.md) · [CLI artifacts](docs/design/20-cli-artifacts.md)

## Go interop

Use Go’s ecosystem without leaving Goop:

```goop
module main

import go "strings"

let main () =
  print_line (strings.ToUpper "goop")
```

Hand-written `{ val … }` blocks still work; bare imports load curated `.gosig` stubs.
More: [`writing_tools.goop`](docs/examples/writing_tools.goop) · [`extern_demo.goop`](docs/examples/extern_demo.goop)

## How Goop compares

| | Go | Rust | OCaml | Goop |
|---|---|---|---|---|
| Sum types + exhaustive `match` | ❌ | ✅ | ✅ | ✅ |
| No null by default (`option`) | ❌ | ✅ | ✅ | ✅ |
| Branded IDs | ❌ | ✅ | ✅ | ✅ |
| Native Go stdlib + deploy | ✅ | ❌ | ❌ | ✅ |

[STYLE.md](docs/design/STYLE.md) for everyday syntax.

## Status

**[v1.10.0](https://github.com/Macho0x/Goop/releases/tag/v1.10.0)** — maps, zero-cost brands, bare-import Go stubs, green Ubuntu + Windows CI.

[CHANGELOG](CHANGELOG.md) · [RELEASE_NOTES](RELEASE_NOTES.md)

## FAQ

**Production-ready?** Compiler, checker, codegen, LSP, and CI e2e ship today. We are not claiming production load readiness yet.

**Need OCaml or Z3?** No. Pattern matching from Rust/Swift/Kotlin transfers. Z3 is optional (`[check] smt = true`).

## Documentation

| | |
|---|---|
| [Tutorial](docs/tutorial/README.md) | Getting started |
| [Stdlib](docs/stdlib/README.md) | Prelude and `std.*` |
| [Examples](docs/examples/) | Checked in CI |
| [Writing tools](docs/guides/writing-tools.md) | Files + maps via `import go` |
| [Contributing](CONTRIBUTING.md) | Build and test |

## License

MIT / Apache-2.0 dual license.
