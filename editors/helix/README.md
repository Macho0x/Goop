# Goop Support for Helix

Language and LSP configuration for the [Goop language](https://github.com/Macho0x/Goop).

## Features

- **File type** — `*.goop` recognized as Goop
- **LSP** — diagnostics, completions, hover, and go-to-definition via `goop lsp`
- **Comments / indent** — `//` and `(* ... *)`, 2-space indent

There is no Tree-sitter grammar yet, so Helix will not syntax-highlight Goop until one exists. LSP features still work. A TextMate grammar lives at [`../../syntaxes/goop.tmLanguage.json`](../../syntaxes/goop.tmLanguage.json) (used by [VS Code](../vscode/README.md) and [Zed](../zed/README.md)).

## Prerequisites

Build the compiler (LSP needs it) and put `goop` on your `PATH`:

```bash
cd src && go build -o ../goop ./cmd/goop
```

## Installation

Merge the contents of [`languages.toml`](./languages.toml) into your Helix config:

```bash
# append (or copy sections by hand)
cat editors/helix/languages.toml >> ~/.config/helix/languages.toml
```

Or paste this into `~/.config/helix/languages.toml`:

```toml
[language-server.goop]
command = "goop"
args = ["lsp"]

[[language]]
name = "goop"
language-id = "goop"
scope = "source.goop"
injection-regex = "goop"
file-types = ["goop"]
roots = ["goop.toml"]
comment-tokens = "//"
block-comment-tokens = { start = "(*", end = "*)" }
indent = { tab-width = 2, unit = "  " }
language-servers = ["goop"]
```

Reload Helix (`:config-reload`) or restart after editing.

### Absolute path to the binary

If `goop` is not on `PATH`:

```toml
[language-server.goop]
command = "/path/to/goop"
args = ["lsp"]
```

## What you should see

| Feature | How to verify |
|---|---|
| **Language** | Open any `.goop` file — status line / `:lang` shows `goop` |
| **LSP diagnostics** | Open a file with a type error — diagnostics appear |
| **Hover / completion** | Space-k hover; completion on identifiers |

## Troubleshooting

| Problem | Fix |
|---|---|
| Treated as plain text | Confirm `languages.toml` was merged; reload config |
| No LSP diagnostics | Build `goop`; ensure `goop` is on `PATH` or set `command` to an absolute path |
| LSP fails to start | Check Helix logs (`hx --health goop` / log file) for `goop lsp` spawn errors |

See also [CONTRIBUTING.md](../../CONTRIBUTING.md), [VS Code](../vscode/README.md), and [Neovim](../neovim/README.md).
