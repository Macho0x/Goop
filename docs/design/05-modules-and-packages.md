# Goop Modules and Packages

## Module declaration

Every Goop source file begins with a module declaration:

```goop
module MyModule
```

A file-level module corresponds to a Go package. The module name determines the emitted Go package name and default file path.

## Nested modules (OCaml-style, minimal)

```goop
module Inner = struct
  let add (a: int) (b: int) : int = a + b
end

open Inner
```

Also supported:

| Form | Role |
|------|------|
| `module M = struct … end` | Nested structure |
| `module M : S = …` | Inline sealing (no `.mli`) |
| `module type S = sig … end` | Signature |
| `module type of M` | Synthesize signature |
| `module rec` | Recursive modules |
| `S with type` / `with module` / `:=` | Signature constraints |
| `module F (X : S) = struct … end` / `functor` | Functors |
| `(module M : S)` / `(val e : S)` | First-class modules |
| `open` / `open!` / `let open` / `M.(…)` | Local visibility |
| `include` | Re-export into current module |
| `let module M = … in …` | Local module |

See [14-ocaml-parity.md](14-ocaml-parity.md) and [`modules.goop`](../examples/modules.goop). Everyday projects still use one file-level `module` plus Go-style imports.

## Unified imports (Go-style)

Goop keeps Go-style import syntax (intentional extension). Legacy top-level-only `open` as the sole import mechanism and `extern "go"` were removed earlier.

```goop
module Main

import (
  go "fmt"
  go "strconv" {
    type NumError
    val Atoi : string -> (int, string)
  }
  goop "std.io"
  httpx go "net/http"
  orderbook goop "github.com/you/app/orderbook"
)

import goop . "std.list"   (* dot import: unqualified exports *)
```

| Form | Meaning | Block contents |
|------|---------|----------------|
| `import go "path"` | Go package | Optional free-function `val` signatures, selector `val (x : T).M` signatures, and opaque `type Name` imports |
| `import goop "path"` | Goop module | Forbidden |
| `import goop . "path"` | Dot import | Forbidden |
| `alias go "path"` | Go import with local alias | Optional |
| `alias goop "path"` | Qualified Goop import | Forbidden |

Logical paths like `"std.io"` resolve via `goop.toml` `[mappings]` or built-in defaults.
Imported Go types can be used in `implements`; see
[17-go-implements.md](17-go-implements.md). A selector declaration has the
form `val (x : T).M : τ`: use an arrow type for a Go method and a non-arrow
type for a Go field. See [18-go-methods.md](18-go-methods.md).

## Inline Go / C

```goop
@[go] {
  func helper() int { return 42 }
}
val helper : unit -> int

@[c] {
  int add(int a, int b) { return a + b; }
}
val add : int -> int -> int
```

See [15-lang-embeds.md](15-lang-embeds.md).

## Visibility

```goop
private let helper x = x + 1
let main () = helper 1   (* OK in same module *)
```

`private type` brands ADT constructors at the module boundary (preferred over removed `newtype`).

## Configuration and packages (1.0)

Goop packages are **Go modules**. There is no separate Goop registry in 1.0:
publish by hosting a Git repo under a normal module path, and install with
`goop get`. Pins live in `goop.toml` `[dependencies]` and `goop.lock`.

See [11-package-manager.md](11-package-manager.md) for the full story:
`goop get`, `$GOOP_HOME/pkg/mod`, lockfile format, import resolution, and
what is deliberately out of scope.

## Compilation unit

All sibling `.goop` files in a directory that share the same file-level
`module Name` (other than `main` / `Main`) are merged into one compilation
unit and emit one Go package. `private` is package-wide across those files.
Duplicate top-level names → **MODULE001**. Merge also runs when a package is
loaded as an `import goop` dependency.

Independent programs in a flat folder (e.g. `docs/examples/*.goop`) use
`module main` and are **not** merged.

Transitive `import goop` is resolved for type-checking and test builds.
See [33-sdk-blockers.md](33-sdk-blockers.md).
