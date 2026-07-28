# Goop web playground
#
# Type Goop in the browser, see diagnostics and generated Go.
# The browser does **not** run `go build` on the output — that stays on the host CLI.

## Layout

```
playground/
  index.html      # UI shell
  style.css
  app.js          # loads WASM, wires Check/Compile
  examples.js     # embedded snippets (hello, shapes, orderbook, …)
  build.sh        # builds goop.wasm + copies wasm_exec.js
  goop.wasm       # build artifact (not committed)
  wasm_exec.js    # from the Go distribution (copied by build.sh)

src/cmd/playground-wasm/main.go   # thin WASM wrapper (check / compile)
```

The WASM entry lives under the compiler module (`src/`) so it can import
`goop.dev/compiler/internal/...` without a second `go.mod`.

## Prerequisites

- Go 1.25+ (same as the compiler)
- A local static file server (Python, `npx serve`, etc.)

## Build WASM

From the repo root:

```bash
./playground/build.sh
```

Or manually from `src/`:

```bash
cd src
GOOS=js GOARCH=wasm go build -o ../playground/goop.wasm ./cmd/playground-wasm
```

### `wasm_exec.js`

The Go WASM runtime glue must match your Go toolchain. Copy it from GOROOT:

```bash
# Go 1.24+
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" playground/

# Older Go toolchains used:
#   $(go env GOROOT)/misc/wasm/wasm_exec.js
```

`build.sh` does this for you.

## Hosted playground

After GitHub Pages is enabled (Settings → Pages → Source: **GitHub Actions**),
the playground is at <https://macho0x.github.io/Goop/>. Workflow:
[`.github/workflows/pages.yml`](../.github/workflows/pages.yml).

## Serve locally

WASM fetch requires HTTP (not `file://`):

```bash
cd playground
python3 -m http.server 8080
```

Then open http://localhost:8080/

Alternatives:

```bash
npx --yes serve -l 8080 .
# or:  go run golang.org/x/tools/cmd/goimports@latest  # (not a server)
# or:  ruby -run -e httpd . -p 8080
```

## API (JS globals from WASM)

After load, the page exposes:

| Function | Returns (JSON string) |
|----------|------------------------|
| `goopCheck(src)` | `{ ok, diagnostics }` |
| `goopCompile(src)` | `{ ok, diagnostics, go }` |

Source is analyzed in-memory as `playground.goop` (no filesystem). Imports that need disk / Go packages are not resolved in the playground.

## Known limitations

- Shows **generated Go + Goop diagnostics only** — does not compile or run the Go output.
- No `import go` / `import goop` module graph (in-memory single file).
- Refinement SMT (Z3) is off by default; unproven refinements stay warnings.
- Large programs may hitch the UI (WASM work is synchronous on the main thread).
