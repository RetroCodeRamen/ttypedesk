package proto

import (
	"reflect"
	"testing"

	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
)

// roundTrip encodes payload through Envelope/JSON exactly as the wire does,
// then decodes it back into a fresh T and returns it.
func roundTrip[T any](t *testing.T, typ MessageType, payload T) T {
	t.Helper()
	data, err := Encode(typ, "win1", payload)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	env, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	got, err := DecodePayload[T](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	return got
}

func TestMouseEventRoundTrip(t *testing.T) {
	want := MouseEvent{X: 10, Y: 20, Button: 1, Action: "press", Ctrl: true}
	got := roundTrip(t, TypeMouse, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("MouseEvent round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestInitPayloadRoundTrip(t *testing.T) {
	want := InitPayload{WindowID: "w1", Cols: 80, Rows: 24}
	got := roundTrip(t, TypeInit, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("InitPayload round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestReadyPayloadRoundTrip(t *testing.T) {
	want := ReadyPayload{Err: "boom"}
	got := roundTrip(t, TypeReady, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("ReadyPayload round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestNotifyPayloadRoundTrip(t *testing.T) {
	want := NotifyPayload{Title: "Hi", Body: "there", Icon: "🔔"}
	got := roundTrip(t, TypeNotify, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("NotifyPayload round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestLaunchPayloadRoundTrip(t *testing.T) {
	want := LaunchPayload{Action: "terminal"}
	got := roundTrip(t, TypeLaunch, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("LaunchPayload round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestOpenPathPayloadRoundTrip(t *testing.T) {
	want := OpenPathPayload{Path: "/tmp/foo.txt"}
	got := roundTrip(t, TypeOpenPath, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("OpenPathPayload round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestEncodeReqRoundTripsReqID(t *testing.T) {
	data, err := EncodeReq(TypeLoadCredential, "win1", "r7", LoadCredentialRequest{Key: "k"})
	if err != nil {
		t.Fatalf("EncodeReq: %v", err)
	}
	env, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.ReqID != "r7" {
		t.Fatalf("ReqID = %q, want %q", env.ReqID, "r7")
	}
	p, err := DecodePayload[LoadCredentialRequest](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if p.Key != "k" {
		t.Fatalf("Key = %q, want %q", p.Key, "k")
	}
}

func TestEncodeLeavesReqIDEmpty(t *testing.T) {
	data, err := Encode(TypeNotify, "win1", NotifyPayload{Title: "t"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	env, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.ReqID != "" {
		t.Fatalf("ReqID = %q, want empty for a fire-and-forget Encode", env.ReqID)
	}
}

func TestSaveCredentialRequestRoundTrip(t *testing.T) {
	want := SaveCredentialRequest{Key: "tok", Value: []byte{0x00, 0xFF, 0x10}}
	got := roundTrip(t, TypeSaveCredential, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("SaveCredentialRequest round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestCredentialSavedResponseRoundTrip(t *testing.T) {
	want := CredentialSavedResponse{Err: "disk full"}
	got := roundTrip(t, TypeCredentialSaved, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("CredentialSavedResponse round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestLoadCredentialRequestRoundTrip(t *testing.T) {
	want := LoadCredentialRequest{Key: "tok"}
	got := roundTrip(t, TypeLoadCredential, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("LoadCredentialRequest round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestCredentialLoadedResponseRoundTrip(t *testing.T) {
	want := CredentialLoadedResponse{Value: []byte("secret")}
	got := roundTrip(t, TypeCredentialLoaded, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("CredentialLoadedResponse round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestPickFileRequestRoundTrip(t *testing.T) {
	want := PickFileRequest{StartDir: "/home/x", Extensions: []string{"png", "jpg"}}
	got := roundTrip(t, TypePickFile, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("PickFileRequest round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestFilePickedResponseRoundTrip(t *testing.T) {
	want := FilePickedResponse{Path: "/home/x/a.png", Ok: true}
	got := roundTrip(t, TypeFilePicked, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("FilePickedResponse round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestClipboardValueResponseRoundTrip(t *testing.T) {
	want := ClipboardValueResponse{Text: "clipped"}
	got := roundTrip(t, TypeClipboardValue, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("ClipboardValueResponse round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestClipboardSetRequestRoundTrip(t *testing.T) {
	want := ClipboardSetRequest{Text: "new clip"}
	got := roundTrip(t, TypeClipboardSet, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("ClipboardSetRequest round-trip mismatch: want %+v, got %+v", want, got)
	}
}

func TestAudioChunkPayloadRoundTrip(t *testing.T) {
	samples := []int16{0, 1, -1, 32767, -32768}
	want := AudioChunkPayload{PCM: EncodeAudioChunk(samples)}
	got := roundTrip(t, TypeAudioChunk, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("AudioChunkPayload round-trip mismatch: want %+v, got %+v", want, got)
	}
	gotSamples := DecodeAudioChunk(got.PCM)
	if !reflect.DeepEqual(samples, gotSamples) {
		t.Fatalf("DecodeAudioChunk(got.PCM) = %v, want %v", gotSamples, samples)
	}
}

func TestSnapshotWindowRoundTrip(t *testing.T) {
	want := Snapshot{
		Cols: 80,
		Rows: 24,
		Windows: []SnapshotWindow{
			{
				ID: "w1", Title: "bash",
				X: 3, Y: 4, W: 40, H: 12, Z: 1,
				Focused: true, Kind: "pty",
				Cells: []cell.Cell{{Rune: 'a'}},
				Cols:  40, Rows: 12,
			},
		},
	}
	got := roundTrip(t, TypeSnapshot, want)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("Snapshot round-trip mismatch: want %+v, got %+v", want, got)
	}
	// Explicitly pin down the bug this guards against: X/Y and W/H used to
	// share JSON tags ("x" and "w"), which made encoding/json drop all four
	// fields silently instead of erroring.
	gw := got.Windows[0]
	if gw.X != 3 || gw.Y != 4 || gw.W != 40 || gw.H != 12 {
		t.Fatalf("window geometry lost in round-trip: got X=%d Y=%d W=%d H=%d", gw.X, gw.Y, gw.W, gw.H)
	}
}
