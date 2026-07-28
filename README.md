# TTYPE Desk

[![GitHub](https://img.shields.io/badge/GitHub-RetroCodeRamen%2Fttypedesk-181717?logo=github)](https://github.com/RetroCodeRamen/ttypedesk)

<p align="center">
  <img src="images/Desktop.png" alt="TTYPE Desk screenshot — draggable windows, a taskbar, and a start menu, all rendered in a terminal" width="700">
</p>

> **A Windows-9x-shaped desktop, hallucinated entirely out of terminal cells.**

Somewhere around the fourth time I alt-tabbed between six `tmux` panes and thought *"this would be so much nicer with a title bar,"* I decided the correct fix was obviously to build an entire draggable, resizable, taskbar-having window manager — inside the terminal I was already complaining about. This is that. No X11, no Wayland, no compositor. Just cells, glyphs, and a truecolor terminal that has agreed to pretend very hard.

**TTYPE Desk** multiplexes real TUI programs (bash, vim, htop — anything with a PTY) via **libvterm**, hosts native apps through an in-process **App SDK**, and can render images as truecolor half-block “graphics,” because if you're going to commit to the bit, you might as well be able to open a JPEG in it too. Draggable windows, a Start menu, a taskbar, scrollback, a command palette — everything a desktop needs, none of what it doesn't (a compositor, a display server, or any right to exist).

## Requirements

- Go 1.22+
- gcc / ar (to build vendored libvterm — yes, there's C in here; a terminal emulator without a real VT100 parser is just a very confident text box)
- A truecolor-capable host terminal (`COLORTERM=truecolor` recommended — this is a 2026 desktop wearing 1998's clothes, it still wants the good colors)

libvterm **0.3.3** is vendored under `third_party/libvterm-0.3.3` (no system package required — one less thing to go wrong on someone else's machine).

## Install (one-liner)

```bash
curl -fsSL https://raw.githubusercontent.com/RetroCodeRamen/ttypedesk/master/install.sh | bash
```

Linux only. Installs build deps (`apt`/`dnf`/`pacman`/`zypper`/`apk`, best-effort
outside Debian/Ubuntu, prompting for `sudo` right there in your terminal if it's
needed — nothing is ever stored), bootstraps a per-user Go toolchain if yours is
too old, clones the repo, builds it, and drops `ttypedesk` in `~/.local/bin`.

Verified against clean containers on Ubuntu 24.04, Debian 12, and Fedora
(apt/dnf paths, both root-without-`sudo` and regular-user-with-`sudo`) —
including `ttypedesk -update` running from its own already-executing binary.
Other distros (pacman/zypper/apk) are best-effort and less exercised;
cloning manually and running `./build.sh` (below) is always the fallback.

### Updating

```bash
ttypedesk -update
```

Re-runs the installer above in place: pulls the latest `master`, rebuilds,
and reinstalls to `~/.local/bin/ttypedesk`. Same thing as re-running the
one-liner by hand.

## Build

```bash
./build.sh
# or, if you enjoy typing:
#   (cd third_party/libvterm-0.3.3/build && gcc -std=c99 -O2 -I../include -I../src -c ../src/*.c && ar rcs libvterm.a *.o)
#   go build -o bin/ttypedesk ./cmd/ttypedesk
```

## Run

```bash
./bin/ttypedesk
```

That's it. No installer wizard, no EULA, no "would you like to also install a browser toolbar." Just a desktop, appearing where a desktop has no business appearing.

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
| **F8** | Copy-mode — keyboard scrollback selection (hjkl/arrows, Space/v select, Enter/y copy, Esc cancel) |
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

Yes, it's in the corner. Yes, clicking it does what you think. We know exactly what we're doing.

- **Programs ▶** — Notes, Calendar, Clock, Image Viewer, plus user apps with menu=`programs`
- **System ▶** — Terminal, Files, Settings, **Manual**, **System folder**, **Add Program…**, **Manage Programs…**, plus user apps with menu=`system`
- **App Store 🛍** — install extra apps from configured GitHub catalogs (default: [ttypedesk-apps](https://github.com/RetroCodeRamen/ttypedesk-apps)); installed apps register themselves into Programs/System automatically. See [docs/appstore.md](docs/appstore.md)
- **Quit**

**Add Program…** — set **Desk name** (e.g. Task Manager), **Command** (e.g. `htop`), emoji, **Start menu** folder (Programs or System), optional desktop shortcut.

**Manage Programs…** — list custom apps; Delete/Enter to remove (also drops their desktop shortcut).

Mouse: hover opens flyouts; click to launch. Keyboard: ↑↓, →/Enter into submenu, ←/Esc back.

Taskbar tray: **🔔** opens Notification Center; clock opens Calendar.

### Troubleshooting

It's a desktop environment hand-rolled inside a terminal emulator inside (probably) another terminal emulator. Something will eventually go sideways. When it does, it leaves a note instead of just quietly dying — that's the log file:

Log file (panics, deadlocks, app crashes): `~/.config/ttypedesk/ttypedesk.log`  
Override with `TTYPEDESK_LOG=/path/to/file` or `TTYPEDESK_LOG_LEVEL=debug`.

Native app panics are isolated to that window (red crash screen; Esc/Q closes it) instead of taking the whole desktop down with them. The main event/draw loop also recovers and keeps going when possible, because one misbehaving `htop` window is not a reason to lose your other twelve.

```bash
./bin/ttypedesk                     # default shell terminal
./bin/ttypedesk -e htop             # open htop in a window
./bin/ttypedesk -image photo.png    # open image viewer
./bin/ttypedesk -clock -e vim
./bin/ttypedesk -listen /tmp/ttypedesk.sock
./bin/ttypedesk -attach /tmp/ttypedesk.sock
```

## Config

`~/.config/ttypedesk/config.json` — FPS, shell, theme, taskbar dock, desktop icons, wallpaper, notify, **file associations** / default apps, App Store sources, and Files options. See [docs/associations.md](docs/associations.md), [docs/files.md](docs/files.md), and [docs/appstore.md](docs/appstore.md).

The file is hot-reloaded: edit it directly (or let a second instance / sync tool write it) and the running desktop picks it up within ~2 seconds — no restart required.

Defaults: **no terminal on start**; **restore last session** from `~/.config/ttypedesk/session.json` (toggle in Settings → Desktop). Set `open_terminal_on_start` if you want the old behavior.

Default look is the **XP** theme (Windows XP–inspired chrome) with the **Bliss** wallpaper (`builtin:bliss`) — the most 2001 rectangle of grass ever committed to a git repo. Other packs: **Scarlet**, **Bumble**, **Bubble**, **Sprout** (Settings → Appearance, or palette: `scarlet` / `bumble` / …). Over SSH the desktop stays solid unless you set `wallpaper.ssh_mode` to `keep`, because streaming a bitmap wallpaper over a laggy SSH session is a great way to relearn patience.

### Logging

Session log: `~/.config/ttypedesk/ttypedesk.log` (or `$TTYPEDESK_LOG`).  
Level: `$TTYPEDESK_LOG_LEVEL` = `debug` | `info` | `warn` | `error` (default `info`).  
Panics and LaunchAction failures are written here — check this file when something freezes or crashes.

## Remote attach (thin)

```bash
# session host
./bin/ttypedesk -listen /tmp/ttypedesk.sock

# another terminal — type and click into whatever's focused/on-screen, Ctrl+Q to detach
./bin/ttypedesk -attach /tmp/ttypedesk.sock
```

Keyboard and mouse (press/drag/release/wheel) forward to the host; window chrome — dragging, resizing, taskbar, Start menu — is still host-side only. See [docs/remote.md](docs/remote.md) for how a future RDP/VNC decoder plugs in as a graphical surface.

## Versioning

`MAJOR.MINOR.YYMMNN` — e.g. `0.3.260722`. `MAJOR.MINOR` lives in [VERSION](VERSION)
and is bumped by hand; `YYMMNN` is computed by [scripts/version.sh](scripts/version.sh)
(year, month, and a count of commits so far this calendar month) and baked
into the binary at build time (`./build.sh`, or `ttypedesk -version` to
check what's running). Still `0` for major — no compatibility promises yet,
config shape and internal APIs are still moving.

Every commit gets tagged automatically (`v0.3.260722`, …) via a `post-commit`
hook once `core.hooksPath` points at [.githooks](.githooks) — `./build.sh`
sets that the first time you run it in a fresh clone.

## Roadmap

Upcoming work is tracked in [ROADMAP.md](ROADMAP.md) — a growing pile of ideas ranging from "reasonable" to "an ASCII video player," in no particular order of restraint. SSH streaming notes: [docs/ssh.md](docs/ssh.md).

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
