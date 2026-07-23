# Files & default apps

**Files** is a native folder manager: list or grid, toolbar, scrollbar, mkdir/rename, clipboard, and trash.

## Opening Files

- Start → System → **Files**
- Desktop **Files** / **Home** / **Trash** icons
- Start → System → **System folder** (`files:system`) — Manual markdown on disk
- Any `files:/absolute/or/relative/path` action

## Opening files

Double-click / Enter on a file uses **associations**:

| Kind | Default |
|------|---------|
| Text / code / markdown / config | Editor role → usually `pty:nano` |
| Images (png, jpg, …) | Image Viewer |
| Unknown extension | Notification: no default app for that type |

Change defaults in **Settings → Default apps** (cycle editor, set custom command, reset associations).

## Trash

Deletes prefer the XDG trash (`~/.local/share/Trash/…`) when available; otherwise Desk may use a config-local trash. The desktop **Trash** shortcut opens the trash files directory in Files.

## Tips

- Files can open many instances (each folder path is fine)
- Settings and Calendar prefer a **single** window (focus existing)
- Last-visited directory can be remembered (`files.start_dir` / LastDir in config)
- Select a **PNG / JPEG / GIF** and press **W** (or toolbar **Wall**) to set it as the desktop wallpaper — Bliss stays the default until you pick one
- Press **D** (or toolbar **Desk**) to add a **desktop shortcut** for the selected file or folder
