# Goop 1.11.0

UX train for Lisette-parity tryability: hosted playground, one-line install,
`goop new`, clearer diagnostics, quieter channel stubs, and VS Code VSIX on
releases.

## Highlights

- **Playground:** GitHub Pages deploy (`https://macho0x.github.io/Goop/`) — enable
  Pages → GitHub Actions once in repo settings.
- **Install:** `curl …/scripts/install.sh | bash`
- **`goop new`:** scaffold a project in seconds
- **RESULT001** + `help:` tips on common diagnostics
- **Directional chans** map cleanly (`time.After`-style stubs)
- **VSIX** attached to GitHub Releases

## Workflow

```bash
curl -fsSL https://raw.githubusercontent.com/Macho0x/Goop/main/scripts/install.sh | bash
goop new hello && cd hello
goop check main.goop && goop build main.goop
```

See `CHANGELOG.md`, `docs/tutorial/README.md`, and the [playground](https://macho0x.github.io/Goop/).
