# 25. Decimal / money (M1)

**Status:** Scaffold + working MVP (2026-07-27). Library choice: **`shopspring/decimal`**.

## Why this exists

Goop’s trading-safety story ([12-trading-bot-safety.md](12-trading-bot-safety.md))
targets compile-time prevention of venue/bot failure modes, but prices and
sizes in examples still use `float`. IEEE-754 `float64` is the wrong type for
money: binary fractions cannot represent most decimal cents exactly, so
round-trips and comparisons drift.

**Policy:** money, prices, and trading quantities must use fixed-point
`Decimal`, not `float` / `float64`. Lisette is float64-only for money; this
is a domain differentiator.

The trading safety matrix does not yet encode a DECIMAL001 lint; until it
does, this is a documented convention enforced by reviews and examples.

## Library choice

| Option | Decision |
|--------|----------|
| `shopspring/decimal` | **Chosen** — mature, widely used on Go, value type, clear API |
| Custom big.Rat wrapper | Rejected for MVP (reinvents shopspring) |
| `cockroachdb/apd` | Deferred (heavier; revisit if shopspring precision limits bite) |

## Surface (MVP)

Goop module: `import goop "std.decimal"` / `import goop . "std.decimal"`.

| Export | Role |
|--------|------|
| `OfString` / `OfInt` | Construct |
| `Add` / `Sub` / `Mul` / `Div` | Arithmetic |
| `Cmp` / `Equal` / `ToString` | Compare / display |
| Method sugar | `a.Add b`, `a.Cmp b`, `d.String ()` (no `+` overloading) |

Implementation: hand-written `import go "github.com/shopspring/decimal" { … }`
in `std/decimal/decimal.goop`. H5 auto-sigs are not used yet.

Go helper: `std/decimal/api.go` re-exports shopspring under
`github.com/Macho0x/Goop/std/decimal` for mixed Go packages and as the
intended long-term import boundary. Cache builds of the Goop module still
import shopspring **directly**; `goop build` runs `go mod tidy` in the
sandbox so the third-party module resolves.

## What works today

- `goop check` / `goop build` of `docs/examples/decimal_money.goop`
- Construct from string (`RequireFromString`) and int
- Add / sub / mul / div / cmp / equal / string via methods and wrappers
- Third-party `import go` packages resolve in the cache sandbox via `go mod tidy`

**Noise:** typecheck prints a `gosig fallback … go.mod file not found` warning
for shopspring because the optional `go/types` loader has no module context at
check time. Hand-written vals still apply; build succeeds after tidy.

## Gaps / scaffolding

| Gap | Status |
|-----|--------|
| H5 curated `.gosig` for shopspring | Not ready — hand-written vals |
| H6 `(T, error)` → `result` | Not ready — `OfString` panics on bad input |
| Prelude auto-open of `Decimal` | Deferred — use `import goop "std.decimal"` |
| Operator overloading (`+` on Decimal) | Out of scope — method-call sugar only |
| DECIMAL001 / typecheck ban of float money | Not implemented |
| Wire Go helper as the sole `import go` target | Scaffold only (`api.go` + `go.mod`) |
| Migrate `orderbook.goop` prices off `float` | Follow-up |

## Example

See [`docs/examples/decimal_money.goop`](../examples/decimal_money.goop) and
[`docs/stdlib/std-decimal.md`](../stdlib/std-decimal.md).

```bash
./goop build docs/examples/decimal_money.goop && ./goop-out
```
