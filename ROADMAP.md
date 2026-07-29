# TTYPE Desk roadmap

Tracked work beyond the current MVP. Order is preference, not a hard schedule.

**Product feel:** a real desktop in the terminal — floating windows, taskbar, Start menu, **desktop icons**, folder browsing, and first-party apps (including **Settings**) — with emoji available as icons wherever an icon is needed (not tied to specific app types). Designed to **stream well over SSH** (see [docs/ssh.md](docs/ssh.md)).

---

## Taskbar & chrome

- [x] Clock separator `|` before time on the taskbar
- [x] **Taskbar docking** — top / bottom / left / right (Windows-style); default top. Design: [docs/taskbar-dock.md](docs/taskbar-dock.md)
- [x] Clickable clock → opens Calendar (see Calendar)
- [x] **Notification tray button** next to clock (`🔔` + badge) — opens Notification Center (see System notifications)

## System notifications

Desktop-wide service: any app (Calendar, Settings test, future apps) can post; shell shows banners and a center. **Not** owned by Calendar.

Design: [docs/notifications.md](docs/notifications.md)

- [x] `notify.Service` (post / dismiss / list / subscribe)
- [x] Popup banner near taskbar (queue, auto-dismiss)
- [x] Tray button beside clock + Notification Center (view / dismiss / dismiss all)
- [x] `uiapp.Context.Notify(...)` for native apps
- [x] Settings: banners, auto-dismiss, SSH badge-only, test notification
- [x] Optional session history persistence (`notifications.json`, Settings toggle)
- [x] Calendar reminders post into this service (after Calendar local MVP)

## Wallpaper

- [x] **Image wallpaper** — load a picture, convert to truecolor half-block cells, use as desktop background (icons on top). Design: [docs/wallpaper.md](docs/wallpaper.md)
- [x] Settings: wallpaper mode `solid` | `pattern` | `image` + path/fit; SSH can force solid
- [x] Files as wallpaper picker (**W** / toolbar Wall); Settings → Browse in Files…
- [x] **Theme packs** — XP (Bliss), Scarlet, Bumble, Bubble, Sprout: each pairs chrome colors + builtin wallpaper (`builtin:<id>`)

## Icons & emoji (everywhere)

Emoji (and other pictographs) are **optional icon glyphs** for desktop shortcuts, Start menu rows, taskbar buttons, title bars, and in-app toolbars (e.g. a Save button drawing 💾).

- [x] **Unicode width plumbing** — shared `pkg/uwidth` so double-width emoji don’t break hit-testing/layout
- [x] **Manifest / config `icon` field** — free-form string (emoji or short text); used when present
- [x] **SDK drawing** — `Canvas` documents emoji-safe text; optional `DrawIcon` helper
- [x] **Fallback** — Settings → Input → ASCII icon substitutes (`palette.ascii_icons`)

## Calendar (clickable clock)

Big feature — plan before coding deep. Design: [docs/calendar.md](docs/calendar.md)

- [x] Click taskbar clock opens Calendar app / month popover
- [x] Month view + day agenda; create/edit **local** events
- [x] Reminders → **system notifications** (not a calendar-only toast UI)
- [x] Google Calendar sync (OAuth2 PKCE loopback flow, opt-in, read-only — see [docs/calendar.md](docs/calendar.md))
- [x] Microsoft Graph calendar sync (OAuth2 PKCE loopback flow, opt-in, read-only)
- [x] Settings → Calendar (Google/Microsoft connect+disconnect, reminder lead time, timezone)

## Window manager

- [x] **Arbitrary mouse resize** — drag any edge / all four corners (not only bottom-right); hit-test with a 1-cell border grip; sync PTY/`TIOCSWINSZ` continuously while dragging
- [x] Discoverable resize grips (cursor hint / thicker corner glyph)
- [x] Double-click title bar to maximize
- [x] Snap / half-screen tile shortcuts — `Alt+Ctrl+Arrows` (left/right/top/bottom); same side again restores
- [x] Persist window geometry in session/config
- [x] Autostart / startup layout — restore last session (not open-terminal-by-default)
- [x] Bell flash / taskbar attention — PTY BEL flashes the window’s taskbar chip until focused

## Desktop icons & file management

Goal: the empty “wallpaper” area becomes a real desktop — icons you can open, plus a way to browse folders.

- [x] **Desktop icon layer** — config-backed shortcuts on the desktop field (label + optional emoji/icon + launch target); click to open
- [x] **Icon is data, not an app type** — any shortcut or app manifest may set `icon: "🌐"` or `icon: "💾"` or leave it blank / use ASCII; no hardcoded emoji→app mapping
- [x] **Folder manager** — native Files app: list/grid, scrollbar SDK, open via associations, mkdir/rename/clipboard/trash. Design: [docs/files.md](docs/files.md), [docs/associations.md](docs/associations.md)
- [x] Drag icons to reposition (persist layout)
- [x] **Default apps / associations** — text→nano (configurable), images→Image Viewer; notify when unset

## Settings application (first-party)

A real **Settings** window (native `uiapp`), not only editing JSON by hand — the control panel for TTYPE Desk.

- [x] **Settings app** in Start menu (icon configurable, e.g. ⚙️)
- [x] Pages / sections:
  - Appearance — wallpaper modes, taskbar dock, theme packs (XP / Scarlet / Bumble / Bubble / Sprout)
  - Desktop — icons, SSH solid mode, session restore / open-terminal-on-start, **autostart list**
  - Terminal — shell, scrollback, FPS (local + SSH)
  - Notifications — banners, dismiss, SSH badge-only, test
  - Default apps — editor / image / browser roles + associations
  - [x] Apps — Add/Manage Programs, role summary, System folder
  - [x] Input — remappable Desk hotkeys; Start→palette; ASCII icon substitutes
  - [x] Advanced — log path / open log, config paths, effective FPS
- [x] Read/write `~/.config/ttypedesk/config.json` (and grow schema as needed)
- [x] Apply theme/FPS/wallpaper live where safe; Save writes config

## Config as product

- [x] **App roles** — `role:terminal` / `{editor}` / `{browser}` / `{filemgr}` / `{image}` expand via `Roles` (browser unset until configured)
- [x] **Autostart / startup layout** — `autostart: ["terminal","notes",…]` LaunchAction list (Settings → Desktop); session restore remains separate
- [x] **Remappable hotkeys** — Settings → Input; `hotkeys` map in config (`f3`, `alt+/`, `ctrl+shift+f`, …)
- [x] Hot reload of theme/desktop icons where practical — external edits to `config.json` (hand-edited, synced, written by another instance) are picked up within ~2s and applied live via the same path Settings-app Save already uses; no restart needed

## Universal command palette (distinctive)

A single **type-to-run** overlay for the whole desk — not another Start menu clone. Open with **Ctrl+Space** / **Ctrl+P** (remappable). Start menu defaults to **F10** / **Ctrl+Esc** (Alt+Space is legacy — hosts like Guake often steal it). Fuzzy-match verbs + nouns; Enter runs.

Design: [docs/command-palette.md](docs/command-palette.md)

Example queries (feel target):

```text
open notes
wifi connect
find readme
play coheed
calculate 0xff * 16
ssh server
install doom
```

Phases:

- [x] **Palette UI** — centered DOS-style popup; filter list; Esc dismiss; recent history
- [x] **Core verbs** — `open` / `launch` / apps / `find` / windows / quit wired to LaunchAction / OpenPath / focus / scrollback find
- [x] **Calculator** — `calculate …` / `= …` integer expr (hex, `**`, bitwise); Enter copies result
- [x] **Shell one-shots** — `ssh host`, `run htop` → PTY (`pty:…`)
- [x] **Recipes** — `recipes` in config (default samples: `wifi connect`, `install doom` with confirm)
- [x] **Deeper providers** — closed, won't build for 1.0: this line was already tagged "(optional; keep out of WM core)" in the roadmap's own text, and stays that way
- [x] Recipe **confirm** — Enter twice for `confirm: true` recipes (palette stays open)
- [x] Settings → optional “replace Start with palette” (`palette.start_opens_palette`)
- [x] Persist recent palette queries (`palette_history.json`)

Hotkeys: **Ctrl+Space** / **Ctrl+P** (remappable as `palette` / `palette_alt`).

This is a **product differentiator**: the floating WM is familiar; the palette is how power users *drive* it without hunting menus.

## GUI–TUI App Bridge (Browsh-inspired — not Browsh)

Use Browsh’s *architecture* as the template: off-screen GUI → RGBA → half-block cells → floating window + input remap. **Do not depend on the Browsh binary.**

See [docs/gui-bridge.md](docs/gui-bridge.md).

- [x] **BridgeSurface** — `internal/bridge.BridgeSurface`, implements `surface.Surface` directly for now (see docs/gui-bridge.md for why the pluggable-backend interface isn't extracted yet — only one backend exists)
- [x] **BrowserNest** — closed, won't build: superseded by DisplayNest (`bridge:firefox` already covers "browse the web" through the generic X11 backend; no concrete need for a narrower backend has come up)
- [x] **DisplayNest** — Xvfb nest for arbitrary GUI apps (X11 only, no Wayland yet)
- [x] **RemoteNest** — descoped, won't build for 1.0: the only maintained Go RDP client (`nakagami/grdp`) is GPL-3.0-licensed, which would force this project's own licensing; not worth the tradeoff for this feature
- [x] App/desktop manifests can launch bridge targets — `bridge:<cmd>` LaunchAction, Add Program's Command field works same as any other launch string
- [x] **Text legibility (AT-SPI overlay)** — real characters instead of raster noise for text-heavy apps, via the Linux accessibility tree (`internal/bridge/atspi.go`). Works well for native GTK/Qt apps (validated against `zenity`/`gtk3-demo`); does **not** work for Electron apps (validated against Cursor — matches a known open VS Code accessibility issue). Non-fatal/optional: no `dbus`/`at-spi2-core` on the host just means raster-only, same as before this existed.
- [x] Perf: overscan buffer (capture launches with headroom beyond the requested cell size), adaptive frame budget over SSH (backs off from the flat 10fps when a frame gets expensive to produce), XRandR live resize (grows/shrinks the live Xvfb screen via RANDR instead of only ever rescaling a fixed-size buffer — debounced off a live drag-resize, capped at a 1920x1080 ceiling per bridged window)

## Terminal

- [x] Keyboard copy-mode (tmux-like) — **F8**; hjkl/arrows move (scrolling into history at the edges), PgUp/PgDn page, g/G jump top/bottom, Space/v select, Enter/y copy, Esc/q cancel. Reuses the existing mouse-selection render/copy path.
- [x] Scrollback scrollbar / indicator (wheel + right-border bar; drag thumb / arrows / page track)
- [x] Scrollback search — **F3** / **Alt+/** find bar (Ctrl+Shift+F if host allows); Enter / Shift+Enter; yellow highlights
- [x] Better system clipboard — OSC 52 plus `wl-copy` / `xclip` / `xsel` fallbacks

## Help & Manual

- [x] **Manual** app — Start → System → Manual (TOC + chapters, focus-or-reuse)
- [x] **System folder** — `~/.config/ttypedesk/System/` markdown copy; Start → System → System folder

## App Store

Start ▸ App Store — install extra apps from configured GitHub catalogs (fetch `index.json`, run install script in a terminal, register launchers once detected). Design: [docs/appstore.md](docs/appstore.md)

- [x] Catalog fetch from one or more `app_sources` repos
- [x] Detect / install / auto-register flow with Start ▸ Programs
- [x] Default catalog: [ttypedesk-apps](https://github.com/RetroCodeRamen/ttypedesk-apps)
- [x] Per-source trust/warning UI (scripts run unsandboxed) — confirm-to-trust, persisted per source

## Apps & platform

- [ ] Out-of-process App SDK (stdio / Unix socket NDJSON)
- [x] **Comprehensive Host / App API** — `uiapp.Host` gained `PlayAudio` (`internal/audio`, wraps `oto/v3`, fixed 48kHz/stereo output — no per-call resampling), `uiapp.NewMediaClock()` (play/pause/position, no Host needed — pure local timing), `PickFile` (`apps/filepicker`, a small modal browser, not a second Files app), and `SaveCredential`/`LoadCredential` (`internal/credstore`, one file per key under `~/.config/ttypedesk/credentials/`, 0600). "Background workers" needs no dedicated API — apps just spawn goroutines directly, same as any other Go code; prerequisite for media & chat apps below is now in place
- [x] Session save/restore (open windows + geometry)
- [ ] More sample native apps as needed (beyond Clock / Notes / Settings / Files)
- [ ] Proto `notify` message for out-of-process apps (hooks system notification service)

## First-party media apps

The App/Host API this depended on (decode pipeline hooks, media clock,
file picker) landed — see the Apps & platform section above.

### Winamp-style audio player

Native `uiapp` music player with **classic Winamp energy** (not a flat “Spotify clone”): skinnable chrome feel in cells, playlist, EQ-ish bars, transport.

- [x] **Amp** — `apps/amp`; open local files (via the file picker), playlist, play/pause/stop/next/prev
- [x] Visualizer bars from PCM (windowed peak amplitude across 16 bars, half-block-rendered — not a real FFT spectrum; see `apps/amp/decode.go`)
- [x] Layout: single window, playlist + transport + visualizer together — not separate floating skin/EQ panes, which felt like unearned complexity for a v1 with no actual EQ processing to show
- [x] Formats via host decode — ffmpeg subprocess (`-f s16le` raw PCM out), no linked decoder library; a soft runtime dependency, checked at launch with a clear error if missing
- [ ] Wire to audio-stream companion when over SSH so sound plays on the laptop — depends on the Audio streaming section below, not yet built

### ASCII video player

Terminal-native video: live **frames → ASCII / half-block cells**, not a GUI nest.

- [x] **Vid** — `apps/vid`; open video file (file picker), play/pause, scrub (Left/Right seek — an input-side ffmpeg `-ss` restart, not a live random-access seek; there's no such thing against a raw decode pipe)
- [x] Live converter path: `internal/ffdecode` decodes raw RGB24 frames via ffmpeg, `internal/gfx.EncodeHalfBlockFit` (unchanged, shared with wallpaper/imageview/Bridge) turns them into the ▀▄ half-block grid at window size
- [x] Adaptive FPS under SSH — a fixed lower decode frame rate over SSH (`config.OverSSH()`), not a live-measured budget; resolution itself doesn't adapt in this first pass
- [x] Pipes raw frames from an `ffmpeg` subprocess — never a linked decoder library, matching Amp and the Bridge's own Xvfb posture (soft runtime dependency)
- [x] Audio track via the same decode-to-`Host.PlayAudio` path Amp uses (`internal/ffdecode.DecodeAudio`, shared) — decoded as its own separate ffmpeg process against the same file, not multiplexed through one process; a file with no audio track (or one that fails to decode) degrades to video-only rather than failing the whole thing

## Messenger (no Desk-operated server)

Native chat client that **connects to an existing network** — we do not run or manage a chat backend.

**Preferred backend: [Matrix](https://matrix.org/)** (open Client-Server API). Users register on a public homeserver (e.g. `matrix.org`) or any HS they already use; TTYPE Desk is just another client. Zero server ops on our side; E2EE and rooms come for free from the ecosystem.

Alternatives if Matrix is too heavy for v1:

- **IRC** (Libera.Chat, etc.) — simplest protocol, great for hacker aesthetic, weaker DMs/history
- **XMPP** — federated, many public servers; smaller modern client ecosystem than Matrix

Avoid building around Discord/Slack as the primary path (proprietary APIs / ToS). Bridges into Matrix stay an upstream concern, not ours.

- [x] **Chat** app — `apps/chat`, using [mautrix-go](https://github.com/mautrix/go); login (homeserver + username + password), room list, timeline, send text
- [x] Persist session token under `~/.config/ttypedesk/` (not in git) — `internal/credstore`, same store OAuth2 calendar tokens use
- [x] Notifications for backgrounded-room activity via system notify service — not real `m.mentions` parsing yet, just "a message arrived in a room that isn't the one in view" (the common case that actually needs a nudge); suppressed for the user's own echoed messages and the currently-selected room
- [x] Phase 1: Matrix text rooms (shipped); Phase 2: E2EE, attachments/reactions — explicitly not built, a real separate undertaking (device verification, key backup, cross-signing), not an incremental add
- [x] Design note: kept UI DOS/Win9x-adjacent (status bar, room list | messages)

## Remote attach

- [x] Bidirectional attach (input over socket) — keyboard to focused window; mouse press/drag/release/wheel hit-tests and forwards into content area, focusing on press. Window chrome (drag/resize/taskbar/Start menu) still local-only.
- [ ] Binary cell-diff framing (replace JSON snapshots)

## Audio streaming (later — after main desktop)

Stream server audio to a local companion client over SSH/attach. Cool for media in bridge apps **and** Amp/Vid; **not** near-term.

Design: [docs/audio-stream.md](docs/audio-stream.md)

- [ ] Audio capture on host (Pulse/PipeWire monitor and/or bridge backends)
- [ ] Encode + mux (prefer attach protocol frames; SSH port-forward OK for MVP)
- [ ] `ttypedesk-audio` (or combined remote client) play-only receiver
- [ ] Settings: enable / bitrate / mute
- [ ] Optional later: mic uplink
- [ ] Feed Amp / Vid playback into this path when remote

## Later / optional (distro & extras)

Parked unless packaging becomes a goal:

- antiX / boot-into-TTYPE-Desk session
- System tray metrics (CPU/mem) — easy to clutter; keep optional
- Virtual desktops / tiling / multi-monitor
- Plugin marketplace
- Power/network/locale control panels (distro concern)
