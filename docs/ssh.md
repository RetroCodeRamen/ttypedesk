# SSH streaming

TTYPE Desk is designed to feel good over **SSH**, not only on a local TTY.

## Principles

1. **Cell diffs over full frames** — window content uses dirty-row updates; avoid blasting the full desktop every tick.
2. **Frame budget** — lower FPS when `SSH_CONNECTION` / `SSH_TTY` is set (default **15** FPS over SSH, **30** locally; configurable).
3. **Cheap wallpaper** — over SSH, solid desktop fill instead of dense pattern redraws when possible.
4. **Truecolor stays on** — modern SSH handles it; we do not strip color by default (optional later).
5. **No chatty side channels** — clipboard OSC 52 only on explicit copy, not every frame.

## Detection

```text
SSH_CONNECTION or SSH_TTY set  →  ssh mode
```

Override FPS with config `fps` / `ssh_fps`, or env `TTYPEDESK_FPS`.

## Attach protocol

`-listen` / `-attach` is a second path for remote UI; SSH-to-the-host-TTY remains the primary “stream the desktop” mode. Both should stay bandwidth-conscious.
