# Settings, notifications & calendar

## Settings

**Start → System → Settings** (focuses an existing Settings window if open).

Pages include:

- **Appearance** — wallpaper modes, theme packs (XP / Scarlet / Bumble / Bubble / Sprout), fit, taskbar dock
- **Terminal & performance** — FPS, SSH FPS, shell, scrollback
- **Desktop** — icons on/off, SSH solid desktop, session restore, open-terminal-on-start, **autostart** (comma-separated actions)
- **Notifications** — banners, dismiss, SSH badge-only, persist history, max history, test, clear
- **Default apps** — editor / image / **browser** roles and file associations (`role:editor`, `{browser}`, …)
- **Apps** — Add / Manage Programs, custom program count, terminal & file-manager roles, System folder
- **Input (hotkeys)** — remappable Desk bindings; Start→palette; ASCII icon substitutes
- **Advanced** — open log file, config/notify paths, effective FPS
- **About** — paths and environment hints

Use **Save configuration** to write `~/.config/ttypedesk/config.json`. Many changes apply live.

App roles can be used in launch strings and recipes: `role:browser`, `{editor}`, `{terminal}`, `{filemgr}`.

## Notifications

Desktop-wide service (not owned by Calendar):

- Popup **banner** near the taskbar (queue, auto-dismiss)
- Tray **🔔** + **Notification Center** (view / dismiss / dismiss all)
- Native apps can call `Context.Notify(...)`
- Over SSH you can prefer badge-only (less visual noise)

## Calendar

Click the taskbar **clock** or open Calendar from Programs.

- Month view + day agenda
- Local events stored under `~/.config/ttypedesk/calendar/`
- Reminders post into the **system notification** service (not a separate toast UI)

Cloud sync (Google / Microsoft) is planned later — see the project ROADMAP in the source tree.
