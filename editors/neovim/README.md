# Goop Support for Neovim

Filetype detection and LSP integration for the [Goop language](https://github.com/Macho0x/Goop).

## Features

- **Filetype** — `*.goop` → `filetype=goop`
- **LSP** — diagnostics, completions, hover, go-to-definition, and format via `goop lsp`
- **Buffer defaults** — `//` comments, 2-space indent

There is no Tree-sitter grammar yet. For TextMate-style highlighting, see the [VS Code grammar](../vscode/syntaxes/goop.tmLanguage.json) (canonical: [`../../syntaxes/goop.tmLanguage.json`](../../syntaxes/goop.tmLanguage.json)).

## Prerequisites

Build the compiler (LSP needs it) and put `goop` on your `PATH`:

```bash
cd src && go build -o ../goop ./cmd/goop
# then copy/symlink ./goop somewhere on PATH, or point cmd at the binary
```

## Quick config (Neovim 0.11+)

Add this runtime path (or clone/symlink `editors/neovim` into your pack path), then enable the server:

```lua
-- ~/.config/nvim/init.lua (or a plugin file)

vim.opt.runtimepath:append("/path/to/Goop/editors/neovim")

vim.lsp.config("goop", {
  cmd = { "goop", "lsp" },
  filetypes = { "goop" },
  root_markers = { "goop.toml", ".git" },
})
vim.lsp.enable("goop")
```

Open a `.goop` file — `:set filetype?` should show `goop`, and LSP diagnostics should appear.

### Absolute path to the binary

If `goop` is not on `PATH`:

```lua
vim.lsp.config("goop", {
  cmd = { "/path/to/goop", "lsp" },
  filetypes = { "goop" },
  root_markers = { "goop.toml", ".git" },
})
vim.lsp.enable("goop")
```

## Alternative: nvim-lspconfig style

If you prefer `nvim-lspconfig` (or Neovim < 0.11), register a custom server:

```lua
vim.filetype.add({ extension = { goop = "goop" } })

local configs = require("lspconfig.configs")
local util = require("lspconfig.util")

if not configs.goop then
  configs.goop = {
    default_config = {
      cmd = { "goop", "lsp" },
      filetypes = { "goop" },
      root_dir = util.root_pattern("goop.toml", ".git"),
      single_file_support = true,
    },
  }
end

require("lspconfig").goop.setup({})
```

With only the filetype pack on `runtimepath`, you can skip `vim.filetype.add` — `ftdetect/goop.lua` already registers `*.goop`.

## lazy.nvim example

```lua
{
  dir = "/path/to/Goop/editors/neovim",
  name = "goop",
  lazy = false,
  config = function()
    vim.lsp.config("goop", {
      cmd = { "goop", "lsp" },
      filetypes = { "goop" },
      root_markers = { "goop.toml", ".git" },
    })
    vim.lsp.enable("goop")
  end,
}
```

## What you should see

| Feature | How to verify |
|---|---|
| **Filetype** | Open any `.goop` file — `:set filetype?` → `goop` |
| **LSP attached** | `:lua =vim.lsp.get_clients({ name = "goop" })` is non-empty |
| **Diagnostics** | Open a file with a type error — signs/virtual text appear |
| **Format** | `:lua vim.lsp.buf.format()` — uses the `goop` LSP formatter |

## Troubleshooting

| Problem | Fix |
|---|---|
| Plain text / no filetype | Ensure `editors/neovim` is on `runtimepath`, or add `vim.filetype.add({ extension = { goop = "goop" } })` |
| No LSP | Build `goop`; confirm `which goop` or set absolute `cmd` |
| Client never attaches | Check `:LspInfo` / `:checkhealth vim.lsp`; filetype must be `goop` |
| Stale binary | Rebuild `goop` and restart Neovim |

See also [CONTRIBUTING.md](../../CONTRIBUTING.md), [VS Code](../vscode/README.md), and [Helix](../helix/README.md).
