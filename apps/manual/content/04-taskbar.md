# Taskbar & Start menu

## Taskbar

The taskbar can dock on **top, bottom, left, or right** (Settings → Appearance → Taskbar position).

Contents (order depends on dock side):

- **Start** — opens the Start menu
- **Window chips** — one per open window; click to focus or restore; BEL attention may flash yellow until focused
- **🔔** — Notification Center (badge when unread)
- **Clock** — click opens Calendar

Vertical docks are thicker so labels like ` Start ` stay readable.

## Start menu

Open with the Start button, **F10**, **Ctrl+Esc**, or **Alt+Space** if your host terminal passes it through (Guake often steals Alt+Space). Remap under Settings → Input.

| Folder | Contents |
|--------|----------|
| **Programs ▶** | Notes, Calendar, Clock, Image Viewer, plus apps you added with menu=`programs` |
| **System ▶** | Terminal, Files, Settings, **Manual**, **System folder**, Add/Manage Programs, plus menu=`system` apps |
| **Quit** | Exit TTYPE Desk |

Mouse: hover opens flyouts; click launches. Keyboard: ↑↓, →/Enter into submenu, ←/Esc back.

## Add Program / Manage Programs

**Add Program…** registers a custom launcher: display name, command (e.g. `htop`), icon glyph, Start menu folder (Programs or System), optional desktop shortcut.

**Manage Programs…** lists and deletes those entries (and their desktop shortcuts).
