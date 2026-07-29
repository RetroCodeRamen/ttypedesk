# Audio streaming

**Goal:** when using TTYPE Desk over SSH (or attach), play server-side audio on the **local** machine via the same remote client used for the rest of the desktop.

## Why a client

SSH TTYs don't carry PCM. Something on the laptop must receive audio and play it. TTYPE Desk on the server captures the default sink's monitor; the existing `-attach` client plays it back — no separate companion binary.

## How it works

```text
Server (TTYPE Desk host)              Client (-attach)
┌─────────────────────┐               ┌──────────────────┐
│ parec (default sink │──FrameAudio──►│ internal/audio    │
│   .monitor)          │   (raw PCM)   │  .Play → speakers │
└──────────┬──────────┘               └────────▲─────────┘
           │                                   │
           └──── same -listen/-attach socket ──┘
```

`FrameAudio` chunks are muxed onto the same connection as `FrameDiff` cell updates and `FrameJSON` input, using the length-prefixed frame envelope from `internal/proto/binary.go` — one connection carries UI and sound together, no dedicated port or second SSH tunnel. Raw PCM, not a codec: over a local link or an already-compressed SSH transport, the format-negotiation and CPU cost of encoding didn't pay for itself yet.

## Capture (server)

`internal/audiocap` shells out to `parec --device=@DEFAULT_SINK@.monitor` — no linked capture library, matching the project's `ffmpeg`-for-Amp/Vid posture of soft runtime dependencies. This covers PulseAudio directly and PipeWire systems running `pipewire-pulse`, which is the default on nearly every modern desktop distro. It's desktop-wide: whatever's audible on the host — Amp, Vid, a bridge backend, system sounds — is already in the capture, so nothing separately routes those apps into the stream.

## Settings

Settings → Audio streaming: **Enabled** (decided once per attach connection, at connect time) and **Mute** (checked live on every chunk, so it takes effect immediately without reattaching — capture keeps running, chunks are just dropped while muted). No bitrate control: there's no codec in the raw-PCM path to tune.

## Non-goals (still)

- Bidirectional mic uplink — deferred, same as originally planned.
- Perfect low-latency gaming audio.
- An encoded (Opus) path — raw PCM covers the LAN/SSH case; revisit if bandwidth becomes a real complaint.
