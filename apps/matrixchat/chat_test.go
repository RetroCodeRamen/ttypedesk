package matrixchat

import (
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
)

func uiEvent(key string) uiapp.Event {
	return uiapp.Event{Kind: uiapp.EventKey, Key: key}
}

func uiEventRune(r rune) uiapp.Event {
	return uiapp.Event{Kind: uiapp.EventKey, Rune: r}
}

func TestShortSender(t *testing.T) {
	cases := map[string]string{
		"@alice:example.org": "alice",
		"@bob:matrix.org":    "bob",
		"noatsign":           "noatsign",
	}
	for in, want := range cases {
		if got := shortSender(in); got != want {
			t.Errorf("shortSender(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 3); got != "hel" {
		t.Errorf("truncateRunes = %q, want hel", got)
	}
	if got := truncateRunes("hi", 10); got != "hi" {
		t.Errorf("truncateRunes with n > len should return unchanged, got %q", got)
	}
	if got := truncateRunes("x", 0); got != "" {
		t.Errorf("truncateRunes with n=0 should return empty, got %q", got)
	}
}

func TestEnsureRoomCreatesOnce(t *testing.T) {
	a := New()
	r1 := a.ensureRoom("!room1:example.org")
	r2 := a.ensureRoom("!room1:example.org")
	if r1 != r2 {
		t.Error("ensureRoom created a second entry for the same room ID")
	}
	if len(a.rooms) != 1 {
		t.Errorf("len(rooms) = %d, want 1", len(a.rooms))
	}
}

func TestEnsureRoomSetsInitialSelection(t *testing.T) {
	a := New()
	if a.selRoom != -1 {
		t.Fatalf("selRoom = %d, want -1 before any room exists", a.selRoom)
	}
	a.ensureRoom("!room1:example.org")
	if a.selRoom != 0 {
		t.Errorf("selRoom = %d, want 0 after the first room is created", a.selRoom)
	}
	a.ensureRoom("!room2:example.org")
	if a.selRoom != 0 {
		t.Errorf("selRoom = %d, want unchanged 0 after a second room is created", a.selRoom)
	}
}

func TestRoomLabelFallsBackToID(t *testing.T) {
	r := &roomInfo{id: "!abc:example.org"}
	if got := r.label(); got != "!abc:example.org" {
		t.Errorf("label() = %q, want the raw room ID before a name is known", got)
	}
	r.name = "General"
	if got := r.label(); got != "General" {
		t.Errorf("label() = %q, want General once a name is known", got)
	}
}

func TestApplyMessageAppendsAndCapsTimeline(t *testing.T) {
	a := New()
	rid := id.RoomID("!room1:example.org")
	for i := 0; i < maxTimeline+10; i++ {
		a.apply(update{kind: "message", roomID: rid, sender: "@bob:example.org", body: "msg", ts: time.Now()})
	}
	r := a.room(rid)
	if r == nil {
		t.Fatal("room was never created by apply(message)")
	}
	if len(r.timeline) != maxTimeline {
		t.Errorf("len(timeline) = %d, want capped at %d", len(r.timeline), maxTimeline)
	}
}

func TestApplyRoomnameUpdatesExistingRoomOnly(t *testing.T) {
	a := New()
	// roomname for a room that doesn't exist yet: should not create one —
	// there's nothing useful to show for a room with a name but no
	// messages and no join confirmation.
	a.apply(update{kind: "roomname", roomID: "!ghost:example.org", name: "Ghost"})
	if len(a.rooms) != 0 {
		t.Errorf("roomname update created a room entry: %+v", a.rooms)
	}

	a.ensureRoom("!room1:example.org")
	a.apply(update{kind: "roomname", roomID: "!room1:example.org", name: "General"})
	if got := a.room("!room1:example.org").name; got != "General" {
		t.Errorf("room name = %q, want General", got)
	}
}

func TestApplySyncerrReturnsToLoginMode(t *testing.T) {
	a := New()
	a.mode = modeConnecting
	a.apply(update{kind: "syncerr", err: fixtureErr("boom")})
	if a.mode != modeLogin {
		t.Errorf("mode = %d, want modeLogin after a syncerr", a.mode)
	}
	if a.status == "" {
		t.Error("status was not set to explain the sync error")
	}
}

type fixtureErr string

func (e fixtureErr) Error() string { return string(e) }

func TestLoginKeyFieldNavigation(t *testing.T) {
	a := New()
	if a.field != 0 {
		t.Fatalf("field = %d, want 0 initially", a.field)
	}
	_ = a.loginKey(uiEvent("Tab"))
	if a.field != 1 {
		t.Errorf("field = %d, want 1 after Tab", a.field)
	}
	_ = a.loginKey(uiEvent("Tab"))
	if a.field != 2 {
		t.Errorf("field = %d, want 2 after second Tab", a.field)
	}
	_ = a.loginKey(uiEvent("Tab"))
	if a.field != 0 {
		t.Errorf("field = %d, want wrapped to 0", a.field)
	}
}

func TestLoginKeyTypingGoesToCurrentField(t *testing.T) {
	a := New()
	for _, r := range "matrix.org" {
		_ = a.key(uiEventRune(r))
	}
	if a.homeserver != "matrix.org" {
		t.Errorf("homeserver = %q, want matrix.org", a.homeserver)
	}
	_ = a.loginKey(uiEvent("Tab"))
	for _, r := range "alice" {
		_ = a.key(uiEventRune(r))
	}
	if a.username != "alice" {
		t.Errorf("username = %q, want alice", a.username)
	}
}

func TestSendComposeRequiresClientAndRoom(t *testing.T) {
	a := New()
	a.compose = "hello"
	a.sendCompose() // no client, no room selected — must not panic
	if a.compose != "hello" {
		t.Error("compose was cleared despite no client/room being available to send to")
	}
}

// recordingHost implements uiapp.Host, recording every Notify call —
// everything else is a no-op. Used to test apply(message)'s notification
// suppression rules without needing a real Matrix connection.
type recordingHost struct {
	notified []string
}

func (h *recordingHost) Notify(title, body, icon string) {
	h.notified = append(h.notified, title+": "+body)
}
func (h *recordingHost) Launch(string) error   { return nil }
func (h *recordingHost) OpenPath(string) error { return nil }
func (h *recordingHost) SetTitle(string)       {}
func (h *recordingHost) RequestClose()         {}
func (h *recordingHost) WindowID() string      { return "test" }
func (h *recordingHost) SaveCredential(string, []byte) error {
	return nil
}
func (h *recordingHost) LoadCredential(string) ([]byte, error) {
	return nil, errNotExistFixture
}
func (h *recordingHost) PickFile(string, []string, func(string, bool))         {}
func (h *recordingHost) PlayAudio(<-chan []int16) (uiapp.AudioPlayback, error) { return nil, nil }
func (h *recordingHost) ClipboardGet() string                                  { return "" }
func (h *recordingHost) ClipboardSet(string)                                   {}

var errNotExistFixture = fixtureErr("not found")

func newAppWithHost(t *testing.T) (*App, *recordingHost) {
	t.Helper()
	host := &recordingHost{}
	ctx := uiapp.NewContext("test", 70, 22, func() {})
	ctx.SetHost(host)
	a := New()
	a.ctx = ctx
	return a, host
}

func TestApplyMessageNotifiesForBackgroundedRoom(t *testing.T) {
	a, host := newAppWithHost(t)
	a.apply(update{kind: "message", roomID: "!room1:example.org", sender: "@bob:example.org", body: "hi", ts: time.Now()})
	if len(host.notified) != 1 {
		t.Fatalf("notified = %v, want exactly 1 notification for a message in a new/backgrounded room", host.notified)
	}
}

func TestApplyMessageDoesNotNotifyForSelectedRoom(t *testing.T) {
	a, host := newAppWithHost(t)
	a.ensureRoom("!room1:example.org") // becomes selRoom=0
	a.apply(update{kind: "message", roomID: "!room1:example.org", sender: "@bob:example.org", body: "hi", ts: time.Now()})
	if len(host.notified) != 0 {
		t.Errorf("notified = %v, want none — message arrived in the currently selected room", host.notified)
	}
}

func TestApplyMessageDoesNotNotifyForOwnEchoedMessage(t *testing.T) {
	a, host := newAppWithHost(t)
	a.client = &mautrix.Client{UserID: "@alice:example.org"}
	a.apply(update{kind: "message", roomID: "!room1:example.org", sender: "@alice:example.org", body: "hi", ts: time.Now()})
	if len(host.notified) != 0 {
		t.Errorf("notified = %v, want none — this is the user's own message echoed back by sync", host.notified)
	}
}

func TestSendComposeIgnoresBlank(t *testing.T) {
	a := New()
	a.ensureRoom("!room1:example.org")
	a.compose = "   "
	a.sendCompose()
	if a.compose != "   " {
		t.Error("blank compose should be left alone (nothing to send), not cleared")
	}
}
