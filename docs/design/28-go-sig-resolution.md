# Go signature resolution (`.gosig`)

How Goop finds and generates Go FFI stubs for `import go "…"`.

Companion to H5 (curated generator in `src/internal/gosiggen`) and M7
(`goop get-go-sig`). Generated artifacts are **cache-only**; hand-curated
overrides live in the project tree.

## Locations

| Path | Role |
|------|------|
| `<project>/goop-sigs/<safe-name>.gosig` | Hand-curated **override** (wins) |
| `$GOOP_HOME/build/go-sigs/<safe-name>.gosig` | Generated **cache** (default write target) |

`$GOOP_HOME` defaults to `~/.cache/goop` (same as compile/build; see
[20-cli-artifacts.md](20-cli-artifacts.md)).

`<safe-name>` is the import path with `/` and `.` replaced by `_`, plus
`.gosig` — e.g. `encoding/json` → `encoding_json.gosig`,
`github.com/shopspring/decimal` → `github_com_shopspring_decimal.gosig`.

## Resolution order (compile / typecheck)

When the compiler sees a **bare** `import go "P"` (no `{ val … }` block):

1. **Project override** — if `<projectRoot>/goop-sigs/<safe(P)>.gosig`
   exists as a regular file, use it. Always wins over cache.
2. **Generated cache** — if `$GOOP_HOME/build/go-sigs/<safe(P)>.gosig`
   exists, use it.
3. **Miss** — for curated packages, auto-generate into the cache once;
   otherwise leave unbound (callers can add an explicit `{ val … }` block
   and/or use the runtime `gosig` fallback for declared names).

Explicit `import go "P" { … }` blocks are authoritative: stubs are **not**
merged on top of them (avoids collisions with hand-written FFI).

API: `gosiggen.ResolveSigPath` / `gosiggen.LoadImportBindings`.

## CLI: `goop get-go-sig`

```bash
goop get-go-sig encoding/json
goop get-go-sig github.com/shopspring/decimal
```

1. Loads the Go package via `go/packages` (from the nearest `go.mod` dir).
2. Emits a `.gosig` with exported types, funcs, methods, consts/vars that
   map to representable Goop types.
3. Writes under `$GOOP_HOME/build/go-sigs/` (never into the project tree).
4. Prints **warnings** for every skipped / unrepresentable export and hints
   at `goop-sigs/` for overrides.
5. Notes when `P` is outside the curated H5 set (quality may vary).

Does **not** overwrite an existing `goop-sigs/` override; it only refreshes
the cache. If an override is present, the CLI notes that the override still
wins at compile time.

H5 also ships `goop gen-sig` for the curated/smoke batch (`--curated`,
`--smoke`, `--out`). Prefer `get-go-sig` for arbitrary third-party paths;
`gen-sig` for regenerating the H5 set.

## Unrepresentable types

Exports that cannot be mapped are **skipped**, not silently degraded:

| Go shape | Behavior |
|----------|----------|
| `map[K]V` | Emitted as `map[K] V` |
| `chan T` / `<-chan T` / `chan<- T` | Emitted as `T chan` |
| Anonymous `struct` / non-empty anonymous `interface` | Skipped |
| Multi-result other than `(T, error)` | Emitted as Goop product tuples when elems map |
| `complex*`, `unsafe.Pointer` | Skipped |
| `(T, error)` | Generator may note `TODO(H6)`; **typecheck+codegen coerce** call sites to `result` unless `import go raw` |
| `any` / `interface{}` | Mapped to `obj` (typecheck treats as `any`) |

Skipped exports appear in the generated file comment **and** as
`goop: warning: unrepresentable …` on stderr from `get-go-sig`.

## Curated vs arbitrary

- **H5 curated set** (`gosiggen.CuratedPackages`): stdlib packages with
  expected mapping quality for 1.0.
- **M7 arbitrary paths**: same generator API (`gosiggen.Generate` /
  `GenerateAndWrite`); no curated gate. Quality warnings are louder.

## Generate on first bare `import go` — shipped

When a Goop module uses bare `import go "P"` with no override and no cache hit,
the compiler calls the same generator for curated packages, writes the cache
entry, emits unrepresentable-type warnings, and continues. Explicit
`goop get-go-sig P` remains the offline / CI prefetch path.

## Override workflow

```bash
# 1. Generate into cache
goop get-go-sig net/http

# 2. Copy into the project and edit
mkdir -p goop-sigs
cp "$GOOP_HOME/build/go-sigs/net_http.gosig" goop-sigs/
# edit goop-sigs/net_http.gosig

# 3. Compile always prefers goop-sigs/net_http.gosig
```

Commit `goop-sigs/` when the hand stub is part of the project's FFI
contract. Do not commit `$GOOP_HOME` cache files.
