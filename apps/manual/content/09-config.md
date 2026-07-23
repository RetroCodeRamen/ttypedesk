# Config, SSH & troubleshooting

## Important paths

| Path | Purpose |
|------|---------|
| `~/.config/ttypedesk/config.json` | Main settings, icons, theme, associations |
| `~/.config/ttypedesk/session.json` | Session restore snapshot |
| `~/.config/ttypedesk/ttypedesk.log` | Runtime log (panics, launch failures) |
| `~/.config/ttypedesk/System/` | On-disk Manual chapters (markdown) |
| `~/.config/ttypedesk/calendar/` | Local calendar events |

Override log path with `TTYPEDESK_LOG`; level with `TTYPEDESK_LOG_LEVEL=debug|info|warn|error`.

**Hot reload:** `config.json` is watched for changes — edit it by hand (or let another tool write it) and the running desktop picks it up within a couple seconds, no restart needed. Saving from within Settings applies the same way, just instantly instead of on the next poll.

## SSH

Desk detects SSH (`SSH_CONNECTION` / `SSH_TTY`) and can:

- Lower FPS (`ssh_fps`, default often 15)
- Prefer a **solid** desktop (skip heavy wallpaper) unless wallpaper `ssh_mode` is `keep`
- Prefer notification **badge-only** if configured

Attach mode (thin secondary viewer) uses `-listen` / `-attach` sockets — see project docs for remote attach details.

## CLI examples

```text
./bin/ttypedesk
./bin/ttypedesk -e htop
./bin/ttypedesk -image photo.png
./bin/ttypedesk -clock -e vim
./bin/ttypedesk -listen /tmp/ttypedesk.sock
./bin/ttypedesk -attach /tmp/ttypedesk.sock
```

## When something goes wrong

1. Read `~/.config/ttypedesk/ttypedesk.log`
2. Native app crashes show a red in-window crash screen — **Esc** / **Q** closes that window; the desktop keeps running
3. Try Settings → reset theme / desktop icons
4. Need a deeper product map? Source tree `ROADMAP.md` and `docs/` are for developers; **this Manual** is the in-desk user guide

## System folder vs Manual app

- **Manual** — comfortable in-desk reader (Start → System → Manual)
- **System folder** — same chapters as files you can open with your editor association

Both are kept in sync from the version shipped inside TTYPE Desk.
