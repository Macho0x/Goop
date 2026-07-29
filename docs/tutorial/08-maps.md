# 8. Maps

Goop has a first-class `map[K] V` type that lowers to a Go `map[K]V`. Use the
prelude `Map.*` helpers — no `@[go]` required for ordinary table lookups.

See [STYLE.md](../design/STYLE.md) and [29-maps.md](../design/29-maps.md).

## Create and update

```goop
let table : map[string] int = Map.make () in
let _ = Map.add table "foo" 1 in
let _ = Map.add table "bar" 2 in
()
```

Maps are Go reference types: `Map.add` / `Map.remove` mutate in place and
return `unit`.

## Lookup

```goop
match Map.get table "foo" with
| Some n -> println (int_to_string n)
| None -> println "missing"
```

| Binding | Type | Notes |
|---|---|---|
| `Map.make` | `unit -> map['k] 'v` | Empty map |
| `Map.get` | `map['k] 'v -> 'k -> 'v option` | Missing key → `None` |
| `Map.add` | `map['k] 'v -> 'k -> 'v -> unit` | Insert / overwrite |
| `Map.remove` | `map['k] 'v -> 'k -> unit` | Delete key |
| `Map.mem` | `map['k] 'v -> 'k -> bool` | Membership |
| `Map.size` | `map['k] 'v -> int` | Entry count |

## Type spelling

- Write `map[K] V` (Go-like brackets for the key; value follows).
- Arrows bind outside: `map[string] int -> unit` means `(map[string] int) -> unit`.
- Function-valued maps need parentheses: `map[string] (int -> int)`.

## Optional `std.map` import

```goop
import goop . "std.map"

let m = make ()   (* same as Map.make *)
```

## Runnable example

See [`maps.goop`](../examples/maps.goop):

```bash
./goop check docs/examples/maps.goop
```

## Common errors

| Message | Cause | Fix |
|---|---|---|
| `type mismatch` on `Map.get` | Wrong key or value type | Annotate `map[K] V` or check call args |
| Unexpected parse around `->` | Function-valued map without parens | Use `map[string] (int -> int)` |
| Unbound `Map` | Typo / shadowed name | Prelude `Map` is always in scope |

Full catalog: [error reference](../design/10-error-reference.md).

## Next steps

- [Go / C interop](04-go-interop.md) — maps also appear in Go stubs
- [Writing tools](../guides/writing-tools.md) — files + maps via `import go`
- [Maps design](../design/29-maps.md)
