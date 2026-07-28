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

```bash
(cd src && go build -o ../goop ./cmd/goop)
./benchmarks/run.sh
```

## Interpreting results

- **Provisional.** Single machine, `count=1`, noisy laptop CPUs. Re-run with
  `-count=5` / `benchstat` before citing numbers.
- **Hand `adt_match`** uses a tagged struct + `switch`. Goop multi-ctor ADTs
  still lower to **interface + type switch** — expect a gap.
- **Hand `branded_id`** uses `type OrderID string`. Goop **zero-cost brands
  (H4c)** lower single-ctor primitive/string ADTs the same way — roundtrip
  should match hand Go closely.
- **`list_fold` map** allocates heavily on both sides; fold is allocation-free.

## Indicative numbers (2026-07-28)

Machine: Linux amd64, 11th Gen Intel i7-1165G7 @ 2.80GHz.  
Compiler: Goop 1.12.0 (built from `src/`). One `./benchmarks/run.sh` pass.

| Bench | Hand ns/op | Generated ns/op | Notes |
|-------|------------|-----------------|-------|
| list_fold FoldAdd | ~1357 | ~2579 | 0 allocs both |
| list_fold MapInc | ~74k | ~93k | heavy allocs both |
| adt_match Area | ~3.8 | ~6.8 | multi-ctor interface lowering |
| branded_id Roundtrip | ~0.36 | ~0.36 | zero-cost brands ≈ hand Go |

Treat these as **smoke / scaffolding**, not a ranking. Re-run locally before
any public claim.
