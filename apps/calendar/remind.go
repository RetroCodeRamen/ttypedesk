package calendar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// StorePath is the local events JSON file.
func StorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "calendar-events.json"
	}
	return filepath.Join(home, ".config", "ttypedesk", "calendar", "events.json")
}

// LoadEvents reads local calendar events (best-effort).
func LoadEvents() []Event {
	data, err := os.ReadFile(StorePath())
	if err != nil {
		return nil
	}
	var evs []Event
	_ = json.Unmarshal(data, &evs)
	return evs
}

// RemindAt is when a reminder should fire for an event.
func RemindAt(ev Event, lead time.Duration) time.Time {
	if ev.AllDay {
		y, m, d := ev.Start.Date()
		loc := ev.Start.Location()
		// All-day: remind at 09:00 local on that day (minus lead).
		return time.Date(y, m, d, 9, 0, 0, 0, loc).Add(-lead)
	}
	return ev.Start.Add(-lead)
}

// DueReminder is an event whose reminder window is active now.
type DueReminder struct {
	Event Event
	At    time.Time
}

// DueReminders returns events that should notify now and have not been
// marked in `already` for today's reminder slot.
func DueReminders(now time.Time, lead time.Duration, already map[string]time.Time) []DueReminder {
	if lead <= 0 {
		lead = 5 * time.Minute
	}
	var out []DueReminder
	for _, ev := range LoadEvents() {
		at := RemindAt(ev, lead)
		// Fire window: [at, at+2m] roughly, or until shortly after event start.
		end := ev.Start.Add(2 * time.Minute)
		if ev.AllDay {
			end = at.Add(15 * time.Minute)
		}
		if now.Before(at) || now.After(end) {
			continue
		}
		key := ev.ID + "|" + at.Format("2006-01-02T15:04")
		if prev, ok := already[key]; ok && prev.Equal(at) {
			continue
		}
		// also skip if we notified this event id today
		if prev, ok := already[ev.ID]; ok {
			py, pm, pd := prev.Date()
			ny, nm, nd := now.Date()
			if py == ny && pm == nm && pd == nd {
				continue
			}
		}
		out = append(out, DueReminder{Event: ev, At: at})
	}
	return out
}
