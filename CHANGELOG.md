# Changelog

## 1.11.0

### Tooling / UX

- **GitHub Pages playground** workflow (`pages.yml`); base-path-safe assets;
  transparent-Go and maps examples.
- **One-line install:** `scripts/install.sh` downloads the latest release binary.
- **`goop new [dir]`** scaffolds `goop.toml` + `main.goop` (`--force` to overwrite).
- **Diagnostic `help:` tips** for common codes in `report.Render`.
- **RESULT001:** warn on discarded `result` values in `begin` sequences
  (`[check] discarded_result`).
- Release attaches **VS Code `.vsix`**; optional Marketplace / Open VSX publish
  when secrets are set (shell-gated; no `secrets` in `if:`).

### Interop

- Directional Go channels (`<-chan` / `chan<-`) map to Goop `T chan` in
  typecheck gosig fallback and gosiggen.

### Docs / examples

- Sample apps: `http_hello.goop`, `cli_files.goop`; benchmark README refresh
  (zero-cost brands ≈ hand Go).

## 1.10.1

### Docs

- Tutorial chapter 8 (maps); expanded getting-started (`goop doc`), interop
  (bare `.gosig` auto-load), and branding (zero-cost note).
- Syntax / STYLE / type-system / prelude / stdlib references cover `map[K] V`
  and `Map.*`; stdlib README reflects shipped `goop doc`.
- Design status sync: branded IDs H4c, gosig auto-load, FFI boundary, roadmap
  `goop doc`, freeze checklist.

## 1.10.0

### Language

- **First-class maps:** `map[K] V` lowers to Go `map[K]V`; prelude
  `Map.make` / `get` / `add` / `remove` / `mem` / `size`; thin `std.map`.
- **Zero-cost brands (H4c):** single-constructor ADTs with primitive/string
  payloads emit Go defined types instead of interface+struct boxing.

### Interop

- **Bare-import `.gosig` autoload:** `import go "pkg"` loads project
  `goop-sigs/` overrides or `$GOOP_HOME` cache (curated generate-on-miss).
  Explicit `{ val … }` blocks stay authoritative.
- **`obj` ≡ `any`** in stubs; **multi-result** Go returns emit Goop product
  tuples when representable; **Go maps** in gosiggen.
- Curated stubs under `goop-sigs/` for `os`, `path/filepath`, `bytes`,
  `bufio`, `strings`, `encoding/json`.

### CI / docs

- Windows unit tests: explicit `.exe` binary paths; portable gosiggen path
  assertions.
- CI sig-corpus smoke (`gen-sig --smoke` + writing-tools/maps check).
- Design docs: maps, freeze checklist; writing-tools guide; self-host lexer spike.

## 1.9.0

### Interop

- **Curated Go sig generator:** `goop gen-sig` / `goop get-go-sig` emit `.gosig`
  stubs via `go/types` into `$GOOP_HOME/build/go-sigs/` (`goop-sigs/` overrides).
- **`(T, error)` → `result`:** `import go` call sites see `result T` by default;
  opt out with `import go raw "…"`.
- **Extern multi-value returns:** Go multi-results map to Goop tuples for
  hand-written and refined externs.
- Variadic `...any` / `go_slice` declarations are preserved over gosig’s
  `[]interface{}` → list rewrite.

### Ergonomics

- **Web playground:** `playground/` WASM check/compile UI with embedded examples.
- **Editors:** Neovim + Helix LSP client packaging (`editors/neovim`, `editors/helix`).
- **CLI:** `goop version`, `goop lint`, `goop doc`, `goop repl`.
- **Codegen hard errors:** unhandled AST nodes fail at Goop compile time (no
  silent `/* TODO */` in generated Go).

### Domain fit / stdlib

- **Branded IDs:** design decision — single-ctor ADT + `private`; zero-cost
  codegen planned (`docs/design/21-branded-ids.md`).
- **`std.decimal`:** shopspring/decimal wrapper + money example.
- **Stdlib doctrine:** Go’s stdlib for coverage; thin `std.string` / `std.chan` /
  `std.ref` wrappers. String interpolation: **no** for 1.0.

### CI / docs

- Windows CI job; package-registry doctrine (Go modules); FFI boundary,
  inference-holes, diagnostics corpus, benchmark harness.

## 1.8.0

### Tooling

- **Cache-only compile/build:** `goop compile` and `goop build` write generated
  Go under `$GOOP_HOME/build` by default. Project trees stay `.goop`-only.
- **`goop build` wires `import goop` deps** into the build sandbox (same model
  as `goop test`), then runs `go build` there. Main packages emit `./goop-out`.
- Flags: `--in-tree` (legacy beside-source output), `--emit-map` (source maps;
  off by default), `--no-source-map` kept as a no-op alias.
- No more leaked temporary `go.mod` in the source directory after build.

### Tests / CI

- CLI integration tests for cache defaults, in-tree, maps, deps, library
  builds, and mixed `--in-tree` packages.
- CI smoke: `goop build` on `hello.goop` and assert no `.go` left in examples.

### Docs

- Tutorial / README / CONTRIBUTING updated for cache-only workflow.
- New [20-cli-artifacts.md](docs/design/20-cli-artifacts.md).

## 1.7.0

### Strings

- Lexer escapes: `\xHH` (2 hex), `\ooo` (1–3 octal ≤ 255), `\e` (ESC), plus
  existing `\n \t \r \\ \" \'`.
- Prelude: `String.length` / `String.sub` (Go byte semantics).

### FFI

- Implementor `T ptr` unifies with Go interfaces (`ptr_of {…}` for
  `slog.Handler` without `@[go]` ctors).
- Field/method short-name binds skip on collision with imported type names.
- **Go struct literals:** gosig `LookupType` / `Assignable`; typed
  `{ level = … }` / `ptr_of {}` for imported structs (`HandlerOptions`,
  `Mutex`, `Buffer`); Level → Leveler field assignability.
- Native `slog.Level` `>=` and `Value.Kind` / `Record` field imports.

### Tests

- `string_escape_test`, `string_slice_len_test`, `go_heap_ptr_ctor_test`,
  `go_level_compare_test`, `go_slog_record_value_test`,
  `go_handler_options_test`, `go_go_struct_literal_test`
- `lexer_test.go` unit coverage for escapes

### Docs

- Grammar / syntax / error-reference escape docs; prelude string ops;
  18-go-methods struct literals; 16-treelog-feedback 1.7 section.

## 1.6.0

### Cross-package codegen

- Open-imported record types register in codegen; literals and updates emit
  `pkg.Name{…}` (not anonymous `struct{…}`).
- Option fields on imported records qualify constructors
  (`pkg.NewOptionWriterSome`, `pkg.NewOptionStringNone`).
- Capitalized open-export calls share the local flatten / unit-elision path
  via `resolveCallTarget` (`pkg.NewScope(…)`, including partial apps).

### Parser / DX

- Multiline juxtaposition when the next token is `LPAREN` (or the arg line is
  indented past the callee).
- LSP `uriToPath` URL-decodes `%20` etc.; workspace `rootUri` fallback for
  `module_root` when `FindProjectRoot` fails on encoded paths.

### FFI / types

- Opaque heap types and pointer-receiver methods documented and tested as
  `T ptr` (`Buffer ptr` for `*bytes.Buffer`).
- `gosig`: `SetLoadDir`, `LookupVar`, named Go types → `TGoNamed` / `TPtr` /
  slices; untyped const strings normalize (`untyped int` → `int`).

### Tests

- `import_record_literal_test`, `import_capitalized_call_test`,
  `multiline_paren_app_test`, fixtures under `tests/fixtures/{recpkg,callpkg}/`
- Buffer-ptr updates: `go_method_import_test`, docs examples / tutorial

## 1.5.0

### Call lowering (OCaml-faithful)

- Capitalized multi-arg lets lower to one Go call (`Add(2, 3)`), not curried
  `Add(2)(3)`.
- `unit` parameters are erased from Go signatures; `f ()` / `M.f ()` no longer
  pass `struct{}{}` (including cross-package Goop calls and `time.Now ()`).
- `let x = if c then a else b` lowers via an IIFE so if-expressions work on
  let RHS and in expression position.
- Option/result Go type names use a consistent `optionTypeSuffix` so helpers
  match call sites (`OptionQuoteparams`).

### Tests

- `capitalized_multi_arg_test`, `go_unit_elision_test`, `if_expr_let_test`

### Docs

- Design note [19-call-lowering.md](docs/design/19-call-lowering.md); lowering
  and treelog feedback updates.

## 1.4.0

### Go method and field FFI

- `import go` signature blocks now accept `val (x : T).M : τ` selector
  declarations for imported Go methods and fields.
- Method calls lower to Go selectors, including callbacks, `go_slice` indexing,
  variadic `any` values, and pointer receivers.
- Added `bytes.Buffer` method-call and native `slog.Handler` examples, plus
  method/field lowering, package, embed, tutorial, and error-reference docs.

## 1.3.0

### Go interface FFI

- `import go` signature blocks now accept `type Name` for opaque Go named
  types, including interfaces.
- Added `implements Interface for Type with … end`, lowering methods to Go
  pointer receivers and emitting compile-time interface assertions.
- Added FFI support for `error`, `'a ptr` with `ptr_of` / `null` /
  `is_null`, and `'a go_slice` with conversion and collection helpers.
- Added native Goop examples for `fmt.Stringer` and `log/slog.Handler`.
- Added design, tutorial, error-reference, roadmap, and release documentation
  for the interface FFI.

## 1.2.3

### Editor

- VS Code extension **0.3.7**: move `editor.tokenColorCustomizations` / `textMateRules` to top-level `configurationDefaults` so theme overrides (including `keyword.embed.goop` amber `#D7BA7D` for `@[go]` / `@[c]`) apply under Dracula and other themes — language-scoped nesting was ignored by VS Code/Cursor

## 1.2.2

### Editor / LSP

- VS Code extension **0.3.6**: grammar keywords catch up to 1.1+ (`continue`, `discontinue`, `downto`, `functor`, `not`); `array` type scope; deprecated surface marked (`extern`/`guard`/`is`/`panic`/`using`)
- LSP: `documentFormattingProvider` via `goop fmt` engine (Format Document in the editor)
- LSP: fix Content-Length framing (no longer loses body bytes to `bufio.Scanner`); flush stdout after messages
- Extension sets `editor.defaultFormatter` to `goop.goop`

## 1.2.1

### Editor / hygiene

- Syntax: `@[go]` / `@[c]` tags use unified `keyword.embed.goop` (amber `#D7BA7D`); embed bodies keep normal Go/C colors (no block-wide tint)
- VS Code extension 0.3.4; grammar synced to Zed
- Remove unused `desugarIs` / `desugarAs`, `parseADTTypeKind`, dummy `var g`

## 1.2.0

### Breaking — lang embeds and Go imports

- `import golang` / `alias golang` / `@golang { }` / keyword `golang` removed
- Use `import go "path"`, `@[go] { … }`, and (new) `@[c] { … }`
- Migration errors point at the new spellings

### Added

- Generalized `LangEmbedDecl` (`@[go]` / `@[c]`); unknown langs hard-error
- Full cgo-shaped `@[c]`: preamble + `import "C"` + primitive `val` auto-wrappers (`int`, `int32`/`int64`, `float`/`float64`, `bool`, `string`, `unit`)
- Design note [15-lang-embeds.md](docs/design/15-lang-embeds.md); example [`cgo_demo.goop`](docs/examples/cgo_demo.goop)
- E2E: `c_embed_*`, renamed `import_go` / `go_embed` tests

### Docs / tooling

- Tutorial, grammar, TextMate (`@[go]`/`@[c]` scopes), README, stdlib notes updated

## 1.1.1

### Tests and examples overhaul

- Pruned overlapping/stale examples; removed non-CI `trading_binance.goop` from `tests/`
- Renamed misleading `async_test` / `guards_test` / `trading_newtype_test` to `go_chan` / `match_when` / `trading_branded_ids`
- Fixed `concurrency.goop` pedagogy; added `modules.goop` and `exceptions.goop`
- Stronger e2e: `select`, channel close, multi-effect, exception types, list/array asserts; FCM asserts sealed export
- `select` codegen unwraps `C0Chan`; `perform` inside parenthesized `go` bodies is rejected
- `std.array` param renamed `default` → `init` (Go keyword clash)

## 1.1.0

### OCaml parity mega-ship

- Control: `for … downto`, `let open` / `M.(…)` / `open!`, `exception` patterns, memoizing `Lazy.force` / `Lazy.from_val`
- Modules: inline `module M : S =` sealing (no `.mli`), `module type of`, `module rec`, `with type`/`with module`/`:=`, functors, first-class pack/unpack, real `include` re-exports
- Types: extensible variants, GADT result types, open/closed polymorphic variants, labelled/optional args
- Objects: `inherit` / `virtual` / `initializer` / `constraint` / `class type`, `#method`, object method codegen
- Effects: shallow CPS with real resume + `continue` / `discontinue`; attributes `[@@]`/`[@]`/`[%]` parse-and-strip (`@[go]` remains the only active extension)

## 1.0.1

### Documentation
- README first-viewport restructure for GitHub visitors; status wording fixed to shipped **1.0.1**
- Example renames: `branded_ids`, `match_patterns`, `linear_resource`, `chan_async`, `result_match` (folded/removed `using.goop`)
- Real `effects.goop` demo + stronger `effect_handler_test.goop`
- Doc accuracy: PARSE019/020 marked obsolete → PARSE-MIG016; deferred-analysis banner; STYLE drops `let*` as preferred; overview drops F# peer pitch

## 1.0.0

### Breaking — OCaml-aligned surface

Removed non-OCaml duplicate syntax (PARSE-MIG010–018):

- `let mutable` / binding `<-` → `ref` / `!` / `:=` (array `arr.(i) <-` kept)
- `?`, `result { }`, `async { }`, `region { }`, `guard` / `is` / expr `as` → `match`
- `newtype` → single-ctor ADT + `private`
- `with { io }` effect rows → OCaml 5-style `effect` / `perform` / handlers (CPS-lowered)
- `panic` / `panic_message` → `failwith`
- `%` → `mod` (`land` / `lor` / `lxor` added)

Kept: Go-style `import go` / `import goop`, `go` / `go (move …)`, `@[go] { }`, linear/`owned_chan`, `where` refinements.

### Added

- `while`, `function`, `exception` / `raise` / `try`/`with`/`finally`, `assert`, `lazy`, `//` comments
- Nested modules (`struct`/`sig`/`open`/`include`/functors/`let module`), `.mli` export check
- Polymorphic variants, GADTs (approx), labelled args, `[| … |]` array literals
- Minimal OCaml OOP (`class` / `object` / `new`)
- Effect handlers via CPS / free-monad lowering (effectful Go is not idiomatic by design)
- Optional Z3 SMT for refinements (`[check] smt = true`)
- Style guide: `docs/design/STYLE.md`, parity matrix: `docs/design/14-ocaml-parity.md`

### Examples & tests

- LUT decision logic refactored to `situation` ADT + `match`
- 49 e2e tests; new coverage for ref/while, exceptions, mod, function, array lit, effects, modules

## 0.8.2

### Documentation
- Sync all docs with compiler: version/chapter counts, `goop.toml` `deadlock` key, array/stdlib coverage
- Add NIL001, LINEAR008, DEADLOCK001 to error reference; fix trading-bot safety matrix (LINEAR001 typo)
- Replace stale `extern "go"` examples with `import go` / `@[go]` across syntax, lowering, and grammar docs
- Add arrays/for/begin/qualified constructors to `03-syntax.md`; array lowering to `spec/lowering.md`
- Update `prelude.md` and `builtins.md` with `Array.*`; rewrite Zed editor README for Goop

## 0.8.1

### Documentation & stdlib
- Tutorial chapter 7 (arrays and loops); semantics, lowering, and error-reference updates
- `std.array` thin re-export module; `goop.toml` mapping

### Tests
- Trading decision LUT test: qualified constructors, `begin`/`end` assertions
- Parser, typecheck, and fmt unit tests for OCaml surface syntax
- `std_array_test.goop`; `arrays.goop` example

## 0.8.0

### OCaml surface syntax
- Arrays via `Array.make`, `Array.length`, `arr.(i)`, `arr.(i) <- v`, and `'a array` types
- `for i = lo to hi do ... done` loops
- `begin ...; ... end` sequencing blocks
- Qualified constructors `Type.Ctor`

### Codegen
- Fix top-level record literal thunks, float paren precedence, option-in-record prescan, tuple match

### Tests & docs
- OCaml-style trading decision LUT test (375-cell universe)
- Eight new integration tests; design doc 13 and std-array reference
