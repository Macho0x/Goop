# Contributing to Goop

Thank you for contributing. This guide covers build/test workflow, documentation expectations, and editor tooling.

## Before you start

- [Design overview](docs/design/01-overview.md), [STYLE.md](docs/design/STYLE.md), and [roadmap](docs/design/07-roadmap.md)
- [Language tutorial](docs/tutorial/README.md) — how the language fits together
- [Standard library reference](docs/stdlib/README.md) — prelude, builtins, `std.*`
- [TODO.md](TODO.md) — open work
- Error codes: [10-error-reference.md](docs/design/10-error-reference.md), [12-trading-bot-safety.md](docs/design/12-trading-bot-safety.md)

## OCaml-aligned style

Goop 1.0 uses OCaml spelling for everyday constructs (`ref`/`!`/`:=`, `match`, `failwith`, `mod`, …). Do not reintroduce removed sugar (`?`, `result { }`, `let mutable`, `newtype`, `with { io }`, `panic`, `%`). See [STYLE.md](docs/design/STYLE.md).

## Build and test

```bash
cd src
go build -o ../goop ./cmd/goop

# Or install a release binary:
# curl -fsSL https://raw.githubusercontent.com/Macho0x/Goop/main/scripts/install.sh | bash

# Go compiler unit tests
go test ./...

# Goop end-to-end tests
../goop test ../tests/

# Scaffold smoke
../goop new /tmp/goop-new-smoke --force && ../goop check /tmp/goop-new-smoke/main.goop

# All examples (CI does this)
for f in ../docs/examples/*.goop; do ../goop check "$f"; done

# Cache-only build smoke (no .go left in docs/examples)
../goop build ../docs/examples/hello.goop && ./goop-out && rm -f ./goop-out
```

Generated Go lives under `$GOOP_HOME/build` by default. See [20-cli-artifacts.md](docs/design/20-cli-artifacts.md).

### GitHub Pages (playground)

The playground deploys via [`.github/workflows/pages.yml`](.github/workflows/pages.yml).
**One-time:** repo Settings → Pages → Source → **GitHub Actions**. Until that is
set, the workflow may succeed while <https://macho0x.github.io/Goop/> still 404s.

### Where tests and examples live

| Tree | Job | Not for |
|---|---|---|
| `docs/examples/*.goop` | Canonical teachable demos; CI `goop check` | Edge-case matrices, removed syntax |
| `tests/*_test.goop` | Runnable e2e with `assert` | Live network demos, `assert true` stubs |
| `src/internal/*_test.go` | Parser/AST, LINEAR*/PARSE-MIG negatives, CPS corners | User-facing galleries |

## Dependencies for optional checks

- **Z3** (optional): when `[check] smt = true` in `goop.toml`, the refinement checker shells out to `z3 -in` to prove VCs, falling back to the built-in interval solver if Z3 is missing or inconclusive. Install via your package manager (`pacman -S z3`, `apt install z3`, `brew install z3`). CI does not require Z3 unless SMT tests are enabled. Without Z3, refinements still work via the built-in solver + runtime guards.

## Making changes

### Compiler / language

Touch `src/internal/` (lexer, parser, typecheck, codegen, safety passes) and add or update `tests/*.goop` e2e files for user-visible behavior.

### Prelude or `std.*`

| Change | Update |
|---|---|
| New prelude binding | `src/internal/prelude/prelude.go` + `docs/stdlib/prelude.md` |
| New `std.*` export | `std/*/*.goop` + matching `docs/stdlib/std-*.md` |
| New builtin type | `src/internal/typecheck/` + `docs/stdlib/builtins.md` |

### Examples

Add runnable files under `docs/examples/`. CI runs `goop check` on every file.

### Design docs

Keep `docs/design/`, `TODO.md`, and `docs/design/07-roadmap.md` in sync when behavior changes.

### Syntax highlighting

Canonical grammar: [`syntaxes/goop.tmLanguage.json`](syntaxes/goop.tmLanguage.json).

After editing:

```bash
./scripts/sync-syntax.sh
```

Reload VS Code / Zed after grammar changes.

### Formatting

- Goop sources: `goop fmt`
- Go in `src/`: `gofmt`

## Editor extensions

### VS Code / Cursor

```bash
./scripts/install-editor-extension.sh
```

Then reload the window (`Developer: Reload Window`). Workspace recommendations do not install local extensions automatically.

Provides syntax highlighting, LSP via `goop lsp`, and optional **Goop File Icons** theme (`Preferences: File Icon Theme`).

See [editors/vscode/README.md](editors/vscode/README.md).

### Zed

```bash
cd editors/zed && make install
```

### GitHub highlighting

GitHub requires [Linguist](https://github.com/github-linguist/linguist) registration. Submission package: [`docs/github-linguist/`](docs/github-linguist/README.md).

Until Linguist merges Goop, [`.gitattributes`](.gitattributes) maps `*.goop` to F# for approximate highlighting.

## Documentation accuracy

Documentation must match the compiler, not aspirational design:

1. **Verify** — run `goop check` on any example you add or change.
2. **Trace to source** — prelude from `prelude.go`, `std.*` from `std/*/*.goop`, syntax from `docs/design/03-syntax.md` and `src/internal/parser/`.
3. **Update together** — compiler change + e2e test + relevant doc page (tutorial, stdlib, or design doc). Prefer OCaml forms from [STYLE.md](docs/design/STYLE.md).
4. **Error codes** — new diagnostics need entries in `docs/design/10-error-reference.md` (migration errors: PARSE-MIG010–018).

## Pull requests

- One logical change per PR when possible.
- Include e2e tests for user-visible behavior.
- Note breaking changes and migration steps.
- If you change prelude, `std.*`, or public CLI behavior, update the stdlib reference or tutorial.

## Project structure (quick reference)

```
src/cmd/goop/          CLI entry (check, build, lsp, test, …)
src/internal/          Compiler implementation
std/                   std.io, std.list, std.array, std.option, std.result,
                       std.string, std.chan (→ std/channel), std.ref
                       (+ std.decimal when present)
tests/                 End-to-end .goop tests
docs/examples/         Runnable examples (CI-checked)
docs/tutorial/         Step-by-step tutorial
docs/stdlib/           Hand-written API reference
docs/design/           Language design documents
syntaxes/              Canonical TextMate grammar
editors/vscode/        VS Code extension
editors/zed/           Zed extension
```

## Not yet available

- `goop doc` — automated documentation generator ([TODO.md](TODO.md))

## License

By contributing, you agree your contributions are licensed under the project’s MIT / Apache-2.0 dual license.
