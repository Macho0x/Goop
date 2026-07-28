# Goop Lowering to Go

This document describes how Goop constructs are translated into Go. The target is Go 1.26 or later.

Pure / non-effectful code aims for idiomatic Go. Effect-handler code may emit CPS / free-monad helpers (documented exception).

## Identifiers

- Goop identifiers become title-cased for exported Go identifiers.
- Type variables are erased; generic functions are monomorphized or use Go generics where applicable.
- Goop keywords that conflict with Go keywords are escaped with a trailing underscore.

## Types

| Goop | Go |
|---|---|
| `int` | `int` |
| `int64` | `int64` |
| `float` | `float64` |
| `bool` | `bool` |
| `string` | `string` |
| `bytes` | `[]byte` |
| `unit` | `struct{}` |
| `'a list` | `[]T` |
| `'a array` | `[]T` |
| `'a ref` | `*T` |
| `'a option` | generated `OptionT` struct |
| `('ok, 'err) result` | generated `ResultOkErr` struct |
| tuple | generated struct with positional fields |
| record | Go struct with exported fields |
| ADT | Go interface + one struct per variant |

## Expressions

| Goop | Go |
|---|---|
| `let x = e1 in e2` | `x := e1; e2` or `const x = e1; e2` |
| `if c then t else f` | `if c { t } else { f }` |
| `match e with \| Pi -> ei` | `switch v := e.(type) { case ... }` |
| `f x y` | `f(x, y)` for fully applied multi-arg functions |
| `fun x -> e` | `func(x T) U { return e }` |
| `function \| P -> e` | `func(x T) U { switch … }` |
| `x :: xs` | `append([]T{x}, xs...)` |
| `[a; b; c]` | `[]T{a, b, c}` |
| `{ x = 1; y = 2 }` | `Point{X: 1, Y: 2}` |
| `ref e` | `func() *T { p := new(T); *p = e; return p }()` (or equivalent) |
| `!r` | `*r` |
| `r := e` | `*r = e` |
| `Array.make n default` | `make([]T, n)` + init loop |
| `Array.length arr` | `len(arr)` |
| `arr.(i)` | `arr[i]` |
| `arr.(i) <- v` | `arr[i] = v` |
| `for i = lo to hi do body done` | `for i := lo; i <= hi; i++ { body }` |
| `while c do body done` | `for c { body }` |
| `begin e1; e2; en end` | block or IIFE |
| `failwith msg` | `panic(msg)` |
| `raise e` | `panic(e)` |
| `try e with \| …` | `defer` + `recover` wrapper |
| `try e finally f` | `defer f; e` |
| `perform e` | `__goop_perform(e)` (CPS path) |
| `r.x` | `r.X` |
| `e \|> f` | `f(e)` |
| `n mod m` | `n % m` (Go remainder) |

## Pattern guards

Guards lower to `if` statements inside the corresponding type-switch case.

## Option and Result

Same tagged-struct pattern as before (`OptionTag`, `NewOptionIntSome`, …). Prefer `match` in source — there is no `?` lowering.

## Modules

A Goop module `Foo.Bar` lowers to Go package `bar` in directory `foo/bar`. Nested `struct` modules are handled by the module pass (minimal). Exported values are title-cased.

## Match exhaustiveness

The compiler proves exhaustiveness. The lowered Go type switch includes a `default` that panics (unreachable for well-typed Goop).

## Partial application

Partially applied functions → closures. Fully applied multi-parameter functions → direct multi-parameter Go functions.

## Effect handlers

Effectful functions are CPS-rewritten before codegen. Pure functions are unchanged.

| Goop | Go |
|---|---|
| no `perform` | direct-style |
| `perform` / handlers | `__goop_perform` / `__goop_handle` CPS runtime |

## Refinement contracts

| Goop | Go |
|---|---|
| `(x: T where P)` parameter | `if !(P) { panic(...) }` at entry (unless proven) |
| `: U where Q` return type | `defer` check with named return |

## Linear types

| Goop | Go |
|---|---|
| `type handle : 1` | `type Handle = interface{}` or extern Go type |
| `f (h: handle) : unit` | `func F(h Handle) { ... }` |

## Source locations

Each generated Go construct maps back to the originating `.goop` location via comments or source maps.
