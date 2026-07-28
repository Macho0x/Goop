# Goop 1.14.0

Decimal annotations work across modules; generics-in-sigs honesty (catalog + GOSIG004).

## Highlights

- **`import goop` re-exports FFI opaque types** — e.g. `price: Decimal` after
  `import goop . "std.decimal"` (check + build).
- **GOSIG004** warns when a hand `{ val … }` names a generic Go export.
- Curated **TODO(generics)** skip catalog in
  [32-go-generics-sigs.md](docs/design/32-go-generics-sigs.md).
- Self-hosting remains **not planned**.

## Try it

```goop
import goop . "std.decimal"

type Quote = { price: Decimal; qty: int }
```

See `CHANGELOG.md` and the [playground](https://macho0x.github.io/Goop/).
