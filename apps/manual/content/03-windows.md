# Windows

Every floating frame is a **window** managed by the desktop.

## Chrome

- **Title bar** — drag to move; double-click to maximize / restore
- **▬** — minimize to the taskbar
- **□** — maximize / restore
- **X** — close
- **Borders & corners** — drag any edge or corner to resize (focused windows show yellow corner grips)
- **Shadow** — decorative depth behind windows (theme)

## Focus & stacking

- Click a window (or its taskbar chip) to focus it
- Focused windows get a stronger title/border color
- **Alt+Tab** cycles focus

## Snap / tile

**Alt+Ctrl+Arrow** snaps the focused window to half of the desktop field (left / right / top / bottom). Press the **same** side again to restore the previous size.

## Move & resize from the keyboard

| Keys | Action |
|------|--------|
| Alt+Arrows | Nudge / move |
| Alt+Shift+Arrows | Resize |
| Alt+Ctrl+Arrows | Snap half-screen |
| Ctrl+M | Minimize |
| Ctrl+W | Close |

## Session restore

If **Restore last session** is on (Settings → Desktop), open windows and geometry are saved periodically and on quit to `~/.config/ttypedesk/session.json`, then restored next launch.

By default Desk does **not** force-open a terminal on start; turn that on under Desktop if you prefer the old behavior.
