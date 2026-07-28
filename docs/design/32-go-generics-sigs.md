# 32 — Go generics in `.gosig` / FFI (policy)

**Status (1.14):** policy locked; generator skips + **GOSIG004** honesty landed.
No monomorphization.

When changing this area, follow
[31-language-update-checklist.md](31-language-update-checklist.md).

## Problem

Go 1.18+ packages export generic functions and types. Goop’s gosig pipeline
maps concrete Go types into Goop; unconstrained type parameters have no
faithful surface yet.

## MVP policy (locked)

1. **Skip** unrepresentable generic exports in auto-generated stubs, or emit a
   `TODO(generics)` comment and omit the `val`.
2. **Warn** with **GOSIG004** when a hand `{ val … }` names a generic that
   cannot be mapped — do not invent fake monomorphic types. (Always on; not
   gated by `[check] verify_ffi`.)
3. **Do not** monomorphize common instantiations in 1.14.
4. Prefer thin `@[go]` wrappers that expose a concrete API when users need a
   generic Go helper.

## Relation to shipped tooling

- Auto-load + cache + overrides: [28-go-sig-resolution.md](28-go-sig-resolution.md)
- Generator: [23-gosig-generator.md](23-gosig-generator.md)
- Boundary honesty: [27-ffi-boundary.md](27-ffi-boundary.md)
- Optional hand-sig arity check: `[check] verify_ffi` → **GOSIG003**
- Hand generic honesty: **GOSIG004** (always warn)

## Curated skip catalog (1.14 baseline)

Regenerate: `goop gen-sig <pkg>` for each entry in `CuratedPackages`
(`src/internal/gosiggen/curated.go`), then inspect `(* Skipped exports: … *)`.

| Package | Skipped | Notes |
|---------|--------:|-------|
| strings | 0 | |
| fmt | 0 | |
| errors | 1 | `AsType` — **TODO(generics)** (Go 1.26+) |
| strconv | 2 | complex numbers (not generics) |
| bytes | 0 | |
| io | 0 | |
| os | 0 | |
| time | 0 | |
| context | 1 | `Context.Done` anon struct chan (not generics) |
| sync | 2 | `OnceValue`, `OnceValues` — **TODO(generics)** |
| sync/atomic | 8 | `Pointer.*` methods — **TODO(generics)**; also `unsafe.Pointer` funcs |
| sort | 0 | |
| math | 0 | |
| math/rand | 0 | |
| net | 0 | |
| net/http | 0 | |
| database/sql | 0 | |
| encoding/json | 0 | |
| encoding/csv | 0 | |
| encoding/base64 | 0 | |
| crypto/sha256 | 0 | |
| log/slog | 0 | |

Generic-heavy stdlib outside curated (e.g. `slices`) can skip dozens of
exports; use `@[go]` or hand concrete wrappers for the few APIs you need.

## Follow-ups

- Decide whether `forall` / constrained poly in Goop is worth a later release
  (only if wrappers become unbearable for a dogfood-critical package).
- Monomorphization remains deferred.
