#!/usr/bin/env bash
# Build Goop playground WASM + copy wasm_exec.js into this directory.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/src"
OUT="$ROOT/playground"

GOROOT="$(go env GOROOT)"
WASM_EXEC=""
for candidate in "$GOROOT/lib/wasm/wasm_exec.js" "$GOROOT/misc/wasm/wasm_exec.js"; do
  if [[ -f "$candidate" ]]; then
    WASM_EXEC="$candidate"
    break
  fi
done

if [[ -z "$WASM_EXEC" ]]; then
  echo "error: wasm_exec.js not found under GOROOT=$GOROOT" >&2
  exit 1
fi

echo "→ copying wasm_exec.js"
cp "$WASM_EXEC" "$OUT/wasm_exec.js"

echo "→ building goop.wasm (GOOS=js GOARCH=wasm)"
(
  cd "$SRC"
  GOOS=js GOARCH=wasm go build -o "$OUT/goop.wasm" ./cmd/playground-wasm
)

echo "done: $OUT/goop.wasm"
echo "serve with:  cd playground && python3 -m http.server 8080"
