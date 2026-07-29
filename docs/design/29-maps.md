# 29 — Maps (`map[K] V`)

Status: implemented (language + gosiggen)

## Surface

```goop
let m : map[string] int = Map.make () in
let _ = Map.add m "x" 1 in
match Map.get m "x" with
| Some n -> println (int_to_string n)
| None -> ()
```

- Spelling: `map[K] V` (Go-like brackets for the key; value follows).
- Arrows bind outside: `map[string] int -> unit` means `(map[string] int) -> unit`.
- Function-valued maps need parentheses: `map[string] (int -> int)`.

## Prelude

| Binding | Type | Effect |
|---------|------|--------|
| `Map.make` | `unit -> map['k] 'v` | pure |
| `Map.get` | `map['k] 'v -> 'k -> 'v option` | pure |
| `Map.add` | `map['k] 'v -> 'k -> 'v -> unit` | pure (mutates) |
| `Map.remove` | `map['k] 'v -> 'k -> unit` | pure (mutates) |
| `Map.mem` | `map['k] 'v -> 'k -> bool` | pure |
| `Map.size` | `map['k] 'v -> int` | pure |

Maps are Go reference types: `Map.add` / `Map.remove` mutate in place and return `unit`.

## Lowering

`map[K] V` → Go `map[K]V`. Ops lower to `make`, index assign, `delete`, comma-ok lookup, `len`.

## FFI

gosiggen emits `map[K] V` for Go `map[K]V` when key and value are representable.
