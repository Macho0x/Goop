# Goop Type System

## Design goals

1. Sound, global type inference (Hindley-Milner style).
2. Null safety through explicit `option`.
3. Exhaustive pattern matching over algebraic data types.
4. Explicit, typed error handling through `result` (plus OCaml exceptions for bugs).
5. Parametric polymorphism without higher-kinded types.
6. Optional SMT-backed refinements and OCaml 5-style effect handlers (minimal).

## Core types

### Primitive types

| Goop type | Lowered Go type | Notes |
|---|---|---|
| `int` | `int` | Platform word size |
| `int8` … `int64` | same | Signed fixed-width |
| `uint` … `uint64` | same | Unsigned fixed-width |
| `float` | `float64` | Default float |
| `float32` | `float32` | |
| `bool` | `bool` | |
| `string` | `string` | Immutable UTF-8 |
| `rune` | `rune` | Unicode code point |
| `bytes` | `[]byte` | Mutable byte slice |
| `unit` | `struct{}` | The `()` value |
| `'a ref` | `*T` | Mutable reference cell |

### Type variables

```goop
let identity (x: 'a) : 'a = x
let pair (a: 'a) (b: 'b) : 'a * 'b = (a, b)
```

### Tuples

```goop
let p : int * string = (42, "hello")
let (x, y) = p
```

## Algebraic data types

```goop
type color = Red | Green | Blue

type shape =
  | Circle of { radius: float }
  | Rect of { width: float; height: float }
  | Point
```

### Branding (private + single-ctor ADT)

There is no `newtype` keyword. Brand IDs with a single-constructor ADT (optionally `private`):

```goop
type order_id = Order_id of string
type symbol = Symbol of string

let oid = Order_id "ord-1"
```

Single-constructor ADTs over primitive or `string` payloads lower to zero-cost
Go defined types where safe ([21-branded-ids.md](21-branded-ids.md)).

### Maps

`map[K] V` is a first-class type that lowers to Go `map[K]V`. Keys and values
are ordinary Goop types; prelude `Map.*` covers make/get/add/remove/mem/size.
See [29-maps.md](29-maps.md).

### Record types

```goop
type point = { x: float; y: float }
let origin : point = { x = 0.0; y = 0.0 }
let moved = { origin with x = 1.0 }
```

### Row polymorphism (optional)

```goop
let print_name (r: { name: string | .. }) : unit =
  print_line r.name
```

## `option` and `result`

```goop
type 'a option = None | Some of 'a
type ('ok, 'err) result = Ok of 'ok | Error of 'err
```

No `null`. Recoverable errors use `result` + `match`. Bugs use `failwith` / `raise`.

## Pattern matching

`match` is exhaustive. Patterns may nest, use `when` guards, or-patterns, and active patterns.

```goop
let (|Positive|_|) (n: int) : int option =
  if n > 0 then Some n else None
```

## Generics

Parametric polymorphism only — no higher-kinded types (matches Go generics).

## Effect handlers (shipped, minimal)

Goop 1.0 supports OCaml 5-style `effect` / `perform` / handlers. Effectful code may be CPS-transformed before codegen (not idiomatic Go). Pure code stays direct-style.

```goop
effect Flip : unit -> bool
```

Surface `with { io }` effect **rows** on arrow types are removed (PARSE-MIG016). Internal effect tags on prelude bindings still guide inference where applicable.

See [06-effects-and-safety.md](06-effects-and-safety.md) and `docs/examples/effects.goop`.

## Refinement contracts + SMT

`where` clauses on parameters and returns are refinement contracts. Goop 1.0 ships:

1. **Built-in interval / VC solver** for simple integer arithmetic.
2. **Optional Z3 SMT** (`[check] smt = true` in `goop.toml`) for stronger proofs.
3. **Runtime guards** as fallback when a VC is unproven.

```goop
let safeDiv (a: int) (b: int where b <> 0) : int = a / b

let clamp (x: int) (lo: int) (hi: int where hi >= lo) : int
  where result >= lo && result <= hi = ...
```

- Proven call sites may skip guards; disproven sites are compile errors (REFINE001).
- Exported entry points retain guards for FFI safety.

See `docs/examples/contracts.goop` and [08-deferred-features-analysis.md](08-deferred-features-analysis.md).

## Linear resource types

```goop
type handle : 1
```

Flow-sensitive discharge checking (`internal/linear`). First use = hand-off. Erased in Go output.

## What is intentionally absent

- **Null.** Use `option`.
- **Implicit conversions.** All numeric coercions are explicit.
- **Higher-kinded types.** Go cannot express them.
- **Full dependent types (Idris/Agda style).** SMT-backed refinements cover the practical fragment; full dependent programming is out of scope.
- **Borrow checker with lifetimes (Rust style).** Opt-in linear `: 1` only.
- **Implicit side effects.** Prefer `result` / handlers / explicit IO over hidden mutation.

## Type inference

Hindley-Milner with let-polymorphism. Top-level declarations may omit types; exported functions generally need annotations at module boundaries.

Bootstrap layers:

1. Local constraint solving for monomorphic expressions.
2. Unification-based polymorphism for `let` bindings.
3. Optional `gopls` fallback for hard higher-order cases.
4. Refinement VC generation + optional Z3.
