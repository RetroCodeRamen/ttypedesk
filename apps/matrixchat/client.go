package chat

import (
	"context"
	"fmt"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// update is how connect/sync/send's background goroutines report back —
// never by touching App fields directly (Handle/Draw run under
// AppSurface's own lock, these goroutines don't). Drained only from Draw,
// same contract as every other background-goroutine result channel in
// this codebase (apps/calendar/sync.go, apps/amp, apps/vid).
type update struct {
	kind string // "connected" | "message" | "roomname" | "joined" | "senderr" | "syncerr" | "disconnected"

	client *mautrix.Client // set on "connected"
	sess   session         // set on "connected", for persisting

	roomID id.RoomID
	sender string
	body   string
	ts     time.Time
	name   string // for "roomname"

	err error
}

// connect logs in (or resumes an existing session if sess is non-nil) and
// starts syncing, posting every update to updates. Runs until ctx is
// cancelled or the sync loop ends on its own (auth failure, etc.).
func connect(ctx context.Context, homeserver, username, password string, sess *session, updates chan<- update) {
	var client *mautrix.Client
	var newSess session
	var err error

	if sess != nil {
		client, err = mautrix.NewClient(sess.HomeserverURL, id.UserID(sess.UserID), sess.AccessToken)
		if err == nil {
			client.DeviceID = id.DeviceID(sess.DeviceID)
			newSess = *sess
			// Confirm the stored token still works before committing to it —
			// an expired/revoked token should fall through to a fresh login
			// instead of syncing forever against a server that's just
			// rejecting every request.
			if _, werr := client.Whoami(ctx); werr != nil {
				err = werr
			}
		}
	} else {
		client, err = mautrix.NewClient(homeserver, "", "")
		if err == nil {
			var resp *mautrix.RespLogin
			resp, err = client.Login(ctx, &mautrix.ReqLogin{
				Type: mautrix.AuthTypePassword,
				Identifier: mautrix.UserIdentifier{
					Type: mautrix.IdentifierTypeUser,
					User: username,
				},
				Password:         password,
				StoreCredentials: true,
			})
			if err == nil {
				newSess = session{
					HomeserverURL: client.HomeserverURL.String(),
					UserID:        resp.UserID.String(),
					DeviceID:      resp.DeviceID.String(),
					AccessToken:   resp.AccessToken,
				}
			}
		}
	}
	if err != nil {
		select {
		case updates <- update{kind: "syncerr", err: fmt.Errorf("chat: %w", err)}:
		case <-ctx.Done():
		}
		return
	}

	select {
	case updates <- update{kind: "connected", client: client, sess: newSess}:
	case <-ctx.Done():
		return
	}

	syncer := mautrix.NewDefaultSyncer()
	syncer.OnEventType(event.EventMessage, func(_ context.Context, evt *event.Event) {
		msg := evt.Content.AsMessage()
		if msg == nil {
			return
		}
		u := update{kind: "message", roomID: evt.RoomID, sender: evt.Sender.String(), body: msg.Body, ts: time.UnixMilli(evt.Timestamp)}
		select {
		case updates <- u:
		case <-ctx.Done():
		}
	})
	syncer.OnEventType(event.StateRoomName, func(_ context.Context, evt *event.Event) {
		rn := evt.Content.AsRoomName()
		if rn == nil {
			return
		}
		u := update{kind: "roomname", roomID: evt.RoomID, name: rn.Name}
		select {
		case updates <- u:
		case <-ctx.Done():
		}
	})
	client.Syncer = syncer

	if joined, jerr := client.JoinedRooms(ctx); jerr == nil {
		for _, rid := range joined.JoinedRooms {
			u := update{kind: "joined", roomID: rid}
			select {
			case updates <- u:
			case <-ctx.Done():
				return
			}
		}
	}

	err = client.SyncWithContext(ctx)
	if err != nil && ctx.Err() == nil {
		select {
		case updates <- update{kind: "syncerr", err: fmt.Errorf("chat: sync: %w", err)}:
		default:
		}
	}
	select {
	case updates <- update{kind: "disconnected"}:
	default:
	}
}

// sendText sends body to roomID and reports failure only — a successful
// send arrives back through the normal sync loop as a "message" update
// (the homeserver echoes your own messages back), so there's no separate
// "sent" update to wire up.
func sendText(ctx context.Context, client *mautrix.Client, roomID id.RoomID, body string, updates chan<- update) {
	if _, err := client.SendText(ctx, roomID, body); err != nil {
		select {
		case updates <- update{kind: "senderr", err: fmt.Errorf("chat: send: %w", err)}:
		case <-ctx.Done():
		}
	}
}
