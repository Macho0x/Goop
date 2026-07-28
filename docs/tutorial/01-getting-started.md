# 1. Getting started

Goop compiles to Go. You write `.goop` files; the toolchain type-checks them and
emits Go into a build cache under `$GOOP_HOME/build` (default `~/.cache/goop/build`).
Your project tree stays `.goop`-only unless you pass `--in-tree`.

## Your first program

```goop
module main

let main () =
  print_line "Hello, Goop!"
```

`print_line` is a **prelude** binding — available in every file without an import. It lowers to `fmt.Println`.

## Build the compiler

**Install (release binary):**

```bash
curl -fsSL https://raw.githubusercontent.com/Macho0x/Goop/main/scripts/install.sh | bash
goop version
```

**Or build from source:**

```bash
cd src
go build -o ../goop ./cmd/goop
```

## Scaffold a project

```bash
goop new hello
cd hello
goop check main.goop
goop build main.goop
./goop-out
```

`goop new` writes `goop.toml` and `main.goop`. Use `--force` to overwrite in a non-empty directory.

## Check a file

```bash
./goop check docs/examples/hello.goop
# OK: docs/examples/hello.goop parsed and type-checked successfully
```

`goop check` runs parsing, type inference, effect checking, and safety passes (exhaustiveness, linear analysis, nil channels, refinements).

## Build and run

```bash
./goop build docs/examples/hello.goop
# wrote ~/.cache/goop/build/build-…/main.go
# build succeeded in …
./goop-out
# Hello, Goop!
```

`goop build` compiles the entry file and transitive `import goop` deps into the
cache, then runs `go build` there. For `module main`, the binary is written to
`./goop-out` in the current working directory.

| Command | Default output | Notes |
|---------|----------------|-------|
| `goop compile` | `$GOOP_HOME/build/compile-*/` | Emit Go only; no `go build` |
| `goop build` | `$GOOP_HOME/build/build-*/` + `./goop-out` for main | Full Goop build |
| `goop test` | `$GOOP_HOME/build/test-*/` (ephemeral) | Runs `*_test.goop` |
| `goop doc` | Markdown on stdout | API docs for modules / stubs |
| `goop repl` | stdin session; temp `$GOOP_HOME/build/repl-*/` | Compile-to-Go REPL |

## REPL

```bash
./goop repl
```

```
goop> 1 + 1
2
goop> let double (x: int) : int = x + x
ok
goop> double 21
42
goop> :type double 21
- : int
goop> :quit
```

The REPL has no interpreter — each input is type-checked, compiled to a small Go
program, and run with `go run`. Declarations accumulate in the session.
Printable expression types today: `int`, `float`, `string`, `bool`, `unit`.
Commands: `:help`, `:quit` / `:q`, `:type <expr>`, `:reset`. End a line with `\`
to continue on the next line.

Flags:

- `--in-tree` — write `.go` beside the `.goop` source (legacy / mixed Go+Goop)
- `--emit-map` — write `.map.json` next to the generated `.go`
- `--no-source-map` — accepted alias; maps are off by default

### Document modules

```bash
./goop doc docs/examples/hello.goop
```

`goop doc` extracts module-level docs from `.goop` sources. See
[20-cli-artifacts.md](../design/20-cli-artifacts.md) for cache layout and all
artifact commands.

## Project layout

```
my-project/
  goop.toml          # feature flags, std module mappings
  main.goop          # module main
  lib/
    utils.goop       # module Utils
```

Every file starts with `module Name`. The module name should match the intended Go package name.

## Functions and types

```goop
let double (x: int) : int = x + x

type color = Red | Green | Blue

let name (c: color) : string =
  match c with
  | Red -> "red"
  | Green -> "green"
  | Blue -> "blue"
```

Types are inferred when omitted. `match` must be **exhaustive** — the compiler rejects missing variants (see [chapter 6](06-safety-checks.md)).

## Run tests

```bash
./goop test tests/
```

## Next

[2. Types and patterns →](02-types-and-patterns.md)
