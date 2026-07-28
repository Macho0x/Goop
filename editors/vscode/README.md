# Install the Goop VS Code / Cursor extension

Syntax highlighting, LSP diagnostics, and optional `.goop` file icons.

## From a GitHub Release (recommended)

Each `v*` release attaches `goop-X.Y.Z.vsix`. Download it from
[Releases](https://github.com/Macho0x/Goop/releases), then:

```bash
code --install-extension goop-X.Y.Z.vsix
# or: cursor --install-extension goop-X.Y.Z.vsix
```

If repository secrets `VSCE_PAT` / `OVSX_TOKEN` are configured, the release
workflow also publishes to the VS Marketplace / Open VSX.

## Quick install from source

From the repo root:

```bash
./scripts/install-editor-extension.sh
```

Then **reload the window**: `Ctrl+Shift+P` → **Developer: Reload Window**

> Workspace recommendations do **not** auto-install local extensions.
> You must run the install script once (or install manually below).

## Prerequisites

Install the compiler (LSP needs `goop` on `PATH` or `goop.path` setting):

```bash
curl -fsSL https://raw.githubusercontent.com/Macho0x/Goop/main/scripts/install.sh | bash
# or: cd src && go build -o ../goop ./cmd/goop
```

## Manual install

```bash
cd editors/vscode
npm install
npx @vscode/vsce package --out goop.vsix
cursor --install-extension goop.vsix    # or: code --install-extension goop.vsix
```

Reload the window after installing.

## What you should see

| Feature | How to verify |
|---|---|
| **Syntax highlighting** | Open any `.goop` file — keywords, `match`, `@[go]`/`@[c]` amber tags, strings colored |
| **Language mode** | Bottom-right status bar shows **Goop** |
| **LSP diagnostics** | Open a file with a type error — squiggles appear |
| **Hover / go-to-definition / completion** | Hover a binding; F12 on a top-level `let`; Ctrl+Space for completions |
| **Format Document** | `Shift+Alt+F` / Format Document — uses `goop` LSP (`goop fmt` engine) |
| **File icon** | Optional — **Preferences: File Icon Theme** → **Goop File Icons** (uses transparent-corner icon) |

## File icons (optional)

VS Code/Cursor do not show custom file icons from the language definition alone.
Enable the bundled icon theme:

1. `Ctrl+Shift+P` → **Preferences: File Icon Theme**
2. Choose **Goop File Icons**

Or add to your user/workspace settings:

```json
"workbench.iconTheme": "goop-file-icons"
```

> This icon theme only defines `.goop`; other files use VS Code defaults.

## Settings

| Setting | Default | Description |
|---|---|---|
| `goop.path` | *(auto)* | Path to `goop` binary. Empty = try `goop` in workspace root, then `PATH`. |

This workspace sets `"goop.path": "${workspaceFolder}/goop"` in `.vscode/settings.json`.

## Troubleshooting

| Problem | Fix |
|---|---|
| No highlighting at all | Extension not installed — run `./scripts/install-editor-extension.sh` and reload |
| Plain text / wrong language | Click language mode (bottom-right) → select **Goop** |
| No LSP errors | Build `./goop`; check **Output** panel → **Goop Language Server** |
| No file icon | Enable **Goop File Icons** theme (optional, see above) |
| Stale colors after grammar change | Re-run install script, reload window |
| `Cannot read properties of null (reading 'length')` | Rebuild `./goop`, reinstall extension 0.3.5+ (`./scripts/install-editor-extension.sh`), reload window — fixed LSP notification handling |
| `@[go]` / `@[c]` still pink (theme keyword color) | Reinstall extension **0.3.7+** (`./scripts/install-editor-extension.sh`) and reload — token colors must be top-level, not under `[goop]` |

## Grammar source

Canonical grammar: [`../../syntaxes/goop.tmLanguage.json`](../../syntaxes/goop.tmLanguage.json)

After editing, run `../../scripts/sync-syntax.sh` and reinstall the extension.
