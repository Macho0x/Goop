# Goop Syntax (1.0 — OCaml-aligned)

Normative surface for Goop 1.0. Prefer OCaml spelling for every construct; see [STYLE.md](STYLE.md) and [14-ocaml-parity.md](14-ocaml-parity.md).

Goop is case-sensitive. Blocks use explicit delimiters (`let … in`, `begin/end`, `for/do/done`, `while/do/done`). Indentation is stylistic, not structural.

## Comments

```goop
(* Block comments; nested (* ok *). *)
// Line comments also supported
```

## String and character literals

Double-quoted strings and character literals support escapes decoded by the lexer:

| Escape | Meaning |
|--------|---------|
| `\n` `\t` `\r` `\\` `\"` `\'` | usual |
| `\e` | ESC (`0x1b`) |
| `\xHH` | byte from exactly two hex digits |
| `\ooo` | byte from 1–3 octal digits (value ≤ 255) |

```goop
let red = "\x1b[31m"
let reset = "\033[0m"
let dim = "\e[2m"
```

Invalid or incomplete escapes are lexer errors (see [10-error-reference.md](10-error-reference.md)).

## Identifiers and keywords

- Identifiers: letter or `_`, then letters, digits, `_`, or `'`.
- Type variables: `'a`, `'key`.
- Keywords include: `let`, `type`, `match`, `with`, `if`, `then`, `else`, `fun`, `function`, `module`, `import`, `go`, `goop`, `mutable` (record fields only), `rec`, `and`, `in`, `as` (patterns), `when`, `true`, `false`, `unit`, `private`, `go`, `move`, `chan`, `ref`, `while`, `for`, `do`, `done`, `begin`, `end`, `try`, `raise`, `exception`, `failwith`, `effect`, `perform`, `mod`, `open`, `include`, `struct`, `sig`, `functor`, `class`, `object`, `new`, `lazy`.

Removed (PARSE-MIG010–018): `let mutable`, `?`, computation expressions, `is`/`guard`/expr `as`, `newtype`, `with { io }` rows, `panic`, `%`.

## Modules and imports

```goop
module MyModule

import (
  go "fmt"
  goop . "std.io"
)

let x = 1
```

Keep Go-style `import go` / `import goop`. Nested `module M = struct … end`, `sig`, functors, and inline `module M : S = …` sealing are supported. There is no `.mli` — see [05-modules-and-packages.md](05-modules-and-packages.md) and [14-ocaml-parity.md](14-ocaml-parity.md).

## Value declarations

```goop
let pi = 3.14159

let square (x: float) : float = x *. x

let rec factorial (n: int) : int =
  if n <= 1 then 1
  else n * factorial (n - 1)
```

### Mutation: `ref` / `!` / `:=`

```goop
let r = ref 0 in
r := !r + 1
```

Record fields may be `mutable`. Array cells use `arr.(i) <- v`. There is no `let mutable` and no binding `<-` (PARSE-MIG010/011).

## Functions

Curried by default. Anonymous `fun` and pattern-matching `function`:

```goop
let add (x: int) (y: int) : int = x + y
let f = fun x -> x + 1
let g = function | Some x -> x | None -> 0
```

## Type declarations

```goop
type point = { x: float; y: float }
type option 'a = None | Some of 'a
type handle : 1   (* linear resource *)
```

### Branding (no `newtype`)

```goop
type order_id = Order_id of string
(* or private type order_id = Order_id of string *)
```

## Pattern matching

```goop
match expr with
| Pattern1 -> expr1
| Pattern2 when guard -> expr2
| _ -> default_expr
```

Patterns: `_`, literals, constructors, tuples, lists, records, `as` aliases, `when` guards, or-patterns. No expr-level `is` / `as` / `guard` macros.

## Exceptions

```goop
exception Boom
exception Fail of string

let f () =
  try
    raise (Fail "oops")
  with
  | Fail msg -> msg
  | Boom -> "boom"
```

Bugs: `failwith "msg"` (lowers to Go `panic`). Recoverable domain errors: `('ok, 'err) result` + `match`.

## Effects (OCaml 5-style, minimal)

```goop
effect Flip : unit -> bool

(* perform / handlers; effectful code may CPS-lower — see 06-effects-and-safety.md *)
```

No `with { io }` effect rows on arrow types.

## Pipelines and operators

```goop
data |> List.filter (fun x -> x > 0) |> List.map (fun x -> x * 2)
```

Integer remainder: `mod` (not `%`). Also `land` / `lor` / `lxor`.

## Refinement `where`

```goop
let safeDiv (a: int) (b: int where b <> 0) : int = a / b
```

SMT (optional Z3) proves VCs when possible; otherwise runtime guards. See [02-type-system.md](02-type-system.md).

## Go interop

```goop
import (
  go "github.com/example/lib" {
    val loadConfig : string -> (config, error) result
  }
)

@[go] {
  func helper() int { return 42 }
}
val helper : unit -> int
```

## Concurrency

```goop
let _ = go (fun () -> println "hello")

let r = ref 0 in
let _ = go (move r) (fun () -> r := !r + 1)
```

`go` / `go (move …)` and channels are intentional Goop extensions.

## Arrays, loops, sequencing

```goop
let arr = Array.make 10 0 in
arr.(0) <- 42

for i = 0 to n - 1 do
  arr.(i) <- f i
done

while !r < 3 do
  r := !r + 1
done

begin
  println "step 1";
  42
end
```

## Maps

```goop
let m : map[string] int = Map.make () in
let _ = Map.add m "x" 1 in
match Map.get m "x" with
| Some n -> println (int_to_string n)
| None -> ()
```

- Spelling: `map[K] V`. Arrows bind outside (`map[string] int -> unit`).
- Function-valued maps need parentheses: `map[string] (int -> int)`.
- Prelude: `Map.make` / `get` / `add` / `remove` / `mem` / `size` (see [29-maps.md](29-maps.md)).

Bare `import go "pkg"` (no `{ val … }` block) loads `.gosig` stubs from
`goop-sigs/` → `$GOOP_HOME` cache → curated generate-on-miss; explicit blocks
stay authoritative ([28-go-sig-resolution.md](28-go-sig-resolution.md)).

## Modules / OOP / effects (brief)

Minimal OCaml forms: nested `struct`/`sig`/functors, `class`/`object`/`new`, `effect`/`perform`/handlers. Prefer [STYLE.md](STYLE.md) for everyday code; see [14-ocaml-parity.md](14-ocaml-parity.md) for the parity matrix.
