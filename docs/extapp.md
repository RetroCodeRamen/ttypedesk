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
- `req_id` — present only on request/response message pairs (credentials,
  the file picker, clipboard reads — see below). The app sets it when
  sending the request; the host echoes it back unchanged on the matching
  reply, so several in-flight requests don't need to resolve in send
  order. Omitted (or ignore it) for every other message type.
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

### Credentials, file picker, clipboard (request/response)

These are request/response pairs — set `req_id` on the request (any string
unique among your own in-flight requests; the host never interprets it,
just echoes it back), match it against the reply's `req_id`.

| App → host | Payload | Host → app reply | Reply payload |
|---|---|---|---|
| `save_credential` | `{key, value}` | `credential_saved` | `{err?}` |
| `load_credential` | `{key}` | `credential_loaded` | `{value?, err?}` |
| `pick_file` | `{start_dir?, extensions?}` | `file_picked` | `{path?, ok}` |
| `clipboard_get` | *(none)* | `clipboard_value` | `{text}` |

`value` (in both credential messages) and `bytes`/PCM fields elsewhere are
always base64 strings on the wire — this is just how every `[]byte` Go
field here gets encoded (`encoding/json`'s default), not a special case
you need to handle differently.

`load_credential`'s `err` is set (and `value` meaningless) when nothing's
been saved under `key` yet — expect this on first run, not just on a real
failure.

`file_picked` doesn't necessarily arrive quickly — it fires whenever the
user actually interacts with the picker window the host opened, which
could be much later than the request (or never, if they just leave it
open — though they can't; picking or Esc/Cancel are the only ways out).
`ok: false` means cancelled, `path` is meaningless.

### Clipboard write (fire-and-forget)

| Type | Payload | Meaning |
|---|---|---|
| `clipboard_set` | `{text}` | Write the shared system clipboard (`uiapp.Host.ClipboardSet`). No reply — nothing meaningful can fail. |

### Audio playback (fire-and-forget)

Not request/response — `play_audio` starts a stream, repeated
`audio_chunk` messages carry PCM, `stop_audio` ends it. No reply to any
of these.

| Type | Payload | Meaning |
|---|---|---|
| `play_audio` | *(none)* | Start streaming to the shared audio output (`uiapp.Host.PlayAudio`). A second `play_audio` before a `stop_audio` is ignored, not a stream swap. |
| `audio_chunk` | `{pcm}` | One chunk of interleaved 16-bit signed little-endian PCM samples, base64-encoded, at the fixed 48kHz/stereo `internal/audio` uses — decode to that rate/channel count, there's no per-call resampling. Send at roughly real-time pace (don't blast the whole track as fast as possible) — see `cmd/extapp-hello`'s `streamSineTone` for a minimal real example, including the exact byte layout via `internal/proto.EncodeAudioChunk`. |
| `stop_audio` | *(none)* | Stop the stream. |

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
- **Periodic timer ticks.** There's no `timer` message — a `screen_diff`
  can be sent at any time regardless of host messages, so an app wanting
  to animate independently just runs its own ticker and pushes redraws
  on its own schedule (see `cmd/extapp-hello`'s clock for exactly this).

As of the request/response messages above, out-of-process apps have full
parity with `uiapp.Host` — credentials, the file picker, clipboard, and
audio playback are all covered, the same as in-process apps get.
