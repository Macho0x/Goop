#!/usr/bin/env bash
# selfhost-spike.sh — typecheck/build the optional self-host lexer spike.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/src"
go build -o /tmp/goop-spike ./cmd/goop
export GOOP_HOME="${GOOP_HOME:-$ROOT/.goop-spike-home}"
mkdir -p "$GOOP_HOME"
/tmp/goop-spike check ../spike/selfhost-lexer/lexer.goop
/tmp/goop-spike build ../spike/selfhost-lexer/lexer.goop
echo "selfhost spike OK"
