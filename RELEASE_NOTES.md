# Goop 1.13.0

Refinement train: more codes on the wire, ROW001, DECIMAL001, FFI honesty,
LSP warning parity, and a Go-generics-in-sigs policy doc.

## Highlights

- **TYPE/PARSE/IMPORT** stable prefixes + `help:` tips
- **ROW001** + safer row-param codegen; **DECIMAL001** (`money_float`)
- **GOSIG003** optional hand-sig verify (`verify_ffi`)
- **LSP** shows safety warnings by default
- Docs: FFI honesty, prelude FFI helpers, [32-go-generics-sigs.md](docs/design/32-go-generics-sigs.md)

## Config

```toml
[check]
money_float = "warn"
verify_ffi = false
```

See `CHANGELOG.md` and the [playground](https://macho0x.github.io/Goop/).
