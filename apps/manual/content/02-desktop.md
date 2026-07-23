# Desktop & icons

The large area behind windows is the **desktop field**. Icons are shortcuts stored in your config — not tied to a special “app type.” An icon is just a label, optional glyph (emoji or ASCII), position, and a launch **action**.

## Default icons

On first run you typically get shortcuts such as:

| Icon | Opens |
|------|--------|
| Terminal | A shell window |
| Files | Folder manager |
| Home | Files at your home directory |
| Trash | Files at the trash folder |
| Notes | Scratch notepad |
| Settings | Control panel |

## Using icons

- **Click** — launch the action
- **Drag** — reposition; layout is saved to `~/.config/ttypedesk/config.json`
- **Settings → Desktop → Reset default desktop icons** — restore the stock set

Hide icons entirely with **Settings → Desktop → Show desktop icons**.

## Actions (what an icon can open)

Examples of action strings:

- `terminal` — new PTY shell
- `files` / `files:home` / `files:trash` / `files:/some/path`
- `files:system` — this Manual’s on-disk System folder
- `settings`, `calendar`, `notes`, `manual`
- `pty:htop` — run a command in a PTY window
- `prog:<id>` — a program you added via **Add Program…**
- `image:/path/to.png` — Image Viewer

Icons are data: pick any emoji or short text as the glyph. There is no fixed emoji→app mapping.
