// Package proto defines versioned messages for TTYPE Desk (in-process now, IPC later).
package proto

import (
	"encoding/json"

	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
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

	// The out-of-process App SDK (internal/extapp; see docs/extapp.md)
	// reuses the types above for the lifecycle it shares with in-process
	// uiapp.App (Key/Mouse/Resize/Focus host->app; ScreenDiff app->host
	// for Draw; TitleChanged app->host for Host.SetTitle; CloseWindow
	// app->host for Host.RequestClose) and adds these three for the parts
	// that don't already have an equivalent:
	TypeInit     MessageType = "init"      // host -> app: window id + initial size, once at startup
	TypeReady    MessageType = "ready"     // app -> host: Init complete (Err set on failure)
	TypeNotify   MessageType = "notify"    // app -> host: Host.Notify
	TypeLaunch   MessageType = "launch"    // app -> host: Host.Launch
	TypeOpenPath MessageType = "open_path" // app -> host: Host.OpenPath

	// Request/response pairs (correlated via Envelope.ReqID — the app sets
	// it on the request, the host echoes it back on the matching reply, so
	// several in-flight requests of different kinds don't need to be
	// resolved in send order) covering the rest of uiapp.Host that v1 of
	// this SDK originally shipped without: credential storage, the file
	// picker, and clipboard access.
	TypeSaveCredential   MessageType = "save_credential"   // app -> host, replied by TypeCredentialSaved
	TypeCredentialSaved  MessageType = "credential_saved"  // host -> app
	TypeLoadCredential   MessageType = "load_credential"   // app -> host, replied by TypeCredentialLoaded
	TypeCredentialLoaded MessageType = "credential_loaded" // host -> app
	TypePickFile         MessageType = "pick_file"         // app -> host, replied by TypeFilePicked (async — whenever the user actually picks/cancels)
	TypeFilePicked       MessageType = "file_picked"       // host -> app
	TypeClipboardGet     MessageType = "clipboard_get"     // app -> host, replied by TypeClipboardValue
	TypeClipboardValue   MessageType = "clipboard_value"   // host -> app
	TypeClipboardSet     MessageType = "clipboard_set"     // app -> host, fire-and-forget (no reply — nothing meaningful can fail)

	// Audio playback (uiapp.Host.PlayAudio): fire-and-forget, not
	// request/response — TypePlayAudio starts a stream, repeated
	// TypeAudioChunk messages carry PCM, TypeStopAudio ends it. No reply
	// to any of these; a genuine playback failure just never plays audio,
	// which is already how the in-process Host.PlayAudio error is mostly
	// used (apps generally don't hard-fail just because sound didn't
	// start).
	TypePlayAudio  MessageType = "play_audio"  // app -> host: start streaming
	TypeAudioChunk MessageType = "audio_chunk" // app -> host: one chunk of PCM
	TypeStopAudio  MessageType = "stop_audio"  // app -> host: stop streaming
)

// Envelope wraps every message. ReqID correlates a request/response pair
// (e.g. TypeLoadCredential -> TypeCredentialLoaded) — set by the caller on
// the request, echoed back unchanged on the reply. Empty for fire-and-
// forget message types, which is most of them.
type Envelope struct {
	Version int             `json:"v"`
	Type    MessageType     `json:"type"`
	Window  string          `json:"window,omitempty"`
	ReqID   string          `json:"req_id,omitempty"`
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

// InitPayload is the out-of-process App SDK's one-time startup message
// (see TypeInit): the window id an app needs for logging/reference, and
// its initial canvas size (also delivered again via a normal TypeResize
// on every later resize).
type InitPayload struct {
	WindowID string `json:"window_id"`
	Cols     int    `json:"cols"`
	Rows     int    `json:"rows"`
}

// ReadyPayload answers TypeInit. Err is empty on success; a non-empty Err
// marks the window crashed with that message, same as an in-process app
// panicking in Init.
type ReadyPayload struct {
	Err string `json:"err,omitempty"`
}

// NotifyPayload is TypeNotify's payload — see uiapp.Host.Notify.
type NotifyPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon,omitempty"`
}

// LaunchPayload is TypeLaunch's payload — see uiapp.Host.Launch.
type LaunchPayload struct {
	Action string `json:"action"`
}

// OpenPathPayload is TypeOpenPath's payload — see uiapp.Host.OpenPath.
type OpenPathPayload struct {
	Path string `json:"path"`
}

// SaveCredentialRequest is TypeSaveCredential's payload — see
// uiapp.Host.SaveCredential. Value is base64-encoded automatically by
// encoding/json ([]byte fields always are).
type SaveCredentialRequest struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

// CredentialSavedResponse answers TypeSaveCredential. Err is empty on
// success.
type CredentialSavedResponse struct {
	Err string `json:"err,omitempty"`
}

// LoadCredentialRequest is TypeLoadCredential's payload.
type LoadCredentialRequest struct {
	Key string `json:"key"`
}

// CredentialLoadedResponse answers TypeLoadCredential. Err is empty on
// success; a non-empty Err (matching os.ErrNotExist's message when
// nothing was saved under Key) means Value is meaningless.
type CredentialLoadedResponse struct {
	Value []byte `json:"value,omitempty"`
	Err   string `json:"err,omitempty"`
}

// PickFileRequest is TypePickFile's payload — see uiapp.Host.PickFile.
type PickFileRequest struct {
	StartDir   string   `json:"start_dir,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
}

// FilePickedResponse answers TypePickFile, asynchronously, whenever the
// user actually picks a file or cancels — not immediately like the other
// request/response pairs here.
type FilePickedResponse struct {
	Path string `json:"path,omitempty"`
	Ok   bool   `json:"ok"`
}

// ClipboardValueResponse answers TypeClipboardGet.
type ClipboardValueResponse struct {
	Text string `json:"text"`
}

// ClipboardSetRequest is TypeClipboardSet's payload.
type ClipboardSetRequest struct {
	Text string `json:"text"`
}

// AudioChunkPayload is TypeAudioChunk's payload: PCM holds
// proto.EncodeAudioChunk's binary encoding (interleaved int16 samples,
// little-endian — see internal/proto/binary.go), base64-encoded
// automatically by encoding/json since it's a []byte field. Reused as-is
// rather than a JSON int array: half the bytes over the wire, and it's
// already implemented for the remote-attach audio path (Phase 8).
type AudioChunkPayload struct {
	PCM []byte `json:"pcm"`
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

// Encode builds a fire-and-forget envelope (no ReqID). Use EncodeReq for a
// request that expects a correlated reply.
func Encode(typ MessageType, window string, payload any) ([]byte, error) {
	return EncodeReq(typ, window, "", payload)
}

// EncodeReq builds an envelope with a ReqID, for request/response message
// pairs (see Envelope's doc comment).
func EncodeReq(typ MessageType, window, reqID string, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return json.Marshal(Envelope{Version: Version, Type: typ, Window: window, ReqID: reqID, Payload: raw})
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
