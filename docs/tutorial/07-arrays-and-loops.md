# 7. Arrays and loops

Goop uses **OCaml-style** arrays and imperative loops for LUTs, buffers, and in-place updates. See [STYLE.md](../design/STYLE.md).

## Dynamic arrays

Type: `'a array` (postfix, like OCaml `int array`).

```goop
let arr = Array.make 10 0 in
assert (Array.length arr = 10 && arr.(0) = 0)
```

| Syntax | Meaning |
|---|---|
| `Array.make n default` | Allocate `n` slots, each initialized to `default` |
| `Array.length arr` | Number of elements |
| `arr.(i)` | Read index `i` (`int`) |
| `arr.(i) <- v` | Write index `i` in place |

Element writes do **not** require a `ref` for the array binding — the slice cells are mutable. Use `ref` only if you need to rebind the array variable itself.

See [`arrays.goop`](../examples/arrays.goop) and [`trading_decision_lut.goop`](../examples/trading_decision_lut.goop).

## Filling a LUT with `for`

```goop
let lut = Array.make 100 default in
for i = 0 to 99 do
  lut.(i) <- compute i
done
```

- Bounds `from` and `to` must be `int`.
- The loop is **inclusive** on both ends.
- The loop variable is visible only inside `do ... done`.

## `while` and `ref`

```goop
let r = ref 0 in
while !r < 3 do
  r := !r + 1
done
```

## Sequencing with `begin` / `end`

```goop
let sum =
  begin
    arr.(0) <- 1;
    arr.(1) <- 2;
    arr.(0) + arr.(1)
  end
```

## Qualified constructors

```goop
type Color = Red | Green | Blue

let label c =
  match c with
  | Color.Red -> "red"
  | Color.Green -> "green"
  | Color.Blue -> "blue"
```

## Optional `std.array` import

```goop
import goop . "std.array"

let xs = make 3 0   (* same as Array.make *)
```

See [std.array](../stdlib/std-array.md).

## Common errors

| Message | Cause | Fix |
|---|---|---|
| PARSE-MIG010 / MIG011 | `let mutable` or binding `<-` | Use `ref` / `:=` / `!`; keep `arr.(i) <-` |
| `type mismatch` on `arr.(i)` | Index or array type wrong | Ensure `i : int` and `arr : 'a array` |
| `expected DONE, got ...` | Missing `done` after `for`/`while` | Close with `done` |

Full catalog: [error reference](../design/10-error-reference.md).

## Next steps

- [Maps →](08-maps.md)
- [Safety checks](06-safety-checks.md)
- [OCaml surface syntax](../design/13-ocaml-surface-syntax.md)
- [Go lowering](../design/04-go-lowering.md)
