# Diagnostic corpus

Lisette advertises **250+** diagnostics. Goop’s catalog lives in
[10-error-reference.md](10-error-reference.md). Counted from `###` headings
there (2026-07-28, release 1.13):

| Prefix | Count | Notes |
|--------|------:|-------|
| PARSE / PARSE-MIG | ~22 | PARSE001+ and MIG* on the wire; MIG001 retired |
| UNIFY | 22 | includes UNIFY020–022 |
| TYPE | 13 | TYPE002–013 on the wire (TYPE001 obsolete) |
| CLI | 13 | |
| LEX | 10 | |
| LINEAR | 8 | |
| VIS | 2 | |
| IMPORT | 4 | IMPORT001–004 |
| EXHAUST | 3 | |
| REFINE | 3 | |
| CODEGEN | 3 | |
| GOSIG | 3 | GOSIG001–003 (`verify_ffi`) |
| UNUSED | 2 | |
| RESULT / OPTION | 2 | |
| ROW | 1 | ROW001 open-row literal |
| DECIMAL | 1 | DECIMAL001 (`money_float`) |
| DEADLOCK | 1 | |
| NIL | 1 | |
| FFI-IMPL | 1 | |
| **Total** | **~120+** | unique codes |

CLI diagnostics also print a short `help:` line via `src/internal/report`.

**LSP:** type/parse errors always; safety **errors** always; safety **warnings**
(RESULT/OPTION/UNUSED/VIS/DECIMAL/EXHAUST redundant/REFINE/DEADLOCK/…) as
`Warning` by default, or elevated to `Error` when the matching `[check]` key
is `"error"`.

## `goop lint`

`goop check` runs the same parse → typecheck → safety pipeline on a **single
file**. `goop lint <file-or-dir>` packages that pipeline for CI:

- Accepts a file or directory (recurses for `*.goop`; `check` does not)
- Prints each diagnostic, then `N error(s), M warning(s)`
- Exits non-zero on errors; `goop.toml` `[check]` severities elevate warnings

## `[check]` keys (1.13)

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
| `money_float` | `warn` | DECIMAL001 |
| `verify_ffi` | `false` | GOSIG003 |
| `smt` | `false` | optional Z3 |
| `effect_inference` | `true` | effect row inference |
