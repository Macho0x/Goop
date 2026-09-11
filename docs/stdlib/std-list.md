# std.list

**Source:** `std/list/list.goop`  
**Import:** `import goop "std.list"` or `import goop . "std.list"`

Combinators live in the **prelude** as `List.filter` / `List.map` / `List.fold` /
`List.find` (Go generics — `goop build` works). This module re-exports
`Filter` / `Map` / `Fold` as thin aliases of those helpers.

## Exports

| Name | Type | Description |
|---|---|---|
| `Filter` | `('a -> bool) -> 'a list -> 'a list` | Keep elements where `f` is true |
| `Map` | `('a -> 'b) -> 'a list -> 'b list` | Map a function over every element |
| `Fold` | `('acc -> 'a -> 'acc) -> 'acc -> 'a list -> 'acc` | Left fold |

Prefer prelude spelling in new code:

```goop
let evens = List.filter (fun n -> n mod 2 = 0) xs
let evens2 = xs.filter (fun n -> n mod 2 = 0)
```

List construction (`[]`, `::`) is builtin. Keep this module thin.
