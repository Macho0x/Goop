# Goop 1.10.0

Goop 1.10.0 closes the self-host readiness gaps after 1.9: first-class maps,
zero-cost branded IDs, bare-import `.gosig` autoload, and a green Windows CI
matrix.

## Highlights

- **Maps:** `map[K] V` + prelude `Map.*` (and thin `std.map`).
- **Zero-cost brands (H4c):** single-ctor primitive/string ADTs lower to Go
  defined types (no interface boxing).
- **Interop:** bare `import go "…"` loads `goop-sigs/` / cache stubs; `obj`≡`any`;
  multi-results as tuples; curated toolchain stubs.
- **CI:** Windows exe-path fixes; sig-corpus smoke on Ubuntu.

## Workflow

```bash
cd src && go build -o ../goop ./cmd/goop
../goop version
../goop check docs/examples/maps.goop
../goop check docs/examples/writing_tools.goop
./scripts/selfhost-spike.sh   # optional
```

See `CHANGELOG.md` and `docs/design/30-freeze-checklist.md`.
