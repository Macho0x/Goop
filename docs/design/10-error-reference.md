# Goop Compile-Time Error Reference

This document catalogs compile-time errors and warnings the Goop compiler
can produce. It is designed for LLM-assisted debugging: given an error
message, an LLM can look up the error code, understand the cause, and
suggest a fix.

## On the wire vs catalog IDs

Many codes are **on the wire**: the compiler prefixes the human message with
`CODE:` (for example `TYPE011: cannot assign…`, `RESULT001: …`). Those prefixes
are stable and match `help:` tips from `internal/report`.

Some older catalog entries are **documentation IDs only** (historical PARSE-MIG
notes, or CLI wrappers). If a code is not emitted by the current compiler, the
entry says so. Prefer codes you actually see in `goop check` / `goop lint` output.

---

## How errors are reported

Goop errors include source locations in the format `file:line:col: message`.
Warnings are reported on stderr but do not stop compilation. Errors stop
compilation and return exit code 1.

Error messages fall into three severity levels:

| Level | Behavior |
|---|---|
| **Error** | Stops compilation; reported on stderr; exit code 1 |
| **Warning** | Reported on stderr; does NOT stop compilation |
| **Silent** | No output (e.g., proven refinements skip runtime checks) |

---

## Categories

| Prefix | Category | Source package |
|---|---|---|
| LEX | Lexer errors | `internal/lexer` |
| PARSE / PARSE-MIG | Parser / removed-syntax | `internal/parser` |
| TYPE | Type errors | `internal/typecheck` |
| VIS | Visibility | `internal/typecheck`, `internal/visibilitywarns` |
| IMPORT | Module imports | `internal/typecheck` |
| FFI-IMPL | Go interface implementation errors | `internal/typecheck` |
| UNIFY | Unification errors | `internal/types/unify` |
| EXHAUST | Exhaustiveness warnings | `internal/exhaustive` |
| LINEAR | Linear discharge errors | `internal/linear`, `internal/channelrace` |
| DEADLOCK | Deadlock warnings | `internal/deadlock` |
| RESULT / OPTION | Discarded result/option | `internal/discardedresult` |
| UNUSED | Unused locals/imports | `internal/unused` |
| CODEGEN | Codegen failures | `internal/codegen` |
| GOSIG | Go signature load/fallback | `internal/gosig`, CLI |
| NIL | Nil channel errors | `internal/nilchan` |
| REFINE | Refinement solver errors/warnings | `internal/refine` |
| CLI | CLI/file/system errors | `cmd/goop`, `internal/config` |

Corpus size and `goop lint` are summarized in
[26-diagnostics.md](26-diagnostics.md).

---

## LEX — Lexer Errors

Lexer errors prevent token production. The lexer stops on the first error;
the parser cannot proceed.

### LEX001: Unexpected character

- **Error code**: `LEX001`
- **Severity**: Error
- **Message**: `unexpected character %q`
- **Example**: `test.goop:5:3: unexpected character '@'`
- **Trigger**: The lexer encounters a character that is not a valid Goop token
  start. Goop recognizes letters, digits, underscores, operators (`+`, `-`,
  `*`, `/`, `=`, `<`, `>`, `!`, `|`, `&`, `.`, `^`, `~`, `:`, `;`, `,`,
  `?`, `%`), and delimiters (`(`, `)`, `[`, `]`, `{`, `}`, `"`, `'`).
  Characters like `@`, `#`, `` ` `` produce this error.
- **Fix**: Remove or replace the invalid character with valid Goop syntax.
- **Bad**: `let x = @ 1`
- **Good**: `let x = 1`

### LEX002: Unterminated block comment

- **Error code**: `LEX002`
- **Severity**: Error
- **Message**: `unterminated block comment (depth %d)`
- **Example**: `test.goop:1:1: unterminated block comment (depth 1)`
- **Trigger**: A block comment `(* ... *)` is opened but never closed before
  end of file. The `depth` indicates the nesting level — a value > 1 means
  you opened more `(*` than you closed `*)`.
- **Fix**: Add the missing `*)` to close the block comment.
- **Bad**:
  ```goop
  (* This is a comment that never ends
  let x = 1
  ```
- **Good**:
  ```goop
  (* This is a comment *)
  let x = 1
  ```

### LEX003: Unterminated string literal (newline)

- **Error code**: `LEX003`
- **Severity**: Error
- **Message**: `unterminated string literal`
- **Example**: `test.goop:3:10: unterminated string literal`
- **Trigger**: A string literal `"..."` contains a newline before the closing
  `"`. Goop does not support multi-line string literals.
- **Fix**: Close the string on the same line, or use `\n` for newlines.
- **Bad**:
  ```goop
  let s = "hello
  world"
  ```
- **Good**:
  ```goop
  let s = "hello\nworld"
  ```

### LEX004: Unterminated string escape

- **Error code**: `LEX004`
- **Severity**: Error
- **Message**: `unterminated string escape`
- **Example**: `test.goop:2:15: unterminated string escape`
- **Trigger**: A backslash `\` appears at the end of a string literal or
  immediately before EOF with no following character to escape.
- **Fix**: Add the escaped character after the backslash, or remove the
  backslash.
- **Bad**: `let s = "hello\"` or `let s = "hello\`
- **Good**: `let s = "hello\\"` or `let s = "hello\""`

### LEX005: Unterminated string literal (EOF)

- **Error code**: `LEX005`
- **Severity**: Error
- **Message**: `unterminated string literal starting at %d:%d`
- **Example**: `test.goop:5:1: unterminated string literal starting at 5:1`
- **Trigger**: A string literal reaches EOF without a closing `"` (and no
  newline was encountered — see LEX003).
- **Fix**: Add the closing `"` at the end of the string.
- **Bad**:
  ```goop
  let s = "hello
  ```
  (file ends without closing quote or newline)
- **Good**:
  ```goop
  let s = "hello"
  ```

### LEX005b: Invalid / incomplete string or character escape

- **Error code**: (message-based; same ERROR token stream as other LEX codes)
- **Severity**: Error
- **Messages**:
  - `invalid string escape \%c` / `invalid character escape \%c`
  - `incomplete hex escape in string literal`
  - `invalid hex digit in string escape`
  - `octal escape out of range in string literal`
- **Example**: `test.goop:1:10: incomplete hex escape in string literal`
- **Trigger**: Unsupported escape (e.g. `\q`), `\x` with fewer than two hex
  digits, non-hex after `\x`, or octal value &gt; 255 (`\400`).
- **Fix**: Use `\n \t \r \\ \" \' \e`, `\xHH` (exactly two hex digits), or
  `\ooo` (1–3 octal digits ≤ 255).
- **Good**: `let s = "\x1b[31m"` or `let s = "\033[0m"` or `let s = "\e[0m"`

### LEX006: Unexpected end of file after single quote

- **Error code**: `LEX006`
- **Severity**: Error
- **Message**: `unexpected end of file after single quote`
- **Example**: `test.goop:3:7: unexpected end of file after single quote`
- **Trigger**: A single quote `'` is followed immediately by EOF. It could be
  the start of a character literal `'x'` or a type variable `'a`, but the
  file ends before the lexer can determine which.
- **Fix**: Complete the character literal or type variable.
- **Bad**:
  ```goop
  let x = '
  ```
- **Good**: `let x = 'a'` or `let f (x : 'a) = x`

### LEX007: Unterminated character literal

- **Error code**: `LEX007`
- **Severity**: Error
- **Message**: `unterminated character literal`
- **Example**: `test.goop:2:10: unterminated character literal`
- **Trigger**: A single quote starts a character literal but the closing `'`
  is missing. This happens when the content between quotes is not followed
  by another single quote (e.g., due to a newline or unexpected character).
- **Fix**: Add the closing `'`.
- **Bad**: `let ch = 'ab'`
- **Good**: `let ch = 'a'`

### LEX008: Invalid float literal

- **Error code**: `LEX008`
- **Severity**: Error
- **Message**: `invalid float literal %q`
- **Example**: `test.goop:1:5: invalid float literal "1.2e999999"`
- **Trigger**: A token that looks like a floating-point number (digits `.`
  digits) cannot be parsed as a valid 64-bit float. This can happen with
  extreme exponents that overflow `float64`.
- **Fix**: Use a value within the `float64` range (approximately
  `±1.8e308`).
- **Bad**: `let f = 1e9999`
- **Good**: `let f = 1.5`

### LEX009: Invalid integer literal

- **Error code**: `LEX009`
- **Severity**: Error
- **Message**: `invalid integer literal %q`
- **Example**: `test.goop:1:5: invalid integer literal "99999999999999999999"`
- **Trigger**: A token that looks like an integer cannot be parsed as a
  valid 64-bit signed integer. This happens when the value exceeds
  `9223372036854775807` (max int64).
- **Fix**: Use a smaller integer value, or use a float if precision is less
  critical.
- **Bad**: `let n = 99999999999999999999`
- **Good**: `let n = 42`

---

## PARSE — Parser Errors

Parser errors indicate that the token stream does not conform to Goop's
grammar. The parser attempts error recovery to report multiple errors.

### PARSE-MIG001: (retired — `open` remains valid)

- **Status**: Not emitted. `open M` is still valid Goop (module opening).
- **Note**: Older drafts claimed `open` was removed; that was incorrect. Prefer
  `import goop "…"` for package imports; keep `open` for opening a module
  binding into the current scope.

### PARSE-MIG002: `extern "go"` removed (v0.3)

- **Error code**: `PARSE-MIG002` (on the wire)
- **Severity**: Error
- **Message**: `PARSE-MIG002: 'extern' is removed; use import go "path" { val ... }`
- **Fix**: Replace `extern "go" "fmt" { val X : … }` with `import go "fmt" { val X : … }`.

### PARSE-MIG010: `let mutable` removed (1.0)

- **Message**: `'let mutable' is removed; use ref / := / !`
- **Fix**: `let r = ref 0 in r := !r + 1` instead of `let mutable x = 0` / `x <- …`.

### PARSE-MIG012: `?` error propagation removed (1.0)

- **Message**: `'?' error propagation is removed; use match on result`
- **Fix**: `match f () with | Error e -> Error e | Ok x -> …`.

### PARSE-MIG013: Computation expressions removed (1.0)

- **Message**: `'result { … }' / async / region / using CEs are removed`
- **Fix**: Plain `match` on `result`; `try`/`finally` or explicit cleanup for resources.

### PARSE-MIG014: Kit match macros removed (1.0)

- **Message**: `'guard' / 'is' / expression 'as' macros are removed; use match`
- **Fix**: Rewrite to `match` with patterns and `when` guards.

### PARSE-MIG015: `newtype` removed (1.0)

- **Message**: `'newtype' is removed; use a single-constructor ADT and private`
- **Fix**: `type order_id = Order_id of string` (optionally `private type`).

### PARSE-MIG016: Effect rows removed (1.0)

- **Message**: `effect row with { … } is removed; use effect handlers`
- **Fix**: Drop `with { io }` on arrow types; use `effect` / `perform` / handlers when needed.

### PARSE-MIG017: `panic` keyword removed (1.0)

- **Message**: `'panic' is removed; use failwith / raise`
- **Fix**: `failwith "msg"` or `raise E`.

### PARSE-MIG018: `%` removed (1.0)

- **Message**: `'%' is removed; use mod`
- **Fix**: `n mod m` instead of `n % m`.

### PARSE001: Expected token

- **Error code**: `PARSE001`
- **Severity**: Error
- **Message**: `expected %s, got %s`
- **Example**: `test.goop:3:10: expected EQUALS, got SEMI`
- **Trigger**: The parser expects a specific token at the current position
  but finds something else. This is the most common parser error, generated
  by `p.expect(...)` at over 40 call sites throughout the parser. Common
  triggers:
  - Missing `=` in a `let` binding: `let x 42` → `expected EQUALS, got INT`
  - Missing `in` in a local `let`: `let x = 1 x` → `expected IN, got IDENT`
  - Missing `then` in `if`: `if x 42 else 0` → `expected THEN, got INT`
  - Missing `->` in a match arm: `| Some x : 1` → `expected ARROW, got COLON`
  - Missing `{` or `}` in record/block syntax
  - Missing `(` or `)` in function calls/tuples
- **Fix**: Add the expected token. Check the grammar:
  - Function or binding definition: `let name param1 param2 = body`
  - Local let: `let name = expr in body`
  - If expression: `if cond then expr else expr`
  - Match: `match expr with | pattern -> body | pattern -> body`
  - Record: `{ field1 = value1; field2 = value2 }`
- **Bad**: `let x 42`
- **Good**: `let x = 42`

### PARSE002: Unexpected `module` after first declaration

- **Error code**: `PARSE002`
- **Severity**: Error
- **Message**: `unexpected 'module' after first declaration`
- **Example**: `test.goop:5:1: unexpected 'module' after first declaration`
- **Trigger**: A `module` keyword appears after a `let`, `type`, or `extern`
  declaration. The `module` declaration must be the first statement in the
  file.
- **Fix**: Move the `module` declaration to the top of the file.
- **Bad**:
  ```goop
  let x = 1
  module MyApp
  ```
- **Good**:
  ```goop
  module MyApp
  let x = 1
  ```

### PARSE003: `import` must appear before any declarations

- **Error code**: `PARSE003`
- **Severity**: Error
- **Message**: `'open' must appear before any declarations`
- **Example**: `test.goop:5:1: 'open' must appear before any declarations`
- **Trigger**: An `open` statement appears after a `let`, `type`, or `extern`
  declaration. All `open` statements must appear immediately after `module`
  and before any declarations.
- **Fix**: Move all `open` statements to the top of the file, after `module`
  but before any `let`/`type`/`extern`.
- **Bad**:
  ```goop
  let x = 1
  open Std.IO
  ```
- **Good**:
  ```goop
  open Std.IO
  let x = 1
  ```

### PARSE004: Unexpected token at top level

- **Error code**: `PARSE004`
- **Severity**: Error
- **Message**: `unexpected token %s at top level`
- **Example**: `test.goop:10:1: unexpected token IDENT at top level`
- **Trigger**: A token appears at the top level of a module that is not the
  start of a valid top-level declaration. Top level declarations must begin
  with `let`, `type`, or `extern`.
- **Fix**: Wrap the expression in a `let` binding.
- **Bad**:
  ```goop
  module MyApp
  f x = x + 1
  ```
- **Good**:
  ```goop
  module MyApp
  let f x = x + 1
  ```

### PARSE005: Expected active pattern name

- **Error code**: `PARSE005`
- **Severity**: Error
- **Message**: `expected active pattern name, got %s`
- **Example**: `test.goop:3:3: expected active pattern name, got INT`
- **Trigger**: Inside an active pattern definition `let (|...|_|)`, the name
  between the pipes is not an identifier or constructor.
- **Fix**: Use a valid identifier between the pipes.
- **Bad**: `let (|42|_|) x = ...`
- **Good**: `let (|Positive|_|) x = x > 0`

### PARSE006: Expected binding name

- **Error code**: `PARSE006`
- **Severity**: Error
- **Message**: `expected binding name, got %s`
- **Example**: `test.goop:3:5: expected binding name, got INT`
- **Trigger**: A `let` binding is followed by something that is not a
  valid identifier or constructor name.
- **Fix**: Provide a valid binding name after `let`.
- **Bad**: `let 42 = expr` or `let = expr`
- **Good**: `let x = expr` or `let MyFunc x = ...`

### PARSE007: Expected type name

- **Error code**: `PARSE007`
- **Severity**: Error
- **Message**: `expected type name, got %s`
- **Example**: `test.goop:5:6: expected type name, got EQUALS`
- **Trigger**: The `type` keyword is followed by something that is not a
  valid type name (identifier or constructor token).
- **Fix**: Provide a valid type name.
- **Bad**: `type 123 = ...`
- **Good**: `type Color = Red | Green | Blue`

### PARSE008: Invalid linear quantity

- **Error code**: `PARSE008`
- **Severity**: Error
- **Message**: `expected '1' for linear quantity, got %v`
- **Example**: `test.goop:3:12: expected '1' for linear quantity, got 2`
- **Trigger**: A type declaration has `: N` where N is not `1`. Only
  `: 1` is valid for declaring a linear type.
- **Fix**: Use `: 1` for linear types, or remove the quantity annotation.
- **Bad**: `type handle : 2`
- **Good**: `type handle : 1`

### PARSE009: Expected string for extern language

- **Error code**: `PARSE009`
- **Severity**: Error
- **Message**: `expected string literal for extern language, got %s`
- **Example**: `test.goop:5:8: expected string literal for extern language, got IDENT`
- **Trigger**: The `extern` keyword is not followed by a string literal for
  the language name.
- **Fix**: Wrap the language name in quotes.
- **Bad**: `extern go`
- **Good**: `extern "go"`

### PARSE010: Expected string for extern path

- **Error code**: `PARSE010`
- **Severity**: Error
- **Message**: `expected string literal for extern path, got %s`
- **Example**: `test.goop:5:13: expected string literal for extern path, got IDENT`
- **Trigger**: After the language string in an `extern` declaration, the Go
  import path is not a string literal.
- **Fix**: Wrap the import path in quotes.
- **Bad**: `extern "go" fmt`
- **Good**: `extern "go" "fmt"`

### PARSE011: Expected extern binding name

- **Error code**: `PARSE011`
- **Severity**: Error
- **Message**: `expected extern binding name, got %s`
- **Example**: `test.goop:6:6: expected extern binding name, got INT`
- **Trigger**: Inside an `extern` block, a `val` is followed by something
  that is not an identifier.
- **Fix**: Provide a valid identifier name for the extern binding.
- **Bad**: `extern "go" "fmt" { val 123 : int }`
- **Good**: `extern "go" "fmt" { val Println : string -> unit }`

### PARSE012: Expected `val` inside extern block

- **Error code**: `PARSE012`
- **Severity**: Error
- **Message**: `expected 'val' inside extern block, got %s`
- **Example**: `test.goop:6:3: expected 'val' inside extern block, got IDENT`
- **Trigger**: Inside an `extern` block, something other than `val` appears
  at the start of a binding.
- **Fix**: Use `val` to declare each extern binding.
- **Bad**: `extern "go" "fmt" { Println : string -> unit }`
- **Good**: `extern "go" "fmt" { val Println : string -> unit }`

### PARSE013: Unexpected token in expression

- **Error code**: `PARSE013`
- **Severity**: Error
- **Message**: `unexpected token %s in expression`
- **Example**: `test.goop:4:10: unexpected token RBRACE in expression`
- **Trigger**: A token appears in an expression position that cannot start
  any valid expression. This is the catch-all for expression parsing
  failures.
- **Fix**: Check that the token makes sense at this position. Common causes:
  - Missing operand for a binary operator
  - Closing bracket/paren/brace without an expression inside
  - Keyword used as an expression
- **Bad**: `let x = + 5`
- **Good**: `let x = 5`

### PARSE014: Expected field name

- **Error code**: `PARSE014`
- **Severity**: Error
- **Message**: `expected field name, got %s`
- **Example**: `test.goop:3:5: expected field name, got INT`
- **Trigger**: Inside a record literal or record pattern, something that is
  not an identifier appears where a field name is expected.
- **Fix**: Use an identifier for the field name.
- **Bad**: `let r = { 42 = e1; "name" = e2 }`
- **Good**: `let r = { x = 42; name = "hello" }`

### PARSE015: Expected field name after `.`

- **Error code**: `PARSE015`
- **Severity**: Error
- **Message**: `expected field name after '.', got %s`
- **Example**: `test.goop:3:7: expected field name after '.', got INT`
- **Trigger**: A `.` is followed by a token that is not an identifier or
  constructor.
- **Fix**: Follow `.` with a valid field name.
- **Bad**: `x.42`
- **Good**: `x.field`

### PARSE016: Expected identifier after `as`

- **Error code**: `PARSE016`
- **Severity**: Error
- **Message**: `expected identifier after 'as', got %s`
- **Example**: `test.goop:5:15: expected identifier after 'as', got INT`
- **Trigger**: In a pattern alias (`pattern as name`), the token after `as`
  is not an identifier.
- **Fix**: Follow `as` with a variable name to bind the matched value.
- **Bad**: `| Some _ as 42 -> ...`
- **Good**: `| Some _ as val -> ...`

### PARSE017: Unexpected token in pattern

- **Error code**: `PARSE017`
- **Severity**: Error
- **Message**: `unexpected token %s in pattern`
- **Example**: `test.goop:5:15: unexpected token EQUALS in pattern`
- **Trigger**: A token appears in a pattern position that cannot start any
  valid pattern. Valid pattern starts are: `_`, identifiers, constructors,
  literals, `(`, `{`, `[`.
- **Fix**: Check the pattern syntax.
- **Bad**: `match x with | = y -> ...`
- **Good**: `match x with | y -> ...`

### PARSE018: Expected field name in record pattern

- **Error code**: `PARSE018`
- **Severity**: Error
- **Message**: `expected field name in record pattern, got %s`
- **Example**: `test.goop:5:7: expected field name in record pattern, got INT`
- **Trigger**: Inside a record pattern, something other than an identifier
  appears where a field name is expected.
- **Fix**: Use an identifier for the field name.
- **Bad**: `match x with | { 42 = pat } -> ...`
- **Good**: `match x with | { field = pat } -> ...`

### PARSE019 / PARSE020: Effect-row parse errors (obsolete in 1.0)

- **Error codes**: `PARSE019`, `PARSE020`
- **Status**: **Obsolete in 1.0.** Surface effect rows (`… with { io }`) were removed.
- **Use instead**: [PARSE-MIG016](#parse-mig016-effect-rows-removed-10) — drop the row annotation; use `effect` / `perform` / handlers when needed.
- **Do not** treat `with { io | .. }` as valid 1.0 syntax.

### PARSE021: Expected `..` after `|` in record type

- **Error code**: `PARSE021`
- **Severity**: Error
- **Message**: `expected '..' after '|' in record type`
- **Example**: `test.goop:5:15: expected '..' after '|' in record type`
- **Trigger**: In a record type `{ ... }`, the `|` pipe character is not
  followed by `..`. This can happen both in the middle of the record type
  and at the end.
- **Fix**: Use `| ..` for row polymorphism (open records). If you don't want
  an open record, remove the `|`.
- **Bad**: `type T = { x : int | }` or `type T = { x : int; | y : string }`
- **Good**: `type T = { x : int; | .. }` (open) or `type T = { x : int }` (closed)

### PARSE022: Unexpected token in type

- **Error code**: `PARSE022`
- **Severity**: Error
- **Message**: `unexpected token %s in type`
- **Example**: `test.goop:4:15: unexpected token EQUALS in type`
- **Trigger**: A token appears in a type position that cannot start any
  valid type expression.
- **Fix**: Check the type syntax.
- **Bad**: `let f x : = int -> int = x + 1`
- **Good**: `let f x : int -> int = x + 1`

### PARSE023: Expected field name or index after `.`

- **Error code**: `PARSE023`
- **Severity**: Error
- **Message**: `expected field name or index after '.', got %s`
- **Example**: `test.goop:3:5: expected field name or index after '.', got RPAREN`
- **Trigger**: After `.` in an expression, the parser expects either a field name (`x.field`) or OCaml-style indexing (`arr.(i)`). An empty `.( )` or a token that cannot start a field/index produces this error.
- **Fix**: Use `arr.(index)` for arrays or `record.field` for record fields.
- **Bad**: `arr.()`
- **Good**: `arr.(0)`

---

## TYPE — Type Checker Errors

Type errors indicate that the program's types do not match or that a name
cannot be resolved. All type errors stop compilation.

### TYPE001: Unsupported extern language (obsolete — PARSE-MIG002)

- **Error code**: `TYPE001`
- **Severity**: Error
- **Message**: `only 'go' extern is supported, got %q`
- **Example**: `test.goop:2:1: only 'go' extern is supported, got "c"`
- **Trigger**: An `extern` declaration uses a language other than `"go"`.
  Currently only Go FFI is implemented.
- **Fix**: Change the language to `"go"`.
- **Bad**: `extern "c" "libc.so" { val puts : string -> unit }`
- **Good**: `extern "go" "fmt" { val Println : string -> unit }`

### TYPE002: Extern binding name conflict

- **Error code**: `TYPE002`
- **Severity**: Error
- **Message**: `extern binding %q conflicts with existing name`
- **Example**: `test.goop:3:3: extern binding "Println" conflicts with existing name`
- **Trigger**: An extern binding's name collides with a name already bound
  in the environment (a previous let binding, type, or extern).
- **Fix**: Rename the extern binding or the existing binding to resolve
  the conflict.
- **Bad**:
  ```goop
  let Println s = ...
  extern "go" "fmt" { val Println : string -> unit }
  ```
- **Good**:
  ```goop
  let myPrint s = ...
  extern "go" "fmt" { val Println : string -> unit }
  ```

### TYPE003: Unhandled expression in type inference

- **Error code**: `TYPE003`
- **Severity**: Error
- **Message**: `internal error: unhandled expression %T in type inference (compiler bug — please report); rewrite using a supported form or add an explicit type annotation on the enclosing binding`
- **Example**: `test.goop:5:10: internal error: unhandled expression *ast.SomeExpr in type inference …`
- **Trigger**: An AST expression node of a type that the typechecker does
  not know how to infer. With a complete 1.0 AST surface this indicates a
  compiler bug (a new node without an infer case).
- **Fix**: Rewrite using supported forms, or annotate the enclosing binding.
  Report the issue if it occurs with valid Goop code. See
  [21-inference-holes.md](21-inference-holes.md).

### TYPE004: Unsupported binary operator

- **Error code**: `TYPE004`
- **Severity**: Error
- **Message**: `unsupported binary operator %s; Goop 1.0 supports + - * / mod land lor lxor +. -. *. /. = == != <> < > <= >= ^ && || :: |>; rewrite using those operators, or annotate the enclosing binding with an explicit type and use a library function`
- **Example**: `test.goop:5:12: unsupported binary operator @; Goop 1.0 supports …`
- **Trigger**: A binary operator token appears in a `BinaryExpr` that is not
  one of the supported operators above (normally unreachable from the
  parser).
- **Fix**: Use a supported operator or restructure the code.
- **Bad**: hand-built AST with an unsupported op
- **Good**: `let x = a mod b` (integer remainder)

### TYPE005: Undefined constructor pattern

- **Error code**: `TYPE005`
- **Severity**: Error
- **Message**: `undefined constructor pattern: %s`
- **Example**: `test.goop:8:5: undefined constructor pattern: Red`
- **Trigger**: A constructor name in a match pattern is not found in the
  environment. This means the ADT was not declared or imported.
- **Fix**: Ensure the ADT type is declared and the constructor name is
  correct.
- **Bad**:
  ```goop
  type Color = Red | Blue
  let describe c = match c with | Green -> "green"
  ```
- **Good**:
  ```goop
  type Color = Red | Blue
  let describe c = match c with | Red -> "red"
  ```

### TYPE006: Constructor takes no argument

- **Error code**: `TYPE006`
- **Severity**: Error
- **Message**: `constructor %s takes no argument`
- **Example**: `test.goop:8:7: constructor Red takes no argument`
- **Trigger**: In a pattern match, a constructor that has no payload is
  used with an argument pattern (e.g., `Red(x)` when `Red` is a simple
  variant with no data).
- **Fix**: Remove the argument from the pattern, or change the type
  definition to include payload.
- **Bad**:
  ```goop
  type Color = Red | Blue
  let f c = match c with | Red(x) -> x
  ```
- **Good**:
  ```goop
  type Color = Red of int | Blue
  let f c = match c with | Red(x) -> x
  ```

### TYPE007: Record has no field (in pattern check)

- **Error code**: `TYPE007`
- **Severity**: Error
- **Message**: `record has no field %q`
- **Example**: `test.goop:6:10: record has no field "z"`
- **Trigger**: A record pattern mentions a field name that does not exist on
  the matched record type. This is checked during pattern matching.
- **Fix**: Use only field names that exist in the record type definition.
- **Bad**:
  ```goop
  type Point = { x : int; y : int }
  let f p = match p with | { x = x; z = z } -> z
  ```
- **Good**:
  ```goop
  type Point = { x : int; y : int }
  let f p = match p with | { x = x; y = y } -> y
  ```

### TYPE008: Tuple pattern arity mismatch

- **Error code**: `TYPE008`
- **Severity**: Error
- **Message**: `tuple pattern arity mismatch: %d vs %d`
- **Example**: `test.goop:6:10: tuple pattern arity mismatch: 3 vs 2`
- **Trigger**: A tuple pattern has a different number of elements than the
  tuple type being matched.
- **Fix**: Match the number of elements in the tuple type, or adjust the
  type definition.
- **Bad**:
  ```goop
  let f t = match t with | (x, y, z) -> x + y + z
  (* called with 2-tuple *)
  ```
- **Good**:
  ```goop
  let f t = match t with | (x, y) -> x + y
  ```

### TYPE009: Expected tuple type for tuple pattern

- **Error code**: `TYPE009`
- **Severity**: Error
- **Message**: `expected tuple type for tuple pattern`
- **Example**: `test.goop:6:10: expected tuple type for tuple pattern`
- **Trigger**: A tuple pattern `(a, b)` is used to match a value that is
  not a tuple type.
- **Fix**: Only use tuple patterns on tuple types.
- **Bad**:
  ```goop
  let f (x : int) = match x with | (a, b) -> a + b
  ```
- **Good**:
  ```goop
  let f (x : int * int) = match x with | (a, b) -> a + b
  ```

### TYPE010: Type mismatch (unification failure)

- **Error code**: `TYPE010`
- **Severity**: Error
- **Message**: `%v` (wraps the `UnifyError` message — see UNIFY section)
- **Example**: `test.goop:5:10: type mismatch: got int, expected string`
- **Trigger**: Two types cannot be unified. This is the catch-all for type
  mismatch errors from the unification engine. The specific reason is given
  by the wrapped `UnifyError`.
- **Fix**: See the specific UNIFY error code referenced in the message.
- **Bad**: `let x : string = 42`
- **Good**: `let x : int = 42`

### TYPE011: Cannot assign to immutable binding

- **Error code**: `TYPE011`
- **Severity**: Error
- **Message**: `TYPE011: cannot assign to immutable binding %q; use := for refs or <- for arrays/mutable fields`
- **Example**: `test.goop:4:3: TYPE011: cannot assign to immutable binding "x"; …`
- **Trigger**: Assignment targets a plain immutable `let` binding.
- **Fix**: `let r = ref 0 in r := 1`, or `arr.(i) <- v` for array cells. Mutable record fields use `r.field <- v`.
- **Bad**:
  ```goop
  let x = 0 in
  x <- 1
  ```
- **Good**:
  ```goop
  let r = ref 0 in
  r := 1
  ```

### TYPE012: Invalid assignment target

- **Error code**: `TYPE012`
- **Severity**: Error
- **Message**: `invalid assignment target`
- **Example**: `test.goop:5:3: invalid assignment target`
- **Trigger**: The left-hand side of `<-` is neither an array index (`arr.(i)`) nor a mutable record field.
- **Fix**: Use `arr.(index) <- v`, `r.field <- v` on `mutable` fields, or `ref` with `:=`.
- **Bad**: `(f x) <- 1`
- **Good**: `arr.(0) <- 1` or `r := 1`

### TYPE013: Qualified constructor not defined

- **Error code**: `TYPE013`
- **Severity**: Error
- **Message**: `constructor %s.%s is not defined`
- **Example**: `test.goop:6:5: constructor Color.Yellow is not defined`
- **Trigger**: A qualified constructor `Type.Ctor` names a type prefix and constructor, but that constructor is not a variant of the named type (or the type is unknown).
- **Fix**: Use a constructor declared on the type, or drop the qualifier when unambiguous.
- **Bad**:
  ```goop
  type Color = Red | Green
  let c = Color.Blue
  ```
- **Good**:
  ```goop
  type Color = Red | Green
  let c = Color.Red
  ```

---

## FFI-IMPL — Go Interface Implementation Errors

### FFI-IMPL001: Invalid `implements` declaration

- **Error code**: `FFI-IMPL001`
- **Severity**: Error
- **Messages**: `unknown implementation type %q` or `method %q requires a receiver parameter`
- **Trigger**: An `implements … for T` declaration names an unknown Goop
  implementation type, or one of its methods omits its first receiver
  parameter.
- **Fix**: Declare `T` before the `implements` declaration and make every
  method's first parameter the receiver (for example, `let String (p : point)
  : string = …`).

---

## FFI-METHOD — Go Method and Field Import Errors

Method and field imports use the normal parser and type-unification diagnostics;
1.4.0 does not introduce separate numbered `FFI-METHOD` diagnostics. Check
the receiver declaration first: `val (x : T).M : A -> B` is a method, while
`val (x : T).F : U` is a field. A wrong receiver, argument list, or result
type is reported as the corresponding `TYPE010`/`UNIFY` mismatch or by Go at
build time.

---

## UNIFY — Unification Errors

All unification errors produce a message in the format:

```
type mismatch: got <actual>, expected <expected> (<reason>)
```

These are emitted through `TYPE010` (the typechecker wraps them in a
`TypeError`). The `<reason>` identifies the specific mismatch.

### UNIFY001: Occurs check failure

- **Error code**: `UNIFY001`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (occurs check: %s occurs in %s)`
- **Example**: `test.goop:5:10: type mismatch: got 'a, expected 'a list (occurs check: 'a occurs in list('a))`
- **Trigger**: A type variable is unified with a type that contains itself.
  This is the classic "occurs check" failure from Hindley-Milner type
  inference, indicating a self-referential type like `'a = list('a)` where
  `'a` already appears in the structure. This is extremely rare in
  practice and usually indicates a type annotation error.
- **Fix**: This typically means the program tried to construct an infinite
  type. Review your type annotations.
- **Bad**: A function that returns itself as part of a recursive type
  without proper ADT wrapping.
- **Good**: Use an explicit ADT for recursive types.

### UNIFY002: Expected primitive type

- **Error code**: `UNIFY002`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (expected primitive type)`
- **Example**: `test.goop:4:10: type mismatch: got int -> int, expected int (expected primitive type)`
- **Trigger**: An operation expects a primitive type (`int`, `float`,
  `string`, `bool`, `unit`) but a compound type (function, tuple, record,
  ADT, etc.) was provided.
- **Fix**: Ensure the expression evaluates to a primitive type.
- **Bad**: `let x = (fun a -> a) + 1`
- **Good**: `let x = (fun a -> a) 1 + 1`

### UNIFY003: Different primitive types

- **Error code**: `UNIFY003`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (different primitive types)`
- **Example**: `test.goop:4:10: type mismatch: got string, expected int (different primitive types)`
- **Trigger**: Two primitive types are unified but they are different
  (e.g., `int` vs `string`).
- **Fix**: Change one side to match the expected type.
- **Bad**: `let x : int = "hello"`
- **Good**: `let x : string = "hello"`

### UNIFY004: Expected a function

- **Error code**: `UNIFY004`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (expected a function)`
- **Example**: `test.goop:4:10: type mismatch: got int, expected int -> int (expected a function)`
- **Trigger**: A value is used as a function (in application) but its type
  is not a function type.
- **Fix**: Use a function value for application.
- **Bad**: `let x = 42 10`
- **Good**: `let x = (fun y -> y + 1) 10`

### UNIFY005: Expected a tuple

- **Error code**: `UNIFY005`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (expected a tuple)`
- **Example**: `test.goop:4:10: type mismatch: got int, expected int * string (expected a tuple)`
- **Trigger**: A tuple operation is used on a non-tuple type.
- **Fix**: Ensure the expression has a tuple type.
- **Bad**: `let f t = match t with | (a, b) -> a` where `t` is `int`
- **Good**: `let f (t : int * string) = match t with | (a, b) -> a`

### UNIFY006: Tuple arity mismatch

- **Error code**: `UNIFY006`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (tuple arity mismatch: %d vs %d)`
- **Example**: `test.goop:4:10: type mismatch: got int * string * bool, expected int * string (tuple arity mismatch: 3 vs 2)`
- **Trigger**: Two tuple types have different numbers of elements.
- **Fix**: Make the tuples have the same number of elements.
- **Bad**: `let (x, y, z) : int * string = ("hello", 42, true)`
- **Good**: `let (x, y) : int * string = (42, "hello")`

### UNIFY007: Expected a record

- **Error code**: `UNIFY007`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (expected a record)`
- **Example**: `test.goop:4:10: type mismatch: got int, expected { x : int } (expected a record)`
- **Trigger**: A record operation (field access, record pattern) is used on
  a non-record type.
- **Fix**: Ensure the value has a record type.
- **Bad**: `let x = 42.x`
- **Good**: `let r = { x = 42 }; r.x`

### UNIFY008: Record has no field (unification)

- **Error code**: `UNIFY008`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (record has no field %q)`
- **Example**: `test.goop:4:10: type mismatch: got { x : int }, expected { y : int } (record has no field "y")`
- **Trigger**: A record provides a field that does not exist on the other
  side during unification.
- **Fix**: Add the missing field, or remove the reference to it.
- **Bad**: `let r : { x : int } = { y = 42 }`
- **Good**: `let r : { x : int } = { x = 42 }`

### UNIFY009: Record has unexpected field

- **Error code**: `UNIFY009`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (record has unexpected field %q)`
- **Example**: `test.goop:4:10: type mismatch: got { x : int; y : int }, expected { x : int } (record has unexpected field "y")`
- **Trigger**: When unifying two closed records, one side has an extra field
  that does not exist on the other side.
- **Fix**: Remove the extra field or use row polymorphism (open records with
  `| ..`) to allow extra fields.
- **Bad**: `let r : { x : int } = { x = 1; y = 2 }`
- **Good**: `let r : { x : int } = { x = 1 }`

### UNIFY010: Expected an ADT

- **Error code**: `UNIFY010`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (expected an ADT)`
- **Example**: `test.goop:4:10: type mismatch: got int, expected Color (expected an ADT)`
- **Trigger**: An ADT (algebraic data type) is expected but a different type
  was provided.
- **Fix**: Provide a value of the correct ADT.
- **Bad**:
  ```goop
  type Color = Red | Blue
  let f (c : Color) = ...
  let x = f 42
  ```
- **Good**: `let x = f Red`

### UNIFY011: Different ADT names

- **Error code**: `UNIFY011`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (different ADT names)`
- **Example**: `test.goop:4:10: type mismatch: got Color, expected Shape (different ADT names)`
- **Trigger**: Two ADT types are unified but they refer to different type
  names.
- **Fix**: Use values of the same ADT type.
- **Bad**:
  ```goop
  type Color = Red | Blue
  type Shape = Circle | Square
  let f (c : Color) = ...
  let x = f Circle
  ```
- **Good**: `let x = f Red`

### UNIFY012: ADT arity mismatch

- **Error code**: `UNIFY012`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (ADT arity mismatch)`
- **Example**: `test.goop:4:10: type mismatch: got option(int), expected option(int, string) (ADT arity mismatch)`
- **Trigger**: A generic ADT is used with the wrong number of type
  parameters.
- **Fix**: Provide the correct number of type arguments.
- **Bad**: Using `Option` with two type arguments when it's defined with one.
- **Good**: Match the type parameter count from the type definition.

### UNIFY013: Expected a type constructor

- **Error code**: `UNIFY013`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (expected a type constructor)`
- **Example**: `test.goop:4:10: type mismatch: got int, expected list(int) (expected a type constructor)`
- **Trigger**: A type constructor (like `list`, `option`, `result`) is
  expected but a different type was provided.
- **Fix**: Use the correct type constructor.
- **Bad**: Implicitly treating `int` as `list(int)`
- **Good**: Wrap in the correct type constructor.

### UNIFY014: Different type constructors

- **Error code**: `UNIFY014`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (different type constructors)`
- **Example**: `test.goop:4:10: type mismatch: got option(int), expected list(int) (different type constructors)`
- **Trigger**: Two type constructor applications have the same constructor
  name but refer to different types (or the same name is used differently).
- **Fix**: Ensure consistent use of type constructors.

### UNIFY015: Type constructor arity mismatch

- **Error code**: `UNIFY015`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (type constructor arity mismatch)`
- **Example**: `test.goop:4:10: type mismatch: got list(int, string), expected list(int) (type constructor arity mismatch)`
- **Trigger**: A type constructor is applied to the wrong number of type
  arguments.
- **Fix**: Provide the correct number of type arguments to the type
  constructor.
- **Bad**: Using `list` with two type arguments
- **Good**: `list(int)`

### UNIFY016: Expected a channel type

- **Error code**: `UNIFY016`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (expected a channel type)`
- **Example**: `test.goop:4:10: type mismatch: got int, expected chan(int) (expected a channel type)`
- **Trigger**: A channel operation (send/receive) is used on a non-channel
  type.
- **Fix**: Ensure the value is a channel type.
- **Bad**: `let x = 42; Chan.send x 1`
- **Good**: `let ch = Chan.make () : int Chan.t; Chan.send ch 1`

### UNIFY017: Unknown type

- **Error code**: `UNIFY017`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (unknown type)`
- **Example**: `test.goop:4:10: type mismatch: got <custom>, expected int (unknown type)`
- **Trigger**: The unification algorithm encounters a type it does not
  recognize. This is a catch-all for types not handled by the structural
  cases. This is extremely rare and indicates an internal compiler issue.
- **Fix**: If this occurs, it is likely a compiler bug. Try simplifying the
  type expression.

### UNIFY018: Effect row mismatch

- **Error code**: `UNIFY018`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (effect row mismatch)`
- **Note (1.0)**: Surface `with { … }` annotations are removed (PARSE-MIG016). This code may still appear from internal/prelude effect tags during unification.
- **Fix**: Prefer effect handlers for user-declared effects; do not reintroduce arrow-type effect rows.

### UNIFY019: Effect row missing effect

- **Error code**: `UNIFY019`
- **Severity**: Error (via TYPE010)
- **Message**: `type mismatch: got %s, expected %s (effect row missing effect: %s)`
- **Note (1.0)**: Same as UNIFY018 — surface rows removed; internal tags may still unify.

### UNIFY020: Expected a map type

- **Error code**: `UNIFY020`
- **Severity**: Error (via TYPE010)
- **Message**: includes `UNIFY020: expected a map type`
- **Fix**: Pass a `map[K] V` value, or adjust the annotation.

### UNIFY021: Expected a pointer

- **Error code**: `UNIFY021`
- **Severity**: Error (via TYPE010)
- **Message**: includes `UNIFY021: expected a pointer`
- **Fix**: Use a `T ptr` / FFI pointer value.

### UNIFY022: Expected a go_slice

- **Error code**: `UNIFY022`
- **Severity**: Error (via TYPE010)
- **Message**: includes `UNIFY022: expected a go_slice`
- **Fix**: Convert with `go_slice_of_list` or declare the FFI slice type.

---

## VIS — Visibility

### VIS001: Private binding access / naming

- **Error code**: `VIS001`
- **Severity**: Error
- **Message**: `VIS001: cannot access private binding %q from module …` or
  `VIS001: private binding %q must use mixedCaps (lower initial)`
- **Fix**: Keep private names module-local; use mixedCaps for private bindings;
  export (drop `private`) if cross-module access is required.

### VIS002: Private type in public API

- **Error code**: `VIS002`
- **Severity**: Warning by default; `[check] private_in_public`
- **Message**: `VIS002: public %q exposes private type %q`
- **Trigger**: A non-`private` let/type signature mentions a same-module `private` type or constructor.
- **Fix**: Make the type public, or mark the API `private`.

---

## IMPORT — Module imports

### IMPORT001: Module not found

- **Error code**: `IMPORT001`
- **Severity**: Error
- **Message**: `IMPORT001: module %q not found`
- **Fix**: Fix the import path or add a `[mappings]` entry in `goop.toml`.

### IMPORT002: Duplicate import

- **Error code**: `IMPORT002`
- **Severity**: Error
- **Message**: `IMPORT002: duplicate import …`
- **Fix**: Remove the duplicate.

### IMPORT004: Import binding name conflict

- **Error code**: `IMPORT004`
- **Severity**: Error
- **Message**: `IMPORT004: import binds %q which conflicts with existing name`
- **Fix**: Rename the binding or remove the conflicting declaration.

### IMPORT003: Import resolve / context failure

- **Error code**: `IMPORT003`
- **Severity**: Error
- **Message**: `IMPORT003: …`
- **Fix**: Run check/build with a real source file path so imports can resolve.

---

## EXHAUST — Exhaustiveness Warnings

Warnings from the pattern exhaustiveness checker. These do NOT stop
compilation but indicate potential bugs (missing match cases, unreachable
code).

### EXHAUST001: Unreachable pattern (wildcard)

- **Error code**: `EXHAUST001`
- **Severity**: Warning
- **Message**: `unreachable pattern: all values already matched by previous wildcard`
- **Example**: `test.goop:10:3: unreachable pattern: all values already matched by previous wildcard`
- **Trigger**: In a match expression, a wildcard pattern (`_`) appears, and
  a later arm also has a wildcard pattern. The second wildcard can never be
  reached because the first matches everything.
- **Fix**: Remove the unreachable arm, or reorder the arms.
- **Bad**:
  ```goop
  match x with
  | _ -> "anything"
  | Some v -> "got some"
  ```
- **Good**:
  ```goop
  match x with
  | Some v -> "got some"
  | _ -> "anything"
  ```

### EXHAUST002: Unreachable pattern (constructor)

- **Error code**: `EXHAUST002`
- **Severity**: Warning
- **Message**: `unreachable pattern: constructor %q already covered`
- **Example**: `test.goop:10:5: unreachable pattern: constructor "Red" already covered`
- **Trigger**: A match arm for a constructor appears after an earlier arm
  already covers that constructor (without a guard). Since the previous arm
  handles all values of that constructor, the later arm is dead code.
- **Fix**: Remove the duplicate arm or restructure. If the second arm has a
  guard condition that the first lacks, the warning is not emitted (guarded
  patterns do not fully cover the constructor).
- **Bad**:
  ```goop
  match c with
  | Red -> "red"
  | Red -> "also red"
  ```
- **Good**:
  ```goop
  match c with
  | Red -> "red"
  | Blue -> "blue"
  ```

### EXHAUST003: Non-exhaustive match

- **Error code**: `EXHAUST003`
- **Severity**: Error (fatal on `goop check` / `goop build` by default)
- **Message**: `non-exhaustive match: missing constructor(s): %s`
- **Example**: `test.goop:8:3: non-exhaustive match: missing constructor(s): Blue, Green`
- **Trigger**: A match expression does not cover all possible constructors
  of the matched ADT, and no wildcard pattern (`_`) is present to catch the
  remaining cases.
- **Fix**: Add arms for the missing constructors, or add a wildcard catch-all.
- **Bad**:
  ```goop
  type Color = Red | Blue | Green
  let describe c = match c with | Red -> "red"
  ```
- **Good**:
  ```goop
  type Color = Red | Blue | Green
  let describe c = match c with
  | Red -> "red"
  | Blue -> "blue"
  | Green -> "green"
  ```
  Or:
  ```goop
  let describe c = match c with
  | Red -> "red"
  | _ -> "other"
  ```

---

## LINEAR — Linear Discharge Errors

Errors from the linear resource checker. Linear types (declared with `: 1`
syntax) must be used exactly once — handed off or discharged — on every
control-flow path. All linear errors stop compilation.

### LINEAR001: Variable not discharged

- **Error code**: `LINEAR001`
- **Severity**: Error
- **Message**: `linear variable %q not discharged on all paths (%s)`
- **Example**: `test.goop:5:10: linear variable "fh" not discharged on all paths (function open_file)`
- **Trigger**: A linear variable was bound but never used (handed off) on
  some code path. This is the most general discharge error, triggered at
  function exit, binding boundaries, and region exits. The `%s` gives
  context about the scope (e.g., `function f`, `binding x`, `lambda`).
- **Fix**: Ensure the linear variable is passed to a function that consumes
  it, or explicitly discharged, on every code path.
- **Bad**:
  ```goop
  type file_handle : 1
  let open_file () : file_handle = ...
  let bad () =
    let fh = open_file () in
    ()  (* fh never used — leak detected *)
  ```
- **Good**:
  ```goop
  type file_handle : 1
  let open_file () : file_handle = ...
  let close_file (fh : file_handle) = ...
  let good () =
    let fh = open_file () in
    close_file fh
  ```

### LINEAR002: Variable used after discharge

- **Error code**: `LINEAR002`
- **Severity**: Error
- **Message**: `linear variable %q used after being discharged`
- **Example**: `test.goop:7:5: linear variable "fh" used after being discharged`
- **Trigger**: A linear variable is used more than once. After the first use
  (hand-off), the variable is "discharged" and cannot be referenced again.
  This is the linear equivalent of a use-after-move/use-after-free error.
- **Fix**: Use the linear variable exactly once. If you need to access it
  multiple times, consider whether the type should be linear or whether
  you need to restructure the code (e.g., use a borrow pattern).
- **Bad**:
  ```goop
  type file_handle : 1
  let read_file (fh : file_handle) = ...
  let close_file (fh : file_handle) = ...
  let double_use () =
    let fh = open_file () in
    read_file fh;   (* first use — discharged *)
    close_file fh   (* second use — ERROR *)
  ```
- **Good**:
  ```goop
  type file_handle : 1
  let read_and_close (fh : file_handle) =
    read_file fh;  (* pass ownership to read_file, no return value *)
    ()             (* or restructure so close_file is the only consumer *)
  ```

### LINEAR003: Variable not discharged in then-branch

- **Error code**: `LINEAR003`
- **Severity**: Error
- **Message**: `linear variable %q not discharged in then-branch`
- **Example**: `test.goop:6:10: linear variable "fh" not discharged in then-branch`
- **Trigger**: Inside an `if expr then ... else ...`, a linear variable that
  is live before the `if` is not used (discharged) in the then-branch.
- **Fix**: Ensure the linear variable is consumed in both branches (or the
  branch that retains it passes ownership on).
- **Bad**:
  ```goop
  let f (fh : file_handle) (flag : bool) =
    if flag then
      close_file fh
    else
      ()  (* fh not discharged in else branch *)
  ```
- **Good**:
  ```goop
  let f (fh : file_handle) (flag : bool) =
    if flag then
      close_file fh
    else
      close_file fh
  ```

### LINEAR004: Variable not discharged in else-branch

- **Error code**: `LINEAR004`
- **Severity**: Error
- **Message**: `linear variable %q not discharged in else-branch`
- **Example**: `test.goop:8:10: linear variable "fh" not discharged in else-branch`
- **Trigger**: Same as LINEAR003 but for the else-branch. A linear variable
  live before the `if` is not discharged in the else path.
- **Fix**: See LINEAR003 — discharge the variable in both branches.

### LINEAR005: Variable not discharged in match arm

- **Error code**: `LINEAR005`
- **Severity**: Error
- **Message**: `linear variable %q not discharged in match arm %d`
- **Example**: `test.goop:10:12: linear variable "fh" not discharged in match arm 2`
- **Trigger**: In a match expression, a linear variable that is live before
  the match is not discharged in one of the match arms. Each arm must
  independently discharge all live linear variables.
- **Fix**: Add code to discharge the linear variable in each match arm.
- **Bad**:
  ```goop
  type option = Some of file_handle | None
  let f (opt_fh : option) =
    match opt_fh with
    | Some fh -> close_file fh
    | None -> ()  (* no discharge needed, but all live variables still checked *)
  ```
- **Good**: Ensure all live linear variables from before the match are
  handled in every arm.

### LINEAR006: Data race — shared between goroutines

- **Error code**: `LINEAR006`
- **Severity**: Error by default; configurable via `goop.toml` `[check] concurrent` (`warn` | `error` | `off`)
- **Config key**: `concurrent`
- **Message**: `potential data race: mutable variable %q shared between multiple goroutines`
- **Example**: `test.goop:12:5: potential data race: mutable variable "counter" shared between multiple goroutines`
- **Trigger**: A `ref` (or other mutable binding) is captured by closures passed to
  multiple `go` expressions. This creates a data race because multiple
  goroutines can read/write the same cell concurrently.
- **Fix**: Avoid sharing mutable state between goroutines. Use channels
  or synchronization primitives instead.
- **Bad**:
  ```goop
  let counter = ref 0 in
  let _ = go (fun () -> counter := !counter + 1) in
  let _ = go (fun () -> counter := !counter + 1) in
  ()
  ```
- **Good**: Use channels for communication between goroutines.

### LINEAR007: Data race — mutable capture by goroutine

- **Error code**: `LINEAR007`
- **Severity**: Error by default; configurable via `goop.toml` `[check] concurrent` (`warn` | `error` | `off`)
- **Config key**: `concurrent`
- **Message**: `potential data race: mutable variable %q captured by goroutine is still accessible in spawning scope`
- **Example**: `test.goop:10:5: potential data race: mutable variable "counter" captured by goroutine is still accessible in spawning scope`
- **Trigger**: A `ref` is captured by a `go` closure while the
  spawning scope can still access it. This creates a data race
  between the goroutine and the spawning code.
- **Fix**: Ensure the binding is not accessed in the spawning scope
  after the goroutine starts, or use proper synchronization. Use
  `go (move var) (...)` to mark a binding as transferred into the
  goroutine when the parent will not use it again.
- **Bad**:
  ```goop
  let x = ref 0 in
  let _ = go (fun () -> x := 42) in
  x := 1  (* race with goroutine *)
  ```
- **Good**:
  ```goop
  let counter = ref 0 in
  let _ = go (move counter) (fun () -> counter := !counter + 1) in
  ()
  ```

### LINEAR008: Channel-mediated data race

- **Error code**: `LINEAR008`
- **Severity**: Error by default; configurable via `goop.toml` `[check] concurrent` (`warn` | `error` | `off`)
- **Config key**: `concurrent`
- **Message**: `potential channel-mediated data race: mutable variable %q sent on channel while still accessible in spawning scope`
- **Example**: `test.goop:12:3: potential channel-mediated data race: mutable variable "state" sent on channel while still accessible in spawning scope`
- **Trigger**: A mutable binding is sent on a channel inside a `go` closure (`Chan.send ch var`) while the spawning scope can still read or write that binding after the `go` expression.
- **Fix**: Stop accessing the binding in the parent after handing it off, or copy the value before sending. Use `go (move var)` when transferring ownership.
- **Bad**:
  ```goop
  let state = ref 0 in
  let ch = Chan.make () in
  let _ = go (fun () -> Chan.send ch state) in
  state := !state + 1   (* race with goroutine via shared ref *)
  ```
- **Good**: Send a copy, or use `go (move state)` and do not touch `state` in the parent afterward.

---

## DEADLOCK — Deadlock Warnings

Narrow static analysis for circular channel communication between two goroutines.

### DEADLOCK001: Potential channel deadlock

- **Error code**: `DEADLOCK001`
- **Severity**: Warning by default; configurable via `goop.toml` `[check] deadlock` (`warn` | `error` | `off`)
- **Config key**: `deadlock`
- **Message**: `potential channel deadlock between goroutines (circular send/recv on unbuffered channels)`
- **Example**: `test.goop:8:3: DEADLOCK001: potential channel deadlock between goroutines (circular send/recv on unbuffered channels)`
- **Trigger**: Two goroutines each perform a straight-line `Chan.send` then `Chan.recv` on different channels in opposite order (classic unbuffered deadlock pattern). Only simple, non-looping goroutine bodies are analyzed.
- **Fix**: Reorder operations, use buffered channels, or introduce a third goroutine to break the cycle. For complex concurrency, rely on Go's runtime deadlock detector and testing.
- **Bad**:
  ```goop
  (* G1: send ch1, recv ch2; G2: send ch2, recv ch1 — can deadlock *)
  let _ = go (fun () -> Chan.send ch1 1; Chan.recv ch2) in
  let _ = go (fun () -> Chan.send ch2 2; Chan.recv ch1) in
  ```
- **Good**: Ensure at least one side can complete without blocking, or buffer one channel.

---

## RESULT / OPTION — Discarded values

### RESULT001: Result value discarded

- **Error code**: `RESULT001`
- **Severity**: Warning by default; configurable via `goop.toml` `[check] discarded_result` (`warn` | `error` | `off`)
- **Config key**: `discarded_result`
- **Message**: `RESULT001: result value is discarded; handle with match or bind with let _ = …`
- **Trigger**: In a `begin … end` sequence, a non-final expression has type `result` and is not bound or matched.
- **Fix**: `match` the value, propagate it as the sequence result, or discard explicitly with `let _ = e in …`.
- **Bad**:
  ```goop
  begin
    parseInt "x";   (* RESULT001 *)
    ()
  end
  ```
- **Good**:
  ```goop
  begin
    let _ = parseInt "x" in
    ()
  end
  ```

### OPTION001: Option value discarded

- **Error code**: `OPTION001`
- **Severity**: Warning by default; `[check] discarded_option`
- **Message**: `OPTION001: option value is discarded; handle with match or bind with let _ = …`
- **Trigger**: Same as RESULT001, but the discarded value has type `option`.
- **Fix**: `match`, or `let _ = …`.

---

## UNUSED — Unused bindings and imports

### UNUSED001: Unused local binding

- **Error code**: `UNUSED001`
- **Severity**: Warning by default; `[check] unused`
- **Message**: `UNUSED001: unused binding %q`
- **Trigger**: A let-in / function parameter is never referenced. Names `_` / `_foo`
  and unused **unit**-typed value bindings (sequencing) are ignored. Go FFI
  imports are not reported here.
- **Fix**: Use the binding, rename to `_`, or remove it.

### UNUSED002: Unused import

- **Error code**: `UNUSED002`
- **Severity**: Warning by default; `[check] unused`
- **Message**: `UNUSED002: unused import %q`
- **Trigger**: A Goop module import qualifier is never referenced (Go `import go`
  packages are skipped). Expression uses like `mod.foo` / `Mod.Foo`, type
  annotations `mod.T`, and local opens `Mod.( … )` all count as references.
- **Fix**: Remove the import or use a binding from it.

---

## ROW — Open row parameters

### ROW001: Missing field in row-parameter record literal

- **Error code**: `ROW001`
- **Severity**: Error
- **Message**: `ROW001: record literal is missing field %q required by row parameter of %s`
- **Fix**: Supply every field required by the open row type. CODEGEN002 remains a codegen fallback.

---

## DECIMAL — Money as float

### DECIMAL001: Float used for money-ish name

- **Error code**: `DECIMAL001`
- **Severity**: Warning by default; `[check] money_float`
- **Message**: `DECIMAL001: %q uses float for money; use std.decimal.Decimal instead`
- **Fix**: Use `std.decimal` or integer cents; rename non-money floats away from money allowlist (`price`, `px`, …).

---

## CODEGEN — Code generation

### CODEGEN001: Unhandled expression

- **Error code**: `CODEGEN001`
- **Severity**: Error
- **Message**: `CODEGEN001: …`
- **Fix**: Simplify the construct or report a compiler bug.

### CODEGEN002: Missing record field for row parameter

- **Error code**: `CODEGEN002`
- **Severity**: Error
- **Message**: `CODEGEN002: record literal is missing field %q required by row parameter of %s`
- **Fix**: Supply every field required by the row parameter. Prefer **ROW001** at typecheck for record literals; CODEGEN002 remains defense-in-depth.

### CODEGEN003: Missing Go method receiver

- **Error code**: `CODEGEN003`
- **Severity**: Error
- **Message**: `CODEGEN003: …`
- **Fix**: Pass the Go method receiver as the first argument.

---

## GOSIG — Go signatures

### GOSIG001: `.gosig` load failure

- **Error code**: `GOSIG001`
- **Severity**: Warning / diagnostic on load failure
- **Fix**: Fix goop-sigs override syntax or regenerate with `goop gen-sig`.

### GOSIG002: Signature fallback

- **Error code**: `GOSIG002`
- **Severity**: Warning (stderr)
- **Fix**: Add an explicit `{ val … }` block or improve the stub mapping.

### GOSIG003: Hand signature arity mismatch

- **Error code**: `GOSIG003`
- **Severity**: Warning (stderr); `[check] verify_ffi` (default off)
- **Fix**: Align the hand `{ val … }` arity with the Go package, or leave `verify_ffi = false`.

### GOSIG004: Hand signature names generic Go export

- **Error code**: `GOSIG004`
- **Severity**: Warning (stderr); always on (not gated by `verify_ffi`)
- **Message**: `GOSIG004: hand signature for %s.%s references generic Go export; omit it or wrap with @[go] — see docs/design/32-go-generics-sigs.md`
- **Fix**: Omit the unmappable generic from the hand block, use a thin `@[go]`
  wrapper with a concrete API, or wait for a future monomorphization design.
  See [32-go-generics-sigs.md](32-go-generics-sigs.md).

---

## TAG — Record field Go tags

### TAG001: Invalid `@[tag]` on record field

- **Error code**: `TAG001`
- **Severity**: Error (parse)
- **Message**: `TAG001: unknown field attribute @[…]` / `TAG001: @[tag] requires a string payload…`
- **Trigger**: A record field has `@[…]` that is not `@[tag "…"]`, or `@[tag]` lacks a string.
- **Fix**: Write `@[tag "json:\"name,omitempty\""]` (payload = exact Go backtick body). See [33-sdk-blockers.md](33-sdk-blockers.md).

### MODULE001: Duplicate name when merging sibling modules

- **Error code**: `MODULE001`
- **Severity**: Error
- **Message**: `MODULE001: duplicate top-level name %q when merging …`
- **Trigger**: Two sibling `.goop` files share the same non-`main` module name and define the same top-level binding.
- **Fix**: Rename one binding, or split into different modules/directories.

---

## NIL — Nil Channel Errors

Flow-sensitive analysis for channel initialization before use.

### NIL001: Possible nil channel use

- **Error code**: `NIL001`
- **Severity**: Error
- **Message**: `possible use of nil channel %q before initialization`
- **Example**: `test.goop:6:3: NIL001: possible use of nil channel "ch" before initialization`
- **Trigger**: A channel identifier is used in `Chan.send`, `Chan.recv`, `Chan.close`, `<-` send, or passed to a goroutine before the checker can prove it was initialized via `Chan.make ()` or `OwnedChan.make ()` on all paths.
- **Fix**: Initialize the channel with `let ch = Chan.make ()` (or `OwnedChan.make ()`) before any send, receive, or close. Ensure initialization happens on every control-flow path.
- **Bad**:
  ```goop
  let ch = () in   (* not a channel *)
  Chan.send ch 42
  ```
- **Good**:
  ```goop
  let ch = Chan.make () in
  Chan.send ch 42
  ```

---

## REFINE — Refinement Solver Errors/Warnings

The refinement solver checks contract predicates at call sites. There are
three possible outcomes for each refinement:

| Outcome | Behavior | When |
|---|---|---|
| **Proven** | Silent — no runtime check emitted | Solver proves predicate holds at compile time |
| **Unproven** | Warning — runtime check emitted | Solver cannot determine whether predicate holds |
| **Disproven** | Error — compilation stops | Solver proves predicate is violated |

### REFINE001: Disproven refinement

- **Error code**: `REFINE001`
- **Severity**: Error
- **Message**: `refinement violated: cannot satisfy %s at %s`
- **Example**: `test.goop:10:5: refinement violated: cannot satisfy x > 0 at line 10`
- **Trigger**: A function call has a refinement-annotated parameter (e.g.,
  `(x : int where x > 0)`) and the solver can prove that the actual
  argument violates the constraint at the call site. The compiler stops
  with an error.
- **Fix**: Ensure the argument satisfies the refinement contract. Check
  the call site's path constraints (e.g., from `if` conditions, guards).
- **Bad**:
  ```goop
  let sqrt (x : int where x >= 0) = ...
  let _ = sqrt (-5)  (* -5 >= 0 is disproven → ERROR *)
  ```
- **Good**:
  ```goop
  let sqrt (x : int where x >= 0) = ...
  let _ = sqrt 4   (* 4 >= 0 is proven → no error *)
  ```

### REFINE002: Unproven refinement

- **Error code**: `REFINE002`
- **Severity**: Warning by default; configurable via `goop.toml` `[check] refinement_unproven` (`warn` | `error` | `off`)
- **Config key**: `refinement_unproven`
- **Message**: `could not prove refinement %s at %s — runtime check emitted`
- **Example**: `test.goop:15:5: WARNING: could not prove refinement n > 0 at line 15 — runtime check emitted`
- **Trigger**: A refinement-annotated parameter receives an argument whose
  constraint the solver cannot prove or disprove at compile time. A runtime
  `panic` check is emitted in the generated Go code to catch violations at
  runtime.
- **Fix**: Add more path constraints (e.g., `if` conditions, match guards)
  to help the solver, or restructure the code so the relationship is
  statically obvious. This is not a compile error but may cause runtime
  failures.
- **Bad**:
  ```goop
  let sqrt (x : int where x >= 0) = ...
  let x = read_int ()   (* compiler can't know if x >= 0 *)
  let _ = sqrt x        (* WARNING: runtime check for x >= 0 *)
  ```
- **Good**:
  ```goop
  let sqrt (x : int where x >= 0) = ...
  let x = read_int () in
  if x >= 0 then
    sqrt x   (* solver sees x >= 0 in then-branch → PROVEN *)
  else
    ...
  ```

### REFINE003: Proven refinement (silent)

- **Error code**: `REFINE003`
- **Severity**: Silent
- **Message**: (none)
- **Example**: (no output)
- **Trigger**: The solver proves that the refinement holds at the call site.
  No runtime check is emitted and no warning/error is reported.
- **Fix**: N/A — this is the desired outcome.
- **Good**:
  ```goop
  let sqrt (x : int where x >= 0) = ...
  let _ = sqrt 4   (* runtime check skipped *)
  ```

---

## CLI — Command-Line and File Errors

These errors occur before or during the compiler pipeline and are not
associated with a specific source location.

### CLI001: Usage help

- **Error code**: `CLI001`
- **Severity**: Error
- **Message**:
  ```
  Usage: goop [--in-tree] [--emit-map] [--no-source-map] [--color] [-i] <command> <file.goop>
  Commands: lex, parse, check, compile, build, test, get, resolve, lsp, fmt (format)
  compile/build write to $GOOP_HOME/build by default; --in-tree writes beside source
  ```
- **Example**: Running `c0` with no arguments.
- **Trigger**: The `c0` command is invoked with fewer than 2 arguments (and
  not the `test` subcommand).
- **Fix**: Provide a valid command and source file.

### CLI002: File read error

- **Error code**: `CLI002`
- **Severity**: Error
- **Message**: `error reading %s: %v`
- **Example**: `error reading /path/to/missing.goop: open /path/to/missing.goop: no such file or directory`
- **Trigger**: The source file cannot be read (does not exist, permissions
  error, etc.).
- **Fix**: Ensure the file path is correct and readable.

### CLI003: Unknown command

- **Error code**: `CLI003`
- **Severity**: Error
- **Message**: `unknown command: %s`
- **Example**: `unknown command: foo`
- **Trigger**: An unrecognized subcommand is used.
- **Fix**: Use one of the listed commands.

### CLI004: Lex error

- **Error code**: `CLI004`
- **Severity**: Error
- **Message**: `lex error: %v`
- **Example**: `lex error: test.goop:5:3: unexpected character '@'`
- **Trigger**: The `lex` command encounters a lexer error in the source file.
- **Fix**: See the specific LEX error code in the message.

### CLI005: Parse error

- **Error code**: `CLI005`
- **Severity**: Error
- **Message**: `parse error: %v` or `FAIL: parse error: %v`
- **Example**: `parse error: test.goop:10:1: expected EQUALS, got IDENT`
- **Trigger**: The parser encounters a syntax error.
- **Fix**: See the specific PARSE error code in the message.

### CLI006: Codegen error

- **Error code**: `CLI006`
- **Severity**: Error
- **Message**: `codegen error: %v`
- **Example**: `codegen error: encoding source map: ...`
- **Trigger**: Code generation fails. This includes source map encoding
  errors.
- **Fix**: This is usually an internal error. Check that the program
  typechecks successfully first.

### CLI007: Write error

- **Error code**: `CLI007`
- **Severity**: Error
- **Message**: `write error: %v` or `cache dir: %v`
- **Example**: `write error: write /home/user/.cache/goop/build/compile-…/main.go: permission denied`
- **Trigger**: The generated Go file (or build cache directory) cannot be written.
  Default output is under `$GOOP_HOME/build` (see [20-cli-artifacts.md](20-cli-artifacts.md)).
  With `--in-tree`, output is beside the `.goop` source.
- **Fix**: Ensure `$GOOP_HOME` (or the source directory for `--in-tree`) is writable. Check disk space.

### CLI008: Temp directory error (build)

- **Error code**: `CLI008`
- **Severity**: Error
- **Message**: `temp dir error: %v`
- **Example**: `temp dir error: mkdir /tmp/c0-build-xxxxx: permission denied`
- **Trigger**: A temporary directory for the `build` command cannot be
  created.
- **Fix**: Check `/tmp` permissions or set `TMPDIR`.

### CLI009: Go build failed

- **Error code**: `CLI009`
- **Severity**: Error
- **Message**: `go build failed:` followed by Go compiler output.
- **Example**: `go build failed:\n./output.go:10:2: undefined: fmt`
- **Trigger**: The `build` command generates Go code but the Go compiler
  fails to compile it. This can happen if the generated Go code has
  issues or required imports are unavailable.
- **Fix**: Check the Go compiler's error message. May indicate a Goop compiler
  bug in code generation, or that the generated code references an
  unavailable Go package.

### CLI010: No test files found

- **Error code**: `CLI010`
- **Severity**: Error
- **Message**: `no test files found in %s (matching *_test.goop)`
- **Example**: `no test files found in ./tests (matching *_test.goop)`
- **Trigger**: The `goop test <dir>` command finds no `*_test.goop` files.
- **Fix**: Ensure test files exist and follow the `*_test.goop` naming
  convention.

### CLI011: Gosig fallback warning

- **Error code**: `CLI011` (related wire codes: `GOSIG001` / `GOSIG002`)
- **Severity**: Warning
- **Message**: `goop: gosig fallback for %s.%s: %v` (or `GOSIG002: …`)
- **Example**: `goop: gosig fallback for fmt.Printf: loading package "fmt": ...`
- **Trigger**: The compiler tried to refine the type of an extern binding by
  looking up the real Go function signature, but the lookup failed. The
  declared Goop type is used as a fallback.
- **Fix**: This is a soft warning; compilation continues with the declared
  type. If the declared type is wrong, runtime errors may occur.

### CLI012: Source map error

- **Error code**: `CLI012`
- **Severity**: Warning
- **Message**: `source map create error: %v` or `source map write error: %v`
- **Example**: `source map create error: permission denied`
- **Trigger**: The source map file could not be created or written.
  Source maps are off by default; enable with `--emit-map`.
- **Fix**: Check disk permissions. Source map generation is optional — the
  `.go` output is still generated.

### CLI013: Config read error

- **Error code**: `CLI013`
- **Severity**: Error (in library context)
- **Message**: `reading config %s: %w`
- **Example**: `reading config ./goop.toml: permission denied`
- **Trigger**: The `goop.toml` project configuration file exists but cannot be
  read.
- **Fix**: Check file permissions on `goop.toml`.

---

## Appendix A: Error Count Summary

| Category | Number of error/warning sites |
|---|---|
| LEX (Lexer) | 9 |
| PARSE (Parser) | 23 |
| TYPE (Typechecker, direct) | 13 |
| FFI-IMPL (Go interface implementation) | 2 |
| UNIFY (Unification) | 19 |
| EXHAUST (Exhaustiveness) | 3 |
| LINEAR (Linear checker) | 7 |
| REFINE (Refinement solver) | 2 (+ 1 silent outcome) |
| CLI (CLI/file/system) | 13 |
| **Total** | **91** |

---

## Appendix B: Error Reporting Format

All Goop compile-time errors follow this format:

```
<file>:<line>:<column>: <message>
```

For example:

```
examples/demo.goop:15:10: type mismatch: got int, expected string (different primitive types)
```

Warnings are prefixed with `WARNING:` on stderr:

```
WARNING: could not prove refinement x > 0 at line 15 — runtime check emitted
```

The compiler exit code is:
- `0` — Success (no errors, warnings allowed)
- `1` — Error (compilation failed)
