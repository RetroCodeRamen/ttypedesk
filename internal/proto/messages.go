// Package proto defines versioned messages for TTYPE Desk (in-process now, IPC later).
package proto

import (
	"encoding/json"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

const Version = 1

// MessageType identifies a protocol message.
type MessageType string

const (
	TypeKey          MessageType = "key"
	TypeMouse        MessageType = "mouse"
	TypeResize       MessageType = "resize"
	TypeFocus        MessageType = "focus"
	TypeCreateWindow MessageType = "create_window"
	TypeCloseWindow  MessageType = "close_window"
	TypeMaximize     MessageType = "maximize"
	TypeScreenDiff   MessageType = "screen_diff"
	TypeTitleChanged MessageType = "title_changed"
	TypeBell         MessageType = "bell"
	TypeAttach       MessageType = "attach"
	TypeDetach       MessageType = "detach"
	TypeSnapshot     MessageType = "snapshot"
)

// Envelope wraps every message.
type Envelope struct {
	Version int             `json:"v"`
	Type    MessageType     `json:"type"`
	Window  string          `json:"window,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type KeyEvent struct {
	Rune  rune   `json:"rune,omitempty"`
	Key   string `json:"key,omitempty"`
	Ctrl  bool   `json:"ctrl,omitempty"`
	Alt   bool   `json:"alt,omitempty"`
	Shift bool   `json:"shift,omitempty"`
	Bytes []byte `json:"bytes,omitempty"`
}

type MouseEvent struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Button int    `json:"button"`
	Action string `json:"action"` // press, release, drag, move, wheel
	Ctrl   bool   `json:"ctrl,omitempty"`
	Alt    bool   `json:"alt,omitempty"`
	Shift  bool   `json:"shift,omitempty"`
}

type ResizeEvent struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

type FocusEvent struct {
	Focused bool `json:"focused"`
}

type CreateWindow struct {
	Kind  string `json:"kind"` // pty, app, gfx
	Title string `json:"title"`
	Cmd   string `json:"cmd,omitempty"`
	Path  string `json:"path,omitempty"` // for imageview
	App   string `json:"app,omitempty"`  // clock, imageview
}

type ScreenDiffPayload struct {
	Diff cell.Diff `json:"diff"`
}

type TitleChanged struct {
	Title string `json:"title"`
}

type SnapshotWindow struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	X         int         `json:"x"`
	Y         int         `json:"y"`
	W         int         `json:"w"`
	H         int         `json:"h"`
	Z         int         `json:"z"`
	Focused   bool        `json:"focused"`
	Maximized bool        `json:"maximized"`
	Kind      string      `json:"kind"`
	Cells     []cell.Cell `json:"cells,omitempty"`
	Cols      int         `json:"cols"`
	Rows      int         `json:"rows"`
}

type Snapshot struct {
	Cols    int              `json:"cols"`
	Rows    int              `json:"rows"`
	Windows []SnapshotWindow `json:"windows"`
}

func Encode(typ MessageType, window string, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return json.Marshal(Envelope{Version: Version, Type: typ, Window: window, Payload: raw})
}

func Decode(data []byte) (Envelope, error) {
	var env Envelope
	err := json.Unmarshal(data, &env)
	return env, err
}

func DecodePayload[T any](env Envelope) (T, error) {
	var v T
	if len(env.Payload) == 0 {
		return v, nil
	}
	err := json.Unmarshal(env.Payload, &v)
	return v, err
}
