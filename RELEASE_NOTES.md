# Goop 1.10.1

Docs patch on top of 1.10.0: teaching materials and design status catch up to
the shipped maps / zero-cost brands / bare-import `.gosig` surface. No language
or compiler changes.

## Highlights

- **Tutorial:** new [maps chapter](docs/tutorial/08-maps.md); `goop doc`, bare
  `import go` stub resolution, and zero-cost brands covered in earlier chapters.
- **References:** syntax, STYLE, type system, prelude, and stdlib docs include
  `map[K] V` / `Map.*`.
- **Design status:** H4c / H5 / `goop doc` / freeze checklist aligned with 1.10.

## Workflow

```bash
cd src && go build -o ../goop ./cmd/goop
../goop version
../goop check docs/examples/maps.goop
```

See `CHANGELOG.md` and `docs/tutorial/README.md`.
