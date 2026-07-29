package calsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// msEventsURL is a var, not a const, so tests can point it at an
// httptest server instead of the real Microsoft Graph API.
var msEventsURL = "https://graph.microsoft.com/v1.0/me/calendarView"

type msEventsResponse struct {
	Value []struct {
		ID       string     `json:"id"`
		Subject  string     `json:"subject"`
		IsAllDay bool       `json:"isAllDay"`
		Start    msDateTime `json:"start"`
		End      msDateTime `json:"end"`
	} `json:"value"`
}

// msDateTime is Microsoft Graph's event.start/event.end shape: a
// DateTime with no UTC offset, whose zone is given separately (fixed to
// UTC here via the "Prefer: outlook.timezone" request header, so parsing
// doesn't need to interpret arbitrary Windows timezone names).
type msDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// msDateTimeLayouts covers the fractional-second precision Graph actually
// sends, which varies by endpoint/tenant rather than following one fixed
// format.
var msDateTimeLayouts = []string{
	"2006-01-02T15:04:05.0000000",
	"2006-01-02T15:04:05.000",
	"2006-01-02T15:04:05",
	time.RFC3339,
}

func (m msDateTime) parse() (time.Time, error) {
	var lastErr error
	for _, layout := range msDateTimeLayouts {
		if t, err := time.Parse(layout, m.DateTime); err == nil {
			return t.UTC(), nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

func fetchMicrosoft(ctx context.Context, client *http.Client, from, to time.Time) ([]SyncedEvent, error) {
	u, err := url.Parse(msEventsURL)
	if err != nil {
		return nil, fmt.Errorf("calsync: microsoft: %w", err)
	}
	q := u.Query()
	q.Set("startDateTime", from.UTC().Format("2006-01-02T15:04:05"))
	q.Set("endDateTime", to.UTC().Format("2006-01-02T15:04:05"))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("calsync: microsoft: %w", err)
	}
	req.Header.Set("Prefer", `outlook.timezone="UTC"`)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calsync: microsoft: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calsync: microsoft: HTTP %d: %s", resp.StatusCode, readErrorBody(resp))
	}

	var parsed msEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("calsync: microsoft: %w", err)
	}
	out := make([]SyncedEvent, 0, len(parsed.Value))
	for _, it := range parsed.Value {
		start, err := it.Start.parse()
		if err != nil {
			continue // skip events we can't parse rather than failing the whole sync
		}
		end, err := it.End.parse()
		if err != nil {
			end = start
		}
		out = append(out, SyncedEvent{ID: it.ID, Title: it.Subject, Start: start, End: end, AllDay: it.IsAllDay})
	}
	return out, nil
}
