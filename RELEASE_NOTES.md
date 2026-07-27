# Goop 1.9.0

Goop 1.9.0 is the interop / ergonomics / domain-fit train from `GOOP_UPDATES.md`:
try Goop in the browser, call Go with less ceremony, and ship trading-adjacent
primitives without silent compiler degradation.

## Highlights

- **Go sigs + error coercion:** `goop gen-sig` / `get-go-sig`; `(T, error)` →
  `result` by default (`import go raw` to keep tuples).
- **Playground + editors:** WASM playground; Neovim and Helix LSP packs.
- **CLI:** `version`, `lint`, `doc`, `repl`.
- **No silent codegen TODOs:** unhandled nodes are hard Goop errors.
- **`std.decimal`**, stdlib doctrine, branded-ID decision (ADT surface).

## Workflow

```bash
cd src && go build -o ../goop ./cmd/goop
../goop version
../goop check docs/examples/hello.goop
../goop build docs/examples/hello.goop
./playground/build.sh   # optional WASM playground
```

See `CHANGELOG.md` and `GOOP_UPDATES.md` for the full checklist.
