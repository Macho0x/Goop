# Goop Roadmap

This file presents the same task list as `TODO.md`, organized by development phase.

## Phase 0: Design and bootstrap

- [x] Establish design principles and language name.
- [x] Write core design documents.
- [x] Write specification drafts.
- [x] Bootstrap compiler skeleton in Go.
- [x] Tokenizer and lexer.
- [x] Parser producing an AST.
- [x] Pretty-printer for the AST.
- [x] CLI: `goop lex`, `goop parse`, `goop check`, `goop compile`, `goop build`, `goop test`, `goop get`, `goop resolve`, `goop fmt`, `goop lsp`.

## Phase 1: Minimal viable compiler

- [x] Type checker with HM inference.
- [x] ADT and pattern matching support.
- [x] `option` and `result` built-ins.
- [x] Basic Go code generation.
- [x] End-to-end compile of `hello.goop` to runnable Go binary.
- [x] Source map generation.
- [x] Exhaustive pattern-match checking.

## Phase 2: Usable language

- [x] Modules and imports.
- [x] Records and tuples.
- [x] Lists.
- [x] Standard library prelude.
- [x] Go interop (`import go`, `@[go]` embed blocks).
- [x] Active patterns.
- [x] Feature flags via `goop.toml`.
- [x] Test runner.
- [x] Mutable record fields; `ref` / `!` / `:=` (1.0; `let mutable` removed).

## Phase 3: Production features

- [x] Generics and type constructors (parametric polymorphism; HKT deferred).
- [x] Concurrency primitives (goroutines, channels, select).
- [x] Row polymorphism.
- [x] Layered lambda type inference (bidirectional inference + optional `go/types` fallback).
- [x] Linear resource types (modal linearity, opt-in for resource-kinded types).
- [x] Runtime refinement contracts (`where` clauses) + call-site codegen.
- [x] Built-in refinement solver + optional Z3 SMT.
- [x] Source locations for type errors (file:line:col in all error messages).
- [x] Goroutine sharing analysis (compile-time data race detection).
- [x] Runtime-safe `Chan.close` (closed flag wrapper).
- [x] `OwnedChan` linear channel wrapper.
- [x] `go (move ...)` syntax and linear `go` handoff.
- [x] `goop.toml` check severities (`concurrent`, `refinement_unproven`, `smt`).
- [x] Nil channel detection (flow-sensitive initialization checking).
- [x] Unified import syntax (`import go` / `import goop`).
- [x] **Go interface FFI (1.3.0):** `type` imports, `implements`, pointer/null, `error`, and `go_slice`; native `fmt.Stringer` and `slog.Handler` examples.
- [x] **Go method/field FFI (1.4.0):** `val (x : T).M` selector imports, method calls, field reads, callbacks, `go_slice` indexing, and variadic `any`.
- [x] **Call lowering (1.5.0):** capitalized multi-arg apps, unit param erasure, if-as-expression IIFE, consistent option suffixes ([19-call-lowering.md](19-call-lowering.md)).
- [x] **Cross-package libraries (1.6.0):** imported record literals, Capitalized open-export calls, multiline paren apps, `T ptr` for heap Go types, gosig named types/`LookupVar`, LSP URI decode ([19-call-lowering.md](19-call-lowering.md), [16-treelog-feedback.md](16-treelog-feedback.md)).
- [x] **Treelog FFI completion (1.7.0):** string escapes + `String.length`/`sub`; implementor `ptr_of` → interface; slog Kind/Level; Go struct literals (`HandlerOptions`, `Mutex`, `Buffer`) ([18-go-methods.md](18-go-methods.md), [16-treelog-feedback.md](16-treelog-feedback.md)).
- [x] **Cache-only Goop build (1.8.0):** `goop compile` / `goop build` write under `$GOOP_HOME/build` by default; build wires transitive `import goop` deps; `--in-tree` / `--emit-map` escape hatches ([20-cli-artifacts.md](20-cli-artifacts.md)).
- [x] Package manager (`goop get`, `goop.lock`).
- [x] **OCaml alignment (1.0):** `ref`/`while`/`function`/exceptions/`failwith`/`mod`; remove F# CE, `?`, Kit macros, `newtype`, effect rows, `panic`, `%`.
- [x] **Effect handlers (1.0):** OCaml 5-style `effect` / `perform` / handlers, CPS-lowered.
- [x] Nested modules / `sig` / functors / inline sealing (no `.mli`).
- [x] **OCaml parity 1.1:** downto, local open, exception patterns, Lazy, extensible variants, GADTs/polyvars, objects, shallow effects + attr strip.

## Phase 4: Maturity

- [ ] Self-hosting compiler in Goop.
- [x] IDE support (LSP) - full implementation with diagnostics, hover, definition, completion
- [x] Formatter (`goop fmt` command)
- [ ] Comprehensive standard library.
- [x] Documentation generator (`goop doc` — see [20-cli-artifacts.md](20-cli-artifacts.md)).
- [ ] Stable 1.0 release.

## Documentation

- [x] README (safety-first layout; Go interop and inline `@[go]` sections)
- [x] Design documents ([STYLE.md](STYLE.md), [14-ocaml-parity.md](14-ocaml-parity.md), …)
- [x] Specification drafts
- [x] Examples (`docs/examples/`; CI checks all files)
- [x] `goop.toml` project configuration
- [x] Package manager guide (`docs/design/11-package-manager.md`)
- [x] Language tutorial — `docs/tutorial/` (7 chapters, CI-linked examples)
- [x] Standard library reference — `docs/stdlib/` (hand-written from prelude.go + std/*)
- [x] Contributing guide — `CONTRIBUTING.md` (build, editors, documentation accuracy)

## Deferred or rejected

- Full dependent types (Idris/Agda style) — SMT refinements cover the practical fragment; see `docs/design/08-deferred-features-analysis.md`.
- Borrow checker with lifetimes (Rust style) — modal linearity for resources is the right level for a GC'd target.
- QTT 0-quantity erasure — premature without dependent types or proof terms.
- Capabilities that cannot be enforced at the Go boundary.
- Runtime layer or bytecode VM.
- F# computation expressions / Kit `is`/`as`/`guard` / Dingo `?` — **removed** in favor of OCaml forms (not deferred).
