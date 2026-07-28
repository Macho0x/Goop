# 25. Decimal / money (M1)

**Status:** Working MVP including **cross-module `Decimal` annotations** (1.14).
Library choice: **`shopspring/decimal`**.

When changing this area, follow
[31-language-update-checklist.md](31-language-update-checklist.md).

## Why this exists

Goop’s trading-safety story ([12-trading-bot-safety.md](12-trading-bot-safety.md))
targets compile-time prevention of venue/bot failure modes. IEEE-754 `float64`
is the wrong type for money: binary fractions cannot represent most decimal
cents exactly, so round-trips and comparisons drift.

**Policy:** money, prices, and trading quantities must use fixed-point
`Decimal`, not `float` / `float64`. Lisette is float64-only for money; this
is a domain differentiator.

The trading safety matrix encodes **DECIMAL001** (`[check] money_float`, default
warn) for float used with money-ish names. Prefer `std.decimal` or integer cents
in examples ([`decimal_money.goop`](../examples/decimal_money.goop),
[`orderbook.goop`](../examples/orderbook.goop)).

## Library choice

| Option | Decision |
|--------|----------|
| `shopspring/decimal` | **Chosen** — mature, widely used on Go, value type, clear API |
| Custom big.Rat wrapper | Rejected for MVP (reinvents shopspring) |
| `cockroachdb/apd` | Deferred (heavier; revisit if shopspring precision limits bite) |

## Surface

Goop module: `import goop "std.decimal"` / `import goop . "std.decimal"`.

| Export | Role |
|--------|------|
| `Decimal` | Opaque type — re-exported via `import goop` for annotations / record fields |
| `OfString` / `OfInt` | Construct |
| `Add` / `Sub` / `Mul` / `Div` | Arithmetic |
| `Cmp` / `Equal` / `ToString` | Compare / display |
| Method sugar | `a.Add b`, `a.Cmp b`, `d.String ()` (no `+` overloading) |

Implementation: hand-written `import go "github.com/shopspring/decimal" { … }`
in `std/decimal/decimal.goop`. Codegen emits `type Decimal = decimal.Decimal`
so importers can write `decimal.Decimal` in generated Go.

Go helper: `std/decimal/api.go` re-exports shopspring under
`github.com/Macho0x/Goop/std/decimal` for mixed Go packages.

## Cents vs Decimal

| Use | Prefer |
|-----|--------|
| Money values, display, arithmetic | `std.decimal.Decimal` |
| Order-book ordering, `where n > 0` on prices | integer **cents** (`int`) |

Both avoid float money (DECIMAL001).

## What works today

- `goop check` / `goop build` of `docs/examples/decimal_money.goop` (incl. record fields)
- Cross-module `type Order = { price: Decimal; … }` after `import goop . "std.decimal"`
- Construct from string / int; add / sub / mul / div / cmp / equal / string
- Third-party `import go` packages resolve in the cache sandbox via `go mod tidy`

**Noise:** typecheck may print a `gosig fallback … go.mod file not found` warning
for shopspring because the optional `go/types` loader has no module context at
check time. Hand-written vals still apply; build succeeds after tidy.

## Gaps / scaffolding

| Gap | Status |
|-----|--------|
| Cross-module `Decimal` annotations | **Fixed (1.14)** |
| DECIMAL001 money-float warn | **Shipped** (`[check] money_float`) |
| H5 curated `.gosig` for shopspring | Not ready — hand-written vals |
| H6 `(T, error)` → `result` | Not ready — `OfString` panics on bad input |
| Prelude auto-open of `Decimal` | Deferred — use `import goop "std.decimal"` |
| Operator overloading (`+` on Decimal) | Out of scope — method-call sugar only |
| Wire Go helper as the sole `import go` target | Scaffold (`api.go`); codegen alias also exports |

## Example

See [`docs/examples/decimal_money.goop`](../examples/decimal_money.goop) and
[`docs/stdlib/std-decimal.md`](../stdlib/std-decimal.md).

```bash
./goop build docs/examples/decimal_money.goop && ./goop-out
```
