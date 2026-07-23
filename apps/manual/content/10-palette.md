# Command palette

**Ctrl+Space** or **Ctrl+P** opens a type-to-run overlay (remappable in Settings → Input).

## Examples

| Type | Result |
|------|--------|
| `open notes` / `settings` | Launch or focus apps |
| `find readme` | Scrollback find + fuzzy files under Home/Documents/Downloads |
| `calculate 0xff * 16` or `= 2**10` | Integer calc (hex, `**`, `& \| ^ << >>`); Enter copies |
| `run htop` | New PTY running the command |
| `ssh myhost` | `pty:ssh myhost` |
| `wifi connect` | Recipe → `nmtui` (edit `recipes` in config) |

Empty query shows recent searches and a short app catalog. ↑↓ select, Enter run, Esc close.

## Config

```json
"hotkeys": { "palette": "ctrl+space", "palette_alt": "ctrl+p" },
"palette": { "max_results": 12 },
"recipes": [
  { "match": "wifi connect", "action": "pty:nmtui" }
]
```

Design notes: [docs/command-palette.md](../docs/command-palette.md)
