# Calendar & tray clock

Clicking the taskbar clock opens a **Calendar** app — month view, local events, and opt-in Google/Microsoft sync.

## Status: local events + read-only sync are built

Everything below through "Sync providers" is implemented, not just designed. The one thing that changed from the original sketch: sync is **read-only** (fetch and merge remote events into the local store), not read/write — writing events back to Google/Microsoft isn't built. `RemoteID` also isn't a separate field; the merged `Event.ID` is `"<provider>:<remote id>"` directly, which is enough to know which events came from where and to replace them cleanly on the next sync.

## UX

1. Click clock (or `| HH:MM:SS` region) → focus/open Calendar window (or popover month panel).
2. Month grid + selected day agenda.
3. Create/edit/delete events (title, start/end, all-day, notes).
4. Notifications: post to the **system notification service** (banner + center) — see [notifications.md](notifications.md). Calendar does not own UI for toasts.
5. Settings → Calendar: enable providers, default reminder lead time, timezone.

## Data (local first)

```text
~/.config/ttypedesk/calendar/events.json     # local store — all events, all sources, one file
~/.config/ttypedesk/credentials/             # OAuth tokens (mode 0600, internal/credstore) — never in git
```

Event model (as shipped — `apps/calendar/calendar.go`):

```go
type Event struct {
  ID     string    // "local" events: "e<unix-nano>"; synced: "<provider>:<remote id>"
  Title  string
  Start  time.Time
  End    time.Time
  AllDay bool
  Notes  string
  Source string // "local" | "google" | "microsoft"
}
```

## Sync providers (shipped)

| Provider | Approach |
|----------|----------|
| Google | OAuth2 (PKCE, loopback redirect) + Calendar API v3, read-only |
| Microsoft | OAuth2 (PKCE, loopback redirect) + Graph `calendarView`, read-only |

Implementation: `internal/calsync` (OAuth2 flow + per-provider fetch/parse),
`apps/calendar/sync.go` (merge into the local store), Settings → Calendar
(`apps/settings/calendar_page.go`, connect/disconnect UI).

- Sync is **opt-in** and per-provider in Settings → Calendar — a user pastes
  in their own OAuth Client ID (their own Google Cloud Console / Azure
  Portal app; there's no ttypedesk-wide client, see `internal/calsync`'s
  package doc for why) and connects through a real browser consent flow
  (a local loopback HTTP server catches the redirect; the consent URL is
  also shown in Settings directly, since many users of this project have
  no local browser at all over SSH).
- Merge policy: on each sync, every existing event whose `Source` matches
  the provider just synced is replaced wholesale by that sync's results;
  events from every other source (local edits, the other provider) are
  left untouched. There's no per-event diffing/conflict resolution — a
  provider's event set is just fully refreshed each time.
- Sync runs when the Calendar app is opened, and on demand (**S** key) —
  not a standing background service. A remote event's reminder is only as
  fresh as the last time Calendar was opened or synced; see "Non-goals".
- Token refresh is automatic and silent (`internal/calsync`'s
  `savingTokenSource` persists a refreshed access token back to
  `credstore` the moment it changes) — a user never needs to re-run the
  browser flow just because a short-lived access token expired.

## Notifications

- Calendar (and only calendar) must **not** draw its own toast UI.
- Reminder worker calls `notify.Service.Post(...)` with source `"calendar"`.
- User sees banner + 🔔 center like any other notice; dismiss works the same way.

## Phasing

1. ~~Clickable clock → month UI + **local** events only~~
2. ~~Notification toasts for local events → via **system notify service**~~
3. ~~Google Calendar OAuth sync~~
4. ~~Microsoft Graph sync~~
5. Two-way sync (write events back), background/standing sync (not just on-open), multi-calendar-per-account, real conflict resolution — none of these are built; see "Non-goals"

## Non-goals (current)

- Writing events back to Google/Microsoft (read-only sync only)
- A standing background sync service — sync only runs while the Calendar
  app is (or was recently) open
- Full CalDAV client
- Email invites / RSVP
- Complex recurrence UI (Google/Graph already expand recurring events into
  single instances via `singleEvents`/`calendarView`, so this mostly
  doesn't come up in practice, but there's no RRULE editing UI for local
  events)
