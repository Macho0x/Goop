# WASM entrypoint

The Go WASM wrapper lives in the compiler module so it can import
`goop.dev/compiler/internal/...` without a second `go.mod`:

```
src/cmd/playground-wasm/main.go
```

Build from `src/` (or run `../build.sh`):

```bash
cd ../../src
GOOS=js GOARCH=wasm go build -o ../playground/goop.wasm ./cmd/playground-wasm
```
