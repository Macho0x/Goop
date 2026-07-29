# 5. Concurrency

Goop exposes Go’s concurrency primitives with compile-time safety checks.

## Channels and goroutines

Prelude bindings (no import required):

| Binding | Type |
|---|---|
| `Chan.make` | `unit -> 'a chan` |
| `Chan.send` | `'a chan -> 'a -> unit` |
| `Chan.recv` | `'a chan -> 'a` |
| `Chan.close` | `'a chan -> unit` |

```goop
let main () : unit =
  let ch = Chan.make () in
  let ignored = go (fun () -> Chan.send ch 42) in
  let v = Chan.recv ch in
  assert (v = 42)
```

See [`concurrency.goop`](../examples/concurrency.goop).

## Data race detection

The linear checker flags `ref` (or other mutable state) captured by `go` while still accessible in the spawning scope:

```goop
let race () : unit =
  let counter = ref 0 in
  let ignored = go (fun () -> println (int_to_string (!counter))) in
  println (int_to_string (!counter))   (* error: counter still in scope *)
```

Good patterns: [`race_detection.goop`](../examples/race_detection.goop).

## `go (move ...)`

Transfer a binding into a goroutine when the parent no longer uses it:

```goop
let launch () : unit =
  let counter = ref 0 in
  let dummy = go (move counter) (fun () -> println (int_to_string (!counter))) in
  ()
```

See [`go_move.goop`](../examples/go_move.goop).

## Linear `go` handoff

Linear resources (`owned_chan`, `type handle : 1`) can be handed off to a goroutine — the parent scope discharges the variable. See [`linear_go_handoff.goop`](../examples/linear_go_handoff.goop).

Configure race severity in `goop.toml`:

```toml
[check]
concurrent = "error"   # LINEAR006/007/008: warn | error | off
```

## Channel-mediated races (LINEAR008)

When shared mutable state is sent on a channel inside a `go` closure but the parent still accesses it afterward, the compiler reports **LINEAR008**.

Use `go (move var)` or stop accessing the variable in the parent after the handoff.

## Deadlock lint (DEADLOCK001)

A narrow static check detects the classic two-goroutine circular send/recv pattern on unbuffered channels:

```toml
[check]
deadlock = "warn"   # warn | error | off
```

## Owned channels

Linear `owned_chan` types enforce close safety. See [`owned_chan.goop`](../examples/owned_chan.goop) and [`linear_resource.goop`](../examples/linear_resource.goop).

## Next

[6. Safety checks →](06-safety-checks.md)
