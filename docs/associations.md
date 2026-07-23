# Default apps & file associations

Desk-wide rules for opening files from Files, desktop icons, and `ctx.OpenPath`.

## Defaults

| Kind | Default |
|------|---------|
| Text extensions | `roles.editor` → `pty:nano` |
| png, jpg, jpeg, gif, webp, bmp | `roles.image` → Image Viewer |
| Unmapped extension | Notify: `No default app selected for file type: .<ext>` |

No automatic `xdg-open` / host GUI in v1.

## Config

```json
"roles": {
  "editor": "pty:nano",
  "image": "image",
  "filemgr": "files"
},
"associations": {
  "txt": "role:editor",
  "go": "role:editor",
  "png": "role:image"
}
```

- `role:editor` / `role:image` expand through Roles so one editor change updates all text types.
- Per-ext override: `"md": "pty:nvim"`.
- Extension = last suffix, lowercased (`README.MD` → `md`; `a.tar.gz` → `gz`).

## Settings

**Settings → Default apps**

- Cycle editor: nano / nvim / vim / emacs
- Custom editor command
- Reset associations to shipped defaults

## API

- `config.OpenAction(path) (action, ok)`
- `Server.OpenPath(path)` / `ctx.OpenPath(path)`
- Launch with path bound as argv for `pty:*` (never shell-concatenated)
