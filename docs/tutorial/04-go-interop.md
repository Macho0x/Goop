# 4. Go / C interop

Goop’s primary standard library is **Go itself**. Use `import go` for packages and `@[go]` for inline Go. For C, use `@[c]` (cgo-shaped).

Lowering is **one-way** (Goop → Go). There is no Go → Goop auto-translator.
Compare ideas side-by-side in the [snippet gallery](../examples/gallery/), or
inspect real codegen with `goop compile --stdout` / the playground **Compile** pane.

## Import Go packages

```goop
module main

import (
  go "strings" {
    val ToUpper : string -> string
  }
  go "fmt"
)
```

- **Signature block** — declare types for functions you call from Goop. Explicit
  `{ val … }` / `{ type … }` blocks are **authoritative**: stubs are not merged
  on top of them.
- **Bare import** — `go "fmt"` with no block loads `.gosig` stubs automatically:
  1. project `goop-sigs/` override (wins),
  2. `$GOOP_HOME/build/go-sigs/` cache,
  3. curated generate-on-miss for known packages.

Generate or refresh stubs with `goop gen-sig` / `goop get-go-sig`. For a walkthrough
(files + maps without hand-writing every `val`), see
[Writing tools](../guides/writing-tools.md) and
[`writing_tools.goop`](../examples/writing_tools.goop). Resolution details:
[28-go-sig-resolution.md](../design/28-go-sig-resolution.md).

## Call Go methods and read Go fields

Import only the selectors you need. The receiver appears in the declaration,
but not in the type after `:`:

```goop
import go "bytes" {
  type Buffer
  val (b : Buffer ptr).String : unit -> string
}

let text (b : Buffer ptr) = b.String ()
```

Heap / mutable Go types whose methods use pointer receivers should be typed
as `T ptr` (e.g. `Buffer ptr` for `*bytes.Buffer`). Opaque `type Buffer` alone
maps to the Go value type `bytes.Buffer`.

Construct empty Go structs and option bags with expected-typed `ptr_of`:

```goop
let buf : Buffer ptr = ptr_of {}
let opts : HandlerOptions ptr = ptr_of { level = LevelInfo }
```

Pass implementors as `ptr_of { … }` wherever a Go interface is expected
(`slog.New (ptr_of { last = "" })`).

An arrow type declares a method; a non-arrow type declares a field:

```goop
import go "log/slog" {
  type Attr
  val (a : Attr).Key : string
}
```

For Go slices, use `'a go_slice`; `xs.(i)` lowers to
`go_slice_get xs i`, and `spread xs` passes a slice to a variadic Go method or
function. Use `any_of value` before collecting mixed values into an
`any go_slice`.

## Implement Go interfaces

Import an interface as an opaque Go type, then use `implements` to generate
its pointer-receiver method set from native Goop methods:

```goop
import go "fmt" {
  type Stringer
}

type point = { x : int; y : int }

implements Stringer for point with
  let String (p : point) : string =
    int_to_string p.x ^ "," ^ int_to_string p.y
end
```

This emits a Go assertion that `*point` satisfies `fmt.Stringer`; `@[go]` is
not needed for method bodies expressible in Goop. For complete examples, see
[`go_implements_stringer.goop`](../examples/go_implements_stringer.goop) and
[`go_implements_slog_handler.goop`](../examples/go_implements_slog_handler.goop),
which implements a native `slog.Handler`.

## Inline Go with `@[go]`

```goop
@[go] {
  func greet(name string) string {
    return "Hello, " + name + "!"
  }
}
val greet : string -> string

let main () : unit =
  println (greet "Goop")
```

The `@[go] { ... }` block is embedded Go. The `val` line declares the Goop-visible signature.

## Inline C with `@[c]`

```goop
@[c] {
  #include <string.h>
  int add(int a, int b) { return a + b; }
}
val add : int -> int -> int
```

Bodies become a cgo preamble (`import "C"`). Primitive `val` types are auto-wrapped; richer FFI uses `@[go]` calling `C.*`. See [15-lang-embeds.md](../design/15-lang-embeds.md).

## Import Goop modules

```goop
import goop . "std.io"    (* dot import: Println *)
import io goop "std.io"   (* qualified: io.Println *)
```

See [modules guide](../design/05-modules-and-packages.md).

## Gradual migration

`.goop` and `.go` files can coexist in one module for migration. Default
`goop build` / `goop compile` keep generated Go in `$GOOP_HOME/build` so the
source tree stays `.goop`-only.

For mixed packages (hand-written `.go` beside `.goop`), use:

```bash
goop build --in-tree main.goop
```

That writes the generated file next to sources and runs `go build` in the
source directory (legacy coexistence mode).

Full examples: [`extern_demo.goop`](../examples/extern_demo.goop), [`cgo_demo.goop`](../examples/cgo_demo.goop).

## Next

[5. Concurrency →](05-concurrency.md)
