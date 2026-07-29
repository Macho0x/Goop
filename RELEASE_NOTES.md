# Goop 1.20.0

Codegen fixes for `@[go]` embed imports and multi-file `import goop` packages.

## Highlights

- Hoist `import` out of `@[go]` bodies into the generated package imports
- Identifier-safe Option/Result names for `*` / `[]` types
- Sibling merge when loading Goop dependencies

See `CHANGELOG.md` and [15-lang-embeds.md](docs/design/15-lang-embeds.md).
