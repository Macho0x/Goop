# Goop ↔ Go snippet gallery

Hand-written **pairs**: the same idea in Goop and idiomatic Go.

This is a teaching surface, **not** a translator. There is no Go → Goop
converter. To see what *this* compiler actually emits:

```bash
goop compile --stdout docs/examples/gallery/hello.goop
```

Or open the [playground](https://macho0x.github.io/Goop/), pick an example,
and press **Compile** (Copy copies the generated Go).

| Pair | Idea |
|------|------|
| [`hello.goop`](hello.goop) / [`hello.go`](hello.go) | Prelude `print_line` vs `fmt.Println` |
| [`branded_ids.goop`](branded_ids.goop) / [`branded_ids.go`](branded_ids.go) | Nominal brands vs distinct defined types |
| [`result_match.goop`](result_match.goop) / [`result_match.go`](result_match.go) | `result` + `match` vs `(T, error)` |

Each `.goop` file is checked in CI (`goop check`). The `.go` files are
hand-written for readability and may differ from `goop compile --stdout`
output (helpers, naming, packaging).
