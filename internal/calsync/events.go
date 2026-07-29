package calsync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// SyncedEvent is a provider-agnostic calendar event, converted from
// whatever shape Google/Microsoft's APIs return.
type SyncedEvent struct {
	ID     string
	Title  string
	Start  time.Time
	End    time.Time
	AllDay bool
}

func marshalToken(tok *oauth2.Token) ([]byte, error) {
	data, err := json.Marshal(tok)
	if err != nil {
		return nil, fmt.Errorf("calsync: %w", err)
	}
	return data, nil
}

// FetchEvents fetches events starting in [from, to) for provider, using a
// previously stored token (loaded via loadToken). If the token was
// refreshed during the call (expired access token, valid refresh token),
// the refreshed token is persisted back via saveToken before returning —
// callers never need to re-run Connect just because a token expired.
func FetchEvents(ctx context.Context, provider Provider, clientID string, from, to time.Time, loadToken func() ([]byte, error), saveToken func([]byte) error) ([]SyncedEvent, error) {
	ep, ok := endpoints[provider]
	if !ok {
		return nil, fmt.Errorf("calsync: unknown provider %q", provider)
	}
	raw, err := loadToken()
	if err != nil {
		return nil, fmt.Errorf("calsync: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("calsync: %w", err)
	}

	cfg := &oauth2.Config{ClientID: clientID, Endpoint: ep}
	src := &savingTokenSource{src: cfg.TokenSource(ctx, &tok), save: saveToken, last: tok.AccessToken}
	client := oauth2.NewClient(ctx, src)

	switch provider {
	case Google:
		return fetchGoogle(ctx, client, from, to)
	case Microsoft:
		return fetchMicrosoft(ctx, client, from, to)
	default:
		return nil, fmt.Errorf("calsync: unknown provider %q", provider)
	}
}

// savingTokenSource wraps an oauth2.TokenSource, persisting the token via
// save whenever the access token actually changes (i.e. was refreshed) —
// so a refreshed token survives past this process's lifetime instead of
// silently expiring again next run.
type savingTokenSource struct {
	src  oauth2.TokenSource
	save func([]byte) error

	mu   sync.Mutex
	last string
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	changed := tok.AccessToken != s.last
	s.last = tok.AccessToken
	s.mu.Unlock()
	if changed && s.save != nil {
		if data, err := marshalToken(tok); err == nil {
			_ = s.save(data)
		}
	}
	return tok, nil
}

// readErrorBody caps how much of an error response body gets included in
// a returned error, so a misbehaving endpoint can't blow up log lines.
func readErrorBody(resp *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return string(body)
}
