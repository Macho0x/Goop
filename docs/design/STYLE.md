# Goop Style Guide (OCaml-aligned)

Normative conventions for Goop 1.0. Where two surfaces once existed, **only the OCaml form remains**.

See also: [14-ocaml-parity.md](14-ocaml-parity.md), [03-syntax.md](03-syntax.md).

## Principles

1. One canonical surface per task — the OCaml spelling.
2. Exhaustive `match` over boolean chains on ADTs.
3. `result` for recoverable trading/venue errors; `raise` / `failwith` for bugs.
4. Explicit delimiters (`let … in`, `begin/end`, `for/do/done`, `while/do/done`).
5. Keep Go-style `import` and `go` / `go (move …)` — these are intentional Goop extensions.

## Deduplication (keep / removed)

| Area | Preferred (KEEP) | Removed |
|------|------------------|---------|
| Mutation | `ref` / `!` / `:=`; `mutable` record fields; `arr.(i) <-` | `let mutable` locals; binding `<-` (non-array) |
| Errors | `match` on `result`; `exception` / `raise` / `try/with`; `failwith` | `?`, `result { }`, `guard`, `panic`, `panic_message` |
| Patterns | `match`, `when`, pattern `as`, `function` | expr `is`, expr `as … -> … else …` |
| Branding | `private type` + single-ctor ADT | `newtype` keyword ([21-branded-ids.md](21-branded-ids.md)) |
| Effects | `effect` / `perform` / handlers | `with { io }` on arrow types |
| Monads / bind | plain `match` on `result` | `result { }`, `async { }`, `region { }`, `let*` / `let+` |
| Blocks | `let … in`, `begin/end`, loops | Offside-as-primary; F# CE braces |
| Integer ops | `mod`, `land`, `lor`, `lxor` | `%` |
| Resources | `try … finally` / linear discharge | `region { let! … }` |

## Quick reference

| Construct | Preferred form | Avoid |
|-----------|----------------|-------|
| File header | `module PascalCase` | Wrong package casing |
| Imports (Go) | `import go "path"` (bare loads `.gosig`; `{ val… }` authoritative) | — |
| Imports (Goop) | `import goop "path"` | Dot-import except std helpers |
| Local binding | `let x = e in body` | — |
| Imperative sequence | `begin s1; s2; result end` | Nested `let () =` |
| Array | `Array.make`, `arr.(i)`, `arr.(i) <-` | `arr[i]` |
| Map | `map[K] V`, `Map.make` / `get` / `add` / `remove` / `mem` / `size` | Hand-rolled Go maps in `@[go]` for ordinary tables |
| Loop | `for i = 0 to n - 1 do … done` | C-style `for` |
| While | `while e do … done` | — |
| Mutation | `let r = ref 0 in r := !r + 1` | `let mutable` |
| Pattern match | `match e with \| C -> …` | Long `if/else if` on ADTs |
| Function match | `function \| C -> …` | — |
| Recoverable error | `('ok, 'err) result` + `match` | `raise` for expected failures |
| Bug / abort | `failwith "msg"` / `raise E` | `panic` |
| Branding | `type order_id = Order_id of string` | `newtype` |
| Pipeline | `x \|> f` | — |
| Concurrency | `go (fun () -> …)`, `go (move x) …` | Capturing `ref` without `move` |

## Profiles

| Construct | Library | Application | LUT / hot path |
|-----------|---------|-------------|----------------|
| Errors | `result` return | `match` at edges | `assert` / `failwith` on invariant |
| Loops | recursion / `List` | `for` / `match` | `for` + `arr.(i) <-` |
| Blocks | `let … in` | `begin/end` for init | `begin/end` for fill kernels |

## LUT decision logic

Prefer `match` + `when` with a small `situation` / `market_state` ADT. Keep mutable array population (`Array.make` + nested `for` + `arr.(i) <-`) for performance. Prefer first-class `map[K] V` + `Map.*` over embedding Go maps in `@[go]` when the table is ordinary Goop data.

## Enforcement

Removed sugar (`let mutable`, `?`, `%`, `newtype`, CEs, Kit macros, …) is rejected
at **parse time** with **PARSE-MIG*** codes — not by `goop fmt`. The formatter is a
pretty-printer only (parse → AST → text); it does not typecheck or lint STYLE.

Money/prices should use `std.decimal` (or integer cents); **DECIMAL001** warns on
float used with money-ish names (`[check] money_float`).
