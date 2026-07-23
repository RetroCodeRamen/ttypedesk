# GUI–TUI App Bridge

Browsh proved a pattern: **off-screen GUI → cell framebuffer → TTY**, with input mapped back. TTYPE Desk adopts that pattern as a core subsystem. **We do not embed or depend on the Browsh binary.**

## What Browsh got right (reuse the ideas)

| Idea | Role in TTYPE Desk |
|------|--------------------|
| Cell framebuffer `{rune, fg, bg}` | Same unit as every other surface |
| Half-block `▄` (bg=top, fg=bottom) | Already in `internal/gfx` |
| Separate raster (+ optional text) planes | Bridge can start raster-only |
| Viewport + overscan buffer | Local pan/scroll without waiting on capture |
| Thin input remap | WM owns focus; bridge forwards cell→pixel events |
| Resize = renegotiate grid | Window content size drives capture resolution |

## What we generalize

Browsh’s encoder was **Firefox + webextension**. Ours is a **pluggable backend**:

```text
┌─────────────────────────────────────────────────────────┐
│  TTYPE Desk window (chrome)                             │
│  ┌───────────────────────────────────────────────────┐  │
│  │ BridgeSurface  (encode + input + ProduceDiff)     │  │
│  └──────────────▲────────────────────────┬───────────┘  │
└─────────────────┼────────────────────────┼──────────────┘
                  │ RGBA dirty rects       │ key/mouse/resize
       ┌──────────┴──────────┐    ┌────────▼────────┐
       │ Backend interface   │◄───│ InputInjector   │
       └──────────┬──────────┘    └─────────────────┘
    ┌─────────────┼─────────────┬─────────────────┐
    ▼             ▼             ▼                 ▼
 BrowserNest   Xvfb/Wayland   RDP/VNC          (future)
 (Chromium/    nest + grab    client decode
  Firefox)
```

Any backend that can **produce pixels** and **accept input** can appear as a first-class app with an emoji icon (🌐, etc.).

## Surface kinds (updated mental model)

| Kind | Source |
|------|--------|
| `pty` | Real TUI via libvterm |
| `app` | Native `uiapp` Canvas (Notes, Clock, …) |
| `gfx` | Static/animated images → half-block |
| `bridge` | Live GUI via GUI–TUI Bridge |

## Backend sketches

1. **BrowserNest** — headless browser + capture (Browsh-like *implementation*, our code/control). Good first vertical for 🌐.
2. **DisplayNest** — Xvfb/Weston nested compositor; `cmd` + args for arbitrary GUI apps (the true “run any GUI like Firefox was”).
3. **RemoteNest** — RDP/VNC decode into the same RGBA→cells path (remote desktop goal).

## Non-goals

- Shipping or requiring the upstream Browsh package
- Pixel-perfect desktop replacement for Wayland/X11 users
- Replacing the native App SDK (bridge is for *foreign* GUI; `uiapp` remains for first-party TUI apps)
