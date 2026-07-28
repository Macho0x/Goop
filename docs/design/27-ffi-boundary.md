# 22. FFI boundary guarantees

**Audience:** Goop users who call Go (and C via cgo) from Goop.  
**Status:** Living document. Reflects the shipped 1.x surface, including H6
`(T, error)` → `result` coercion and H5 auto-load of `.gosig` for bare
`import go` (explicit `{ val … }` blocks remain authoritative).

Goop compiles to Go and treats Go's stdlib as its library surface. The
boundary is where Goop's static checks stop applying to foreign bodies, and
where Go values re-enter Goop with only the types you declared.

See also: [15-lang-embeds.md](15-lang-embeds.md), [17-go-implements.md](17-go-implements.md),
[18-go-methods.md](18-go-methods.md), [04-go-lowering.md](04-go-lowering.md),
[STYLE.md](STYLE.md).

## Mental model

| Path | What Goop checks | What Goop trusts |
|------|------------------|------------------|
| Pure Goop | Types, exhaustiveness, linearity, refinements, nil-channel lint | — |
| `import go "pkg" { … }` | Call sites against declared `val` / `type` | That the sig matches the real Go API |
| `implements I for T` | Method shapes vs imported interface | That `I` is the Go interface you meant |
| `@[go] { … }` | Only the trailing `val` signatures | The entire Go body |
| `@[c] { … }` | Primitive `val` marshalling | The C body and cgo toolchain |

**Rule of thumb:** keep `@[go]` small and isolated. Prefer typed `import go`
selectors and `implements` when Goop can express the code.

---

## What Goop guarantees

### Typed call sites for declared imports

If you declare a Go binding, Goop type-checks uses of it:

```goop
import go "strings" {
  val ToUpper : string -> string
}

let s = ToUpper "hi"   (* checked *)
```

Method and field selectors lower to ordinary Go selectors with **exact Go
spelling** preserved (`Key`, `Attrs`, `WriteString` — never auto-remapped to
`key` / `write_string`):

```goop
import go "log/slog" {
  type Attr
  val (a : Attr).Key : string
}

let k = a.Key   (* → a.Key in Go *)
```

### Pointer / nil is explicit at the FFI

Core Goop has no ambient null — use `'a option`. At the **Go** boundary,
nullable pointers are a separate, explicit story:

| Goop | Go |
|------|-----|
| `'a ptr` | `*T` |
| `null` | `nil` |
| `is_null e` | `e == nil` |
| `ptr_of e` | `&e` |

**Guarantee:** Goop does **not** silently collapse Go `nil` into `option`.
Nil stays `ptr` / `null`. You must check with `is_null` (or avoid nil by
construction) before dereference.

**Does not guarantee:** that an opaque Go value is non-nil. Importing
`type Mutex` and receiving a `Mutex ptr` from Go does not prove the pointer
is live.

Heap / pointer-receiver APIs should be declared as `T ptr` (e.g.
`Buffer ptr` for `*bytes.Buffer`). Opaque `type Buffer` alone is the Go
**value** type. There is no auto-coercion between `T` and `T ptr`.

### `error` and `go_slice` are first-class FFI types

| Goop | Go |
|------|-----|
| `error` | `error` |
| `'a go_slice` | `[]T` |
| `go_slice_get` / `go_slice_len` / `go_slice_append` | index / `len` / `append` |
| `go_slice_of_list` / `list_of_go_slice` | identity (lists already lower to slices) |
| `...T` + `spread xs` | variadic / `xs...` |

### `implements` emits a real Go method set

`implements I for T with … end` generates pointer-receiver methods and a
compile-time assertion `var _ pkg.I = (*T)(nil)`. Method names must match Go
case-sensitively. Bodies written in Goop remain under Goop checking; the
assertion catches signature drift when Go builds the package.

### Selector spelling and field names

- Imported method/field names are **case-sensitive Go identifiers**.
- Go struct literals (imported structs) map field names
  **case-insensitively** at the Goop surface (`level` → `Level`) but emit the
  **exact** Go field name in generated code.
- Omitted struct fields are Go zero values.

### Codegen does not silently degrade

Unsupported expressions are hard errors at Goop compile time (file:line).
The compiler must not emit known-broken `/* TODO */` stubs for the Go
toolchain to trip over later.

### `(T, error)` coerces to `result` (H6)

When an `import go` binding’s final return is `(T, error)` (or `T * error`),
Goop presents it as `('ok, error) result` at the call site:

```goop
import go "strconv" {
  val Atoi : string -> (int, error)   (* declared product *)
}

let r = Atoi "42" in
match r with
| Ok n  -> n
| Error e -> 0
```

Codegen wraps the Go multi-value return into `Ok` / `Error` constructors
(not `Err`). Hand-written and generated `.gosig` product forms both get
coercion — typecheck+codegen is the source of truth.

**Raw opt-out:** `import go raw "strconv" { … }` keeps the Goop tuple
`(int, error)` and the multi-value product struct (`F0`, `F1`).

---

## What Goop does **not** guarantee

### Bodies of `@[go]` (and `@[c]`)

```goop
@[go] {
  func sneak(x string) string {
    // Goop does not parse, type-check, or linear-check this body.
    return x
  }
}
val sneak : string -> string
```

Inside `@[go]` / `@[c]` blocks, Goop **voids**:

- Hindley–Milner typing of the body
- Exhaustive `match` / ADT safety
- Linear discharge (`: 1`, `owned_chan`)
- Nil-channel and concurrent-capture analyses
- Refinement / SMT proofs
- STYLE / OCaml-surface constraints

What remains: the block must be brace-balanced; trailing `val` names must
exist in the emitted Go; `unit` args are elided at call sites. **The `val`
signature is a trust boundary** — a wrong signature is unchecked UB relative
to Goop's type system (Go may still reject the package).

Prefer `import go` + native Goop (or `implements`) over embeds whenever
possible.

### Linearity does not cross the boundary

Linear types (`type handle : 1`, `owned_chan`, …) are **compile-time only**.
They are erased in generated Go (`interface{}` or the extern Go type).

| Situation | Guarantee |
|-----------|-----------|
| Linear value used only in Goop | Discharge / single-use checked |
| Linear value handed to a Goop function that consumes it | Checked |
| Linear value passed into `@[go]` or an opaque Go API | **Not tracked inside foreign code** |
| Value returned from Go typed as linear | **Not proven linear** — the annotation is a claim |

Once a linear value enters foreign code, Goop cannot prove close-once,
use-once, or no-alias on the Go side. Do not pass `owned_chan` / `: 1`
handles through `@[go]` unless the embed is the intentional discharge point
and you accept that the checker stops at the call.

Concurrency note: `go (move …)` and LINEAR00x analyses apply to **Goop**
`go` expressions, not to goroutines spawned inside `@[go]`.

### `obj` / `any` is an untyped hole

Go's empty interface (`any` / `interface{}`) is the escape hatch for values
Goop cannot name precisely.

| Spelling | Status |
|----------|--------|
| `any` + `any_of` | **Shipped** — lowers to `interface{}` |
| `obj` | **Shipped** H5 `.gosig` spelling for the same empty-interface mapping (`obj` rewritten to `any` on load) |

**Semantics (both names):**

- No payload type is remembered by the Goop type checker.
- `any_of x` boxes; recovering a concrete type requires Go-side type
  assertions (typically inside `@[go]` or a typed wrapper you trust).
- Variadic slog-style APIs often take `...any`; build slices with `any_of`
  then `spread`.

**Does not guarantee:** that a value pulled from `obj`/`any` matches the
type you expect. Treat it like `interface{}` in Go.

### Extern multi-value returns are limited (M6)

Extern / FFI calls that need multi-value tuple returns beyond the
`(T, error)` story are still constrained. Prefer a single Goop-visible
return (record, ADT, or `result`) via a thin `@[go]` wrapper until M6
closes.

### Import signatures are not yet auto-verified end-to-end

Hand-written `import go { val … }` blocks are checked for **internal**
consistency (arity, Goop types) and lowered to Go calls. They are **not** a
full proof that the upstream Go package exports exactly that API with those
types — a stale sig fails at Go build time or mis-calls at runtime.

---

## Signatures, cache, and overrides (H5)

**Today:** declare the bindings you need inline:

```goop
import go "bytes" {
  type Buffer
  val (b : Buffer ptr).String : unit -> string
}
```

**Shipped (H5):** a `go/types`-driven generator emits `.gosig` stubs for a
curated package set into the build cache (`$GOOP_HOME/build/go-sigs/…`), with
repo overrides under `goop-sigs/`. Bare `import go "…"` auto-loads (override →
cache → generate-on-miss for curated paths). Hand `{ val … }` blocks remain
authoritative and are **not** a full proof against upstream Go — enable
`[check] verify_ffi = true` for **GOSIG003** arity checks. See
[28-go-sig-resolution.md](28-go-sig-resolution.md) and
[32-go-generics-sigs.md](32-go-generics-sigs.md).


| Mechanism | Role |
|-----------|------|
| Generated `.gosig` (cache-only) | Default stubs for curated packages |
| Project `goop-sigs/` override | Hand-curated sig **wins** over generated |
| Inline `import go { … }` | Still valid; explicit surface |

Policy (maintainers): no new Goop syntax for awkward Go idioms — fix the
sig generator or type bridge. Preserve Go selector spelling. `(T, error)`
defaults to `result` (H6); use `import go raw` for the tuple.

Auto-load is shipped for curated packages; prefer bare `import go` plus
overrides when stubs are incomplete. Inline `{ val … }` remains the reliable
escape hatch for third-party packages.

---

## Quick reference: safe vs unsafe at the edge

| Do | Don't |
|----|-------|
| Declare `T ptr` + `is_null` for nullable Go pointers | Assume Go pointers are non-nil |
| Keep `@[go]` to crypto, cgo glue, or thin adapters | Put business logic in embeds |
| Use `result` + `match` — `(T, error)` coerces to `result` by default (H6) | Assume `import go raw` unless you opted in |
| Pass branded IDs / ADTs in Goop; unwrap at the edge | Smuggle domain IDs as bare `string` across modules |
| Discharge linear resources in Goop before calling opaque Go | Pass `owned_chan` into `@[go]` and hope |
| Prefer generated/override sigs once H5 lands | Hand-copy huge stdlib surfaces forever |

---

## Related examples

- [`extern_demo.goop`](../examples/extern_demo.goop) — `import go` + `@[go]`
- [`go_method_calls.goop`](../examples/go_method_calls.goop) — selectors, `Buffer ptr`
- [`go_implements_stringer.goop`](../examples/go_implements_stringer.goop) — `implements`
- [`go_implements_slog_handler.goop`](../examples/go_implements_slog_handler.goop) — handler FFI
- Tutorial: [04-go-interop.md](../tutorial/04-go-interop.md)
