# Goop Strategic Update: Interop, Ergonomics, and Domain Fit

> Status: Revised 2026-07-27. Supersedes the 2026-07-16 draft.
> The Go-interop-first direction from the council review is retained, but the
> original plan covered only the interop axis. This revision adds the two axes
> it ignored — developer ergonomics and domain fit — and reorganizes all work
> into HIGH / MEDIUM / LOW priority tiers.
>
> **Progress tracker (updated 2026-07-27):** items marked ✅ DONE are landed in
> tree; ⏸️ DEFERRED are explicitly out of this release train; unmarked remain.

## 1. What we are doing

Three parallel tracks:

1. **Interop (original plan).** Remove the friction of using Go packages from
   Goop: generated `.gosig` stubs, `(T, error)` → `result` coercion, FFI
   boundary guarantees. Go's stdlib is the Goop stdlib for library coverage.
2. **Ergonomics (new).** Make Goop feel good to write and to try: a web
   playground, wider editor coverage, a consistent stdlib surface, and no
   silent compiler degradation.
3. **Domain fit (new).** Close the gap between Goop's trading-safety identity
   and its actual primitives: decimal/money support and a branded-ID story at
   least as strong as the closest competitor's.

The strategic blind spot of the 2026-07-16 plan: a language can have perfect
Go interop and still lose to Lisette on "does it feel good to write Goop
code." Lisette — same thesis, ML safety on the Go runtime — already ships a
WASM playground, five editor targets, and free tuple-struct newtypes. Interop
is necessary; it is not sufficient.

## 2. What we are not doing

- We are **not** writing a `std.net` or `std.codec` module that mirrors Go's
  stdlib. The Go stdlib is the Goop stdlib for library coverage.
- We are **not** adding new language syntax to make Go interop shorter. The
  boundary gets fixed in the sig generator and the type bridge, not the
  parser.
- We are **not** turning Goop into "Go with exhaustive match." Safe forms
  remain the default, not the opt-in.
- We are **not** adding syntax to chase peers. Each proposed surface change
  (string interpolation, newtype restoration) is a separate, explicit
  decision decided before the 1.0 freeze — not smuggled in as interop work.

## 3. Why this direction

Facts from the repo:

1. Goop already compiles to Go, runs on Go's runtime, and deploys as Go
   binaries. Go's stdlib is already reachable through `import go`; the
   friction is the hand-written `.gosig` ceremony. No `.gosig` files exist in
   the repo today — greenfield problem.
2. The compiler has a **silent degradation path**: the codegen expression
   dispatcher's default case emits `/* TODO: <Type> */` into generated Go
   (`src/internal/codegen/codegen.go:2202`), so unsupported expressions fail
   at *Go* compile time instead of producing a Goop diagnostic. That
   contradicts the "safe defaults" identity.
3. The trading story is thinner than the trading docs. The repo ships
   `docs/design/12-trading-bot-safety.md`, yet `docs/examples/orderbook.goop`
   is skipped in three test files (still uses removed `newtype`), there is no
   decimal type anywhere, and branded IDs require a single-constructor ADT
   wrapper since `newtype` was removed (PARSE-MIG015).
4. The adoption surface is minimal: no playground, LSP clients for VS Code
   and Zed only. Lisette ships a playground and five editors.

---

## 4. HIGH priority

*Do these first. H1 and H2 are one-day hygiene fixes with no language-surface
impact. H3–H6 define whether Goop reaches 1.0 as a credible language.*

### H1 — Make the codegen fallback a hard error (1 day) ✅ DONE 2026-07-27

**Completed:** `errorfAt` helper; expr-dispatch default + silent `/* … */`
fallbacks now hard-error with file:line. Tests:
`TestCodegenUnhandledExprIsHardError`,
`TestCodegenUnhandledExprLocFallsBackToSrcFile`,
`TestCodegenExamplesHaveNoTODOMarkers`.

### H2 — Migrate or delete the orderbook example (1 day) ✅ DONE 2026-07-27

**Completed:** Example already used single-ctor ADTs; `goop check`/`build`
succeed. Restored parser/typecheck/codegen tests; removed stale skips.
Also restored `result.goop` tests (same stale-skip hygiene).

Rule going forward: an example that does not compile is a failing test,
never a skip.

### H3 — Web playground (2–4 weeks) ✅ DONE 2026-07-27 (MVP)

**Completed:** `playground/` static UI + `src/cmd/playground-wasm` WASM
wrapper (`check`/`compile` → diagnostics + generated Go). Embedded examples
in `examples.js` (incl. trading/orderbook). Build: see `playground/README.md`
/`build.sh`. Hosting (GitHub Pages / custom domain) still optional follow-up.

### H4 — Decide the branded-ID story before the freeze (design, 1–2 weeks) ✅ DONE 2026-07-27

**Decision:** Surface = **(b)** single-ctor ADT + `private`; implementation
path = **(c)** zero-cost codegen for single-ctor primitive/string wraps when
safe. **(a)** restore `newtype` rejected for 1.0 (PARSE-MIG015 / STYLE).

Documented in [`docs/design/21-branded-ids.md`](docs/design/21-branded-ids.md).
Zero-cost lowering planned there (optimizer TODO; not blocking design close).

### H5 — Curated auto-sig generator (3–4 months) ✅ DONE 2026-07-27 (foundation)

**Completed:** `src/internal/gosiggen/` (`go/types` → `.gosig`), cache under
`$GOOP_HOME/build/go-sigs/`, override dir `goop-sigs/`, CLI `goop gen-sig`
(`--smoke` / `--curated`). Design: [`docs/design/23-gosig-generator.md`](docs/design/23-gosig-generator.md).
`(T, error)` left as `T * error` + `TODO(H6)` pending H6.

### H6 — Auto-coerce `(T, error)` → `result` (1 month) ✅ DONE 2026-07-27

**Completed:** Typecheck rewrites final `(T, error)` to `result<T, error>` for
`import go` (source of truth even when `.gosig` still shows the product).
Codegen emits Ok/Err wrappers. Opt out: `import go raw "…"`.

---

## 5. MEDIUM priority

### M1 — Decimal/money prelude wrapper (1–2 weeks) ✅ DONE 2026-07-27 (MVP)

**Completed:** `std/decimal` wrapping `shopspring/decimal`; example
`docs/examples/decimal_money.goop`; docs `25-decimal.md` / `std-decimal.md`;
`goop.toml` mapping. OfString uses RequireFromString until H6 lands.

### M2 — Neovim and Helix LSP clients (1–2 weeks) ✅ DONE 2026-07-27

**Completed:** `editors/neovim/`, `editors/helix/`; README mentions four editors.

### M3 — String interpolation: decide yes/no ✅ DONE 2026-07-27 — CLOSED NO

**Decision: NO for 1.0.** [`docs/design/22-string-interpolation.md`](docs/design/22-string-interpolation.md).
Use `fmt.Sprintf` via Go interop. Reopen post-1.0 only with STYLE exception.

### M4 — Stdlib surface consistency (1–2 weeks) ✅ DONE 2026-07-27

**Doctrine:** Go’s stdlib is the Goop stdlib; `std.*` is OCaml/Goop-native only.
**Completed:** doctrine in `docs/stdlib/README.md`; thin `std.string`,
`std.chan`→`std/channel`, `std.lazy`, `std.ref`.

### M5 — Close the known inference holes (2–4 weeks) ✅ DONE 2026-07-27

**Completed:** Catalogue + quality errors for removed/unsupported constructs;
`|>` binary inference; defensive internal-error default.
[`docs/design/24-inference-holes.md`](docs/design/24-inference-holes.md).

### M6 — Extern multi-value tuple returns (1 week) ✅ DONE 2026-07-27

**Completed:** `refineExternType` builds Goop tuples for multi-results;
codegen already emits multi-value assignment. Tests:
`TestExternDeclaredTupleReturn`, `TestExternImportTupleReturn`,
`TestExternTupleCallCodegen`.

### M7 — Arbitrary-package auto-sig generation ✅ DONE 2026-07-27 (MVP)

**Completed:** `goop get-go-sig <path>` + resolution order doc
[`docs/design/28-go-sig-resolution.md`](docs/design/28-go-sig-resolution.md).

### M8 — FFI boundary guarantees document ✅ DONE 2026-07-27

**Completed:** [`docs/design/27-ffi-boundary.md`](docs/design/27-ffi-boundary.md).

### M9 — `goop doc` generator ✅ DONE 2026-07-27 (MVP)

**Completed:** `goop doc <file-or-dir>` → markdown on stdout (`src/cmd/goop/doc.go`).

### M10 — Benchmarks: generated Go vs hand-written Go ✅ DONE 2026-07-27 (harness)

**Completed:** `benchmarks/` (`list_fold`, `adt_match`, `branded_id`) + `run.sh`.

---

## 6. LOW priority

### L1 — Standalone `goop lint` ✅ DONE 2026-07-27

**Completed:** `goop lint <file-or-dir>`; summary counts; diagnostic corpus note
[`docs/design/26-diagnostics.md`](docs/design/26-diagnostics.md).

### L2 — REPL ✅ DONE 2026-07-27 (MVP)

**Completed:** `goop repl` — compile-to-Go session REPL (`:help`, `:q`, `:type`).

### L3 — Package-registry story documented ✅ DONE 2026-07-27

**Completed:** Go modules are the 1.0 registry (`05-modules-and-packages.md`,
`11-package-manager.md`).

### L4 — Windows CI ✅ DONE 2026-07-27

**Completed:** `test-windows` job in `.github/workflows/ci.yml`.

### L5 — Version constant in Go source ✅ DONE 2026-07-27

**Completed:** `src/internal/version` + `goop version`.

### L6 — Self-hosting compiler ❌ NOT PLANNED

Removed from the roadmap. The compiler stays Go; see non-goals in
`docs/design/01-overview.md`. `spike/selfhost-lexer/` is archival only.

---

## 7. Best practices

### 7.1 For maintainers

- **No new syntax for interop.** If a Go idiom is awkward to express, fix
  the sig generator or the type bridge, not the parser.
- **No silent degradation.** The compiler errors at Goop compile time with a
  file:line, or it succeeds. It never emits known-broken Go. (H1 is the
  first enforcement of this rule.)
- **Examples are CI-enforced.** An example that does not compile is a
  failing test, never a skip. (H2 is the first enforcement.)
- **Preserve Go selector spelling.** Do not auto-remap `Key` → `key` or
  `WriteString` → `write_string` at the boundary.
- **Safe defaults.** `(T, error)` → `result` is the default; the raw tuple
  is the opt-out. Nil stays `ptr`/`null`; do not collapse it to `option`.
- **Cache-only artifacts.** Generated `.gosig` files live in the build
  cache, not in the user's repo.
- **Override path documented.** Users must be able to replace a generated
  sig with a hand-curated one, and the compiler must pick the override
  reliably.

### 7.2 For users

- Prefer generated sigs for all Go packages.
- Use `result` combinators (`Result.bind`, `Result.map`, etc.) to handle Go
  errors; do not expect a `?` operator.
- Wrap returned primitive types in branded IDs when domain safety matters.
  (The mechanism — single-ctor ADT + planned zero-cost lowering — is H4.)
- Keep `@[go]` escape blocks small and isolated. They are the boundary where
  Goop's static guarantees disappear.

---

## 8. Priorities and rough timeline

| # | Tier | Work | Status |
|---|------|------|--------|
| H1 | HIGH | Codegen fallback → hard error | ✅ DONE |
| H2 | HIGH | Migrate/delete orderbook example | ✅ DONE |
| H3 | HIGH | Web playground (WASM) | ✅ DONE (MVP) |
| H4 | HIGH | Branded-ID decision | ✅ DONE |
| H5 | HIGH | Curated auto-sig generator | ✅ DONE (foundation) |
| H6 | HIGH | `(T, error)` → `result` coercion | ✅ DONE |
| M1 | MED | Decimal prelude wrapper | ✅ DONE (MVP) |
| M2 | MED | Neovim + Helix LSP clients | ✅ DONE |
| M3 | MED | String interpolation decision | ✅ CLOSED NO |
| M4 | MED | Stdlib surface consistency | ✅ DONE |
| M5 | MED | Close inference holes | ✅ DONE |
| M6 | MED | Extern tuple returns | ✅ DONE |
| M7 | MED | Arbitrary-package auto-sig | ✅ DONE (MVP) |
| M8 | MED | FFI boundary guarantees doc | ✅ DONE |
| M9 | MED | `goop doc` generator | ✅ DONE (MVP) |
| M10 | MED | Benchmarks vs hand-written Go | ✅ DONE (harness) |
| L1 | LOW | Standalone `goop lint` | ✅ DONE |
| L2 | LOW | REPL | ✅ DONE (MVP) |
| L3 | LOW | Registry story documented | ✅ DONE |
| L4 | LOW | Windows CI | ✅ DONE |
| L5 | LOW | Version constant | ✅ DONE |
| L6 | LOW | Self-hosting compiler | ❌ NOT PLANNED |

---

## 9. Open questions

- Should the auto-sig generator live in `goop` itself, or as a separate
  `goop-sig` tool? (Recommendation: start inside `goop`, extract later if
  the cache/CLI surface becomes too complex.) — *Started inside `goop`.*
- How do we represent Go generics in Goop signatures? Go 1.18+ generics are
  real in the wild. A first pass can monomorphize common instantiations or
  emit a warning; a proper design is needed for 1.0.
- ~~What is the exact override resolution order…~~ → see
  [`28-go-sig-resolution.md`](docs/design/28-go-sig-resolution.md).
- How do we test generated sigs in CI? (Suggestion: generate for the curated
  set, run `goop check` against a corpus of example files, fail on new
  warnings.)
- ~~**(H4)** Branded IDs…~~ **Decided:** ADT surface + zero-cost plan
  (`21-branded-ids.md`).
- ~~**(M3)** String interpolation…~~ **NO for 1.0**
  (`22-string-interpolation.md`).
- ~~**(M1)** Which decimal library…~~ **`shopspring/decimal`.**

## 10. Related documents

- `docs/design/STYLE.md` — language surface, including removed features
- `TODO.md` — remaining work (stdlib / 1.0; self-host explicitly not planned)
- `docs/design/07-roadmap.md` — phased plan
- `docs/design/12-trading-bot-safety.md` — the domain-fit motivation for H4, M1
- `docs/design/18-go-methods.md` — FFI method/field lowering guarantees
- `docs/design/20-cli-artifacts.md` — cache-only build model
- `docs/design/21-branded-ids.md` — H4 branded-ID decision
- `docs/design/22-string-interpolation.md` — M3 decision (NO for 1.0)
- `docs/design/23-gosig-generator.md` — H5 generator
- `docs/design/24-inference-holes.md` — M5 catalogue
- `docs/design/25-decimal.md` — M1 decimal
- `docs/design/26-diagnostics.md` — L1 diagnostic corpus
- `docs/design/27-ffi-boundary.md` — M8 FFI guarantees
- `docs/design/28-go-sig-resolution.md` — M7 resolution order
