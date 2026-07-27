# std.chan

**Source:** `std/channel/channel.goop`  
**Import (optional):** `import goop "std.chan"` or `import goop . "std.chan"`

Thin re-export of prelude `Chan.*` and `OwnedChan.*` for import-style access. Prefer prelude names in most code; this module exists so channel APIs are discoverable under `std.*` like `std.array`.

The physical package directory is `std/channel` (Go forbids `package chan`); the logical import path remains `std.chan`.

## Ordinary channels (`'a chan`)

| Name | Type | Prelude |
|---|---|---|
| `make` | `unit -> 'a chan` | `Chan.make` |
| `send` | `'a chan -> 'a -> unit` | `Chan.send` |
| `recv` | `'a chan -> 'a` | `Chan.recv` |
| `close` | `'a chan -> unit` | `Chan.close` |

## Linear channels (`'a owned_chan`)

| Name | Type | Prelude |
|---|---|---|
| `make_owned` | `unit -> 'a owned_chan` | `OwnedChan.make` |
| `send_owned` | `'a owned_chan -> 'a -> unit` | `OwnedChan.send` |
| `recv_owned` | `'a owned_chan -> 'a` | `OwnedChan.recv` |
| `close_owned` | `'a owned_chan -> unit` | `OwnedChan.close` |

Linear discharge still requires closing an `owned_chan` exactly once. See [`owned_chan.goop`](../examples/owned_chan.goop).

## Example

```goop
import goop . "std.chan"

let ping () : int =
  let ch = make () in
  begin
    go (fun () -> send ch 1);
    let n = recv ch in
    close ch;
    n
  end
```

Concurrency primitives (`go`, `select`) remain language features — not `std.*` packages. For Go channel types at the FFI boundary, use `import go`.
