# Diagnostic corpus

Lisette advertises **250+** diagnostics. Goop’s catalog lives in
[10-error-reference.md](10-error-reference.md). Counted from `###` headings
there (2026-07-28, release 1.12):

| Prefix | Count | Notes |
|--------|------:|-------|
| PARSE / PARSE-MIG | ~22 | MIG001 retired (`open` valid); MIG011 removed (see TYPE011) |
| UNIFY | 22 | includes UNIFY020–022 (map/pointer/go_slice) |
| TYPE | 13 | TYPE011 on the wire |
| CLI | 13 | CLI011 text uses `goop:` (not `c0:`) |
| LEX | 10 | includes LEX005b |
| LINEAR | 8 | LINEAR001–008 prefixed on the wire |
| VIS | 2 | VIS001 error; VIS002 warn (`private_in_public`) |
| IMPORT | 3 | IMPORT001–003 |
| EXHAUST | 3 | severity via `goop.toml` `[check]` |
| REFINE | 3 | REFINE003 is silent when proven |
| CODEGEN | 3 | CODEGEN001–003 |
| GOSIG | 2 | GOSIG001/002 |
| UNUSED | 2 | UNUSED001/002 (`unused`) |
| RESULT / OPTION | 2 | RESULT001 / OPTION001 |
| DEADLOCK | 1 | |
| NIL | 1 | |
| FFI-IMPL | 1 | |
| **Total** | **~110+** | unique codes (wire + catalog) |

Roughly **110+** documented codes — about 40% of Lisette’s advertised count.
Gaps are mostly finer-grained type/parse variants, not missing safety passes.

CLI diagnostics also print a short `help:` line for common codes (EXHAUST003,
NIL001, RESULT001, OPTION001, UNUSED*, VIS*, IMPORT*, CODEGEN*, PARSE-MIG*, …)
via `src/internal/report`.

## `goop lint`

`goop check` runs the same parse → typecheck → safety pipeline on a **single
file**. `goop lint <file-or-dir>` packages that pipeline for CI:

- Accepts a file or directory (recurses for `*.goop`; `check` does not)
- Prints each diagnostic, then `N error(s), M warning(s)`
- Exits non-zero on errors; `goop.toml` `[check]` severities (e.g.
  `exhaust_redundant = "error"`, `discarded_result = "error"`,
  `unused = "error"`) elevate warnings to errors

```bash
goop lint path/to/file.goop
goop lint path/to/dir
goop lint          # defaults to "."
```

## `[check]` keys (1.12)

| Key | Default | Codes |
|-----|---------|-------|
| `exhaust_redundant` | `warn` | EXHAUST001/002 |
| `exhaust_missing` | `error` | EXHAUST003 |
| `concurrent` | `error` | LINEAR006/007/008 |
| `refinement_unproven` | `warn` | REFINE002 |
| `deadlock` | `warn` | DEADLOCK001 |
| `discarded_result` | `warn` | RESULT001 |
| `discarded_option` | `warn` | OPTION001 |
| `unused` | `warn` | UNUSED001/002 |
| `private_in_public` | `warn` | VIS002 |
| `smt` | `false` | optional Z3 |
| `effect_inference` | `true` | effect row inference |
