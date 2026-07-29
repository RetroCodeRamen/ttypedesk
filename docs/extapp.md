# Out-of-process App SDK

TTYPE Desk's native apps (`apps/notes`, `apps/files`, `apps/settings`, …)
implement `pkg/uiapp.App` and run in-process, linked into the same Go
binary. This is the other way to write an app: any executable, in any
language, speaking a small NDJSON protocol over its own stdin/stdout.
The desktop spawns it as a subprocess (`internal/surface.ExtAppSurface`)
and treats the window exactly like a native one — same taskbar entry,
same focus/resize/close behavior, same crash isolation.

Launch one with an action string of `extapp:/path/to/binary [args...]`
(a Start menu entry, a hotkey, a palette command, a recipe — anywhere a
launch action is accepted), the same way `pty:` and `bridge:` work.

A full, runnable reference implementation (in Go, but only stdin/stdout
matter — nothing about the protocol requires Go) lives in
[`cmd/extapp-hello`](../cmd/extapp-hello/main.go).

## Transport

Newline-delimited JSON (NDJSON): one `internal/proto.Envelope` per line,
on stdin (host → app) and stdout (app → host).

```json
{"v":1,"type":"init","payload":{"window_id":"w3","cols":40,"rows":12}}
```

- `v` — protocol version (currently always `1`).
- `type` — message type, see below.
- `payload` — type-specific, may be omitted for types that carry none.

Write one compact JSON object per line, `\n`-terminated, flushed
immediately — don't buffer past a line boundary. A line the host can't
parse is silently skipped, not treated as fatal, so one bad log line
mixed into stdout by accident won't tear down the window (but don't rely
on that — write only protocol messages to stdout; use stderr for your own
logging).

## Message types

### Host → app

| Type | Payload | Meaning |
|---|---|---|
| `init` | `{window_id, cols, rows}` | Sent once, right after spawn. Reply with `ready`. |
| `resize` | `{cols, rows}` | Window resized; redraw at the new size and send a fresh `screen_diff`. |
| `key` | `{rune, key, ctrl, alt, shift, bytes}` | A key press. `key` is a name (`"Enter"`, `"Escape"`, `"Up"`, …) for non-printable keys; `rune` carries a printable character; `bytes` carries raw bytes for a few control keys (Ctrl+C etc). |
| `mouse` | `{x, y, button, action, ctrl, alt, shift}` | `action` is `"press"`, `"release"`, `"drag"`, or `"wheel"`. Coordinates are relative to the window's content area (0,0 = top-left cell). |
| `focus` | `{focused}` | Window gained (`true`) or lost (`false`) focus. |

### App → host

| Type | Payload | Meaning |
|---|---|---|
| `ready` | `{err?}` | Reply to `init`. Empty/omitted `err` means startup succeeded; a non-empty `err` marks the window crashed with that message (same as an in-process app panicking in `Init`). |
| `screen_diff` | `{diff: {rect, cells}}` | The window's new content. **v1 requires every `screen_diff` to cover the full grid** — `rect` = `{x:0, y:0, w:cols, h:rows}` and `cells` has exactly `cols*rows` entries, row-major. Partial/rect-scoped diffs aren't supported yet; send a full redraw each time, the host doesn't require you to compute what changed. Send one whenever your state changes — not just in reply to a host message; a self-driven redraw (an animation, a clock, a background job finishing) is exactly as valid as a reactive one. |
| `title_changed` | `{title}` | Set the window's title bar text. |
| `notify` | `{title, body, icon?}` | Post a desktop notification (`uiapp.Host.Notify`). |
| `launch` | `{action}` | Run a desktop launch action (`uiapp.Host.Launch`) — e.g. open another app. |
| `open_path` | `{path}` | Open a file/directory with the desktop's default-app associations (`uiapp.Host.OpenPath`). |
| `close_window` | *(none)* | Ask the host to close this window (`uiapp.Host.RequestClose`) — e.g. the user pressed a Quit key inside the app. |

`cell` matches `pkg/cell.Cell`: `{"rune": 65, "fg": {"r":255,"g":255,"b":255}, "bg": {"r":0,"g":0,"b":128}, "attr": 0}` — `rune` is a Unicode code point (not a UTF-8 byte), colors are 24-bit RGB, `attr` is a bitmask (bit 0 = bold, 1 = underline, 2 = italic, 3 = blink, 4 = reverse, 5 = strike — see `pkg/cell.Attr`).

## Lifecycle

1. Host spawns the process and sends `init`.
2. App replies `ready`, then sends an initial `screen_diff`.
3. Host forwards `key`/`mouse`/`resize`/`focus` as they happen; app sends
   `screen_diff` (and, as needed, `title_changed`/`notify`/`launch`/
   `open_path`/`close_window`) whenever it has something to say — there's
   no requirement to reply to any particular host message.
4. Window closing: the host closes the app's stdin. A well-behaved app
   sees EOF on its next stdin read and exits on its own. If it doesn't
   exit within a couple of seconds, the host kills the process.
5. If the process exits unexpectedly (crashes, or exits before the host
   closed its stdin), the window shows a crash screen — same as an
   in-process app panicking — and the rest of the desktop keeps running.

## What's not in v1

- **Partial diffs.** Every `screen_diff` must be a full-grid redraw (see
  above). Fine for typical text-UI sizes; revisit if a real app finds
  full redraws too expensive.
- **`SaveCredential`/`LoadCredential`, `PickFile`, `PlayAudio`.** These
  exist on `uiapp.Host` for in-process apps but have no wire message yet.
  An out-of-process app that needs credential storage, a file picker, or
  audio playback isn't covered by v1 — add the corresponding message type
  when a real app needs it, rather than speculatively now.
- **Periodic timer ticks.** There's no `timer` message — a `screen_diff`
  can be sent at any time regardless of host messages, so an app wanting
  to animate independently just runs its own ticker and pushes redraws
  on its own schedule (see `cmd/extapp-hello`'s clock for exactly this).
