# Audio streaming (later)

**When:** after the main desktop is solid — not near-term.

**Goal:** when using TTYPE Desk over SSH (or attach), play server-side audio on the **local** machine via a small remote client, ideally tunneled over the same SSH connection.

## Why a client

SSH TTYs don’t carry PCM. Something on the laptop must receive audio and play it (Pulse/PipeWire/ALSA/`afplay`/etc.). TTYPE Desk on the server captures or mixes; the **companion client** renders.

## Sketch

```text
Server (TTYPE Desk host)              Client (your laptop)
┌─────────────────────┐               ┌──────────────────┐
│ App / bridge audio  │               │ ttypedesk-audio  │
│   or Pulse monitor  │──encode──────►│ decode → speakers│
└──────────┬──────────┘   (opus/pcm)  └────────▲─────────┘
           │                                   │
           └──── SSH tunnel / attach mux ──────┘
```

## Transport options (pick later)

1. **SSH `-R`/`-L` TCP** — dedicated port for audio alongside the session (simple).
2. **Same attach mux** — binary frames on the existing `-listen` protocol (`type: audio`) next to cell diffs.
3. **Separate `ssh -W` / unix socket forward** — keep TTY clean.

Prefer (2) long-term so one `ttypedesk` remote client gets UI + sound; (1) is fine for an MVP experiment.

## Capture sources (server)

- Optional: monitor default Pulse/PipeWire sink (desktop-wide)
- Bridge backends (BrowserNest / DisplayNest) may expose their own audio later
- Mute / per-app routing = polish

## Non-goals (v1 audio)

- Perfect low-latency gaming audio
- Bidirectional mic (could be phase 2)
- Replacing PipeWire on the server

## Phasing (when we get here)

1. Spec binary audio frames + tiny `ttypedesk-audio` play-only client  
2. Server: capture default monitor → Opus (or raw PCM for LAN)  
3. Document `ssh -L` one-liner / integrate into attach client  
4. Settings: enable stream, bitrate, mute  
5. Optional mic uplink  

## Dependencies

Stable desktop + attach/SSH story first; notification/calendar/wallpaper/dock higher priority.
