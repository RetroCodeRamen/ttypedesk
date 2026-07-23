# TTYPE Desk

DOS-era floating window manager for the Linux terminal — **TTYPE Desk**.

Multiplexes real TUI programs (bash, vim, htop) via **libvterm**, hosts native apps through an in-process **App SDK**, and can render images as truecolor half-block “graphics.”

## Requirements

- Go 1.22+
- gcc / ar (to build vendored libvterm)
- A truecolor-capable host terminal (`COLORTERM=truecolor` recommended)

libvterm **0.3.3** is vendored under `third_party/libvterm-0.3.3` (no system package required).

## Build

```bash
./build.sh
# or:
#   (cd third_party/libvterm-0.3.3/build && gcc -std=c99 -O2 -I../include -I../src -c ../src/*.c && ar rcs libvterm.a *.o)
#   go build -o bin/ttypedesk ./cmd/ttypedesk
```

## Run

```bash
./bin/ttypedesk
```

### Controls

| Input | Action |
|-------|--------|
| Mouse | Focus, drag title bar, resize edges/corners, minimize/maximize/close |
| Desktop icons | Click to launch; drag to reposition (saved to config) |
| Taskbar buttons | Focus or restore minimized windows |
| Drag in content | Text select (or Shift+drag if app has mouse mode) |
| Middle-click / Ctrl+V | Paste clipboard |
| Ctrl+Shift+C | Copy selection (also auto-copies on mouse-up) |
| Wheel | Scrollback |
| Shift+PgUp / Shift+PgDn | Scrollback |
| Ctrl+Shift+F | Find in scrollback (often stolen by Guake / host terminals) |
| **F3** / **Alt+/** | Find in scrollback (remappable: Settings → Input) |
| **Ctrl+Space** / **Ctrl+P** | Command palette |
| Alt+Arrows | Move window |
| Alt+Shift+Arrows | Resize window |
| Alt+Ctrl+Arrows | Snap to half desktop (repeat same side to restore) |
| Alt+Tab | Cycle focus |
| **F10** / **Ctrl+Esc** | Start menu (preferred; Alt+Space often stolen by host) |
| Alt+Space / Start button | Start menu (if host allows) |
| Ctrl+W | Close focused window |
| Ctrl+M | Minimize focused window |
| Ctrl+Q | Quit |

### Start menu

- **Programs ▶** — Notes, Calendar, Clock, Image Viewer, plus user apps with menu=`programs`
- **System ▶** — Terminal, Files, Settings, **Manual**, **System folder**, **Add Program…**, **Manage Programs…**, plus user apps with menu=`system`
- **Quit**

**Add Program…** — set **Desk name** (e.g. Task Manager), **Command** (e.g. `htop`), emoji, **Start menu** folder (Programs or System), optional desktop shortcut.

**Manage Programs…** — list custom apps; Delete/Enter to remove (also drops their desktop shortcut).

Mouse: hover opens flyouts; click to launch. Keyboard: ↑↓, →/Enter into submenu, ←/Esc back.

Taskbar tray: **🔔** opens Notification Center; clock opens Calendar.

### Troubleshooting

Log file (panics, deadlocks, app crashes): `~/.config/ttypedesk/ttypedesk.log`  
Override with `TTYPEDESK_LOG=/path/to/file` or `TTYPEDESK_LOG_LEVEL=debug`.

Native app panics are isolated to that window (red crash screen; Esc/Q closes it). The main event/draw loop also recovers and keeps the desktop running when possible.

```bash
./bin/ttypedesk                     # default shell terminal
./bin/ttypedesk -e htop             # open htop in a window
./bin/ttypedesk -image photo.png    # open image viewer
./bin/ttypedesk -clock -e vim
./bin/ttypedesk -listen /tmp/ttypedesk.sock
./bin/ttypedesk -attach /tmp/ttypedesk.sock
```

## Config

`~/.config/ttypedesk/config.json` — FPS, shell, theme, taskbar dock, desktop icons, wallpaper, notify, **file associations** / default apps, and Files options. See [docs/associations.md](docs/associations.md) and [docs/files.md](docs/files.md).

Defaults: **no terminal on start**; **restore last session** from `~/.config/ttypedesk/session.json` (toggle in Settings → Desktop). Set `open_terminal_on_start` if you want the old behavior.

Default look is the **XP** theme (Windows XP–inspired chrome) with the **Bliss** wallpaper (`builtin:bliss`). Other packs: **Scarlet**, **Bumble**, **Bubble**, **Sprout** (Settings → Appearance, or palette: `scarlet` / `bumble` / …). Over SSH the desktop stays solid unless you set `wallpaper.ssh_mode` to `keep`.

### Logging

Session log: `~/.config/ttypedesk/ttypedesk.log` (or `$TTYPEDESK_LOG`).  
Level: `$TTYPEDESK_LOG_LEVEL` = `debug` | `info` | `warn` | `error` (default `info`).  
Panics and LaunchAction failures are written here — check this file when something freezes or crashes.## Remote attach (thin)

```bash
# session host
./bin/ttypedesk -listen /tmp/ttypedesk.sock

# another terminal (read-only snapshot viewer)
./bin/ttypedesk -attach /tmp/ttypedesk.sock
```

See [docs/remote.md](docs/remote.md) for how a future RDP/VNC decoder plugs in as a graphical surface.

## Roadmap

Upcoming work is tracked in [ROADMAP.md](ROADMAP.md). SSH streaming notes: [docs/ssh.md](docs/ssh.md).

## Layout

```
cmd/ttypedesk/     entrypoint
internal/
  server/          window manager
  client/          tcell DOS desktop
  vterm/           libvterm cgo wrapper
  tty/             PTY spawn
  surface/         pty / app / gfx surfaces
  gfx/             RGBA → half-block cells
  proto/           cell-diff messages
  attach/          Unix socket attach
  config/
pkg/
  cell/            RGB cell types
  uiapp/           App SDK
apps/clock         reference app
apps/imageview     graphical demo
```
