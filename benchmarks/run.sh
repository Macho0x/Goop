#!/usr/bin/env bash
# Run Goop-generated vs hand-written microbenchmarks.
# Requires: go, and a goop binary (PATH, ./goop from a fresh build, or builds from src/).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BENCH_ROOT="$ROOT/benchmarks"
WORKDIR="${TMPDIR:-/tmp}/goop-bench-$$"
mkdir -p "$WORKDIR"
trap 'rm -rf "$WORKDIR"' EXIT

goop_is_current() {
  local bin="$1"
  # Current CLI prints "goop version X"; stale binaries treat "version" as a file.
  "$bin" version 2>/dev/null | grep -q '^goop version'
}

resolve_goop() {
  if [[ -n "${GOOP_BIN:-}" && -x "$GOOP_BIN" ]]; then
    echo "$GOOP_BIN"
    return
  fi
  local cand
  if command -v goop >/dev/null 2>&1; then
    cand="$(command -v goop)"
    if goop_is_current "$cand"; then
      echo "$cand"
      return
    fi
  fi
  if [[ -x "$ROOT/goop" ]] && goop_is_current "$ROOT/goop"; then
    echo "$ROOT/goop"
    return
  fi
  echo "building goop from src/..." >&2
  (cd "$ROOT/src" && go build -o "$WORKDIR/goop" ./cmd/goop)
  echo "$WORKDIR/goop"
}

GOOP="$(resolve_goop)"
echo "using goop: $GOOP ($("$GOOP" version 2>/dev/null || echo unknown))"
echo

# name|goop_src|gen_pkg|gen_bench_dir|hand_dir
BENCHES=(
  "list_fold|$BENCH_ROOT/list_fold/list_fold.goop|listfold|$BENCH_ROOT/list_fold/gen|$BENCH_ROOT/list_fold/hand"
  "adt_match|$BENCH_ROOT/adt_match/adt_match.goop|adtmatch|$BENCH_ROOT/adt_match/gen|$BENCH_ROOT/adt_match/hand"
  "branded_id|$BENCH_ROOT/branded_id/branded_id.goop|brandedid|$BENCH_ROOT/branded_id/gen|$BENCH_ROOT/branded_id/hand"
)

run_hand() {
  local name="$1" hand_dir="$2"
  local dir="$WORKDIR/hand-$name"
  mkdir -p "$dir"
  cp "$hand_dir"/*.go "$dir/"
  printf 'module goopbench.hand.%s\n\ngo 1.22\n' "$name" >"$dir/go.mod"
  echo "=== HAND: $name ==="
  (cd "$dir" && go test -bench=. -benchmem -count=1)
  echo
}

run_gen() {
  local name="$1" goop_src="$2" pkg="$3" gen_bench="$4"
  local dir="$WORKDIR/gen-$name"
  mkdir -p "$dir"

  echo "compiling $goop_src ..."
  local compile_log out
  compile_log="$dir/compile.log"
  if ! "$GOOP" compile "$goop_src" >"$compile_log" 2>&1; then
    echo "error: goop compile failed for $name:" >&2
    cat "$compile_log" >&2
    echo "hint: build a current compiler: (cd src && go build -o ./goop ./cmd/goop)" >&2
    return 1
  fi
  cat "$compile_log"
  out="$(awk '/^wrote /{print $2; exit}' "$compile_log")"
  if [[ -z "$out" || ! -f "$out" ]]; then
    echo "error: goop compile did not produce a .go file for $name" >&2
    echo "hint: build a current compiler: (cd src && go build -o ./goop ./cmd/goop)" >&2
    return 1
  fi

  cp "$out" "$dir/"
  cp "$gen_bench"/*.go "$dir/"
  printf 'module goopbench.gen.%s\n\ngo 1.22\n' "$name" >"$dir/go.mod"

  echo "=== GENERATED: $name (from $(basename "$out")) ==="
  (cd "$dir" && go test -bench=. -benchmem -count=1)
  echo
}

echo "Provisional microbenchmarks — not a release claim."
echo "Workdir: $WORKDIR"
echo

for entry in "${BENCHES[@]}"; do
  IFS='|' read -r name goop_src pkg gen_bench hand_dir <<<"$entry"
  run_hand "$name" "$hand_dir"
  run_gen "$name" "$goop_src" "$pkg" "$gen_bench"
done

echo "Done. Compare HAND vs GENERATED lines above (ns/op, B/op, allocs/op)."
