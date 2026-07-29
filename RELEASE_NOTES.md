# Goop 1.16.0

Fix false-positive UNUSED002 on real module uses like `sanitize.foo`.

## Highlights

- Imports used via field access, type-prefixed constructors, and local opens count as used
- UNUSED002 diagnostics include file:line; warning output no longer concatenates

See `CHANGELOG.md` and the [playground](https://macho0x.github.io/Goop/).
