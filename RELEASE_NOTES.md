# Goop 1.19.0

Multi-file same-module packages (sibling `.goop` merge) plus 1.18 record `@[tag]`.

## Highlights

- Sibling files sharing a non-`main` module name compile as one Go package
- `module main` files stay independent (examples / scaffolds)
- Record `@[tag "…"]` for Go struct tags (from 1.18)

See `CHANGELOG.md` and [33-sdk-blockers.md](docs/design/33-sdk-blockers.md).
