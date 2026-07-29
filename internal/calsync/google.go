package calsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// googleEventsURL is a var, not a const, so tests can point it at an
// httptest server instead of the real Google Calendar API.
var googleEventsURL = "https://www.googleapis.com/calendar/v3/calendars/primary/events"

type googleEventsResponse struct {
	Items []struct {
		ID      string         `json:"id"`
		Summary string         `json:"summary"`
		Start   googleDateTime `json:"start"`
		End     googleDateTime `json:"end"`
	} `json:"items"`
}

// googleDateTime is Google Calendar API's event.start/event.end shape:
// timed events set DateTime (RFC3339), all-day events set Date (bare
// YYYY-MM-DD) instead.
type googleDateTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

func (g googleDateTime) parse() (t time.Time, allDay bool, err error) {
	if g.Date != "" {
		t, err = time.Parse("2006-01-02", g.Date)
		return t, true, err
	}
	t, err = time.Parse(time.RFC3339, g.DateTime)
	return t, false, err
}

func fetchGoogle(ctx context.Context, client *http.Client, from, to time.Time) ([]SyncedEvent, error) {
	u, err := url.Parse(googleEventsURL)
	if err != nil {
		return nil, fmt.Errorf("calsync: google: %w", err)
	}
	q := u.Query()
	q.Set("timeMin", from.Format(time.RFC3339))
	q.Set("timeMax", to.Format(time.RFC3339))
	q.Set("singleEvents", "true")
	q.Set("orderBy", "startTime")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("calsync: google: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calsync: google: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calsync: google: HTTP %d: %s", resp.StatusCode, readErrorBody(resp))
	}

	var parsed googleEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("calsync: google: %w", err)
	}
	out := make([]SyncedEvent, 0, len(parsed.Items))
	for _, it := range parsed.Items {
		start, allDay, err := it.Start.parse()
		if err != nil {
			continue // skip events we can't parse rather than failing the whole sync
		}
		end, _, err := it.End.parse()
		if err != nil {
			end = start
		}
		out = append(out, SyncedEvent{ID: it.ID, Title: it.Summary, Start: start, End: end, AllDay: allDay})
	}
	return out, nil
}
