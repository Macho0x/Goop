# 31 — Language-update checklist

**Use this on every language / compiler / diagnostics change** before merge or
release. It is the recurring companion to the one-time
[freeze checklist](30-freeze-checklist.md).

Docs must match the compiler, not aspirational design. Prefer OCaml forms from
[STYLE.md](STYLE.md).

## 1. Compiler & tests (hard)

- [ ] `cd src && go test ./...`
- [ ] `go build -o ../goop ./cmd/goop`
- [ ] `../goop test ../tests/`
- [ ] New or changed behavior covered by `tests/*_test.goop` and/or unit tests
- [ ] `goop lint` / safety pipeline still green for touched packages
  (`checkpipeline`, discardedresult, unused, exhaust, linear, …)

## 2. Examples & scaffold

- [ ] `for f in docs/examples/*.goop; do goop check "$f"; done` — clean
  (fix warn-as-noise; prefer `_` / real fixes over turning checks `off`)
- [ ] `goop new /tmp/goop-new-smoke --force && goop check …/main.goop`
- [ ] New features have a teachable `docs/examples/*.goop` when user-visible
- [ ] Playground still builds if WASM / safety wiring changed
  (`playground/build.sh` or CI Pages workflow)

## 3. Config & CLI surface

- [ ] New `[check]` knobs in `src/internal/config/config.go` defaults
- [ ] Root [`goop.toml`](../../goop.toml) and `goop new` template updated
- [ ] Wired through checkpipeline, `lint`, safety/LSP, playground WASM as needed
- [ ] `goop version` / `VERSION` + `src/internal/version/VERSION` only when releasing

## 4. Diagnostics

- [ ] Stable **on-the-wire** code prefixes where applicable (`CODE: message`)
- [ ] [`10-error-reference.md`](10-error-reference.md) entry (wire vs catalog note)
- [ ] [`26-diagnostics.md`](26-diagnostics.md) recount / `[check]` table if codes or knobs changed
- [ ] `help:` tip in [`report.go`](../../src/internal/report/report.go) for new codes
- [ ] Tutorial safety chapter note if users will see the diagnostic

## 5. Documentation sync

| Area | Update when… |
|------|----------------|
| [STYLE.md](STYLE.md) | Preferred spelling / removed sugar changes |
| [03-syntax.md](03-syntax.md) / [grammar.md](../spec/grammar.md) | Parser surface changes |
| [Tutorial](../tutorial/README.md) | User-facing language features |
| [stdlib](../stdlib/README.md) | Prelude / `std.*` / builtins |
| [prelude.go](../../src/internal/prelude/prelude.go) ↔ [prelude.md](../stdlib/prelude.md) | New FFI/prelude bindings |
| [Writing tools](../guides/writing-tools.md) | Interop / maps / files patterns |
| [README.md](../../README.md) Status | Every **release** (version + one-line highlight) |
| [CHANGELOG](../../CHANGELOG.md) / [RELEASE_NOTES](../../RELEASE_NOTES.md) | Every **release** |
| [07-roadmap.md](07-roadmap.md) / [TODO.md](../../TODO.md) | Milestone or deferred work moves |

Also: `[check] verify_ffi` / GOSIG003 when touching hand `import go { val … }`;
**GOSIG004** (always warn) when a hand val names a generic Go export;
`money_float` / DECIMAL001 for trading examples.

## 6. Editors & highlighting

- [ ] TextMate grammar if new keywords/syntax: `syntaxes/goop.tmLanguage.json`
- [ ] `./scripts/sync-syntax.sh` after grammar edits
- [ ] VS Code `package.json` version only on release (with lockfile)

## 7. Interop / sigs (when FFI touched)

- [ ] `goop gen-sig --smoke` and curated `goop-sigs/` overrides still load
- [ ] Example checks that use `import go` (`writing_tools.goop`, `maps.goop`, …)

## 8. Release-only extras

- [ ] Annotated tag `vX.Y.Z`; push branch + tag (no force-push to main)
- [ ] README Status + playground link still accurate
- [ ] Skip junk: `src/goop-out`, playground `*.wasm` build artifacts unless intentional

## Quick copy-paste gate

```bash
cd src && go build -o ../goop ./cmd/goop && go test ./...
../goop test ../tests/
for f in ../docs/examples/*.goop; do ../goop check "$f"; done
../goop new /tmp/goop-new-smoke --force && ../goop check /tmp/goop-new-smoke/main.goop
```
