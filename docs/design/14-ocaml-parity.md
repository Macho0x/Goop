# OCaml Parity Matrix (Goop 1.1)

Status legend: `✅` same · `🔄` different-by-design · `➕` Goop extension · `🆕` added · `⚠️` pragmatic subset · `❌` out of scope

## Expressions & control

| OCaml | Goop | Status |
|-------|------|--------|
| `let` / `let rec` / `and` / `in` | Same | ✅ |
| `fun` / `function` | Same | ✅ |
| `if then else` | Same | ✅ |
| `match` / `when` / `as` (pattern) | Same | ✅ |
| `exception` patterns | `exception P` in `match`/`try` | 🆕 |
| `begin … end` | Same | ✅ |
| `for … to/downto … do … done` | Same | 🆕 `downto` |
| `while … do … done` | Same | ✅ |
| `ref` / `!` / `:=` | Same | ✅ |
| `arr.(i)` / `arr.(i) <-` | Same | ✅ |
| `[| … |]` const arrays | Supported | ✅ |
| `lazy` / `Lazy.force` / `Lazy.from_val` | Memoizing `Lazy.t` | 🆕 |
| `try … with` / `raise` / `exception` | Same | ✅ |
| `failwith` | Same | ✅ |
| `\| A \| B ->` or-patterns | Same | ✅ |
| `~label` / `?optional` | Supported (labels erased in Go) | ⚠️ |
| `\|>` | Same | ✅ |
| `let open` / `M.(…)` / `open!` | Same | 🆕 |
| `;;` | Not used (file = module) | 🔄 |
| `//` comments | Supported (+ `(* *)`) | ✅ |
| `[@@…]` / `[@…]` / `[%…]` | Parsed and stripped | 🆕 (no PPX) |

## Types

| OCaml | Goop | Status |
|-------|------|--------|
| ADTs / records / tuples | Same | ✅ |
| `'a option` / `result` | Builtins | ✅ |
| Polymorphic variants | Open/closed rows | ⚠️ |
| GADTs | Constructor result types preserved | ⚠️ |
| Extensible variants | `type t = ..` / `type t +=` | 🆕 |
| Object / class types | Structural methods, inherit | ⚠️ |
| `private` types | Supported | ✅ |
| `where` refinements + SMT | Extended | ➕ |
| Effect handlers (OCaml 5) | Shallow CPS + `continue`/`discontinue` | ⚠️ |

## Modules

| OCaml | Goop | Status |
|-------|------|--------|
| `module M = struct … end` | Supported | ✅ |
| `module M : S = …` sealing | Inline sealing only | 🆕 |
| `module type` / `sig` | Supported | ✅ |
| `module type of` | Supported | 🆕 |
| `module rec` | Supported | 🆕 |
| `with type` / `with module` / `:=` | Parsed + applied | 🆕 |
| `open` / `include` | `include` re-exports; `open` local | ✅ |
| Functors | Supported | ⚠️ |
| First-class modules | Pack/unpack | ⚠️ |
| `.mli` | **Not used** — seal inline | ❌ |
| `import go` / `import goop` | Goop-only | ➕ |
| `go` / `chan` / `move` | Goop-only | ➕ |

## Intentionally different / out of scope

| Topic | Notes |
|-------|-------|
| Effectful codegen | Shallow CPS only; no deep handlers / stack capture |
| Attributes | `[@…]` parse+strip (no PPX); active `@[…]`: `@[go]` / `@[c]` embeds, `@[tag "…"]` on record fields ([33-sdk-blockers.md](33-sdk-blockers.md)) |
| Imports | Go-style paths kept |
| Concurrency | `go` / channels, not OCaml Domains |
| Stdlib | Thin prelude + `import go`; not full OCaml stdlib |
| `.mli` | Prefer `module M : S = …` |

## Removed (non-OCaml duplicates)

`let mutable`, `?`, `result{}`/`async{}`/`region{}`, `is`/`as`/`guard` macros, `newtype`, `with { io }` rows, `panic` keyword, `%` (use `mod`).
