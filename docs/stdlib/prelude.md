# Prelude reference

The prelude is injected by the type checker before user declarations. Bindings are **shadowable** — a local `let println = ...` hides the prelude version.

**Source:** `src/internal/prelude/prelude.go`

## I/O

| Name | Type | Go lowering |
|---|---|---|
| `println` | `string -> unit` | `fmt.Println` |
| `print` | `string -> unit` | `fmt.Print` |
| `Console.println` | `string -> unit` | `fmt.Println` |

## Strings and numbers

| Name | Type | Go lowering |
|---|---|---|
| `int_to_string` | `int -> string` | `strconv.Itoa` |
| `float_to_string` | `float -> string` | `fmt.Sprintf` |
| `string_concat` | `string -> string -> string` | `+` operator |
| `String.length` | `string -> int` | `len` (byte count) |
| `String.sub` | `string -> int -> int -> string` | `s[i:i+n]` (byte slice; OOB panics) |

In practice, use the `^` operator for string concatenation. `String.length` /
`String.sub` follow Go byte indexing, not Unicode grapheme clusters.
Optional import-style access: [`std.string`](std-string.md).

## Lists

| Name | Type | Go lowering |
|---|---|---|
| `list_length` | `'a list -> int` | `len` |
| `list_append` | `'a list -> 'a list -> 'a list` | `append` |

List syntax (`[]`, `::`, `[a; b]`) is built into the language — see [builtins](builtins.md).

## Arrays

| Name | Type | Go lowering |
|---|---|---|
| `Array.make` | `int -> 'a -> 'a array` | `make([]T, n)` + fill loop |
| `Array.length` | `'a array -> int` | `len` |

Index read `arr.(i)` and write `arr.(i) <- v` are language syntax — see [std.array](std-array.md).

## Maps

| Name | Type | Notes |
|---|---|---|
| `Map.make` | `unit -> map['k] 'v` | Empty map |
| `Map.get` | `map['k] 'v -> 'k -> 'v option` | Missing → `None` |
| `Map.add` | `map['k] 'v -> 'k -> 'v -> unit` | Mutates in place |
| `Map.remove` | `map['k] 'v -> 'k -> unit` | Mutates in place |
| `Map.mem` | `map['k] 'v -> 'k -> bool` | Membership |
| `Map.size` | `map['k] 'v -> int` | Entry count |

`map[K] V` is a language type (lowers to Go `map[K]V`). Optional re-export:
[`std.map`](../stdlib/README.md) (`import goop "std.map"`). Design notes:
[29-maps.md](../design/29-maps.md).

## Refs and abort

| Name | Type | Go lowering |
|---|---|---|
| `ref` | `'a -> 'a ref` | pointer allocation |
| `failwith` | `string -> 'a` | `panic(...)` |
| `assert` | `bool -> unit` | `if !b { panic(...) }` |
| `assert_equal` | `'a -> 'a -> unit` | equality panic |

`!r` and `r := e` are language syntax. There is no `panic_message` — use `failwith`.
Optional import-style constructor: [`std.ref`](std-ref.md) (`make`).

## Channels (`chan`)

| Name | Type |
|---|---|
| `Chan.make` | `unit -> 'a chan` |
| `Chan.send` | `'a chan -> 'a -> unit` |
| `Chan.recv` | `'a chan -> 'a` |
| `Chan.close` | `'a chan -> unit` |

`Chan.close` is runtime-safe (closed-flag wrapper). Nil-channel use before init is caught by **NIL001**.
Optional import-style access: [`std.chan`](std-chan.md).

## Linear channels (`owned_chan`)

| Name | Type |
|---|---|
| `OwnedChan.make` | `unit -> 'a owned_chan` |
| `OwnedChan.send` | `'a owned_chan -> 'a -> unit` |
| `OwnedChan.recv` | `'a owned_chan -> 'a` |
| `OwnedChan.close` | `'a owned_chan -> unit` |

Linear discharge checking ensures channels are closed exactly once. See [`owned_chan.goop`](../examples/owned_chan.goop).
Also available via [`std.chan`](std-chan.md) as `make_owned` / `send_owned` / `recv_owned` / `close_owned`.

## Lazy

| Name | Type | Go lowering |
|---|---|---|
| `Lazy.force` | `'a lazy -> 'a` | custom |
| `Lazy.from_val` | `'a -> 'a lazy` | custom |

`lazy e` is language syntax. There is no `std.lazy` module yet — use prelude `Lazy.*` directly (see [stdlib roadmap](README.md#roadmap-what-still-belongs)).

## HTTP / JSON helpers

Trading-oriented helpers lowering to inline Go:

| Name | Type |
|---|---|
| `http_get_string` | `string -> string` |
| `json_extract_floats` | `string -> int -> float list` |
| `json_extract_strings` | `string -> int -> string list` |

Used by trading demos that call live or synthetic market data helpers (see [`allmids_bot.goop`](../examples/allmids_bot.goop)).

## FFI helpers (Go interop)

| Name | Type | Role |
|---|---|---|
| `any_of` | `'a -> any` | Box a value as Go `interface{}` / `any` |
| `go_slice_len` | `'a go_slice -> int` | Length of an FFI Go slice |
| `go_slice_get` | `'a go_slice -> int -> 'a` | Index into an FFI Go slice |
| `go_slice_append` | `'a go_slice -> 'a -> 'a go_slice` | Append to an FFI Go slice |
| `go_slice_of_list` | `'a list -> 'a go_slice` | Convert list → Go slice |
| `list_of_go_slice` | `'a go_slice -> 'a list` | Convert Go slice → list |

`spread` / `xs.(i)` on go_slices are language/FFI syntax — see [27-ffi-boundary.md](../design/27-ffi-boundary.md).
