# std.string

**Source:** `std/string/string.goop`  
**Import (optional):** `import goop "std.string"` or `import goop . "std.string"`

Thin re-export of prelude string helpers for import-style access. Byte indexing matches Go (`len` / `s[i:i+n]`), not Unicode grapheme clusters.

## Exports

| Name | Type | Prelude equivalent |
|---|---|---|
| `length` | `string -> int` | `String.length` |
| `sub` | `string -> int -> int -> string` | `String.sub` |
| `concat` | `string -> string -> string` | `string_concat` (prefer `^` in source) |
| `of_int` | `int -> string` | `int_to_string` |
| `of_float` | `float -> string` | `float_to_string` |

## Example

```goop
import goop . "std.string"

let label (n: int) : string =
  concat "n=" (of_int n)
```

See also [prelude strings](prelude.md#strings-and-numbers). For richer string APIs, use Go’s `strings` / `strconv` via `import go`.
