# Goop 1.12.0

Diagnostics gap closure: stable error codes on the wire, three new warn-by-default
lints, and an honest error catalog.

## Highlights

- **Stable codes + `help:` tips** for TYPE011, VIS001, IMPORT*, UNIFY020–022,
  PARSE-MIG002, CODEGEN*, GOSIG*, LINEAR001–005
- **UNUSED001/002**, **OPTION001**, **VIS002** (defaults: warn; knobs in `[check]`)
- Catalog cleanup: `open` stays valid; MIG011 → TYPE011; CLI011 says `goop:`

## Config

```toml
[check]
discarded_result = "warn"
discarded_option = "warn"
unused = "warn"
private_in_public = "warn"
```

See `CHANGELOG.md` and `docs/design/10-error-reference.md`.
