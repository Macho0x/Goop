# Goop language tutorial

A step-by-step introduction to Goop. Each chapter links to runnable examples checked by CI (`goop check`).

| Chapter | Topic | Example |
|---|---|---|
| [1. Getting started](01-getting-started.md) | Build, check, first program, REPL | [`hello.goop`](../examples/hello.goop) |
| [2. Types and patterns](02-types-and-patterns.md) | ADTs, `match`, branded IDs | [`shapes.goop`](../examples/shapes.goop) |
| [3. Errors and effects](03-errors-and-effects.md) | `result`, `failwith`, effect handlers | [`result.goop`](../examples/result.goop), [`effects.goop`](../examples/effects.goop), [`exceptions.goop`](../examples/exceptions.goop) |
| [4. Go / C interop](04-go-interop.md) | `import go`, `@[go]`, `@[c]` | [`extern_demo.goop`](../examples/extern_demo.goop), [`cgo_demo.goop`](../examples/cgo_demo.goop) |
| [5. Concurrency](05-concurrency.md) | `go`, `chan`, `ref`, race checks | [`concurrency.goop`](../examples/concurrency.goop), [`race_detection.goop`](../examples/race_detection.goop) |
| [6. Safety checks](06-safety-checks.md) | Exhaustiveness, branding, refinements | [`branded_ids.goop`](../examples/branded_ids.goop), [`trading_order.goop`](../examples/trading_order.goop) |
| [7. Arrays and loops](07-arrays-and-loops.md) | `Array.make`, `for`/`while`, `begin`/`end` | [`arrays.goop`](../examples/arrays.goop), [`trading_decision_lut.goop`](../examples/trading_decision_lut.goop) |
| [8. Maps](08-maps.md) | `map[K] V`, `Map.*` | [`maps.goop`](../examples/maps.goop) |

## Prerequisites

**Try online:** [Playground](https://macho0x.github.io/Goop/)

```bash
# Install release binary, or build from source:
curl -fsSL https://raw.githubusercontent.com/Macho0x/Goop/main/scripts/install.sh | bash
# — or —
cd src && go build -o ../goop ./cmd/goop

goop new hello && cd hello
goop check main.goop
```

## Editor setup

- **VS Code**: install from a [GitHub Release `.vsix`](https://github.com/Macho0x/Goop/releases) or [`editors/vscode`](../../editors/vscode) — syntax highlighting, `.goop` file icon, LSP
- **Zed**: `cd editors/zed && make install`
- **Neovim**: see [`editors/neovim`](../../editors/neovim)
- **Helix**: see [`editors/helix`](../../editors/helix)
- **GitHub**: interim F# highlighting via [`.gitattributes`](../../.gitattributes); full Goop highlighting after [Linguist submission](github-linguist/README.md)

## Further reading

- [Style guide (1.0)](../design/STYLE.md)
- [Syntax reference](../design/03-syntax.md)
- [Type system](../design/02-type-system.md)
- [Maps (`map[K] V`)](../design/29-maps.md)
- [CLI artifacts](../design/20-cli-artifacts.md) — `goop doc`, cache layout
- [Writing tools](../guides/writing-tools.md) — files + maps via `import go`
- Sample apps: [`http_hello.goop`](../examples/http_hello.goop), [`cli_files.goop`](../examples/cli_files.goop)
- [Packages and registry (1.0)](../design/11-package-manager.md) — Go modules + `goop get`; no separate registry
- [Benchmarks](../../benchmarks/README.md) — generated vs hand Go (indicative)
- [Standard library reference](../stdlib/README.md)
- [All examples](../examples/)
- [Playground](https://macho0x.github.io/Goop/)