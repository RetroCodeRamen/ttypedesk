package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/calsync"
	"github.com/ttypedesk/ttypedesk/internal/config"
)

// syncWindow is how far ahead/behind "now" a sync fetches — wide enough to
// cover reminders and a reasonable planning horizon without pulling a
// calendar's entire history on every sync.
const (
	syncBehind = 7 * 24 * time.Hour
	syncAhead  = 60 * 24 * time.Hour
)

// syncResult is how syncOne's background goroutine reports back — never
// by touching App fields directly (Handle/Draw run under AppSurface's own
// lock, but a goroutine started here would bypass it entirely). Draining
// this channel happens only from Draw, i.e. only ever on the single
// thread that already owns every other App field.
type syncResult struct {
	provider string
	events   []calsync.SyncedEvent
	err      error
}

func (a *App) hasEnabledAccount() bool {
	for _, acc := range a.cfg.Calendar.Accounts {
		if acc.Enabled {
			return true
		}
	}
	return false
}

// syncAll fetches events for every enabled account and merges them into
// a.events, one goroutine per account so a slow/unreachable provider
// doesn't block the others. Safe to call with no accounts configured (a
// no-op) and safe to call again while a previous sync is still running.
func (a *App) syncAll() {
	if a.ctx == nil || !a.hasEnabledAccount() {
		return
	}
	if a.syncResults == nil {
		a.syncResults = make(chan syncResult, 4)
	}
	for _, acc := range a.cfg.Calendar.Accounts {
		if !acc.Enabled {
			continue
		}
		a.pendingSyncs++
		a.syncing = true
		go a.syncOne(acc)
	}
}

// syncOne runs on its own goroutine — it must never touch App fields
// directly (see syncResult's doc comment). Context's Load/SaveCredential
// and MarkDirty are themselves safe to call from any goroutine (Context
// already protects its internal state with its own mutex).
func (a *App) syncOne(acc config.CalendarAccount) {
	provider := calsync.Provider(acc.Provider)
	tokenKey := "calendar." + acc.Provider + ".token"
	ctx := a.ctx

	loadToken := func() ([]byte, error) { return ctx.LoadCredential(tokenKey) }
	saveToken := func(tok []byte) error { return ctx.SaveCredential(tokenKey, tok) }

	now := time.Now()
	events, err := calsync.FetchEvents(context.Background(), provider, acc.ClientID,
		now.Add(-syncBehind), now.Add(syncAhead), loadToken, saveToken)

	res := syncResult{provider: acc.Provider, events: events, err: err}
	select {
	case a.syncResults <- res:
	default:
		// Channel briefly full (many accounts finishing at once) — drop
		// rather than block; the next MarkDirty-triggered Draw will still
		// pick up whatever did make it through, and a dropped result just
		// means that one account's sync effectively becomes a no-op this
		// round rather than corrupting anything.
	}
	ctx.MarkDirty()
}

// drainSyncResults applies any results a background syncOne call has
// posted since the last Draw. Called only from Draw, i.e. only ever on
// the single thread that owns every other App field — this is the one
// and only place a.events/a.syncing/a.status/a.dirty are touched as a
// result of a sync, however many accounts are syncing concurrently.
func (a *App) drainSyncResults() {
	if a.syncResults == nil {
		return
	}
	for {
		select {
		case res := <-a.syncResults:
			a.applySyncResult(res.provider, res.events, res.err)
		default:
			return
		}
	}
}

func (a *App) applySyncResult(provider string, synced []calsync.SyncedEvent, err error) {
	if a.pendingSyncs > 0 {
		a.pendingSyncs--
	}
	a.syncing = a.pendingSyncs > 0
	if err != nil {
		a.status = fmt.Sprintf("%s sync failed: %v", provider, err)
		return
	}
	a.events = mergeSynced(a.events, provider, synced)
	a.dirty = true
	_ = a.save()
	a.status = fmt.Sprintf("%s synced (%d events)", provider, len(synced))
}

// mergeSynced replaces every existing event whose Source equals provider
// with a fresh conversion of synced, leaving events from every other
// source (local edits, the other provider) untouched. Pure and
// side-effect-free so it's directly testable without any network/
// goroutine involved.
func mergeSynced(existing []Event, provider string, synced []calsync.SyncedEvent) []Event {
	out := make([]Event, 0, len(existing)+len(synced))
	for _, ev := range existing {
		if ev.Source != provider {
			out = append(out, ev)
		}
	}
	for _, s := range synced {
		out = append(out, Event{
			ID:     provider + ":" + s.ID,
			Title:  s.Title,
			Start:  s.Start,
			End:    s.End,
			AllDay: s.AllDay,
			Source: provider,
		})
	}
	return out
}
