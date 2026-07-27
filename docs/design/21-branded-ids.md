# 21. Branded IDs (H4)

**Status:** Decided for the 1.0 freeze. Surface shipped; zero-cost codegen planned.

## Decision

| Layer | Choice |
|-------|--------|
| **Language surface** | **(b)** Keep single-constructor ADT + optional `private` |
| **Implementation path** | **(c)** Optimize single-constructor ADTs to zero-cost in codegen where safe |
| **Rejected for 1.0** | **(a)** Restore the `newtype` keyword |

`newtype` stays a PARSE-MIG015 migration error. Tutorials, STYLE, and trading
docs already teach the ADT form; restoring a second branding keyword would
reopen a surface that 1.0 deliberately closed.

## Motivation

Branded IDs (`order_id` ≠ `symbol` ≠ `trade_id`) are a flagship trading-safety
primitive: the type checker rejects accidental swaps of venue identifiers that
share the same representation (`string`, `int`, …).

Lisette offers free tuple-struct newtypes (`struct OrderId(int)`). Goop removed
`newtype` in favor of ordinary ADTs. That is the right *surface* (one ADT
story, OCaml-shaped), but the *lowering* still pays a multi-variant tax for
the single-variant case. H4 closes the positioning gap: own the ADT surface,
then erase the runtime cost when it is safe.

## Idiomatic surface

```goop
(* Public brand — constructor usable from other modules *)
type order_id = Order_id of string

(* Opaque brand — only this module may construct / deconstruct *)
private type client_order_id = Client_order_id of string

type symbol = Symbol of string

let oid = Order_id "ord-1"
let sym = Symbol "ETH-USD"

let id_string (oid : order_id) : string =
  match oid with
  | Order_id s -> s
```

Notes:

- Prefer PascalCase constructors that echo the type name (`Order_id`,
  `Symbol`) so branding reads clearly at call sites.
- Use `private type` when other modules must not forge IDs; export smart
  constructors (`of_string`, `of_venue`) that validate.
- Construction is a normal ADT constructor application; unwrap is ordinary
  `match` (or `function`). There is no special unwrap operator.
- Distinct brands do not unify: `order_id` and `symbol` are different types
  even when both wrap `string`.

See [STYLE.md](STYLE.md), [02-type-system.md](02-type-system.md), and
[12-trading-bot-safety.md](12-trading-bot-safety.md).

## Current codegen (as of this decision)

Single-constructor ADTs use the **same** lowering as multi-constructor ADTs:
one Go interface + one struct per variant + `New…` constructor returning the
interface.

For `type order_id = | Order_id of string`, `goop compile` today emits
approximately:

```go
type order_id interface {
	isorder_id()
}

type order_idOrder_id struct {
	Value string
}

func (order_idOrder_id) isorder_id() {}

func Neworder_idOrder_id(v string) order_id {
	return order_idOrder_id{Value: v}
}
```

Unwrap lowers to a type switch on that interface:

```go
switch v := oid.(type) {
case order_idOrder_id:
	s := v.Value
	// …
}
```

Returning a concrete struct through an interface **boxes** the value. For
hot-path ID plumbing that is a real (if small) tax relative to a bare
`string` / `int64`. Dead `NewtypeTypeKind` AST/codegen still exists and would
emit a Go defined type (`type OrderId string`) — zero-cost at the Go level —
but that path is unreachable from the parser (PARSE-MIG015).

## Planned zero-cost lowering (c)

When **all** of the following hold, codegen may use a transparent Go defined
type instead of the interface + variant struct:

1. The ADT has exactly one constructor (not a GADT, not extensible).
2. The payload is a single primitive or string-like type currently mapped to
   a non-interface Go type: `string`, `int`, `int64`, `float`, `bool`, `byte`,
   `unit` (and similarly named prelude aliases once resolved).
3. The optimization does not change Goop type soundness (brands remain
   distinct at the Goop level; Go defined types stay distinct from their
   representation).

### Target shape

```goop
type order_id = Order_id of string
```

→

```go
type order_id string

func Neworder_idOrder_id(v string) order_id {
	return order_id(v)
}
```

Match `Order_id s`:

```go
s := string(oid)
```

(or an equivalent direct conversion; no type switch, no heap box).

Constructor application stays `Order_id e` in Goop source; only the Go
emission changes. Multi-constructor ADTs, record payloads, tuples, and
nested ADT payloads keep the existing interface lowering.

### Non-goals for the first cut

- Do not restore `newtype` syntax or revive `NewtypeTypeKind` as a user feature.
- Do not change exhaustiveness, privacy, or module visibility rules.
- Do not optimize single-ctor ADTs whose payload is a record, tuple, another
  ADT, `option`/`result`, or an interface-mapped type in the first pass.
- Do not silently change Go FFI shapes that already depend on the interface
  form for a given brand (document escape hatches if needed later).

## Implementation notes / TODO

**TODO (codegen):** implement the single-ctor primitive/string fast path in
`emitADTTypes`, constructor emission, and match lowering. Gate on a helper
such as `isZeroCostBrand(td)` so multi-ctor paths stay untouched.

Risk: match, constructor, and `typeToGo` / `TAdt` lowering all assume the
interface + `findVariantStruct` model. A partial patch that only changes type
emission without match/constructor updates will miscompile. Prefer one
focused PR with golden tests for:

- `type order_id = Order_id of string` → `type order_id string`
- construct + match round-trip
- two brands wrapping the same rep do not share a Go type name collision
- a two-ctor ADT still emits interface + structs (regression guard)

Until that lands, branded IDs are **correct and recommended**; they are not
yet zero-cost. The surface decision does not depend on the optimizer.

After the fast path ships, remove or quarantine dead `NewtypeTypeKind`
handling as ordinary cleanup (not a language change).

## Rejected alternative (a)

Restoring `newtype` would duplicate branding (keyword vs ADT), contradict
STYLE.md and PARSE-MIG015, and force another migration for docs/examples that
already teach `Order_id of string`. Zero-cost belongs in codegen, not in a
second surface.

## Related

- [STYLE.md](STYLE.md) — branding row (single-ctor ADT; no `newtype`)
- [04-go-lowering.md](04-go-lowering.md) — general ADT → interface + structs
- [12-trading-bot-safety.md](12-trading-bot-safety.md) — domain motivation
- `docs/examples/branded_ids.goop` — worked example
