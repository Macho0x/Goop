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

## Prerequisites

```bash
cd src && go build -o ../goop ./cmd/goop
../goop check ../docs/examples/hello.goop
```

## Editor setup

- **VS Code**: install [`editors/vscode`](../../editors/vscode) — syntax highlighting, `.goop` file icon, LSP
- **Zed**: `cd editors/zed && make install`
- **GitHub**: interim F# highlighting via [`.gitattributes`](../../.gitattributes); full Goop highlighting after [Linguist submission](github-linguist/README.md)

## Further reading

- [Style guide (1.0)](../design/STYLE.md)
- [Syntax reference](../design/03-syntax.md)
- [Type system](../design/02-type-system.md)
- [Packages and registry (1.0)](../design/11-package-manager.md) — Go modules + `goop get`; no separate registry
- [Standard library reference](../stdlib/README.md)
- [All examples](../examples/)
