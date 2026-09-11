# std-array / Array builtins

Goop provides OCaml-style arrays via prelude bindings (no import required). The `std.array` module is a thin re-export for import-style access.

**Import (optional):** `import goop "std.array"` or `import goop . "std.array"`

## Types

```goop
int array          // int array
decision array     // user type T array
```

## Functions

| Name | Type | Go lowering | Module |
|------|------|-------------|--------|
| `Array.make` / `make` | `int -> 'a -> 'a array` | `make([]T, n)` + fill loop | prelude / `std.array` |
| `Array.length` / `length` | `'a array -> int` | `len(arr)` | prelude / `std.array` |

## Operators

| Syntax | Meaning |
|--------|---------|
| `arr.(i)` | Index read |
| `arr.(i) <- v` | In-place write (array cells are mutable) |

## Example

```goop
let lut = Array.make 100 0 in
for i = 0 to 99 do
  lut.(i) <- compute i
done
```

With `std.array`:

```goop
import goop . "std.array"

let xs = make 3 0
```

See [trading_decision_lut.goop](../examples/trading_decision_lut.goop).
