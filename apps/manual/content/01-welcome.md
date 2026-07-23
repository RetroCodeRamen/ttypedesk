# Welcome to TTYPE Desk

TTYPE Desk is a **floating-window desktop that lives inside your Linux terminal**.

It feels like a classic DOS / early-Windows shell: a desktop field, a taskbar, a Start menu, desktop icons, and windows you can move, resize, minimize, and snap — while still running real terminal programs (bash, vim, htop, …) and first-party apps (Files, Settings, Calendar, …).

## What you are looking at

- **Desktop** — wallpaper or pattern; icons sit on top
- **Taskbar** — Start button, open-window chips, notification bell, clock
- **Windows** — each has a title bar, borders, and content (a PTY terminal or a native app)
- **Start menu** — Programs and System folders, plus Quit

## How to use this Manual

- **Start → System → Manual** opens this reader (one window; reopening focuses it)
- **Ctrl+Space** (or Ctrl+P) opens the **command palette** — type `open manual`, `calculate 0xff*16`, `find readme`, …
- **Start → System → System folder** opens the same documents in Files as `.md` files under `~/.config/ttypedesk/System/`
- Use **↑↓ / PgUp / PgDn / wheel** to scroll; **←→** or click the TOC to change chapter
- Close with the window **X** or **Ctrl+W**

## Design ideas in one sentence

Multiplex real TUIs with **libvterm**, host rich UI with an in-process **App SDK**, and stay usable over **SSH** with a lower frame rate and optional solid desktop.

Next: **Desktop & icons**.
