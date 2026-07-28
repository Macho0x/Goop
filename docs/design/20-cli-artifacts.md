# CLI artifacts: compile, build, test

Goop lowers to Go, but **developers work with `.goop` only**. Generated `.go`
and optional source maps live under `$GOOP_HOME` (default `~/.cache/goop`).

## Locations

| Path | Purpose |
|------|---------|
| `$GOOP_HOME/pkg/mod` | Cloned modules from `goop get` |
| `$GOOP_HOME/build/compile-*` | `goop compile` output |
| `$GOOP_HOME/build/build-*` | `goop build` sandbox (entry + deps + `go.mod`) |
| `$GOOP_HOME/build/test-*` | `goop test` sandbox (removed after each test) |
| `$GOOP_HOME/build/go-sigs/` | Generated `.gosig` stubs (`goop get-go-sig`; see [23-go-sig-resolution.md](23-go-sig-resolution.md)) |

## Commands

### `goop new [dir]`

Scaffold a project directory (default `.`) with `goop.toml` and `main.goop`.
Refuses non-empty directories unless `--force` (overwrites scaffold files).

```bash
goop new hello
cd hello && goop check main.goop && goop build main.goop
```

### `goop check <file.goop>`

Type-check and safety only. Writes nothing. Single file only.

### `goop lint <file-or-dir>`

Same diagnostic pipeline as `check`, but accepts a directory of `.goop`
files, prints a summary count, and exits non-zero on errors. See
[22-diagnostics.md](22-diagnostics.md).

### `goop compile <file.goop>`

Emits Go into `$GOOP_HOME/build/compile-*` and prints the path.
Does not run `go build`.

### `goop build <file.goop>`

1. Type-check + safety on the entry file
2. Compile transitive `import goop` deps into the build sandbox
3. Run `go mod tidy` in the sandbox (resolves third-party `import go` modules)
4. Run `go build` in the sandbox
5. For programs with `func main`, write `./goop-out` in the current working
   directory (`goop-out.exe` on Windows)

Does **not** leave `.go`, `.map.json`, or `go.mod` in the source tree.

### `goop test [dir]`

Discovers `*_test.goop`, builds each in an ephemeral sandbox (same dep wiring
as `goop build`), runs the binary, then deletes the sandbox.

### `goop doc <file-or-dir>`

Emits Markdown API docs to **stdout** for `.goop` modules (and `.gosig` stubs
when present). Covers module name, non-`private` type declarations, and
top-level `let` bindings with parameter/return types when annotated.

Comments (`(* … *)`, `//`) are stripped by the lexer today, so the MVP lists
signatures only — there is no separate `(** … *)` doc-comment form yet.
Directory mode walks for `.goop` / `.gosig` (skips `.git`). Full curated
`.gosig` generation is shipped (H5); `doc` best-effort extracts `module` /
`type` / `val` lines from stubs. It does **not** replace hand-written
`docs/stdlib/` pages (prelude tables, doctrine, Go lowering notes).

## Flags

| Flag | Default | Effect |
|------|---------|--------|
| (none) | cache | Write under `$GOOP_HOME/build` |
| `--in-tree` | off | Write `.go` beside the `.goop` source |
| `--emit-map` | off | Write `.map.json` next to the generated `.go` |
| `--no-source-map` | — | Accepted; maps stay off (compat with older scripts) |

## Mixed Go + Goop

Use `--in-tree` when a package already contains hand-written `.go` files that
must compile together with the generated output.
