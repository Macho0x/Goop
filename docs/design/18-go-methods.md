# 18. Go method and field imports

**Status:** Shipped in Goop **1.4.0**.

## Motivation

Goop 1.3.0 added `implements` so Goop types can *satisfy* Go interfaces.
Treelog and similar libraries also need to *call* methods and read fields on
imported Go types (`Record.Attrs`, `Attr.Key`, `Mutex.Lock`, `Logger.With`).

Opaque `type Record` imports alone are not enough: values cannot project
fields or invoke methods without `@[go]`.

## Surface

### Method and field vals

Inside `import go "path" { … }`:

```goop
import go "log/slog" {
  type Record
  type Attr
  type Value
  type Logger

  val New : Handler -> Logger
  val (r : Record).Attrs : (Attr -> bool) -> unit
  val (a : Attr).Key : string
  val (a : Attr).Value : Value
  val (v : Value).Resolve : unit -> Value
  val (v : Value).String : unit -> string
  val (l : Logger).Info : string -> ...any -> unit
  val (l : Logger).With : ...any -> Logger
}

import go "sync" {
  type Mutex
  val (m : Mutex).Lock : unit -> unit
  val (m : Mutex).Unlock : unit -> unit
}
```

| Form | Meaning |
|------|---------|
| `val Name : τ` | Free function (1.3) |
| `val (x : T).M : τ` | Method `M` on `T`; `τ` is the type *after* the receiver |
| `val (x : T).F : τ` | Field `F` on `T` when `τ` is not a function type starting a call pattern — **fields** are non-arrow types; **methods** have arrow (or `unit -> …`) types |

Disambiguation: if the declared type after `:` is a function type (`->`), the
binding is a **method**; otherwise it is a **field**.

### Calls

```goop
Record.Attrs r (fun a -> true)
r.Attrs (fun a -> true)          // equivalent via field-select-then-app
let k = a.Key in
let s = (v.Resolve ()).String () in
Mutex.Lock mu; ...; Mutex.Unlock mu
log.With (spread args)
```

Lowering preserves Go selector spelling (`Key`, `Attrs`, `Resolve`).

When a field or method short name collides with an imported `type` (e.g.
`type Value` and `val (a : Attr).Value`), only the qualified form
`Attr.Value` / `a.Value` (via field access) is bound; free `Value` stays the type.

### `any`, spread, `go_slice` indexing

```goop
// builtin
type any
val any_of : 'a -> any
val go_slice_get : 'a go_slice -> int -> 'a

xs.(i)   // when xs : 'a go_slice → go_slice_get xs i
spread xs
```

## Non-goals (1.4.0)

- `defer` keyword
- Auto-discovery of all exports from a bare `import go "pkg"`
- Changing OCaml `#method` object semantics

## Pointer receivers and heap types (1.6.0)

Opaque `type Buffer` maps to the Go **value** type. Methods and APIs that
need `*bytes.Buffer` (pointer receivers, `io.Writer`) must declare
`Buffer ptr` on vals and receivers. There is no auto-coercion between
`Buffer` and `Buffer ptr`.

## Go struct literals (1.7.0)

Imported Go **structs** (via `type Name` + gosig) can be constructed with
record literals when the expected type is `S` or `S ptr`:

```goop
import go "log/slog" {
  type HandlerOptions
  type Leveler
  type Level
  val LevelInfo : Level
  val NewJSONHandler : Buffer ptr -> HandlerOptions ptr -> Handler
}

let opts : HandlerOptions ptr = ptr_of { level = LevelInfo }
let mu : Mutex ptr = ptr_of {}
```

Field names match Go exported fields case-insensitively (`level` → `Level`).
Omitted fields are zero. Interface fields accept Go-assignable named types
(e.g. `Level` → `Leveler`).

See also: [17-go-implements.md](17-go-implements.md), [15-lang-embeds.md](15-lang-embeds.md),
[04-go-lowering.md](04-go-lowering.md).
