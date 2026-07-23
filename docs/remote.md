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

`-attach` is read-only today. Bidirectional input is planned; message types (`key`, `mouse`, `resize`, `focus`) already exist.

## True color

Cells always carry 24-bit RGB. Host paint uses `tcell.NewRGBColor`. PTY children receive `COLORTERM=truecolor`. Half-block graphics and the bridge depend on truecolor for usable results.
