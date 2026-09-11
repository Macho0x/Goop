# Goop Support for Zed

Syntax highlighting, file icons, and LSP integration for the [Goop language](https://github.com/Macho0x/Goop).

## Features

- **File icon** — `.goop` files show the Goop logo in the file tree
- **Syntax highlighting** — full TextMate grammar for keywords, types, operators, literals, comments
- **LSP integration** — diagnostics, completions, hover, and go-to-definition via the `goop` compiler
- **Line comments** — `//` support
- **Language config** — proper tab/comment settings for `.goop` files

## Installation

### From source

```bash
cd editors/zed
make install
```

This builds the WASM extension and installs it to `~/.local/share/zed/extensions/installed/goop/`.

### From marketplace

Search for "Goop" in the Zed extensions panel.

After installing, restart Zed or run **zed: reload extensions** from the command palette.

## Prerequisites

Build the compiler (LSP needs it):

```bash
cd src && go build -o ../goop ./cmd/goop
```

## Development

- **Grammar**: edit `grammars/goop.tmLanguage.json` (sync from repo root: `../../scripts/sync-syntax.sh`)
- **Icons**: edit `icons/` SVG files and `icons/theme.json`
- **LSP adapter**: edit `src/lib.rs`
- **Language config**: edit `languages/goop/config.toml`

Run `make install` to rebuild and deploy changes.

## Troubleshooting

| Problem | Fix |
|---|---|
| No highlighting | Run `make install` and reload extensions |
| No LSP diagnostics | Build `goop` at repo root; ensure `goop` is on `PATH` or set in extension config |
| Stale colors after grammar change | Re-run `sync-syntax.sh`, `make install`, reload extensions |

See also [CONTRIBUTING.md](../../CONTRIBUTING.md) and [VS Code extension](../vscode/README.md).
