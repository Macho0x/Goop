# Goop benchmarks: generated Go vs hand-written Go

Minimal harness for **M10**. Scaffolding + runnable microbenchmarks — **not** a
release performance claim.

## Layout

```
benchmarks/
  README.md                 # this file
  run.sh                    # compile Goop → temp dirs, run go test -bench
  list_fold/
    list_fold.goop          # Goop source
    hand/                   # hand-written baseline + benches
    gen/bench_test.go       # benches against generated API (copied by run.sh)
  adt_match/
    adt_match.goop
    hand/
    gen/bench_test.go
  branded_id/
    branded_id.goop
    hand/
    gen/bench_test.go
```

| Microbench | What it measures |
|------------|------------------|
| `list_fold` | Recursive list fold / map (slice + len patterns) |
| `adt_match` | Sum-type match → area over a small shape mix |
| `branded_id` | Single-ctor brand wrap / unwrap / roundtrip |

## How to run

### Automated (`run.sh`)

Needs a **current** `goop` (with `goop version` and cache `compile`). The
repo-root `./goop` binary may be stale; the script builds from `src/` when
needed, or set `GOOP_BIN`:

```bash
# optional: point at a fresh binary
(cd src && go build -o /tmp/goop ./cmd/goop)
export GOOP_BIN=/tmp/goop

./benchmarks/run.sh
```

For each microbench the script:

1. Runs `go test -bench` on `hand/`
2. Runs `goop compile` on the `.goop` file (output under `$GOOP_HOME/build`)
3. Copies generated `.go` + `gen/bench_test.go` into a temp module
4. Runs `go test -bench` on the generated package

### Manual

```bash
# Hand baseline
cd benchmarks/list_fold/hand
go mod init goopbench.hand.list_fold   # once
go test -bench=. -benchmem

# Generated
goop compile ../list_fold.goop          # prints wrote $GOOP_HOME/build/compile-*/listfold.go
# copy that .go beside gen/bench_test.go in a temp dir with go.mod, then:
go test -bench=. -benchmem
```

## Interpreting results

- **Provisional.** Single machine, `count=1`, noisy laptop CPUs. Re-run with
  `-count=5` / `benchstat` before citing numbers.
- **Hand `adt_match`** uses a tagged struct + `switch` (idiomatic Go). Goop
  lowers ADTs to **interface + type switch** — expect a gap until codegen
  improvements land.
- **Hand `branded_id`** uses `type OrderID string`. Goop still lowers
  single-ctor ADTs to interface + wrapper struct (zero-cost path planned;
  see [`docs/design/21-branded-ids.md`](../docs/design/21-branded-ids.md)).
- **`list_fold` map** allocates heavily on both sides (recursive `append`);
  fold is allocation-free.

## Provisional numbers (2026-07-27)

Machine: Linux amd64, 11th Gen Intel i7-1165G7 @ 2.80GHz.  
Compiler: Goop 1.8.0 (built from `src/`). One `run.sh` pass (`-count=1`).

| Bench | Hand ns/op | Generated ns/op | Notes |
|-------|------------|-----------------|-------|
| list_fold FoldAdd (n=256) | ~3300 | ~6000 | 0 allocs both |
| list_fold MapInc (n=256) | ~770k | ~620k | heavy allocs both |
| adt_match Area (5 shapes) | ~14 | ~33 | hand = tagged struct |
| branded_id Roundtrip | ~0.75 | ~2.9 | hand = defined string |

Treat these as **smoke / scaffolding**, not a ranking. Re-run locally before
any public claim.
