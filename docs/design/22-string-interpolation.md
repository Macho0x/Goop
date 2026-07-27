# String interpolation (M3)

**Decision for Goop 1.0: NO**

String interpolation is deferred past 1.0. Formatted strings use `fmt.Sprintf`
(and related `fmt` helpers) via Go interop. Reopen after the 1.0 surface freeze.

## Context

M3 in `GOOP_UPDATES.md` asked whether Goop should add `"…{expr}…"` inside
string literals, desugared to `Sprintf` at typecheck, with a STYLE amendment.
Peers (Lisette `f"…"`, Kit `${…}`, Rust, Swift, Kotlin, Python) ship
interpolation; OCaml core does not — programmers use `Printf` / `Format`.

## Current state (research)

| Layer | Behavior |
|-------|----------|
| Lexer (`lexString`) | Double-quoted strings; escape decoding only (`\n`, `\xHH`, …). No `{` / `$` special cases. |
| Parser | `STRING` tokens → literal expressions. No interpolation AST. |
| Typecheck / codegen | No desugar of string fragments. |
| 1.0 answer today | `import go "fmt"` + `fmt.Sprintf` / `Printf` (already exercised in tests and examples). |
| Docs | [03-syntax.md](03-syntax.md) documents escapes only; [STYLE.md](STYLE.md) has no interpolation rule. |

There is **no** existing interpolation surface to revive.

## Why NO for 1.0

1. **OCaml alignment.** STYLE principle 1: one canonical surface — the OCaml
   spelling. OCaml has no built-in string interpolation; `Printf.sprintf` is
   the idiom. Goop already mirrors that with Go's `fmt.Sprintf`. Adding
   `"{expr}"` would be a peer-language ergonomics extension, not OCaml parity
   and not required for Go interop (STYLE principle 5 reserves extensions for
   `import go` / `go` / `move`).

2. **Freeze risk.** A minimal-looking desugar still needs lexer or post-lex
   rescanning to split literals, parse embedded expressions, escape rules for
   literal `{` / `}`, and error reporting for nested braces / unterminated
   fragments. That is new lexer/parser surface during the 1.0 freeze window.
   Even if the *post*-desugar AST were ordinary `App` nodes, the *front-end*
   complexity is not free.

3. **Ambiguity cost outweighs gain.** Format strings with braces
   (`"{name} = %d"` vs interpolation), nesting, and `"\{"` / `"{{"` conventions
   need a full design. `fmt.Sprintf` already covers formatting without new
   syntax.

4. **Guidance for this decision.** Prefer NO when freeze risk or OCaml
   alignment argues against it; YES only if desugar is clean *without* new AST
   complexity *and* STYLE absorbs it. The first two conditions fail; STYLE
   would need a non-OCaml exception for little 1.0 value.

## 1.0 idiomatic answer

```goop
module Example

import go "fmt"

let greet (name : string) (n : int) : string =
  fmt.Sprintf "hello %s (%d)" name n
```

Use `%s`, `%d`, `%v`, etc. as in Go. Prefer explicit imports of `fmt` rather
than inventing a second formatting surface in the prelude.

## Post-1.0 reopen criteria

Consider again only if all of the following hold:

- 1.0 surface is frozen and shipped.
- A concrete proposal specifies lexer rules, escaping, and desugar target
  (likely still `fmt.Sprintf`) with tests and a STYLE amendment that does not
  contradict OCaml-first principles — or STYLE is deliberately revised.
- Evidence that `Sprintf` friction is high enough in real Goop codebases to
  justify the surface change.

Candidate designs to evaluate later (not chosen now): Lisette-style `f"…"`,
Kit-style `${…}`, or brace-in-string `"…{expr}…"`.

## Non-goals for this decision

- No lexer/parser/typecheck changes.
- No STYLE.md amendment.
- No prelude `sprintf` wrapper (stdlib consistency is M4, separate).

## Related

- [STYLE.md](STYLE.md) — OCaml-aligned surface principles
- [03-syntax.md](03-syntax.md) — string/character literals
- [14-ocaml-parity.md](14-ocaml-parity.md) — parity matrix
- `GOOP_UPDATES.md` § M3
