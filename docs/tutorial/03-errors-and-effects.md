# 3. Errors and effects

## Result type

`result` models success or failure. Propagate with `match` (there is no `?` operator):

```goop
let parseInt (s: string) : (int, string) result = ...

let compute (input: string) : (int, string) result =
  match parseInt input with
  | Error e -> Error e
  | Ok n -> Ok (n + n)
```

See [`result.goop`](../examples/result.goop). Prefer `result` for recoverable trading/venue errors.

Discarding a `result` in a `begin` sequence is a **RESULT001** warning by default
(`[check] discarded_result` in `goop.toml`). Handle it with `match`, or write
`let _ = e in …` to discard deliberately.

## Exceptions and `failwith`

```goop
exception Boom

let bad () = failwith "invariant broken"

let run () =
  try
    raise Boom
  with
  | Boom -> "caught"
```

Use `failwith` / `raise` for bugs. See [`exceptions.goop`](../examples/exceptions.goop) and [STYLE.md](../design/STYLE.md).

## Effect handlers

Goop 1.0 supports OCaml 5-style effects (minimal). Declare with `effect`, invoke with `perform`, handle in `match`. Effectful code may CPS-lower to non-idiomatic Go.

```goop
effect Flip : unit -> bool

let coin () : bool =
  begin
    perform (Flip ());
    false
  end

let run () : bool =
  match coin () with
  | effect (Flip _) k -> k true
  | v -> v
```

Surface `with { io }` rows are removed. See [`effects.goop`](../examples/effects.goop) and [06-effects-and-safety.md](../design/06-effects-and-safety.md).

## Async / channels

Prefer `go` and `chan` over removed `async { }` computation expressions. See [chapter 5](05-concurrency.md) and [`concurrency.goop`](../examples/concurrency.goop).

## Next

[4. Go interop →](04-go-interop.md)
