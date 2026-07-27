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

- Exhaustive ADTs and pattern matching
- Branded IDs (single-ctor ADTs) so `order_id` ≠ `symbol`
- Go interop and deploy — same runtime, stdlib, and binaries
- Cache-only builds — edit `.goop`, run `goop build` / `goop test`
- Editor support — [VS Code](editors/vscode/), [Zed](editors/zed/), [Neovim](editors/neovim/), [Helix](editors/helix/)

### Non-exhaustive match (EXHAUST003)

```goop
type OrderAck =
  | Filled of { order_id: string; qty: int }
  | Rejected of { reason: string }
  | PartialFill of { order_id: string; filled: int; remaining: int }

let handleAck (ack: OrderAck) : string =
  match ack with
  | Filled { order_id; qty } -> order_id
  | Rejected { reason } -> reason
  (* forgot PartialFill — Go compiles; Goop does not *)
```

```
✕ EXHAUST003: non-exhaustive match: missing constructor(s): PartialFill
```

## Getting started

```bash
cd src && go build -o ../goop ./cmd/goop
../goop check docs/examples/hello.goop
../goop build docs/examples/hello.goop   # → ./goop-out (Go stays in $GOOP_HOME/build)
./goop-out
../goop test tests/
../goop repl                 # interactive session (compile-to-Go)
```

Generated `.go` never lands in your project tree by default. See
[CLI artifacts](docs/design/20-cli-artifacts.md) · [tutorial](docs/tutorial/README.md).

## How Goop compares

Compile-time checks that Go leaves to tests, panics, or `-race`. Style: [STYLE.md](docs/design/STYLE.md).

| Safety feature | Go | Rust | OCaml | Goop |
|---|---|---|---|---|
| Sum types + exhaustive `match` | ❌ | ✅ | ✅ | ✅ |
| No null by default (`option`) | ❌ | ✅ | ✅ | ✅ |
| Branded IDs (single-ctor ADT) | ❌ | ✅ | ✅ | ✅ |
| Effect handlers (OCaml 5-style) | ❌ | ❌ | ✅ | ✅ (CPS-lowered) |
| Nil channel misuse | ❌ (runtime) | N/A | N/A | ✅ (NIL001) |
| Data race detection | ⚠️ `-race` | ✅ | ❌ | ✅ (linear, conservative) |
| Refinement / contracts | ❌ | ⚠️ | ⚠️ | ✅ VC + optional Z3 |
| Native Go stdlib + deploy | ✅ | ❌ | ❌ | ✅ |

## Go interop

Call Go packages, implement Go interfaces in Goop, and embed Go when needed:

```goop
module main

import go "strings" {
  val ToUpper : string -> string
}

@[go] {
  func greet(name string) string {
    return "Hello, " + name + "!"
  }
}
val greet : string -> string

let main () : unit =
  print_line (greet (ToUpper "goop"))
```

`import go` / `import goop` · `implements` · Go methods/fields · `@[go]` / `@[c]` embeds.
Examples: [`extern_demo.goop`](docs/examples/extern_demo.goop) · [`go_method_calls.goop`](docs/examples/go_method_calls.goop) · [`go_implements_slog_handler.goop`](docs/examples/go_implements_slog_handler.goop).

## Language & status

Everyday surface: [STYLE.md](docs/design/STYLE.md) · [`docs/examples/`](docs/examples/).

**Current: [v1.8.0](https://github.com/Macho0x/Goop/releases/tag/v1.8.0)** — cache-only `goop build` / `compile` under `$GOOP_HOME/build`, with transitive `import goop` deps. Prior highlights: Go struct/slog FFI (1.7), cross-package libraries (1.6), call lowering (1.5). Full history: [CHANGELOG](CHANGELOG.md) · [RELEASE_NOTES](RELEASE_NOTES.md).

## FAQ

**Production-ready?** The compiler, checker, codegen, LSP, and e2e suite ship and are exercised in CI. Some OCaml corners are pragmatic subsets; we are not claiming production load readiness yet.

**Borgo / Dingo?** Goop is a full compiler with OCaml-aligned syntax and compile-time safety for gradual Go migration. Dingo-style `?` and F# computation expressions were removed in 1.0.

**Need OCaml or Z3?** No. Pattern matching from Rust/Swift/Kotlin transfers. Z3 is optional (`[check] smt = true`).

## Documentation

| Resource | Link |
|---|---|
| [Tutorial](docs/tutorial/README.md) | Getting started through concurrency |
| [Stdlib](docs/stdlib/README.md) | Prelude, builtins, `std.*` |
| [Packages](docs/design/11-package-manager.md) | Go modules, `goop get`, `goop.lock` (no separate registry) |
| [Design](docs/design/) | STYLE, CLI artifacts, FFI |
| [Examples](docs/examples/) | Runnable; CI `goop check` |
| [Contributing](CONTRIBUTING.md) | Build and test workflow |

## License

MIT / Apache-2.0 dual license.
