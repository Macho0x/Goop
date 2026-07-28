# 6. Safety checks

Goop runs a unified safety pipeline on every `goop check` / `goop build` / LSP diagnostic.

## Discarded `result` / `option` (RESULT001 / OPTION001)

Discarding a `result` or `option` in a non-final `begin` statement warns by
default (`[check] discarded_result` / `discarded_option`). Handle with `match`,
or discard explicitly: `let _ = …`.

## Money as float (DECIMAL001)

Float annotations/fields with money-ish names (`price`, `px`, …) warn by default
(`[check] money_float`). Prefer `std.decimal` or integer cents.

## Unused bindings / imports (UNUSED001 / UNUSED002)

Unused locals and Goop imports warn by default (`[check] unused`). Prefer `_`
or `_name` for intentional discards. Unit-typed sequencing bindings
(`let u = assert …`) are not reported.

## Private-in-public (VIS002)

Public APIs that mention a same-module `private` type warn
(`[check] private_in_public`).

## Exhaustive matching (EXHAUST003)

Non-exhaustive `match` is a **compile error** by default:

```
✕ EXHAUST003: non-exhaustive match: missing constructor(s): PartialFill
```

Example: [`trading_order.goop`](../examples/trading_order.goop) — all `OrderAck` variants handled.

## Nominal branding (single-ctor ADT)

Brand primitive types so they cannot be confused (no `newtype` keyword):

```goop
type order_id = Order_id of string
type symbol = Symbol of string

let oid = Order_id "ord-1"
let sym = Symbol "ETH-USD"
```

Raw strings cannot be assigned to `order_id` without the constructor. Single-ctor
brands over primitives/`string` are zero-cost at the Go level (defined types, not
boxed interfaces) — see [21-branded-ids.md](../design/21-branded-ids.md) and
[`branded_ids.goop`](../examples/branded_ids.goop).

## Nil channel detection (NIL001)

Using a channel before initialization is rejected. See [`channel_safety.goop`](../examples/channel_safety.goop).

## Refinement contracts + SMT

The refine pass proves simple arithmetic VCs at compile time. Optional Z3 (`[check] smt = true`) strengthens proofs. **Proven** call sites skip runtime guards; **unproven** sites emit call-site checks; **disproven** sites are compile errors (REFINE001).

```goop
let safeDiv (a: int) (b: int where b <> 0) : int = a / b

let compute (x: int) (y: int) : int =
  if y <> 0 then safeDiv x y else 0   (* proven — no call-site guard *)
```

See [`refinement_solving.goop`](../examples/refinement_solving.goop) and [`contracts.goop`](../examples/contracts.goop).

```toml
[check]
refinement_unproven = "warn"   # warn | error | off
smt = false                    # optional Z3
```

## `goop.toml` check severities

| Key | Default | Controls |
|---|---|---|
| `exhaust_redundant` | `warn` | EXHAUST001/002 |
| `exhaust_missing` | `error` | EXHAUST003 |
| `concurrent` | `error` | LINEAR006/007/008 race warnings |
| `refinement_unproven` | `warn` | REFINE002 unproven VCs |
| `deadlock` | `warn` | DEADLOCK001 channel deadlock lint |
| `discarded_result` | `warn` | RESULT001 |
| `discarded_option` | `warn` | OPTION001 |
| `unused` | `warn` | UNUSED001/002 |
| `private_in_public` | `warn` | VIS002 |
| `money_float` | `warn` | DECIMAL001 |
| `verify_ffi` | `false` | GOSIG003 hand-sig arity check |
| `smt` | `false` | Optional Z3 for refinements |

## Error code reference

- [Trading bot safety matrix](../design/12-trading-bot-safety.md)
- [Full error catalog](../design/10-error-reference.md) (includes PARSE-MIG010–018)

## Next steps

- [Standard library reference](../stdlib/README.md)
- [All examples](../examples/)
- [Contributing](../../CONTRIBUTING.md)
