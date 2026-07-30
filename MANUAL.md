# TTYPE Desk — the manual

The README gets you installed and oriented. This is the deep dive: every
subsystem, what it actually does, how the pieces fit together, and where to
look in the repo if you want to go further. If you'd rather read this
in-desktop, there's also a shorter, task-focused version built in — Start ▸
System ▸ Manual — but this document covers more ground and doesn't require
the desktop to be running.

## Contents

- [Windows & the window manager](#windows--the-window-manager)
- [Taskbar & Start menu](#taskbar--start-menu)
- [Command palette](#command-palette)
- [Desktop icons, wallpaper & themes](#desktop-icons-wallpaper--themes)
- [Files](#files)
- [Settings](#settings)
- [Calendar](#calendar)
- [App Store](#app-store)
- [Notifications](#notifications)
- [Amp (audio player)](#amp-audio-player)
- [Vid (video player)](#vid-video-player)
- [Chat (decentralized LAN messenger)](#chat-decentralized-lan-messenger)
- [Terminal features](#terminal-features)
- [GUI–TUI App Bridge](#guitui-app-bridge)
- [Remote attach](#remote-attach)
- [Config, versioning & the stability policy](#config-versioning--the-stability-policy)
- [Troubleshooting](#troubleshooting)

---

## Windows & the window manager

Every window — a real PTY program, a native app, or a bridged GUI app — is a
`Surface` (`internal/surface`) hosted in a floating window the WM (
`internal/server`) owns: position, size, focus, z-order, minimize/maximize
state. The surface itself only knows how to produce a cell grid and consume
input; it doesn't know it's in a window at all.

- **Move**: drag the title bar, or `Alt+Arrows`.
- **Resize**: drag any edge or corner (not just bottom-right — full 1-cell
  hit-test border), or `Alt+Shift+Arrows`.
- **Snap**: `Alt+Ctrl+Arrows` tiles to a half of the desktop; press the same
  direction again to restore the pre-snap geometry.
- **Maximize**: double-click the title bar, or the taskbar-style maximize
  button in the chrome.
- **Minimize**: `Ctrl+M`, or the minimize button — the window drops to a
  taskbar chip and stops receiving redraws until restored.
- **Focus**: click anywhere in a window, or `Alt+Tab` to cycle.
- **Close**: `Ctrl+W`, or the window's close button. PTY windows send the
  child process a clean shutdown, not a hard kill.
- **Attention**: a PTY window that rings the terminal bell (`\a`) while
  unfocused gets its taskbar chip flashed yellow until you focus it — the
  same idea as a browser tab title blinking, driven by libvterm's bell
  callback (`internal/vterm`).
- **Persistence**: window geometry (position, size, minimized/maximized
  state) is saved into `~/.config/ttypedesk/session.json` and restored on
  next launch by default (Settings → Desktop → "restore last session").
  `open_terminal_on_start` controls whether a fresh shell also opens
  alongside whatever session gets restored.

Resizing a PTY window updates the underlying pty's `TIOCSWINSZ` continuously
while you drag, not just on release — programs like `vim`/`htop` reflow
live, the same as a real terminal emulator.

## Taskbar & Start menu

The taskbar can dock to any of the four edges (Settings → Appearance,
`taskbar.dock` in config) — default is top, matching the classic Windows
layout in the screenshots below. It shows, left to right (or top to bottom
on a vertical dock): the Start button, one chip per open window (click to
focus/restore, drag reorders), and a tray with the notification bell and
clock (click the clock to jump straight to Calendar).

<p align="center">
  <img src="images/start-menu.png" alt="Start menu open, showing Programs / System / Quit" width="640">
</p>

**Start menu** (`F10` / `Ctrl+Esc` by default — `Alt+Space` is legacy and
often intercepted by the host terminal):

- **Programs** — Notes, Calendar, Clock, Image Viewer, plus any app you've
  added with `menu: programs` (via Add Program or the App Store).
- **System** — Terminal, Files, Settings, Manual, System folder, Add
  Program, Manage Programs, plus `menu: system` apps.
- **App Store** — see [App Store](#app-store) below.
- **Quit**.

Mouse: hover a row with a `▶` to open its flyout, click a leaf to launch.
Keyboard: `↑↓` to move, `→`/`Enter` into a submenu, `←`/`Esc` back out.

**Add Program…** registers a custom launcher: a desk name, a command, an
optional emoji, which Start menu folder it lives in, an optional desktop
shortcut, and a **"Launch via GUI-TUI Bridge"** checkbox. Unchecked (the
default), the command runs as a plain shell command in a PTY window — no
`pty:`/`bridge:`/etc. prefix syntax needed or recognized here, just the
command itself (e.g. `htop`). Checked, the same command launches through
the [GUI-TUI Bridge](#guitui-app-bridge) instead — a real X11 app (e.g.
`firefox`, `gimp`), half-block rendered, exactly like `bridge:firefox`
elsewhere in this manual. **Manage Programs…** lists everything you've
added (marking Bridge-backed ones) for deletion, which also drops its
desktop shortcut if it has one.

## Command palette

The palette (`Ctrl+Space` / `Ctrl+P`) is the power-user path — type instead
of navigating menus. It's not a fuzzy app launcher bolted onto the desktop;
it's meant to be a genuinely faster way to drive the whole thing.

<p align="center">
  <img src="images/palette.png" alt="Command palette open with fuzzy results" width="640">
</p>

Queries parse loosely as `verb rest`:

| Query | What happens |
|---|---|
| `open notes` / `notes` | Launch or focus Notes |
| `find readme` | Fuzzy file search / scrollback find |
| `calculate 0xff * 16` (or `= 2**10`) | Inline integer expression; Enter copies the result |
| `run htop` | Opens a PTY window running `htop` |
| `ssh myserver` | Opens a PTY window running `ssh myserver` |
| `wifi connect` | Runs whatever `recipes` entry matches, if configured |

Unrecognized text falls back to fuzzy-matching against app titles, desktop
icons, Start menu programs, and open windows ("focus terminal"). Recipes
(`recipes` in config) are your own shortcuts — a match string, an action,
and an optional `confirm: true` that makes the palette require a second
Enter before running it (useful for anything that shells out to `sudo`).
History persists across restarts (`palette_history.json`); with an empty
query, the palette shows your recent picks first.

Settings → Input can remap the palette/Start hotkeys, and there's a
"replace Start with palette" toggle for people who want one chord for
everything (`palette.start_opens_palette`).

## Desktop icons, wallpaper & themes

Desktop icons are config data (`desktop_icons` — label, optional
emoji/icon, `X`/`Y` cell position, launch action), not a hardcoded list —
drag one to reposition, and the new position is saved. `Icon` is just a
string: an emoji renders as-is; leave it blank (or turn on
Settings → Input → ASCII icon substitutes) to use `■`-style stand-ins
instead, for terminals/fonts that don't render emoji glyphs.

Wallpaper modes (Settings → Appearance, or `wallpaper.*` in config):

- `solid` — a flat color.
- `pattern` — a repeating single character/emoji across the desktop.
- `image` — any picture, downsampled through the same truecolor half-block
  encoder (`internal/gfx`) every graphical surface in the desktop uses,
  fit `cover`/`contain`/`stretch`. Five built-ins ship as `builtin:<id>`,
  each paired with a full theme (chrome colors + wallpaper): **XP**
  (Bliss, the default — see the screenshots on this page), **Scarlet**,
  **Bumble**, **Bubble**, **Sprout**. Switch instantly from Settings or the
  palette (`scarlet`, `bumble`, …) — picks apply and save immediately, no
  separate save step.

Over SSH, `wallpaper.ssh_mode` defaults to forcing solid — redrawing a
bitmap wallpaper every frame over a laggy link is a bad trade; set it to
`keep` if you'd rather keep the real wallpaper remotely anyway.

## Files

The native Files app (`apps/files`, Start ▸ System ▸ Files, or a desktop
icon) is a real folder manager, not a picker dialog: list/grid views, a
scrollbar, mkdir, rename, cut/copy/paste, and a trash (not a hard delete).
Opening a file routes through the same association rules `ctx.OpenPath`
uses everywhere else in the desktop (desktop icons, the palette's `find`,
Add Program's file browser):

| File kind | Opens with |
|---|---|
| Text extensions | `Roles.Editor` (`nano` by default) |
| Images (`png`/`jpg`/`jpeg`/`gif`/`webp`/`bmp`) | `Roles.Image` (Image Viewer) |
| Anything else unmapped | A notification telling you no default app is set — never a silent no-op |

Per-extension overrides live under `associations` in config (e.g.
`"md": "pty:nvim"`); unmapped ones fall through to `role:editor`/
`role:image` so changing your editor role updates every text association
at once. Settings → Default apps has one-click editor cycling (nano / nvim
/ vim / emacs) plus a reset-to-defaults button. An App Store app can also
take over the Files role entirely via `set_role: filemgr` (see
[App Store](#app-store)) — Start ▸ Files, desktop icons, and "open this
folder" all redirect to it until you revert the role in Settings.

## Settings

A real control panel (native `uiapp`, not hand-edited JSON), reachable from
Start ▸ System ▸ Settings, the palette, or `settings` on the command line
of the config path. Every change applies and saves immediately — no
separate Save step, no "are you sure you want to discard changes."

<p align="center">
  <img src="images/settings.png" alt="Settings app open over the desktop" width="640">
</p>

| Section | Covers |
|---|---|
| **Appearance** | Wallpaper mode/path/fit, taskbar dock edge, theme packs |
| **Desktop** | Icons on/off, SSH solid-wallpaper override, session restore / open-terminal-on-start, autostart list |
| **Terminal** | Shell, scrollback size, FPS (separate local vs. SSH budgets) |
| **Notifications** | Banners on/off, auto-dismiss timing, SSH badge-only mode, a test-notification button |
| **Default apps** | Editor/image/file-manager/browser roles, association overrides |
| **Apps** | Add/Manage Programs, a summary of what each role currently points at, System folder shortcut |
| **Input** | Remappable hotkeys, Start-opens-palette toggle, ASCII icon substitutes |
| **Advanced** | Log file path (+ open-log shortcut), config file paths, the FPS actually in effect right now |

Config is additive-only as a matter of policy once the project hits 1.0 —
see [Config, versioning & the stability policy](#config-versioning--the-stability-policy).

## Calendar

Click the taskbar clock, or open it from Start/the palette. Month view plus
a day agenda; create and edit local events directly (no account needed for
this part). Reminders don't have their own bespoke toast UI — they post
into the same desktop-wide [notification service](#notifications)
everything else uses, so a Calendar reminder looks and behaves exactly like
any other notification.

**Google Calendar / Microsoft Graph sync** is opt-in, read-only (fetch and
merge, not two-way), and per-provider from Settings → Calendar: paste in
the Client ID from your own OAuth app (Google Cloud Console / Azure
Portal — there's no ttypedesk-wide client ID, see `docs/calendar.md` for
why) and connect through a real browser consent flow. A local loopback
HTTP server catches the redirect; the consent URL is also shown directly
in Settings, since plenty of this project's users have no local browser
at all over SSH. Sync runs when Calendar opens and on demand (**S** key)
— not a standing background service, so a synced event's reminder is only
as fresh as the last time Calendar was opened. Tokens live under
`~/.config/ttypedesk/credentials/` (never in `config.json`, never in git);
a refreshed access token is saved back automatically, so reconnecting is
never required just because a short-lived token expired.

## App Store

Start ▸ App Store installs extra apps from configured GitHub catalogs — the
default is [ttypedesk-apps](https://github.com/RetroCodeRamen/ttypedesk-apps).
No app-specific logic lives in the desktop binary itself; the App Store is
a generic engine that fetches a catalog's `index.json`, runs each entry's
`detect` check to see what's already installed, and installs the rest on
request.

- **Install** downloads the entry's install script and runs it via
  `pty:bash <script>` in a real terminal window — interactive, unsandboxed,
  same trust model as running any command yourself (it may prompt for a
  sudo password right there in the window).
- **Trust**: entries from a source you haven't trusted yet show
  `⚠ unverified source`. The first Enter/click arms a warning instead of
  installing; a second confirms and trusts that source going forward
  (persisted to config). The default catalog ships pre-trusted.
- Once `detect` passes (checked every ~2s while installing), the entry
  flips to **Installed**, its launcher(s) register into Start ▸ Programs
  automatically, and a notification fires. A newly-registered app never
  silently overwrites an unrelated existing program that happens to share
  its name — it gets a `-store` suffix instead so both coexist.
- An entry can optionally set `set_role` to replace a built-in app (file
  manager, editor, browser, terminal, image viewer) desk-wide, not just add
  itself to the menu — see [Files](#files) for what that looks like from
  the user side, and `docs/appstore.md` for the full catalog format if
  you're writing your own.

Add more sources under `app_sources` in config to pull from additional
catalogs.

## Notifications

A desktop-wide service (`internal/notify`) — any app can post to it
(Calendar reminders, Settings' test button, App Store install completions,
future apps), and it's not owned by any one of them. A banner pops near the
taskbar and auto-dismisses; the tray bell (`🔔`, with an unread badge)
opens the Notification Center for anything you missed or want to review
again. Settings controls banner visibility, auto-dismiss timing, and an
SSH-specific "badge only, no popup" mode for links where a banner mid-frame
would just be noise. Session history can optionally persist to
`notifications.json` across restarts.

## Amp (audio player)

Start ▸ Programs ▸ Amp, or the palette (`amp`). A small Winamp-flavored
player: **O** opens a file (through the same modal [file picker](#files)
Vid also uses), **Space** plays/pauses, **N**/**P** skip next/previous,
**S** stops, **D** removes the selected playlist row, arrows navigate the
playlist, Enter or a click plays a row directly.

Decoding is always an `ffmpeg` subprocess — never a linked decoder
library, so the desktop binary itself stays free of format-specific code.
`ffmpeg` is a soft runtime dependency exactly like `Xvfb` for the
[GUI–TUI Bridge](#guitui-app-bridge): only needed if you actually open
Amp, checked at launch, with a clear status message (not a crash) if it's
missing. The visualizer bars are real peak amplitude computed from the
decoded PCM stream in fixed windows — not a true FFT spectrum, deliberately;
a proper spectrum analyzer was more complexity than a v1 visualizer needs.

Pause is a real pause, not stop-and-restart: it maps directly onto the
underlying audio player's own pause/resume, which also means pausing
doubles as backpressure on the whole pipeline for free — once playback
stops draining decoded samples, the feeding goroutine blocks, which blocks
`ffmpeg`'s own stdout write, with nothing needing an explicit pause signal
of its own. Track position shown in the transport is wall-clock time since
play started (`pkg/uiapp.MediaClock`), not a sample-accurate decode
position — accurate for real-time playback (audio hardware paces itself),
but there's no scrub/seek in this first pass, only Amp's own next/previous.

## Vid (video player)

Start ▸ Programs ▸ Vid, or the palette (`vid`). Terminal-native video —
live frames decoded straight to half-block cells, not a GUI nest running
a real video player (that's what the [GUI–TUI Bridge](#guitui-app-bridge)
is for, e.g. `bridge:mpv`, if you want a raster-only alternative). **O**
opens a file (the same modal [file picker](#files) Amp uses), **Space**
plays/pauses, **Left**/**Right** seek 5 seconds back/forward, **S** stops.

Video decode is `ffmpeg`, always — piped raw RGB24 frames, pre-scaled by
`ffmpeg` itself to roughly the pixel resolution the current window can
actually show (no point piping full source resolution just to downsample
it a moment later), then run through the exact same half-block encoder
(`internal/gfx.EncodeHalfBlockFit`) wallpaper images and the Bridge use.
The file's audio track (if it has one) decodes through a **second**,
independent `ffmpeg` process into the same `Host.PlayAudio` path Amp
uses — not multiplexed through one process. A file with no audio track,
or one whose audio `ffmpeg` can't extract for whatever reason, isn't
fatal: video keeps playing without sound rather than failing outright,
the same "degrade the specific thing, not the whole feature" posture as
the Bridge's AT-SPI text overlay.

**Scrub** (Left/Right) isn't a true random-access seek — there's no such
thing against a live raw-frame pipe. It works by tearing down both
decode processes and restarting them with an input-side `ffmpeg -ss`
(a fast, keyframe-ish seek, not frame-accurate), which is exactly what
happens on Play if you seek while paused too. Frame rate adapts down
over SSH the same blunt way the Bridge's does today: a fixed lower
budget (`config.OverSSH()`), not a live-measured one — resolution itself
doesn't adapt yet.

## Chat (decentralized LAN messenger)

Start ▸ Programs ▸ Chat, or the palette (`chat`). A fully decentralized,
peer-to-peer chat for a trusted local network — there's no server, no
homeserver, and no internet dependency; TTYPE Desk never runs or manages
a chat backend at all, on the LAN or otherwise. Peers find each other via
a UDP broadcast on the LAN and talk directly over TCP.

First launch asks for a display name (no login, no account) — this,
together with a keypair generated and saved automatically the first
time, is your identity to everyone else on the LAN. **Enter** confirms
it. After that: **Tab** switches focus between the room list, the peers
list, and the compose box; **Up**/**Down** navigate whichever list is
focused; **Enter** selects a room, opens a DM with the selected peer, or
sends whatever's in the compose box, depending on which panel has focus.
Selecting **+ New room** at the top of the room list prompts for a name;
**Enter** creates it, **Escape** cancels. The peers list shows everyone
currently visible on the LAN (● online, ○ known but not currently seen)
— selecting one opens (or creates) a direct message with them.

Anyone can create a room, and a room's history syncs across every LAN
computer that's a member of it: join a room and you converge on the same
history everyone else in it already has, not just messages sent after
you joined. Each computer keeps only the most recent 500 messages per
room, saved to `~/.config/ttypedesk/lanchat/` so history survives a
restart. A message landing in a room other than the one currently in
view posts a desktop notification, the same "somewhere you're not
looking" heuristic other apps in TTYPE Desk use.

Messages are **signed but not encrypted** — you can tell who really sent
something and that it wasn't tampered with in transit, but content
itself travels in the clear on the LAN. That's a deliberate scope
decision for a trusted home/office network, not an oversight; hand-rolled
end-to-end encryption done as an afterthought is a common source of real
vulnerabilities. Settings ▸ LAN Chat lets you change your display name
later, or regenerate your identity outright (immediate, no confirmation
— you'll appear as a brand-new, unrecognized peer to everyone else
afterward, since there's no central authority to reassign an old
identity to a new key).

### Matrix Chat

The previous built-in messenger — a federated [Matrix](https://matrix.org/)
client that connects to an existing homeserver you already have an
account on (`matrix.org` or any other), built on
[mautrix-go](https://github.com/mautrix/go) — has moved out of the core
binary. It's still fully maintained and installable from the App Store
(App Store ▸ Matrix Chat); once installed it runs and behaves exactly as
it always did (login screen, room list, timeline, session persisted
under `~/.config/ttypedesk/credentials/`, no E2EE in this first phase).
See the App Store section below for installing it.

## Terminal features

Every PTY window is a real terminal, not a toy: full VT100/xterm parsing
via a vendored libvterm (`third_party/libvterm-0.3.3`, wrapped by
`internal/vterm`), not a reimplementation.

- **Copy-mode** (`F8`) — keyboard-driven scrollback selection, tmux-style:
  `hjkl`/arrows move (scrolling into history at the edges), `g`/`G` jump to
  top/bottom, `Space`/`v` starts a selection, `Enter`/`y` copies it, `Esc`/
  `q` cancels. Reuses the same render/copy path as mouse selection.
  Mouse selection also works directly: drag to select (or `Shift+drag` if
  the program underneath has its own mouse mode enabled), and a selection
  auto-copies on mouse-up.
- **Scrollback search** — `F3` / `Alt+/` (or `Ctrl+Shift+F` if your host
  terminal doesn't steal it first) opens a find bar; `Enter`/`Shift+Enter`
  jump between matches with yellow highlights.
- **Scrollback scrollbar** — wheel scrolls, or drag the thumb/click the
  track on the right border; a visible indicator shows how far back you
  are.
- **Clipboard** — OSC 52 (works over SSH without any host-side clipboard
  tool) with `wl-copy`/`xclip`/`xsel` fallbacks for local sessions.
  `Ctrl+Shift+C` copies the current selection explicitly;
  middle-click/`Ctrl+V` pastes.

## GUI–TUI App Bridge

The Bridge (`internal/bridge`) is how a real, unmodified X11 GUI
application ends up rendered as a window inside the desktop, using the same
technique Browsh popularized for web pages — off-screen capture → cell
grid → floating window — generalized to arbitrary X11 apps instead of just
a browser engine (TTYPE Desk doesn't embed or depend on Browsh itself).

The easiest way to launch one: **Add Program…** (Start ▸ Programs ▸ Add
Program) with **"Launch via GUI-TUI Bridge"** checked — enter the X11
command (`firefox`, `gimp`, any X11 app) and it's registered as a normal
Start-menu/taskbar/palette-searchable launcher that opens through the
Bridge. Under the hood, that's `bridge:<command>` as a launch action —
also usable directly by hand-editing a `desktop_icons` or `recipes` entry
in `config.json` (no in-app editor for either yet), same syntax
`pty:`/`prog:`/etc. use. Under the hood:

- A dedicated, private `Xvfb` spawns per bridged window (nested — X11
  needed only for this feature, nothing else in the desktop touches it).
  `Xvfb` is a soft dependency: not having it installed only breaks
  `bridge:` windows, nothing else.
- The guest app renders into that virtual display; the Bridge reads it back
  via X11's `GetImage` and encodes it through the exact same half-block
  cell encoder (`internal/gfx`) that wallpaper images and the Image Viewer
  use — no separate graphics path.
- Input goes back the other direction over X11's **XTest** extension —
  keys and clicks/scroll translate from cell coordinates to the guest's
  pixel space.
- **Text legibility**: raw pixels alone can't distinguish text from any
  other content — a text-heavy app degrades into colored noise with every
  cell using the same glyph. For native GTK/Qt apps, the Bridge also walks
  the Linux accessibility tree (AT-SPI2, over a private per-window D-Bus
  session) and overlays real characters on top of the raster wherever it
  finds real text — validated against `zenity`/`gtk3-demo`. This doesn't
  help Electron-based apps (Cursor/VS Code and similar) — confirmed
  directly, and it matches a known open upstream Chromium/Electron
  accessibility bug, not a Bridge limitation. Entirely optional and
  non-fatal: without `dbus`/`at-spi2-core` installed, or against an app
  that doesn't expose usable text, bridged windows just stay raster-only,
  exactly as if this feature didn't exist.
- **Perf**: the live capture buffer is deliberately sized with headroom
  beyond the window's current size (so small resizes don't need a full
  Xvfb resize), the capture cadence adapts down automatically over SSH
  when a frame gets expensive to produce, and growing a window past that
  headroom triggers a live RANDR resize of the virtual screen instead of
  just upscaling a fixed-resolution buffer forever.

Not every GUI-adjacent idea lives here: **RemoteNest** (RDP/VNC into the
same path) was evaluated and dropped — the only well-maintained Go RDP
client is GPL-3.0-licensed, which isn't a tradeoff worth making for this
feature. **BrowserNest**, a dedicated headless-browser backend, was
dropped too — `bridge:firefox` through the generic path above already
covers "browse the web" without a second, narrower implementation.

## Remote attach

A thin remote session, distinct from SSHing into a shell and running
`ttypedesk` there yourself (which also works fine, and is the simpler
option if you just want the whole desktop remotely):

```bash
# on the machine actually running the desktop
./bin/ttypedesk -listen /tmp/ttypedesk.sock

# from anywhere else with access to that socket
./bin/ttypedesk -attach /tmp/ttypedesk.sock
```

The attach client forwards keyboard and mouse (press/drag/release/wheel)
to whatever's focused/on-screen on the host, and renders the content of
each window it's told about. `Ctrl+Q` detaches cleanly. Window chrome —
dragging, resizing, the taskbar, the Start menu — stays host-side only;
this is a content-forwarding session, not a second seat at the same
desktop. Wire format is a length-prefixed binary framing: window metadata
(position, size, title, focus) goes out every tick, but a window's cell
grid is only resent once it actually changes since the last frame that
connection saw — each attach connection tracks its own diff state, so
several viewers (or a viewer plus the host's own render loop) never step
on each other's cache. Cheaper than resending every cell as JSON on every
tick, which matters most over SSH — see `docs/ssh.md` for the
streaming-over-SSH notes generally.

Turn on **Settings → Audio streaming** and whatever's audible on the host
(desktop sounds, Amp, Vid, a bridged app) plays on the attached client too
— captured desktop-wide off the default sink's monitor and muxed onto the
same socket as the rest of attach traffic, no second connection or port.
Mute in Settings takes effect immediately without reattaching; Enabled
takes effect on the next `-attach`. See `docs/audio-stream.md` for how it
works.

## Config, versioning & the stability policy

`~/.config/ttypedesk/config.json` holds everything: theme, taskbar dock,
desktop icons, wallpaper, notifications, associations, App Store sources,
Files options, hotkeys, recipes. It's hot-reloaded — edit it by hand, let a
sync tool write it, or run a second instance pointed at the same file, and
the running desktop picks up the change within about two seconds, live,
no restart.

Versioning is `MAJOR.MINOR.YYMMNN` (e.g. `1.0.260752`): `MAJOR.MINOR` is
hand-bumped in `VERSION`, `YYMMNN` is year + month + a running count of
commits so far that calendar month, computed by `scripts/version.sh` and
baked into the binary at build time. `ttypedesk -version` prints what's
actually running. Every commit gets auto-tagged (`v1.0.260752`, …) via a
`post-commit` git hook.

Major version `1` means two specific surfaces stop moving casually:

- **`config.json`** stays additive-only — new fields always ship with a
  safe zero-value default so an old config keeps loading, existing
  field names/types don't change, nothing gets silently repurposed.
- **The App SDK** (`pkg/uiapp` — `App`, `Host`, `Context`, `Canvas`, the
  interface every native app is written against, built-in or third-party)
  gets the same treatment.

Everything else — internal packages, the App Store catalog format, CLI
flags — can still change after 1.0; those were never the stability
contract.

## Troubleshooting

TTYPE Desk is a desktop environment hand-rolled inside a terminal emulator,
which is itself usually inside another terminal emulator. Something will
eventually go sideways, and when it does it leaves a note instead of
quietly dying:

- **Log file**: `~/.config/ttypedesk/ttypedesk.log` (override with
  `$TTYPEDESK_LOG`). Panics and launch-action failures land here — check
  this first when something freezes or a window won't open.
  `$TTYPEDESK_LOG_LEVEL` (`debug`/`info`/`warn`/`error`, default `info`)
  controls verbosity.
- **App crashes are isolated**: a panicking native app shows a red crash
  screen inside its own window (`Esc`/`Q` closes it) instead of taking the
  whole desktop down. The main event/draw loop itself also recovers from a
  panic and keeps going where it can — one misbehaving window shouldn't
  cost you every other one.
- **Colors look wrong / wallpaper looks flat**: your host terminal likely
  isn't advertising truecolor. Set `COLORTERM=truecolor` (most modern
  terminals support this; some need it set explicitly).
- **A hotkey does nothing**: several defaults (`Alt+Space`, `Ctrl+Shift+F`)
  are commonly intercepted by host terminals or window managers before
  they ever reach TTYPE Desk. Use the documented alternate binding (`F10`/
  `Ctrl+Esc`, `F3`/`Alt+/`) or remap it in Settings → Input.
  `bridge:` windows need `Xvfb` on `PATH`; the AT-SPI text overlay
  additionally wants `dbus-daemon` + `at-spi2-registryd`
  (`at-spi2-core`) — both are soft dependencies, so their absence degrades
  the specific feature rather than failing to start.
