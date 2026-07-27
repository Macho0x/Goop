# Writing tools in Goop

Goop’s `std.*` modules stay thin (OCaml/Goop-native helpers). For files,
paths, buffers, and JSON, use Go’s stdlib via `import go`.

## Prerequisites

Generate or override stubs once:

```bash
goop gen-sig --smoke          # strings, fmt, errors, strconv
goop gen-sig os path/filepath bytes bufio encoding/json
```

Project overrides in `goop-sigs/` win over `$GOOP_HOME/build/go-sigs/`.
On `goop check` / `goop build`, curated packages auto-load (and may
auto-generate into the cache on miss).

## Minimal file + map example

See [docs/examples/writing_tools.goop](../examples/writing_tools.goop):

```goop
module main

import go "os"

let main () =
  begin
    let table : map[string] int = Map.make () in
    let _ = Map.add table "ok" 0 in
    match os.ReadFile "go.mod" with
    | Ok _ ->
        begin
          let _ = Map.add table "ok" 1 in
          print_line "read ok"
        end
    | Error _ ->
        print_line "read failed"
  end
```

Notes:

- Prefer stubs from `.gosig` so you can write `import go "os"` without a
  hand-written `{ val … }` block once the cache is warm.
- `(T, error)` returns coerce to `('ok, error) result` (H6) unless you use
  `import go raw`.
- Maps are first-class: `map[K] V` lowers to Go `map[K]V`.

## Doctrine

Do **not** add `std.fs` / `std.net` / `std.json`. Keep using `import go`
for those surfaces ([docs/stdlib/README.md](../stdlib/README.md)).
