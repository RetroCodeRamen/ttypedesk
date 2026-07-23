# Files manager

Native file manager (`apps/files`) — browse, open via desk associations, manage files.

## Views

- **List** — icon + name, vertical scrollbar (SDK `ScrollState` / `DrawScrollbar`)
- **Grid** — multi-column icon tiles; same scrollbar (scrolls by grid rows)

Toggle: toolbar **View**, or Files → Set.

## Open files

Uses desk-wide `OpenPath` / associations (see [associations.md](associations.md)):

- Text → default editor (`roles.editor`, default `pty:nano`)
- png/jpg/jpeg/… → Image Viewer
- Unknown → notification, no silent fallback

Directories always navigate inside Files.

## Ops

| Action | How |
|--------|-----|
| Up / Home | Toolbar or Backspace / Home button |
| Open | Enter / double-click |
| New folder | F7 / New |
| Rename | F2 |
| Copy / Cut / Paste | Ctrl+C / X / V |
| Trash | Delete (XDG `~/.local/share/Trash/files` when possible) |
| Path jump | Ctrl+L |
| Refresh | F5 |
| Settings | toolbar Set |
| Set wallpaper | **W** or toolbar **Wall** on a PNG/JPEG/GIF |
| Send to desktop | **D** or toolbar **Desk** — adds a desktop icon shortcut |

## Config (`files` section)

```json
"files": {
  "view": "list",
  "show_hidden": false,
  "sort": "name",
  "start_dir": "home",
  "confirm_delete": true,
  "last_dir": ""
}
```
