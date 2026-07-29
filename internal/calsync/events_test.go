package calsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestFetchGoogleParsesTimedAndAllDayEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want Bearer test-token", got)
		}
		fmt.Fprint(w, `{
			"items": [
				{"id": "e1", "summary": "Standup", "start": {"dateTime": "2026-07-29T10:00:00Z"}, "end": {"dateTime": "2026-07-29T10:30:00Z"}},
				{"id": "e2", "summary": "Vacation", "start": {"date": "2026-08-01"}, "end": {"date": "2026-08-05"}}
			]
		}`)
	}))
	defer srv.Close()
	origURL := googleEventsURL
	googleEventsURL = srv.URL
	defer func() { googleEventsURL = origURL }()

	client := srv.Client()
	client.Transport = bearerTransport{token: "test-token", base: client.Transport}

	events, err := fetchGoogle(context.Background(), client, time.Now(), time.Now().AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("fetchGoogle: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].ID != "e1" || events[0].Title != "Standup" || events[0].AllDay {
		t.Errorf("events[0] = %+v, want timed Standup/e1", events[0])
	}
	if events[1].ID != "e2" || events[1].Title != "Vacation" || !events[1].AllDay {
		t.Errorf("events[1] = %+v, want all-day Vacation/e2", events[1])
	}
}

func TestFetchGoogleHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error": "insufficient scope"}`)
	}))
	defer srv.Close()
	origURL := googleEventsURL
	googleEventsURL = srv.URL
	defer func() { googleEventsURL = origURL }()

	_, err := fetchGoogle(context.Background(), srv.Client(), time.Now(), time.Now())
	if err == nil {
		t.Fatal("fetchGoogle: want error on HTTP 403, got nil")
	}
}

func TestFetchMicrosoftParsesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Prefer"); got == "" {
			t.Error("Prefer header not set")
		}
		fmt.Fprint(w, `{
			"value": [
				{"id": "m1", "subject": "1:1", "isAllDay": false, "start": {"dateTime": "2026-07-29T14:00:00.0000000", "timeZone": "UTC"}, "end": {"dateTime": "2026-07-29T14:30:00.0000000", "timeZone": "UTC"}}
			]
		}`)
	}))
	defer srv.Close()
	origURL := msEventsURL
	msEventsURL = srv.URL
	defer func() { msEventsURL = origURL }()

	events, err := fetchMicrosoft(context.Background(), srv.Client(), time.Now(), time.Now().AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("fetchMicrosoft: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].ID != "m1" || events[0].Title != "1:1" {
		t.Errorf("events[0] = %+v, want m1/1:1", events[0])
	}
	if events[0].Start.Hour() != 14 {
		t.Errorf("events[0].Start = %v, want hour 14", events[0].Start)
	}
}

func TestSavingTokenSourcePersistsOnlyWhenChanged(t *testing.T) {
	tok := &oauth2.Token{AccessToken: "a1", Expiry: time.Now().Add(time.Hour)}
	var saveCount int
	src := &savingTokenSource{
		src:  oauth2.StaticTokenSource(tok),
		save: func([]byte) error { saveCount++; return nil },
		last: "", // force a save on first Token() call
	}
	if _, err := src.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if _, err := src.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if saveCount != 1 {
		t.Errorf("saveCount = %d, want 1 (only the first call should persist, token never changed after)", saveCount)
	}
}

// bearerTransport injects a fixed Authorization header — a stand-in for
// oauth2.NewClient's real transport, so fetchGoogle's own request assembly
// can be tested without a live token source.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
