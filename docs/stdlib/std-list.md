# std.list

**Source:** `std/list/list.goop`  
**Import:** `import goop "std.list"` or `import goop . "std.list"`

## Exports

| Name | Type | Description |
|---|---|---|
| `Map` | `('a -> 'b) -> 'a list -> 'b list` | Map a function over every element |
| `Filter` | `('a -> bool) -> 'a list -> 'a list` | Keep elements where `f` is true |
| `Fold` | `('acc -> 'a -> 'acc) -> 'acc -> 'a list -> 'acc` | Left fold |

## Implementation

Recursive `match` on `[]` and `::`:

```goop
let rec Map (f: 'a -> 'b) (xs: 'a list) : 'b list =
  match xs with
  | [] -> []
  | x :: rest -> f x :: Map f rest
```

## Example

```goop
import goop . "std.list"

let doubleAll (xs: int list) : int list =
  Map (fun x -> x + x) xs
```

List construction (`[]`, `::`) is builtin — this module adds `Map`, `Filter`,
and `Fold` only. `goop check` covers them; polymorphic lowering through a
Goop module still uses `interface{}`, so prefer local `match` in `goop build`
hot paths.
