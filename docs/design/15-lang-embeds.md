# 15. Language embeds (`@[go]` / `@[c]`)

## Surface

Goop embeds foreign code with a single form:

```goop
@[lang] {
  /* raw body — brace-balanced, opaque to the Goop parser */
}
val name : type   (* zero or more; Goop-visible bindings *)
```

| Lang | Meaning | Lowering |
|------|---------|----------|
| `go` | Inline Go | Body emitted into the generated package; leading `import` decls in the body are **hoisted** into the package import block (avoids mid-file `import`) |
| `c` | Inline C (cgo mirror) | Bodies concatenated into a cgo `/* */` preamble + `import "C"` + auto wrappers |

Unknown langs (`@[rust]`, …) are hard errors. Future langs reuse this grammar.

Multi-value Go returns on `val` bindings are Goop tuples (`(T0, T1, …)`); see
[04-go-lowering.md § Extern multi-value returns](04-go-lowering.md#extern-multi-value-returns).
H5 auto-sigs are optional for hand-written FFI: explicit `import go` / `@[go]`
declarations still work. Bare `import go "pkg"` (no block) also auto-loads
`.gosig` stubs (override → cache → curated generate-on-miss); see
[28-go-sig-resolution.md](28-go-sig-resolution.md).

## Imports

```goop
import go "fmt"
import go "strings" { val ToUpper : string -> string }
import goop "std.io"
```

`go` after `import` is contextual (same keyword as `go expr` concurrency). There is no `import c`.

## Go interfaces without embeds

Go interfaces can be imported with `type Name` and implemented directly in
Goop with `implements`; method bodies that Goop can express no longer require
an `@[go]` escape hatch. `@[go]` remains available for opaque Go helpers and
other code Goop cannot express. See [17-go-implements.md](17-go-implements.md).
Method calls and field reads on imported Go types likewise need no wrapper;
declare them with `val (x : T).M : τ`. See [18-go-methods.md](18-go-methods.md).

## Hard break (1.2.0)

Removed: `import golang`, `alias golang`, `@golang { }`, and the `golang` keyword.
Migration: `golang` → `go`, `@golang` → `@[go]`.

## `@[c]` marshalling (v1)

| Goop | C / cgo |
|------|---------|
| `int` | `C.int` |
| `int32` / `int64` | `C.int32_t` / `C.int64_t` |
| `float` / `float64` | `C.double` |
| `bool` | `C.int` (nonzero) |
| `unit` (return) | no value |
| `string` | `C.CString` in (freed after call) / `C.GoString` out |

Other types: compile error — write `@[go]` wrappers that call `C.*` by hand.
`#cgo` lines inside the `@[c]` body are passed through (cgo understands them).
Modules with `@[c]` require `CGO_ENABLED=1` and a system C toolchain.

## Examples

- [`extern_demo.goop`](../examples/extern_demo.goop) — `import go` + `@[go]`
- [`cgo_demo.goop`](../examples/cgo_demo.goop) — `@[c]` + `val`

## Editor highlighting

In VS Code/Cursor, `@[go]` / `@[c]` (and future `@[lang]`) share one scope (`keyword.embed.goop`, amber `#D7BA7D`). The `{ … }` body uses normal Go/C grammar colors — the embed tag is not tinted with the block. Extension **0.3.7+** sets that color via top-level `editor.tokenColorCustomizations` so it overrides the active theme (Dracula, etc.).
