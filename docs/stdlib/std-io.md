# std.io

**Source:** `std/io/io.goop`  
**Import:** `import goop "std.io"` or `import goop . "std.io"`

Wraps Go’s `fmt` package with a Goop-facing API.

## Exports

| Name | Type | Description |
|---|---|---|
| `Println` | `string -> unit` | Print a line to stdout (`fmt.Println`) |

## Example

```goop
module main

import goop . "std.io"

let main () : unit =
  Println "from std.io"
```

## vs prelude `println`

| | `println` | `std.io.Println` |
|---|---|---|
| Import | None (prelude) | `import goop . "std.io"` |
| Naming | `snake_case` | `PascalCase` |
| Lowering | `fmt.Println` | `fmt.Println` (via `@[go]` embed) |

Use whichever fits your module style. Tests: `tests/std_io_test.goop`.

## Internal (not exported)

The module uses `@[go] { func printLine(...) }` internally. `@[go]` embed blocks and their `val` bindings are module-private.
