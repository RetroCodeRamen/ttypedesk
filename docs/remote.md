# Remote / graphical plug-in model

TTYPE Desk composites **surfaces** that speak a shared cell framebuffer:

```text
{ rune, fg_rgb, bg_rgb, attrs } + dirty rects
```

## Surface kinds

| Kind | Source | Encoder |
|------|--------|---------|
| `pty` | PTY bytes → libvterm | already cells |
| `app` | `uiapp.Canvas` | retained cells |
| `gfx` | RGBA image / demo | half-block `▄` |
| `bridge` | Live GUI (see [gui-bridge.md](gui-bridge.md)) | half-block + input remap |

## GUI–TUI Bridge vs Browsh

**Browsh is a reference architecture, not a dependency.** The bridge is how TTYPE Desk runs GUI programs “the way Browsh ran Firefox”: capture frames, encode to cells, forward input. Backends may include a nested browser, X11/Wayland nest, or RDP/VNC — all behind one `BridgeSurface`.

Details: [gui-bridge.md](gui-bridge.md).

## Attach protocol (MVP)

`-listen <unix.sock>` streams newline-delimited JSON envelopes (`internal/proto`):

- `attach` — hello
- `snapshot` — full window list + cell grids (truecolor RGB)

`-attach` is bidirectional: keyboard input forwards to whatever window is focused on the host, and mouse press/drag/release/wheel hit-test the window under the pointer (focusing it on press) and forward into its content area — the same path local PTY/app mouse input takes. Window chrome (title-bar drag, resize grips, taskbar, Start menu) is not remoted yet; that stays a local-only interaction for now. Detach with **Ctrl+Q**.

## True color

Cells always carry 24-bit RGB. Host paint uses `tcell.NewRGBColor`. PTY children receive `COLORTERM=truecolor`. Half-block graphics and the bridge depend on truecolor for usable results.
