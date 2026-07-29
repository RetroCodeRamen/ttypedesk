package calendar

import (
	"testing"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/calsync"
)

func TestMergeSyncedReplacesOnlyMatchingSource(t *testing.T) {
	local := Event{ID: "local-1", Title: "Dentist", Source: "local"}
	staleGoogle := Event{ID: "google:old", Title: "Stale", Source: "google"}
	msEvent := Event{ID: "microsoft:1", Title: "Standup", Source: "microsoft"}
	existing := []Event{local, staleGoogle, msEvent}

	now := time.Now()
	synced := []calsync.SyncedEvent{
		{ID: "new1", Title: "Team sync", Start: now, End: now.Add(time.Hour)},
	}

	got := mergeSynced(existing, "google", synced)

	var haveLocal, haveMS, haveStaleGoogle, haveNewGoogle bool
	for _, ev := range got {
		switch ev.ID {
		case "local-1":
			haveLocal = true
		case "microsoft:1":
			haveMS = true
		case "google:old":
			haveStaleGoogle = true
		case "google:new1":
			haveNewGoogle = true
			if ev.Title != "Team sync" || ev.Source != "google" {
				t.Errorf("merged google event = %+v, want Title=Team sync Source=google", ev)
			}
		}
	}
	if !haveLocal {
		t.Error("local event was dropped by a google-source merge")
	}
	if !haveMS {
		t.Error("microsoft event was dropped by a google-source merge")
	}
	if haveStaleGoogle {
		t.Error("stale google event survived the merge — should have been replaced")
	}
	if !haveNewGoogle {
		t.Error("freshly synced google event is missing from the merge result")
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3 (local + microsoft + the one new google event)", len(got))
	}
}

func TestMergeSyncedWithEmptySyncedRemovesAllOfThatSource(t *testing.T) {
	existing := []Event{
		{ID: "google:1", Source: "google"},
		{ID: "google:2", Source: "google"},
		{ID: "local-1", Source: "local"},
	}
	got := mergeSynced(existing, "google", nil)
	if len(got) != 1 || got[0].ID != "local-1" {
		t.Errorf("got %+v, want only the local event to survive", got)
	}
}

func TestMergeSyncedIDsAreProviderPrefixed(t *testing.T) {
	synced := []calsync.SyncedEvent{{ID: "abc"}}
	got := mergeSynced(nil, "microsoft", synced)
	if len(got) != 1 || got[0].ID != "microsoft:abc" {
		t.Fatalf("got %+v, want a single event with ID microsoft:abc", got)
	}
}

func TestApplySyncResultMergesAndClearsSyncingWhenLastPending(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // applySyncResult calls save(), which writes to $HOME
	a := &App{pendingSyncs: 1}
	now := time.Now()
	a.applySyncResult("google", []calsync.SyncedEvent{{ID: "e1", Title: "Standup", Start: now}}, nil)

	if a.syncing {
		t.Error("syncing = true, want false after the only pending sync completed")
	}
	if a.pendingSyncs != 0 {
		t.Errorf("pendingSyncs = %d, want 0", a.pendingSyncs)
	}
	if len(a.events) != 1 || a.events[0].ID != "google:e1" {
		t.Errorf("events = %+v, want one merged google event", a.events)
	}
}

func TestApplySyncResultKeepsSyncingWhileOthersPending(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &App{pendingSyncs: 2}
	a.applySyncResult("google", nil, nil)
	if !a.syncing {
		t.Error("syncing = false, want true — one account still pending")
	}
	if a.pendingSyncs != 1 {
		t.Errorf("pendingSyncs = %d, want 1", a.pendingSyncs)
	}
}

func TestApplySyncResultRecordsErrorWithoutTouchingEvents(t *testing.T) {
	a := &App{pendingSyncs: 1, events: []Event{{ID: "local-1", Source: "local"}}}
	a.applySyncResult("google", nil, errSyncFixture)
	if len(a.events) != 1 || a.events[0].ID != "local-1" {
		t.Errorf("events changed on sync error: %+v", a.events)
	}
	if a.status == "" {
		t.Error("status was not set after a sync error")
	}
}

var errSyncFixture = &fixtureErr{"network unreachable"}

type fixtureErr struct{ msg string }

func (e *fixtureErr) Error() string { return e.msg }
