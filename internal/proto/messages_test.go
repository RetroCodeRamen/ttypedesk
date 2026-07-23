package proto

import (
	"reflect"
	"testing"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
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
