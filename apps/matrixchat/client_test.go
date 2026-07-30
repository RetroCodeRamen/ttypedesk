package matrixchat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// newFakeMatrixServer serves just enough of the real Matrix Client-Server
// API (login, joined_rooms, sync, whoami, send) for connect/sendText to
// run against it exactly as they would against a real homeserver — this
// is a genuine wire-protocol test, not a mock of mautrix-go's Go API.
func newFakeMatrixServer(t *testing.T, sendHit *atomic.Bool) *httptest.Server {
	t.Helper()
	var syncCount atomic.Int32
	mux := http.NewServeMux()

	mux.HandleFunc("/_matrix/client/v3/login", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"tok123","device_id":"DEV1","user_id":"@alice:example.org"}`)
	})
	mux.HandleFunc("/_matrix/client/v3/joined_rooms", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"joined_rooms":["!room1:example.org"]}`)
	})
	mux.HandleFunc("/_matrix/client/v3/account/whoami", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"user_id":"@alice:example.org"}`)
	})
	mux.HandleFunc("/_matrix/client/v3/user/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/filter") {
			fmt.Fprint(w, `{"filter_id":"filter1"}`)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		n := syncCount.Add(1)
		if n == 1 {
			fmt.Fprint(w, `{
				"next_batch": "batch1",
				"rooms": {
					"join": {
						"!room1:example.org": {
							"summary": {},
							"state": {"events": [
								{"type":"m.room.name","state_key":"","sender":"@alice:example.org","event_id":"$name1","room_id":"!room1:example.org","origin_server_ts":1000,"content":{"name":"Test Room"}}
							]},
							"timeline": {"events": [
								{"type":"m.room.message","sender":"@bob:example.org","event_id":"$msg1","room_id":"!room1:example.org","origin_server_ts":2000,"content":{"msgtype":"m.text","body":"hello world"}}
							]},
							"ephemeral": {"events": []},
							"account_data": {"events": []}
						}
					}
				}
			}`)
			return
		}
		// Subsequent long-poll turns: a short sleep instead of a real long-poll
		// wait, just so the client's re-poll loop doesn't spin at 100% CPU for
		// the rest of the test's lifetime.
		time.Sleep(20 * time.Millisecond)
		fmt.Fprintf(w, `{"next_batch":"batch%d","rooms":{}}`, n+1)
	})
	mux.HandleFunc("/_matrix/client/v3/rooms/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/send/m.room.message/") {
			if sendHit != nil {
				sendHit.Store(true)
			}
			fmt.Fprint(w, `{"event_id":"$sent1"}`)
			return
		}
		http.NotFound(w, r)
	})

	return httptest.NewServer(mux)
}

func TestConnectLoginSyncAndSendEndToEnd(t *testing.T) {
	var sendHit atomic.Bool
	srv := newFakeMatrixServer(t, &sendHit)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan update, 32)
	go connect(ctx, srv.URL, "alice", "hunter2", nil, updates)

	var gotConnected, gotRoomName, gotMessage bool
	var roomID id.RoomID
	var messageBody string
	var client *mautrix.Client
	deadline := time.After(10 * time.Second)
	for !(gotConnected && gotRoomName && gotMessage) {
		select {
		case u := <-updates:
			switch u.kind {
			case "connected":
				gotConnected = true
				client = u.client
				if u.sess.UserID != "@alice:example.org" {
					t.Errorf("connected session UserID = %q, want @alice:example.org", u.sess.UserID)
				}
				if u.sess.AccessToken != "tok123" {
					t.Errorf("connected session AccessToken = %q, want tok123", u.sess.AccessToken)
				}
			case "joined":
				roomID = u.roomID
			case "roomname":
				gotRoomName = true
				if u.name != "Test Room" {
					t.Errorf("roomname = %q, want Test Room", u.name)
				}
			case "message":
				gotMessage = true
				messageBody = u.body
			case "syncerr":
				t.Fatalf("unexpected syncerr: %v", u.err)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for connected+roomname+message (got connected=%v roomname=%v message=%v)", gotConnected, gotRoomName, gotMessage)
		}
	}
	if messageBody != "hello world" {
		t.Errorf("message body = %q, want %q", messageBody, "hello world")
	}
	if roomID != "!room1:example.org" {
		t.Errorf("roomID = %q, want !room1:example.org", roomID)
	}

	// Now exercise the real send path against the same fake server, using
	// the actual live client connect() handed back — not a second one.
	go sendText(context.Background(), client, roomID, "reply", updates)
	select {
	case u := <-updates:
		if u.kind == "senderr" {
			t.Fatalf("sendText reported an error: %v", u.err)
		}
	case <-time.After(2 * time.Second):
		// sendText only posts on error; no update within a couple seconds
		// plausibly just means it succeeded silently, which is correct
		// behavior — fall through to the sendHit check either way.
	}
	if !sendHit.Load() {
		t.Error("sendText never actually hit the fake server's send endpoint")
	}
}

func TestConnectResumesStoredSessionViaWhoami(t *testing.T) {
	srv := newFakeMatrixServer(t, nil)
	defer srv.Close()

	sess := &session{
		HomeserverURL: srv.URL,
		UserID:        "@alice:example.org",
		DeviceID:      "DEV1",
		AccessToken:   "tok123",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan update, 32)
	go connect(ctx, "", "", "", sess, updates)

	select {
	case u := <-updates:
		if u.kind != "connected" {
			t.Fatalf("first update kind = %q, want connected (got err=%v)", u.kind, u.err)
		}
		if u.sess.UserID != sess.UserID {
			t.Errorf("resumed session UserID = %q, want %q", u.sess.UserID, sess.UserID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a resumed session to connect")
	}
}

func TestConnectRejectsBadStoredToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/_matrix/client/v3/account/whoami", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"errcode":"M_UNKNOWN_TOKEN","error":"Invalid access token"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sess := &session{HomeserverURL: srv.URL, UserID: "@alice:example.org", AccessToken: "expired"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan update, 8)
	go connect(ctx, "", "", "", sess, updates)

	select {
	case u := <-updates:
		if u.kind != "syncerr" {
			t.Fatalf("update kind = %q, want syncerr for a rejected stored token", u.kind)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a syncerr on a bad stored token")
	}
}
