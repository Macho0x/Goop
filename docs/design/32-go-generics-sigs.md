# 32 — Go generics in `.gosig` / FFI (policy)

**Status (1.13):** design-only. No monomorphization in the generator.

## Problem

Go 1.18+ packages export generic functions and types. Goop’s gosig pipeline
maps concrete Go types into Goop; unconstrained type parameters have no
faithful surface yet.

## MVP policy (locked)

1. **Skip** unrepresentable generic exports in auto-generated stubs, or emit a
   `TODO(generics)` comment and omit the `val`.
2. **Warn** (stderr / future GOSIG code) when a hand `{ val … }` names a
   generic that cannot be mapped — do not invent fake monomorphic types.
3. **Do not** monomorphize common instantiations in 1.13.
4. Prefer thin `@[go]` wrappers that expose a concrete API when users need a
   generic Go helper.

## Relation to shipped tooling

- Auto-load + cache + overrides: [28-go-sig-resolution.md](28-go-sig-resolution.md)
- Generator: [23-gosig-generator.md](23-gosig-generator.md)
- Boundary honesty: [27-ffi-boundary.md](27-ffi-boundary.md)
- Optional hand-sig arity check: `[check] verify_ffi` → **GOSIG003**

## Follow-ups (post-1.13)

- Catalog which curated packages lose the most exports to generics skips.
- Decide whether `forall` / constrained poly in Goop is worth a later release.
