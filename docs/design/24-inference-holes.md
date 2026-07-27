# 21 — Type inference holes (1.0 freeze)

Status: closed for the known `"type inference not implemented"` paths (M5).
This catalogue records what Hindley–Milner inference supports in Goop 1.0 versus
constructs that are permanently restricted with user-facing diagnostics.

## Catalogue of former holes

| Site | Former message | Resolution |
|------|----------------|------------|
| `infer` default (`typecheck.go`) | `type inference not implemented for %T` | **Permanent internal error**: every known `ast.Expr` has a case; the default is a compiler-bug report asking the user to rewrite / annotate. |
| `inferBinary` default | `type inference not implemented for binary operator %s` | **Implemented** `==` and `\|>` (parser emitted them as `BinaryExpr` but they were missing from the switch). Remaining ops hit a **permanent restriction** naming the operator, listing supported ops, and suggesting annotation + rewrite. |
| `inferBinary` `%` | `'%' was removed…` | **Permanent restriction** (parser also rejects `%`; typecheck message kept for hand-built AST). Use `mod`. |
| `QuestionExpr` | `'?' operator was removed…` | **Permanent restriction** — use `match` on `result` / `Result.bind`. |
| `GuardExpr` / `IsExpr` / `AsMatchExpr` | thin “was removed” / silent infer | **Permanent restriction** — use `match` (+ `when`). Surface also rejected at parse (`PARSE-MIG014`). |
| `CompExpr` | `computation expressions were removed` | **Permanent restriction** naming the builder — use `match` / `try`/`finally`. |
| `RegionExpr` | silent CE inference | **Permanent restriction** — use `try`/`finally` or explicit cleanup. |
| `ModuleAppExpr` | silent `unit` | **Permanent restriction** — functor application is not an expression form in 1.0. |

No other `"type inference not implemented"` / incomplete-inference strings remain under `src/internal/typecheck/`.

## Supported for 1.0 (inferred)

Expressions with dedicated infer paths (non-exhaustive of the whole language,
focused on former hole areas):

- Literals, identifiers, constructors, application, `if`, `match`, `let … in`,
  `fun` / `function`, records / updates / field access, tuples, lists, arrays,
  indexing, assignment (`:=` / `<-`), `for` / `while` / `begin`, `ref` / `!`,
  `try` / `raise` / `assert` / `lazy` / `perform`, objects / `new`, local
  `module` / `open`, packing/unpacking first-class modules, labelled args,
  `go` / `select` / `using`, method send, polyvars, ptr helpers.
- **Binary operators**: `+ - * /`, `mod land lor lxor`, `+. -. *. /.`,
  `= == != <> < > <= >=`, `^`, `&& ||`, `::`, `\|>`.
- **Pipe**: `x \|> f` (as `BinaryExpr` from the parser, or legacy `PipeExpr`).

## Permanently restricted for 1.0

| Construct | Diagnostic guidance |
|-----------|---------------------|
| `%` | use `mod` |
| `?` propagation | `match` on `('ok, 'err) result` |
| `guard` / `is` / expr `as … else …` | `match` (+ `when`) |
| `result { }` / `async { }` / other CE builders | `match` / ordinary control flow |
| `region { }` | `try`/`finally` or explicit cleanup |
| Functor app as expression `F(M)` | nested `module` + `sig`/`struct` |
| Unknown binary op / unknown AST node | rewrite to supported forms; annotate enclosing binding; report if it should be valid Goop |

Parser migration errors (`PARSE-MIG*`) remain the first line of defense for
removed surface syntax; typecheck messages cover AST that still reaches
inference (legacy desugar paths, tools, or compiler bugs).

## Out of scope (not M5 holes)

- Extern multi-value tuple returns (M6) — separate FFI limitation.
- Incomplete module/functor typing beyond the expression stub above.
- Refinement / effect extras — see other design docs.
