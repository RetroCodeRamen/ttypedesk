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

## Not yet built

1. **BrowserNest** — a dedicated headless-browser backend. Lower priority now that DisplayNest exists: pointing DisplayNest at any browser binary (`bridge:firefox`) already covers the "browse the web" use case; BrowserNest would only be worth it for something a generic X11 nest can't do (e.g. avoiding a full browser window chrome, or a lighter-weight embedded engine).
2. **RemoteNest** — RDP/VNC client decode into the same cells path. A genuinely separate protocol/problem domain from DisplayNest, not a variation of it.
3. Perf follow-ups: overscan/pan buffer, adaptive frame budget under SSH (currently a flat 10fps), XRandR-based live resize instead of the current fixed-Xvfb-resolution-plus-rescale approach.

## Non-goals

- Shipping or requiring the upstream Browsh package
- Pixel-perfect desktop replacement for Wayland/X11 users
- Replacing the native App SDK (bridge is for *foreign* GUI; `uiapp` remains for first-party TUI apps)
