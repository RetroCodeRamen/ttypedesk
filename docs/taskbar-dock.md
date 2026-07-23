# Taskbar docking (design)

Windows-style taskbar position: **top | bottom | left | right**.

Default remains **top** (current DOS layout).

## Layout impact

| Dock | Desktop region | Start / clock / window buttons |
|------|----------------|--------------------------------|
| top | rows `1..H-1` | row `0`, Start left, clock right |
| bottom | rows `0..H-2` | row `H-1` |
| left | cols `1..W-1` | col `0`, vertical Start/icons/clock |
| right | cols `0..W-2` | col `W-1`, vertical |

Window maximize / create geometry must inset by the dock thickness (1 cell v1; optional wider later).

Desktop icons coordinates are relative to the **desktop field**, not the full screen (so docking doesn’t orphan icons).

## Config

```json
{ "taskbar": { "dock": "top", "thickness": 1 } }
```

Settings → Appearance → Taskbar position.

## Hit-testing

All mouse handlers that assume `y == 0` for the bar become `taskbar.Contains(x,y)` and `content.Origin()`.

## Phasing

1. [x] Config + top/bottom (horizontal)
2. [x] Left/right vertical taskbar (7-cell wide; full ` Start ` label, titles, tray)
3. [x] Settings UI + live apply
4. [x] Desktop icons stay in the desktop field when docking / dragging

Vertical thickness is **7** cells so ` Start ` is readable (matches the horizontal Start chip width). Window titles truncate to 7 chars; clock shows `HH:MM`. Horizontal remains 1 row.
