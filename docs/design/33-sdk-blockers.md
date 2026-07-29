# 33 — SDK blockers (Hyperliquid / native Goop ports)

Living policy for gaps that block large native Goop rewrites of Go SDKs
(e.g. goop-hyperliquid). Companion to
[31-language-update-checklist.md](31-language-update-checklist.md).

Until an item is fixed upstream, keep the affected surface as identical
upstream Go and consume it with `import go` — minimize `@[go]` embeds.

## Status

| ID | Topic | Status |
|----|--------|--------|
| B1 | JSON / msgpack field tags on Goop records | **Shipped 1.18** — `@[tag "…"]` on record fields |
| B2 | Multi-file Goop modules → one Go package | **Shipped 1.19** — sibling `.goop` with same `module` merge (not `main`) |
| B3 | Third-party `.gosig` depth | Open — hand `{ type; val }` / project `goop-sigs/` / `goop get-go-sig` |
| B4 | Go generics in FFI (GOSIG004) | Accepted — concrete wrappers; see [32-go-generics-sigs.md](32-go-generics-sigs.md) |
| — | `?` on `result` | **Rejected** — prefer `match`; see [STYLE.md](STYLE.md) |

## B1 — Record `@[tag]`

Emit Go struct tags from record field declarations:

```goop
type Meta = {
  name : string @[tag "json:\"name\""];
  sz   : int option @[tag "json:\"sz,omitempty\""];
}
```

Payload is the exact body inside Go backticks. Multiple keys in one string:
`"json:\"n\" msgpack:\"n\""`.

Unknown `@[…]` on a field → **TAG001**.

## B2 — Multi-file same module

All sibling `.goop` files in a directory that share the same file-level
`module Name` typecheck and emit as one Go package. `private` is package-wide.
Files with `module main` / `Main` are never merged (flat example dirs).
Duplicate names → **MODULE001**. Merge applies to entry files and to packages
loaded via `import goop`.
See [05-modules-and-packages.md](05-modules-and-packages.md).

## B3 — Third-party sigs

No curated trees for ethereum / gorilla / msgpack / vago / fastjson in the
toolchain. Use hand signatures, project `goop-sigs/`, and `goop get-go-sig`.
See [23-gosig-generator.md](23-gosig-generator.md), [28-go-sig-resolution.md](28-go-sig-resolution.md).

## B4 — Generics

Do not monomorphize. Prefer thin Go wrappers exposing concrete APIs; hand
`{ val }` naming a generic warns **GOSIG004**.

## Rejected: `?` on `result`

Verbose `match` is intentional. Do not reintroduce Dingo/`?` or `result { }`.
