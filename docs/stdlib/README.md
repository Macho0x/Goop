# Goop standard library reference

Goop has three API layers:

| Layer | Import needed? | Location |
|---|---|---|
| **Language builtins** | No | `option`, `result`, `'a list`, `chan`, etc. |
| **Prelude** | No | Always in scope; defined in `src/internal/prelude/prelude.go` |
| **`std.*` modules** | Yes — `import goop "std.io"` | `std/` directory |

## Doctrine

**Go’s standard library is the Goop standard library for library coverage.**
Reach Go packages with `import go "net/http" { ... }` (and curated `.gosig`s as they land).

**`std.*` exists only for OCaml-idiomatic / Goop-native primitives** — thin wrappers around language builtins and prelude helpers (`list`, `option`, `result`, `array`, `string`, `chan`, `ref`), plus a small I/O helper. It is **not** a mirror of Go’s stdlib.

| Belongs in `std.*` | Does **not** belong in `std.*` |
|---|---|
| OCaml-shaped helpers (`Map`, `isSome`, `length`) | `std.net`, `std.http`, `std.json`, `std.codec`, … |
| Thin re-exports of prelude (`Array.*`, `Chan.*`, `String.*`) | Re-wrapping every Go package “for consistency” |
| Goop-native linear / effect surfaces (`owned_chan`) | Large framework APIs that already exist in Go |

`std.list` staying small is intentional: list construction is builtin; the module only adds what the language does not.

## Prelude

[Prelude reference](prelude.md) — `print_line`, `ref`, `failwith`, `Chan.*`, `OwnedChan.*`, `Lazy.*`, string helpers, assertions.

## Builtins

[Language builtins](builtins.md) — primitive types, `list`, `array`, `ref`, `option`, `result`, `lazy`, channels.

## std.* modules

| Module | Import path | Role | Reference |
|---|---|---|---|
| `std.io` | `import goop "std.io"` | Thin `fmt` wrapper (`PrintLine`) | [std.io](std-io.md) |
| `std.list` | `import goop "std.list"` | Higher-order list (`Map`) | [std.list](std-list.md) |
| `std.array` | `import goop "std.array"` | Re-export of prelude `Array.*` | [std.array](std-array.md) |
| `std.option` | `import goop "std.option"` | Option predicates | [std.option](std-option.md) |
| `std.result` | `import goop "std.result"` | Result predicates | [std.result](std-result.md) |
| `std.string` | `import goop "std.string"` | Re-export of prelude string ops | [std.string](std-string.md) |
| `std.chan` | `import goop "std.chan"` | Re-export of `Chan.*` / `OwnedChan.*` (`std/channel`) | [std.chan](std-chan.md) |
| `std.ref` | `import goop "std.ref"` | Re-export of prelude `ref` | [std.ref](std-ref.md) |
| `std.decimal` | `import goop "std.decimal"` | Fixed-point money (`shopspring/decimal`) | [std.decimal](std-decimal.md) |

## Roadmap (what still belongs)

| Candidate | Status | Notes |
|---|---|---|
| More `std.list` combinators (`Filter`, `Fold`, …) | Optional | Only if used often enough to beat writing `match`; keep thin |
| `std.lazy` | Deferred | Prelude `Lazy.*` / `lazy e` stay; a thin wrapper does not lower cleanly through polymorphic Goop functions yet |
| Decimal / money | **MVP landed** | `std.decimal` + [25-decimal.md](../design/25-decimal.md); H5/H6 polish remain |
| `std.net` / `std.codec` / … | **Out of scope** | Use `import go` |

## Import forms

```goop
import goop "std.io"           (* qualified: must use module exports by name *)
import goop . "std.io"         (* dot: PrintLine in scope *)
import io goop "std.io"        (* alias: io.PrintLine *)
```

Resolution is configured in `goop.toml` `[mappings]` and defaults in the compiler. See [modules guide](../design/05-modules-and-packages.md).

## Naming conventions

| Layer | Convention | Example |
|---|---|---|
| Prelude | `snake_case` / qualified `Module.name` | `print_line`, `String.length`, `Chan.make` |
| `std.*` re-exports | lowercase matching OCaml module style | `make`, `length`, `concat` |
| `std.*` helpers | `PascalCase` or camelCase | `PrintLine`, `Map`, `isSome` |
| Constructors | `PascalCase` | `Some`, `Ok`, `OrderId` |

Keyword module names (`chan`, `ref`, `lazy`) cannot appear as `module …` headers; `std.chan` lives in `std/channel`, and `std.ref` uses `module Ref`.

## Maintenance

This reference is hand-written from compiler sources (`prelude.go`, `std/*/*.goop`). When adding prelude bindings or `std.*` exports, update the matching page here and `[mappings]` in `goop.toml` / `src/internal/config/config.go`.

Automated `goop doc` generation is not implemented yet ([TODO](../../TODO.md)).
