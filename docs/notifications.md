# System notifications

Notifications are a **desktop service**, not a Calendar feature. Calendar (and any other app) can *post* notifications; the shell owns display, history, and dismissal.

## UX

### Banner (toast)

- Short DOS-style popup near the taskbar (opposite the Start button / near the clock).
- Shows icon (optional emoji), title, body (1–2 lines), auto-dismiss after N seconds (configurable).
- Click banner → focus related window / open payload action if any; or open Notification Center.
- Multiple banners queue; show one at a time (or stack max 2–3).

### Tray button (next to clock)

```text
… [window buttons]  🔔3 | 15:04:05
```

- Glyph configurable (default `🔔` or `*N*` ASCII fallback).
- Badge = unread / undismissed count.
- Click → **Notification Center** panel or window: list newest-first, dismiss one / dismiss all, click item to activate.

### SSH

- Prefer shorter banners, lower max stack, optional “badge only” mode to save bandwidth (Settings).

## API (for apps & calendar)

```go
// internal/notify or pkg/notify
type Notice struct {
  ID       string
  Title    string
  Body     string
  Icon     string    // optional emoji/text
  Urgency  Urgency   // low | normal | critical
  Created  time.Time
  Source   string    // "calendar", "app:notes", "system", …
  Actions  []Action  // optional: {ID, Label}
  Data     map[string]string // e.g. event_id, window_id
}

type Service interface {
  Post(Notice) (id string, err error)
  Dismiss(id string) error
  DismissAll() error
  List() []Notice           // undismissed / recent history
  Subscribe(func(Notice))   // for UI shell
}
```

- Native `uiapp` Context gains `Notify(title, body, …)` that posts to the service.
- Out-of-process apps (later) post via proto message `notify`.
- Calendar posts: “Meeting in 5 minutes” with `Source: calendar` and `Data.event_id`.

## Persistence

```text
~/.config/ttypedesk/notifications.json
```

Enabled by default (`notify.persist`). Cap with `notify.max_hist` (default 50). Toggle / clear in Settings → Notifications.

## Settings

- Enable banners on/off  
- Auto-dismiss seconds  
- Persist history + max history  
- SSH: badge-only vs banners  
- Clear notification history  
- Do-not-disturb schedule (later)

## Phasing

1. In-memory `Service` + banner draw in client + 🔔 button + center list/dismiss  
2. Wire `uiapp.Context.Notify` + sample from Settings “Test notification”  
3. Calendar posts upcoming-event notices into the same service  
4. Persist history + DND  

## Non-goals (v1)

- Push from remote machines (can reuse attach protocol later)  
- Full freedesktop org.freedesktop.Notifications D-Bus (nice bridge later, not required)
