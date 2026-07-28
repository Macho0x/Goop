# 30 — Freeze checklist

Use before declaring a language / interop surface freeze (release cut or
major milestone). Self-hosting is **not planned**; this checklist is not a
gate for a Goop-written compiler.

For **every** language / diagnostics / CLI change (not only freezes), use
[31-language-update-checklist.md](31-language-update-checklist.md).

## CI gate (hard)

- [x] Ubuntu `Test` job green (`go test ./...`, `goop test`, example check, build smoke, sig corpus)
- [x] Windows `Test (Windows)` job green (`go test ./...`, hello check, build smoke)
- [x] No known failing package on `main`

## Language surface

- [x] `map[K] V` + `Map.*` prelude stable ([29-maps.md](29-maps.md))
- [x] Zero-cost single-ctor brands (H4c) for primitive/string payloads ([21-branded-ids.md](21-branded-ids.md))
- [x] `(T, error)` → `result` coercion (H6); `import go raw` documented
- [x] No user-facing `newtype` restore

## Interop

- [x] `.gosig` auto-load on `import go` (override → cache → curated generate-on-miss)
- [x] `obj` ≡ `any` in stubs
- [x] Multi-result products (non-error) emitted as Goop tuples where representable
- [x] Curated overrides present under `goop-sigs/` for toolchain pkgs (`os`, `path/filepath`, `bytes`, `bufio`, `strings`, `encoding/json`)
- [x] `goop gen-sig --smoke` in CI

## Stdlib doctrine

- [x] [docs/stdlib/README.md](../stdlib/README.md) still: no `std.net` / `std.fs` / `std.json`
- [x] [Writing tools](../guides/writing-tools.md) example checks clean

## Optional polish

- [ ] Benchmarks for branded IDs closer to hand Go
