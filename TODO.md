# Goop TODO

This file tracks the remaining work to make Goop a usable language. It is kept in sync with `docs/design/07-roadmap.md`.

## Compiler (bootstrap in Go)

- [x] Project structure and documentation
- [x] Bootstrap compiler skeleton in Go
- [x] Lexer with nested block comments and source locations
- [x] Recursive-descent parser for the core grammar
- [x] AST and pretty-printer
- [x] CLI: lex, parse, check, compile, build, test, get, resolve, fmt, lsp
- [x] Type checker with HM inference
- [x] Exhaustive pattern-match checking
- [x] Go code generation
- [x] Source maps
- [x] Imports and module resolution
- [x] Standard library prelude
- [x] Layered lambda type inference (bidirectional inference + optional `go/types` fallback)
- [x] Build system and package manager (`goop get`, `goop.lock`)

## Language features

- [x] Core syntax design (OCaml-aligned 1.0 — see [STYLE.md](docs/design/STYLE.md))
- [x] ADTs and pattern matching
- [x] Records and tuples
- [x] Lists
- [x] `option` and `result`
- [x] `ref` / `!` / `:=`, `while`, `function`, exceptions, `failwith`, `mod`
- [x] Mutable record fields
- [x] OCaml-style arrays, for loops, begin/end, qualified constructors
- [x] Active patterns
- [x] Row polymorphism
- [x] Concurrency primitives (`go`, `chan`, `select`)
- [x] Go interop (`import go`, `@[go] { }` embed blocks)
- [x] Unified imports (`import go` / `import goop`, dot and aliased forms)
- [x] Go interface FFI (1.3.0): `type` imports, `implements`, `ptr`/`null`, `error`, and `go_slice`; Stringer and slog.Handler examples
- [x] Go method/field FFI (1.4.0): `val (x : T).M` imports, selector lowering, callbacks, `go_slice` indexing, and variadic `any`
- [x] `private` module visibility; branding via single-ctor ADT (no `newtype`)
- [x] Nested modules / `sig` / functors / `.mli` (minimal)
- [x] Golang import 2-tuple returns
- [x] Flow-sensitive goroutine liveness (fewer race false positives)
- [x] std/ modules (`std.io`, `std.list`, `std.array`, `std.option`, `std.result`)
- [x] Effect handlers (OCaml 5-style, CPS-lowered)
- [x] Linear resource types (modal linearity, opt-in for resource-kinded types)
- [x] Runtime refinement contracts + optional Z3 SMT

## Compile-time safety checks

- [x] Source locations for type errors (file:line:col in all error messages)
- [x] Goroutine sharing analysis (compile-time data race detection for mutable captures)
- [x] Runtime-safe `Chan.close` (closed flag wrapper, clear panic messages)
- [x] `OwnedChan` linear channel wrapper (compile-time close safety via linear discharge checking)
- [x] Built-in refinement solver (compile-time VC checking for integer arithmetic)
- [x] Optional Z3 SMT for refinements (`[check] smt = true`)
- [x] Refinement call-site codegen (proven VCs skip guards; exported entry guards)
- [x] Arithmetic refinement solver extensions
- [x] Linear `go` handoff for owned resources
- [x] `go (move ...)` syntax for explicit goroutine transfer
- [x] `goop.toml` severities: `concurrent`, `refinement_unproven`

## Tooling

- [x] LSP server (features: diagnostics, hover, definition, completion)
- [x] Formatter (`goop fmt` via `src/internal/fmt`, parse-tree round-trip)
- [x] Channel-mediated race tracking (LINEAR008)
- [x] Narrow static deadlock lint (DEADLOCK001)
- [x] Test runner
- [x] Cache-only `goop compile` / `goop build` (1.8.0; `$GOOP_HOME/build`; see `docs/design/20-cli-artifacts.md`)
- [x] Documentation generator (`goop doc` on modules; expand coverage as needed)

## Documentation

- [x] README (safety-first layout; interop and `@[go]` sections)
- [x] Design documents
- [x] Specification drafts
- [x] Examples (`docs/examples/`; CI `goop check` on all files)
- [x] `goop.toml` project configuration
- [x] Package manager (`goop get`, `goop.lock`; see `docs/design/11-package-manager.md`)
- [x] Language tutorial — [docs/tutorial/](docs/tutorial/) (7 chapters + examples)
- [x] Standard library reference — [docs/stdlib/](docs/stdlib/) (prelude, builtins, std.*)
- [x] Contributing guide — [CONTRIBUTING.md](CONTRIBUTING.md) (build, editors, doc accuracy)

## Long term

- [ ] Self-hosting compiler
- [ ] Comprehensive standard library
- [ ] Stable 1.0 release

## Deferred or rejected

- Full dependent types (Idris/Agda style) — SMT refinements cover the practical fragment; see `docs/design/08-deferred-features-analysis.md`.
- Borrow checker with lifetimes (Rust style) — modal linearity for resources is the right level for a GC'd target.
- QTT 0-quantity erasure — premature without dependent types or proof terms.
- Capabilities that cannot be enforced at the Go boundary.
- Runtime layer or bytecode VM.
- F# CEs / Kit macros / Dingo `?` / effect rows / `newtype` / `panic` / `%` — **removed** in 1.0 (not deferred).
