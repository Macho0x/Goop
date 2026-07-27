# std.list

**Source:** `std/list/list.goop`  
**Import:** `import goop "std.list"` or `import goop . "std.list"`

## Exports

| Name | Type | Description |
|---|---|---|
| `Map` | `('a -> 'b) -> 'a list -> 'b list` | Map a function over every element |

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

List construction (`[]`, `::`) is builtin — this module only adds `Map`.
That small surface is intentional under the [`std.*` doctrine](README.md#doctrine): prefer writing `match` over growing a Go-stdlib-sized list package.
