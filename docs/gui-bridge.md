# GUI–TUI App Bridge

Browsh proved a pattern: **off-screen GUI → cell framebuffer → TTY**, with input mapped back. TTYPE Desk adopts that pattern as a core subsystem. **We do not embed or depend on the Browsh binary.**

## Status: DisplayNest is built (`internal/bridge`)

The first backend — **DisplayNest**, "run any GUI app" — is implemented and wired in:

- `internal/bridge.BridgeSurface` implements `surface.Surface` (same interface as `PtySurface`/`AppSurface`), so it's just another window kind (`Kind() == "bridge"`) to the rest of the desktop.
- Launch one with `bridge:<command>` as a LaunchAction — e.g. `bridge:xclock`, or set a desktop icon / Add Program entry's Command to a `bridge:` string.
- Under the hood: spawns a per-window `Xvfb` (virtual framebuffer X server), launches `<command>` against it with `DISPLAY` set, and on a 10fps timer reads the root window via X11's `GetImage` and feeds it through the existing `internal/gfx.EncodeHalfBlockFit` (the same half-block encoder wallpaper/imageview use — no new rendering code).
- Input goes back in via the **XTest** X11 extension: keys are injected by temporarily remapping a scratch keycode to whatever keysym is needed (the same trick tools like `xdotool` use — X11 keysyms 0x20–0xFF already equal their Latin-1 code point, so only the non-printable named keys and modifiers need an explicit table, in `internal/bridge/keysym.go`); mouse events are translated from cell coordinates to pixel coordinates proportionally.
- Pure Go — `github.com/jezek/xgb` implements the X11 wire protocol directly, no libX11/cgo dependency. `Xvfb` itself is an external runtime dependency (not vendored): `BridgeSurface.New` fails with a clear error if it's not on `PATH`.
- **Resize** does *not* relaunch Xvfb or the guest process — it just updates the target cols/rows that `EncodeHalfBlockFit` resamples into on the next capture. Relaunching Xvfb on every window resize would kill the guest app's state for no real benefit, since the encoder already rescales.
- Manual verification: `cmd/bridgecheck` (`go run ./cmd/bridgecheck xclock`) launches a real bridged app and reports on captured color variety. Automated coverage: `internal/bridge/bridge_test.go` and `internal/server`'s `TestLaunchActionBridgeCreatesWindow`, both skipped unless `Xvfb`/`x11-apps` are on `PATH` (CI installs them; see `.github/workflows/ci.yml`).

## Text legibility: AT-SPI overlay (native GTK/Qt apps only)

Raw raster capture can't distinguish text from any other pixels — a
text-heavy app degrades into illegible colored half-block noise, since
every cell uses the same glyph and only color carries information (verified
directly: bridging **Cursor** rendered 64 distinct colors but exactly 1
distinct rune across the whole screen). Browsh solves this for web pages by
pulling real DOM text. The equivalent for arbitrary X11 apps is
**AT-SPI2** — the Linux accessibility framework GTK/Qt/Electron apps can
expose their UI tree through over D-Bus — and it's implemented, with one
real, empirically-confirmed limitation:

- **Works well for native GTK/Qt apps.** `internal/bridge/atspi.go` is a
  hand-rolled AT-SPI2 client on `github.com/godbus/dbus/v5` (no mature Go
  AT-SPI wrapper exists to depend on): bus bootstrap → recursive
  `Accessible.GetChildren` tree walk → `Component.GetExtents` for bounding
  boxes → `Text.GetText` for content. Validated against `zenity` and
  `gtk3-demo`: real text, correctly positioned, reliably. Two non-obvious
  findings from getting it working: `GetExtents` returns one `(iiii)`
  struct, not four separate values; and `CoordType=0` ("screen") returns
  `(0,0)` for everything in a window-manager-less Xvfb — `CoordType=1`
  ("window-relative", `atspiCoordWindow` in `atspi.go`) is what actually
  works, which suits this architecture anyway since each `BridgeSurface`
  gets its own dedicated Xvfb.
- **Does not work for Cursor, and likely not for Electron apps generally.**
  Tested directly: Cursor genuinely registers with AT-SPI and builds a
  real, correctly-sized accessible tree (confirmed matching the actual
  window geometry) even with `--force-renderer-accessibility` and
  `org.a11y.Status.IsEnabled`/`ScreenReaderEnabled` forced on before
  launch — but every text node returns the Unicode replacement character
  (U+FFFC) instead of real content. This matches a known, still-open
  upstream VS Code issue (`microsoft/vscode#84833`) where Electron's
  accessibility flag gets stripped/intercepted. Cursor is a VS Code fork
  and inherits the gap.
- **Non-fatal by design.** `BridgeSurface.New` spawns a private
  `dbus-daemon --session` + `at-spi2-registryd` pair alongside its
  dedicated Xvfb (`internal/bridge/dbus.go`), matching the per-window
  isolation model — but if either isn't installed, or the guest simply
  doesn't expose usable text (Electron apps), it logs a warning and the
  bridge just runs raster-only, exactly as it did before this existed. No
  new required dependency, no behavior change for apps that don't benefit.
- **Compositing**: `captureLoop` composites the latest AT-SPI walk result
  on top of each raster frame (`compositeText` in `bridge.go`) — real
  characters replace the half-block glyph in cells covered by a text
  node's bounding box, keeping the raster background color and picking a
  contrasting foreground by luminance. The walk itself runs on its own
  goroutine/ticker (`atspiLoop`, ~3.3fps vs. the raster's 10fps — text
  layout changes far less often than pixels, and a walk costs a D-Bus
  round-trip per node) so a slow tree walk never blocks raster capture.

## What Browsh got right (reused)

| Idea | Role in TTYPE Desk |
|------|--------------------|
| Cell framebuffer `{rune, fg, bg}` | Same unit as every other surface |
| Half-block `▄` (bg=top, fg=bottom) | Reused from `internal/gfx`, unchanged |
| Thin input remap | WM owns focus; bridge forwards cell→pixel events via XTest |
| Resize = renegotiate grid | Window content size drives what the capture resamples into |

## Backend interface — still pluggable, not yet abstracted

The original design called for a `Backend` interface behind `BridgeSurface` so `DisplayNest` would be one of several pluggable capture/input implementations. With only one backend built so far, `BridgeSurface` implements DisplayNest's X11 logic directly rather than through that extra interface layer — premature abstraction with a single implementation just adds indirection with no payoff. If/when a second backend lands (BrowserNest, RemoteNest), that's the point to extract the seam, informed by what the two implementations actually have in common rather than guessed in advance.

## Surface kinds (updated mental model)

| Kind | Source |
|------|--------|
| `pty` | Real TUI via libvterm |
| `app` | Native `uiapp` Canvas (Notes, Clock, …) |
| `bridge` | Live GUI via the GUI–TUI Bridge (DisplayNest) |

(`gfx` window requests are internally handled as an `app` kind wrapping the image viewer — see `internal/server`.)

## Closed decisions (won't build for 1.0)

1. **BrowserNest** — a dedicated headless-browser backend. Closed: pointing DisplayNest at any browser binary (`bridge:firefox`) already covers the "browse the web" use case, and no concrete need for a narrower, second backend has come up.
2. **RemoteNest** — RDP/VNC client decode into the same cells path. Would have been a genuinely separate protocol/problem domain from DisplayNest, not a variation of it — but the only actively-maintained Go RDP client (`nakagami/grdp`) is GPL-3.0-licensed, and ttypedesk has no LICENSE file today (informally all-rights-reserved). Linking a GPL-3 dependency into the binary would force the whole project's licensing, which isn't a tradeoff worth making just for this feature. (VNC alone has a healthier option, `kward/go-vnc`, MIT-licensed — if RemoteNest is revisited later, VNC-only is the path, not RDP.)

## Perf: capture sizing (shipped)

The Bridge's raw-pixel capture used to be a flat 10fps against a fixed
Xvfb resolution that exactly matched the window's initial cell size —
correct, but with two rough edges: no headroom for a window that grows,
and no way to back off frame rate when a frame gets expensive to produce
(e.g. over SSH). Both are addressed now, still in `internal/bridge`:

- **Overscan buffer** — the *current* RandR screen size is set a bit
  (`overscanFactor`, 25%) larger than the window's requested cell size, so
  a small later grow still resamples from real captured pixels instead of
  immediately needing a live resize.
- **Adaptive frame budget over SSH** — `captureLoop` measures its own
  capture+encode cost each tick and, only when `config.OverSSH()`, backs
  the ticker interval off proportionally (down to `sshMinFPS`) when a
  frame gets expensive, recovering back toward the full `captureFPS` once
  frames are cheap again. The local (non-SSH) path stays a flat 10fps —
  there's no bandwidth to save there.
- **XRandR live resize** — growing a window past its current (overscan-
  padded) capture buffer schedules a debounced (`resizeDebounce`, 250ms)
  live resize of the Xvfb screen via RANDR (`growScreen`, run from
  `captureLoop` itself once resizing settles, never inline in `Resize` —
  a live drag-resize can call `Resize` many times a second, and RANDR is a
  blocking X11 round trip). Two non-obvious things learned getting this
  right (both confirmed empirically against a real Xvfb, not assumed from
  docs):
  - RANDR's `SetScreenSize` can never grow a screen past whatever size
    Xvfb was actually **launched** at — no matter how it's invoked. So
    each `BridgeSurface` launches its private Xvfb at a fixed ceiling
    (`xvfbCeilW/H`, 1920×1080 — comfortably covers any realistic terminal
    window at this bridge's per-cell resolution, ~8MB fixed memory per
    bridged window) and immediately RandR-shrinks down to the window's
    real initial size, so `growScreen` always has headroom to grow back
    into later, up to that ceiling.
  - A plain `SetScreenSize` request alone isn't enough — it fails with
    `BadMatch`/`BadValue` unless the screen's CRTC is reconfigured to
    match too (the same dance the `xrandr` CLI performs under the hood:
    create a mode of the target size, attach it to the output, point the
    CRTC at it, then set the screen size). The **order** of those last two
    steps depends on direction: the CRTC's rectangle can never exceed the
    *current* screen rectangle at any intermediate step, so shrinking
    reconfigures the CRTC first (still fits inside the still-large
    screen) then shrinks the screen to match, while growing sets the
    screen size first (a no-op for the still-small CRTC) then grows the
    CRTC to fill it. Getting this backwards fails silently-ish with a
    generic protocol error that doesn't obviously point at "wrong order."

## Non-goals

- Shipping or requiring the upstream Browsh package
- Pixel-perfect desktop replacement for Wayland/X11 users
- Replacing the native App SDK (bridge is for *foreign* GUI; `uiapp` remains for first-party TUI apps)
