# Terminals

A Terminal window is a real **PTY** driven by **libvterm**: your shell and TUI apps think they own a normal terminal.

## Scrollback

- **Mouse wheel** — scroll history (when not on an alternate screen that owns the mouse)
- **Shift+PgUp / Shift+PgDn** — page through history
- **Right-border scrollbar** — ▲ / thumb / ▼ when history exists; drag the thumb or click the track to page
- New output usually jumps you back to the live bottom
- Alternate-screen apps (fullscreen vim, less, …) typically disable scrollback until they leave alt-screen
- **F3** or **Alt+/** — find in scrollback (yellow bar); also **Ctrl+Shift+F** if your host terminal does not steal it (Guake often does)
- While finding: **Enter** / **F3** = older match, **Shift+Enter** / **Shift+F3** = newer; Esc closes; matches highlight in yellow

Scrollback depth is configurable in **Settings → Terminal & performance** (`scrollback` in config).

## Copy & paste

- Drag in the terminal content to select (or **Shift+drag** if the guest enabled mouse mode)
- Selection auto-copies on mouse-up; **Ctrl+Shift+C** also copies
- **Middle-click** or **Ctrl+V** pastes
- OSC 52 may also sync with the host clipboard when supported

## Attention (BEL)

When a program rings the terminal bell, that window’s taskbar chip can flash until you focus it.

## Launching other commands

- Desktop / Start: **Terminal**
- CLI: `./bin/ttypedesk -e htop`
- Action: `pty:nvim` (command after `pty:`)
- Add Program with a shell command
