# Goop Language Overview

## What Goop is

Goop is a compiled, statically typed programming language with OCaml-aligned syntax that targets Go. It is designed for programmers who want OCaml-style type safety, pattern matching, and inference, but who also want Go's runtime, concurrency model, standard library, and deployment characteristics.

## Core principles

1. **Type safety first.** Null safety, exhaustive pattern matching, sound inference, and explicit error handling are defaults, not opt-in features.
2. **OCaml-aligned surface.** Prefer OCaml spelling for every construct; remove duplicate non-OCaml sugar (see [STYLE.md](STYLE.md)).
3. **Compiles to Go.** Pure / non-effectful code emits idiomatic Go. Effect-handler code may emit CPS / free-monad Go (intentional exception).
4. **Eat our own dog food.** The Goop compiler itself is written in Go.
5. **Interop is the point.** Go-style `import go` / `import goop` and `@[go]` embeds are first-class.
6. **Incremental adoption.** Teams can introduce Goop file-by-file.

## Relationship to other languages

| Language | What Goop borrows | What Goop rejects |
|---|---|---|
| **OCaml** | Syntax, modules, functors, objects, exceptions, GADTs, poly variants, OCaml 5 effects (CPS-lowered), pattern matching | Full OCaml stdlib / runtime |
| **Go** | Runtime, goroutines (`go`), channels, standard library, deployment, import paths | Nil everywhere, lack of sum types |
| **Rust** | Result/Option discipline, linear resources (`: 1`) | Full borrow checker / lifetimes as user-facing |

F# computation expressions, Kit match macros (`is`/`as`/`guard`), and Dingo `?` propagation were removed in 1.0 in favor of OCaml forms.

## The design constraint

> **For non-effectful code, the Go emitted by the Goop compiler must be the kind of Go a senior Go engineer would write by hand.**
>
> **For OCaml 5-style resumptive effect handlers, Goop lowers via CPS / free-monad. That output is not idiomatic hand-written Go; this is an explicit, documented exception.**

This means:

- Sum types lower to Go interfaces + per-variant structs (the pattern used by `go/ast`).
- `match` lowers to Go type switches.
- Recoverable errors use `result` + `match` (not `?`).
- Effectful functions may be CPS-transformed before codegen.
- SMT-backed refinements (Z3) prove or insert runtime guards.

## Non-goals

- **Self-hosting the compiler in Goop — not planned.** The compiler remains a
  Go program; there is no L6 / self-host port on the roadmap.
- A full Rust-style borrow checker with lifetimes (linear `: 1` remains).
- Direct machine-code output.
- Replacing Go entirely.
- Bit-identical OCaml stdlib (`Printf`, `Unix`, …) — use `import go`.

## Target audience

- Go teams that want stronger types without leaving the ecosystem.
- ML/Rust developers who need Go's deployment and library story.
- Domain-specific tooling where correctness matters (trading systems, infrastructure, protocols).
