# Diagnostic corpus

Lisette advertises **250+** diagnostics. Goop’s catalog lives in
[10-error-reference.md](10-error-reference.md). Counted from `###` headings
there (2026-07-27):

| Prefix | Count | Notes |
|--------|------:|-------|
| PARSE | 23 | PARSE001–023 (019/020 obsolete in 1.0) |
| UNIFY | 19 | |
| TYPE | 13 | |
| CLI | 13 | |
| PARSE-MIG | 11 | removed-syntax migration errors |
| LEX | 10 | includes LEX005b |
| LINEAR | 8 | includes channel-race LINEAR008 |
| EXHAUST | 3 | severity via `goop.toml` `[check]` |
| REFINE | 3 | REFINE003 is silent when proven |
| DEADLOCK | 1 | |
| RESULT | 1 | RESULT001 discarded result |
| NIL | 1 | |
| FFI-IMPL | 1 | |
| **Total** | **~107** | unique codes |

Roughly **107** documented codes — about 40% of Lisette’s advertised count.
Gaps are mostly finer-grained type/parse variants, not missing safety passes.

CLI diagnostics also print a short `help:` line for common codes (EXHAUST003,
NIL001, RESULT001, PARSE-MIG*, …) via `src/internal/report`.

## `goop lint`

`goop check` runs the same parse → typecheck → safety pipeline on a **single
file**. `goop lint <file-or-dir>` packages that pipeline for CI:

- Accepts a file or directory (recurses for `*.goop`; `check` does not)
- Prints each diagnostic, then `N error(s), M warning(s)`
- Exits non-zero on errors; `goop.toml` `[check]` severities (e.g.
  `exhaust_redundant = "error"`, `discarded_result = "error"`) elevate warnings to errors

```bash
goop lint path/to/file.goop
goop lint path/to/dir
goop lint          # defaults to "."
```
