# 2. Types and patterns

Goop uses algebraic data types (ADTs), records, and exhaustive pattern matching.

## Sum types

```goop
type shape =
  | Circle of { radius: float }
  | Rect of { width: float; height: float }
  | Point
```

Constructors start with a capital letter. Payload variants use `of { fields }`.

## Pattern matching

```goop
let area (s: shape) : float =
  match s with
  | Circle { radius } -> 3.14159 *. radius *. radius
  | Rect { width; height } -> width *. height
  | Point -> 0.0
```

Record patterns destructure fields. The compiler reports **EXHAUST003** if any constructor is missing.

## Records and tuples

```goop
type point = { x: int; y: int }

let origin () : point = { x = 0; y = 0 }
```

Field punning: `{ x; y }` means `{ x = x; y = y }` when `x` and `y` are in scope.

## Lists

Lists are built-in (`'a list`):

```goop
let nums = [1; 2; 3]
let more = 0 :: nums
```

Prelude: `list_length`, `list_append`. Optional `std.list.Map` for higher-order mapping (see [stdlib](../stdlib/std-list.md)).

## Options and results

`option` and `result` are language builtins:

```goop
let parse (s: string) : int option =
  match s with
  | "0" -> Some 0
  | _ -> None
```

```goop
let safeDiv (a: int) (b: int) : int result =
  if b = 0 then Error "division by zero"
  else Ok (a / b)
```

## Branding (no `newtype`)

```goop
type order_id = Order_id of string
type symbol = Symbol of string
```

Single-constructor ADTs over primitive or `string` payloads lower to **zero-cost**
Go defined types (no interface boxing). The surface stays the ADT form — you still
construct with `Order_id "…"`. See [`branded_ids.goop`](../examples/branded_ids.goop),
[STYLE.md](../design/STYLE.md), and [21-branded-ids.md](../design/21-branded-ids.md).

## Active patterns

See [`active_patterns.goop`](../examples/active_patterns.goop). Kit-style `is` / `as` / `guard` macros are removed — use `match`.

## Next

[3. Errors and effects →](03-errors-and-effects.md)
