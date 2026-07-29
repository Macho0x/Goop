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

<p align="center">
  <strong><a href="https://macho0x.github.io/Goop/">Try Goop in the browser →</a></strong><br>
  <sub>Playground: check, compile, and copy generated Go — no install</sub>
</p>

Editors: [VS Code](editors/vscode/) · [Zed](editors/zed/) · [Neovim](editors/neovim/) · [Helix](editors/helix/)

## Goop in practice

**Sum types you can trust** — every variant must be handled:

```goop
type OrderAck =
  | Filled of { order_id: string; qty: int }
  | Rejected of { reason: string }

let describe (ack: OrderAck) : string =
  match ack with
  | Filled { order_id; qty } -> order_id ^ " filled " ^ int_to_string qty
  | Rejected { reason } -> "rejected: " ^ reason
```

**Branded IDs** — `order_id` and `symbol` are not interchangeable strings:

```goop
type order_id = | Order_id of string
type symbol = | Symbol of string

let place (sym: symbol) (oid: order_id) : string =
  "placed"

let main () =
  println (place (Symbol "ETH-USD") (Order_id "ord-1"))
```

**Maps + option** — lookups are `Some` / `None`, not “did I check `ok`?”:

```goop
let main () =
  let prices : map[string] int = Map.make () in
  let _ = Map.add prices "ETH" 3200 in
  match Map.get prices "ETH" with
  | Some px -> println (int_to_string px)
  | None -> println "missing"
```

**Go when you need it** — import packages, or drop in inline Go / C:

```goop
import go "strings"

@[go] {
  func greet(name string) string {
    return "Hello, " + name + "!"
  }
}
val greet : string -> string

@[c] {
  int c_add(int a, int b) { return a + b; }
}
val c_add : int -> int -> int

let main () =
  begin
    println (strings.ToUpper "goop");
    println (greet "world");
    println (int_to_string (c_add 40 2))
  end
```

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

### Playground (fastest)

**[Open the playground](https://macho0x.github.io/Goop/)** — type Goop, run check/compile in the browser, copy generated Go. No install, no local toolchain.

### Install locally

```bash
# One-line install (latest release binary)
curl -fsSL https://raw.githubusercontent.com/Macho0x/Goop/main/scripts/install.sh | bash

# Or build from source
cd src && go build -o ../goop ./cmd/goop
../goop new hello && cd hello
../goop check main.goop
../goop build main.goop   # → ./goop-out
./goop-out
```

Show what the compiler emits (no cache write):

```bash
goop compile --stdout docs/examples/hello.goop
```

Generated Go stays under `$GOOP_HOME/build` by default (unless `--stdout` or `--in-tree`).
Lowering is **one-way** — there is no Go → Goop auto-translator. Teaching pairs:
[docs/examples/gallery/](docs/examples/gallery/).
[Tutorial](docs/tutorial/README.md) ([maps](docs/tutorial/08-maps.md)) · [CLI artifacts](docs/design/20-cli-artifacts.md) · [`maps.goop`](docs/examples/maps.goop) · [`branded_ids.goop`](docs/examples/branded_ids.goop)

## How Goop compares

Compile-time checks Go leaves to tests or panics — without giving up Go’s runtime.

| | Go | Rust | OCaml | Goop |
|---|---|---|---|---|
| Sum types + exhaustive `match` | ❌ | ✅ | ✅ | ✅ |
| No null by default (`option`) | ❌ | ✅ | ✅ | ✅ |
| Recoverable errors as `result` | ❌ (`error`) | ✅ | ✅ | ✅ |
| Branded / nominal IDs | ❌ | ✅ | ✅ | ✅ |
| First-class maps | ✅ | ✅ | ✅ | ✅ |
| Effect handlers (OCaml 5-style) | ❌ | ❌ | ✅ | ✅ |
| Nil-channel / close safety | ❌ (runtime) | N/A | N/A | ✅ |
| Data-race awareness | ⚠️ `-race` | ✅ | ❌ | ✅ (conservative) |
| Refinement contracts | ❌ | ⚠️ | ⚠️ | ✅ (optional Z3) |
| Inline Go embeds (`@[go]`) | — | ❌ | ❌ | ✅ |
| Inline C / cgo (`@[c]`) | ✅ | ❌ | ❌ | ✅ |
| Native Go stdlib + deploy | ✅ | ❌ | ❌ | ✅ |
| Compiles to ordinary Go binaries | — | ❌ | ❌ | ✅ |

[STYLE.md](docs/design/STYLE.md) · [`extern_demo.goop`](docs/examples/extern_demo.goop) · [`cgo_demo.goop`](docs/examples/cgo_demo.goop)

## Status

**[v1.17.0](https://github.com/Macho0x/Goop/releases/tag/v1.17.0)** — prelude `println`; `std.io.Println`. **[Playground](https://macho0x.github.io/Goop/)** is the fastest way to try Goop.

[CHANGELOG](CHANGELOG.md) · [RELEASE_NOTES](RELEASE_NOTES.md) · [Benchmarks](benchmarks/README.md) · [Language-update checklist](docs/design/31-language-update-checklist.md)

## FAQ

**Production-ready?** Compiler, checker, codegen, LSP, and CI e2e ship today. We are not claiming production load readiness yet.

**Need OCaml or Z3?** No. Pattern matching from Rust/Swift/Kotlin transfers. Z3 is optional (`[check] smt = true`).

## Documentation

| | |
|---|---|
| [**Playground**](https://macho0x.github.io/Goop/) | Try Goop in the browser (check, compile, copy Go) |
| [Tutorial](docs/tutorial/README.md) | Getting started through [maps](docs/tutorial/08-maps.md) |
| [Stdlib](docs/stdlib/README.md) | Prelude and `std.*` |
| [Examples](docs/examples/) | Checked in CI |
| [Goop ↔ Go gallery](docs/examples/gallery/) | Hand-written teaching pairs (not a translator) |
| [Writing tools](docs/guides/writing-tools.md) | Files + maps via `import go` |
| [Language-update checklist](docs/design/31-language-update-checklist.md) | Required sweep after language changes |
| [Contributing](CONTRIBUTING.md) | Build and test |

## License

MIT / Apache-2.0 dual license.
