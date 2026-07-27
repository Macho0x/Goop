# 30 — Freeze checklist (pre–self-host)

Use before declaring the surface frozen enough to start L6 (self-host port).

## CI gate (hard)

- [ ] Ubuntu `Test` job green (`go test ./...`, `goop test`, example check, build smoke, sig corpus)
- [ ] Windows `Test (Windows)` job green (`go test ./...`, hello check, build smoke)
- [ ] No known failing package on `main`

## Language surface

- [ ] `map[K] V` + `Map.*` prelude stable ([29-maps.md](29-maps.md))
- [ ] Zero-cost single-ctor brands (H4c) for primitive/string payloads ([21-branded-ids.md](21-branded-ids.md))
- [ ] `(T, error)` → `result` coercion (H6); `import go raw` documented
- [ ] No user-facing `newtype` restore

## Interop

- [ ] `.gosig` auto-load on `import go` (override → cache → curated generate-on-miss)
- [ ] `obj` ≡ `any` in stubs
- [ ] Multi-result products (non-error) emitted as Goop tuples where representable
- [ ] Curated overrides present under `goop-sigs/` for toolchain pkgs (`os`, `path/filepath`, `bytes`, `bufio`, `strings`, `encoding/json`)
- [ ] `goop gen-sig --smoke` in CI

## Stdlib doctrine

- [ ] [docs/stdlib/README.md](../stdlib/README.md) still: no `std.net` / `std.fs` / `std.json`
- [ ] [Writing tools](../guides/writing-tools.md) example checks clean

## Optional before L6

- [ ] `spike/selfhost-lexer/` compiles under bootstrap Goop
- [ ] Benchmarks for branded IDs closer to hand Go
