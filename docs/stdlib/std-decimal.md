# std.decimal

**Source:** `std/decimal/decimal.goop`  
**Import:** `import goop "std.decimal"` or `import goop . "std.decimal"`  
**Design:** [25-decimal.md](../design/25-decimal.md)

Fixed-point decimal for **money and trading quantities**. Do **not** use
`float` / `float64` for prices or sizes — binary floats cannot represent most
decimal amounts exactly (see [trading bot safety](../design/12-trading-bot-safety.md)
and the decimal design note).

**Dependency:** [shopspring/decimal](https://github.com/shopspring/decimal) v1.4.x
(via `import go`; resolved by `go mod tidy` in the build sandbox).

## Exports

| Name | Type | Description |
|---|---|---|
| `Decimal` | opaque type | Available under `import goop . "std.decimal"` for annotations |
| `OfString` | `string -> Decimal` | Parse decimal text (`RequireFromString`; panics on bad input) |
| `OfInt` | `int -> Decimal` | From integer |
| `Add` / `Sub` / `Mul` / `Div` | `Decimal -> Decimal -> Decimal` | Arithmetic |
| `Cmp` | `Decimal -> Decimal -> int` | `-1` / `0` / `+1` |
| `Equal` | `Decimal -> Decimal -> bool` | Exact equality |
| `ToString` | `Decimal -> string` | Display |

Opaque type `Decimal` is re-exported through `import goop` (typecheck + codegen).
You can use it in record fields and annotations:

```goop
import goop . "std.decimal"

type Quote = { price: Decimal; qty: int }

let q = { price = OfString "99.50"; qty = 1 } in
...
```

Method-call sugar works:

```goop
let total = price.Add fee in
let ok = total.equal (OfString "11.50") in
...
```

There is no `+` / `*.` overloading for `Decimal`.

Integer **cents** remain fine when ordering / refinements want `int`
(see [`orderbook.goop`](../examples/orderbook.goop)).

## Example

```goop
module main

import goop . "std.decimal"

let main () : unit =
  let price = OfString "10.00" in
  let fee = OfString "1.50" in
  let total = price.Add fee in
  println (ToString total)
```

Full demo (including a `Decimal` record field):
[`docs/examples/decimal_money.goop`](../examples/decimal_money.goop).

## Status

Working: construct, arithmetic, compare, display, and **cross-module**
`Decimal` annotations via `import goop`. Not yet: H5 auto-sigs for shopspring,
H6 `(Decimal, error)` → `result` for `OfString`, or prelude auto-import.

Go helper for mixed Go packages: `std/decimal/api.go` (cache builds also emit a
`type Decimal = …` alias from the Goop module).
