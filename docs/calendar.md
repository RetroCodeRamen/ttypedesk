# Calendar & tray clock (design)

Clicking the taskbar clock opens a **Calendar** app — month view, local events, later cloud sync and notifications.

## UX

1. Click clock (or `| HH:MM:SS` region) → focus/open Calendar window (or popover month panel).
2. Month grid + selected day agenda.
3. Create/edit/delete events (title, start/end, all-day, notes).
4. Notifications: post to the **system notification service** (banner + center) — see [notifications.md](notifications.md). Calendar does not own UI for toasts.
5. Settings → Calendar: enable providers, default reminder lead time, timezone.

## Data (local first)

```text
~/.config/ttypedesk/calendar/
  events.jsonl          # local store
  credentials/          # OAuth tokens (mode 0600) — never in git
```

Event model (sketch):

```go
type Event struct {
  ID        string
  Title     string
  Start     time.Time
  End       time.Time
  AllDay    bool
  Notes     string
  Source    string // "local" | "google" | "microsoft"
  RemoteID  string
}
```

## Sync providers (phase after local works)

| Provider | Approach |
|----------|----------|
| Google | OAuth2 + Google Calendar API (read/write) |
| Microsoft | OAuth2 + Microsoft Graph calendar |

- Sync is **opt-in** and per-account in Settings.
- Conflict policy v1: last-write-wins with remote id; show source badge on events.
- Offline: local queue of mutations; flush when online.

## Notifications

- Calendar (and only calendar) must **not** draw its own toast UI.
- Reminder worker calls `notify.Service.Post(...)` with source `"calendar"`.
- User sees banner + 🔔 center like any other notice; dismiss works the same way.

## Phasing

1. Clickable clock → month UI + **local** events only  
2. Notification toasts for local events → via **system notify service**  
3. Google Calendar OAuth sync  
4. Microsoft Graph sync  
5. Two-way sync polish / multi-calendar calendars list  

## Non-goals (v1)

- Full CalDAV client (can revisit)
- Email invites / RSVP
- Complex recurrence UI (support RRULE later; v1 = single + simple daily/weekly)
