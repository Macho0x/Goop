# Trading bot safety matrix (Goop 1.0)

Goop targets compile-time prevention of common trading-bot failure modes.

## Bug class → error code

| Bug class | Code | When |
|-----------|------|------|
| Missing venue ack variant | EXHAUST003 | Non-exhaustive `match` on ADT |
| Dead match arm / wildcard | EXHAUST001/002 | Unreachable patterns |
| Nil channel before init | NIL001 | `Chan.send`/`recv` before `Chan.make` |
| Shared `ref` / mutable race | LINEAR006/007 | Linear + goroutine analysis |
| Channel-mediated mutable race | LINEAR008 | `Chan.send` of shared binding still in parent scope |
| Potential channel deadlock | DEADLOCK001 | Circular send/recv between two goroutines |
| Send after close | LINEAR002 | Owned channel linearity |
| Leaked owned channel | LINEAR001 | Channel not discharged |
| Refinement violated at call | REFINE001 | VC disproves precondition |
| Unproven refinement | REFINE002 | Runtime guard emitted |
| Branded ID mix-up | type error | `order_id` vs `symbol` single-ctor ADTs |

## Compile-time vs runtime

- **Compile-time:** exhaustiveness, nil-channel (conservative), linearity, branded ADTs, proven refinements (interval + optional Z3), narrow channel deadlock lint (DEADLOCK001).
- **Runtime:** unproven refinements (REFINE002 guards), external I/O failures, venue API semantics, total deadlocks (Go runtime).

## Recommended patterns

1. Model venue messages as ADTs; handle with exhaustive `match`.
2. Brand IDs: `type order_id = Order_id of string` (no `newtype` keyword; see [21-branded-ids.md](21-branded-ids.md)).
3. Pass position state through `owned_chan` or single-owner linear types.
4. Prefer `result` + `match` for venue errors; `failwith` for bugs.
5. Initialize feeds with `let ch = Chan.make ()` before any send/recv.
6. Transfer shared `ref` into goroutines with `go (move r)` when the parent is done.

See [STYLE.md](STYLE.md), `docs/examples/trading_*.goop`, and `orderbook.goop` for worked examples.

**Money / prices:** use fixed-point `std.decimal` (`Decimal`) for money values
and arithmetic; integer **cents** are fine for order-book ordering and
`where n > 0` refinements. See [25-decimal.md](25-decimal.md). Do not use
`float64` for money (DECIMAL001).
