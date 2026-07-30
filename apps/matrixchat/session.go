package matrixchat

import (
	"encoding/json"
	"fmt"

	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// sessionKey is the credstore key the Matrix session (homeserver, user,
// device, access token) persists under — never in config.json, never in
// git, per the roadmap's explicit call for this.
const sessionKey = "chat.matrix.session"

// session is everything needed to resume a Matrix connection without
// logging in again. No password is ever stored — only what login/whoami
// hand back.
type session struct {
	HomeserverURL string `json:"homeserver_url"`
	UserID        string `json:"user_id"`
	DeviceID      string `json:"device_id"`
	AccessToken   string `json:"access_token"`
}

func loadSession(ctx *uiapp.Context) (*session, error) {
	raw, err := ctx.LoadCredential(sessionKey)
	if err != nil {
		return nil, err
	}
	var s session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	return &s, nil
}

func saveSession(ctx *uiapp.Context, s session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("chat: %w", err)
	}
	return ctx.SaveCredential(sessionKey, data)
}
