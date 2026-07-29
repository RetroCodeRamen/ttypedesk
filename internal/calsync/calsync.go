// Package calsync implements the OAuth2 loopback (PKCE) connect flow and
// event fetching for opt-in Google Calendar / Microsoft Graph sync.
//
// There is no ttypedesk-wide OAuth client: each user configures their own
// Client ID (Settings → Calendar), created in their own Google Cloud
// Console / Azure Portal project. Shipping (and keeping secret) a single
// client ID/secret for public, source-available distribution isn't
// realistic, and Google/Microsoft's "installed app" / public-client flows
// are specifically designed for exactly this bring-your-own-client-ID
// model — no client secret is required on either side when paired with
// PKCE.
package calsync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"golang.org/x/oauth2"
)

// Provider identifies which calendar service an account connects to.
type Provider string

const (
	Google    Provider = "google"
	Microsoft Provider = "microsoft"
)

var endpoints = map[Provider]oauth2.Endpoint{
	Google: {
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
	},
	Microsoft: {
		AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	},
}

var scopes = map[Provider][]string{
	Google:    {"https://www.googleapis.com/auth/calendar.readonly"},
	Microsoft: {"https://graph.microsoft.com/Calendars.Read", "offline_access"},
}

// connectTimeout bounds how long Connect waits for the user to finish the
// consent flow in their browser before giving up. A var, not a const, so
// tests can shrink it rather than waiting out the real 5 minutes.
var connectTimeout = 5 * time.Minute

// Connect runs an interactive OAuth2 loopback (PKCE) flow for provider,
// using the caller's own registered clientID. onURL receives the consent
// URL as soon as it's built, so callers can display it even where no
// local browser exists — the common case for this project, over SSH or
// headless — in addition to this function's own best-effort local browser
// launch. save receives the resulting token, JSON-encoded
// (*oauth2.Token), once the flow completes; it's the caller's job to
// persist it (e.g. via internal/credstore).
func Connect(ctx context.Context, provider Provider, clientID string, onURL func(url string), save func(token []byte) error) error {
	ep, ok := endpoints[provider]
	if !ok {
		return fmt.Errorf("calsync: unknown provider %q", provider)
	}
	if clientID == "" {
		return fmt.Errorf("calsync: %s client ID is not configured", provider)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("calsync: listen: %w", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	cfg := &oauth2.Config{
		ClientID:    clientID,
		Endpoint:    ep,
		RedirectURL: redirectURL,
		Scopes:      scopes[provider],
	}

	verifier := oauth2.GenerateVerifier()
	state, err := randHex(16)
	if err != nil {
		return fmt.Errorf("calsync: %w", err)
	}

	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if provider == Google {
		// Google only issues a refresh_token on the first consent (or
		// when forced) — without these, a token that expires (~1hr)
		// would require re-running the whole browser flow to renew.
		opts = append(opts, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	}
	authURL := cfg.AuthCodeURL(state, opts...)

	if onURL != nil {
		onURL(authURL)
	}
	openBrowser(authURL)

	code, err := waitForCode(ctx, ln, state)
	if err != nil {
		return err
	}

	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return fmt.Errorf("calsync: token exchange: %w", err)
	}
	data, err := marshalToken(tok)
	if err != nil {
		return err
	}
	return save(data)
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// openBrowser is best-effort — Connect works fine without a local browser
// (the URL is also delivered via onURL), since many users of this project
// run entirely over SSH with no local X11/Wayland session at all.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return
	}
	_ = cmd.Start()
}

func waitForCode(ctx context.Context, ln net.Listener, wantState string) (string, error) {
	type result struct {
		code string
		err  error
	}
	resCh := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errStr := q.Get("error"); errStr != "" {
			fmt.Fprintln(w, "Authorization failed — you can close this tab.")
			select {
			case resCh <- result{err: fmt.Errorf("calsync: authorization denied: %s", errStr)}:
			default:
			}
			return
		}
		if q.Get("state") != wantState {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			select {
			case resCh <- result{err: fmt.Errorf("calsync: state mismatch")}:
			default:
			}
			return
		}
		fmt.Fprintln(w, "Connected — you can close this tab and return to TTYPE Desk.")
		select {
		case resCh <- result{code: q.Get("code")}:
		default:
		}
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	// Closing the instant a result arrives can race the handler's own
	// in-flight response write (the browser tab sees a cut-off connection
	// instead of "Connected, you can close this tab") — give it a moment
	// to actually flush first.
	defer time.AfterFunc(200*time.Millisecond, func() { srv.Close() })

	select {
	case res := <-resCh:
		return res.code, res.err
	case <-time.After(connectTimeout):
		return "", fmt.Errorf("calsync: timed out waiting for browser authorization")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
