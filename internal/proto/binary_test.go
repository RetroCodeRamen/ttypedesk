package proto

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

func TestWriteReadFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, FrameJSON, []byte(`{"hello":"world"}`)); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if err := WriteFrame(&buf, FrameDiff, []byte{1, 2, 3}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	r := bufio.NewReader(&buf)
	typ, payload, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame #1: %v", err)
	}
	if typ != FrameJSON || string(payload) != `{"hello":"world"}` {
		t.Errorf("frame #1 = (%d, %q), want (%d, json)", typ, payload, FrameJSON)
	}

	typ, payload, err = ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame #2: %v", err)
	}
	if typ != FrameDiff || !bytes.Equal(payload, []byte{1, 2, 3}) {
		t.Errorf("frame #2 = (%d, %v), want (%d, [1 2 3])", typ, payload, FrameDiff)
	}
}

func TestReadFrameRejectsAbsurdLength(t *testing.T) {
	var buf bytes.Buffer
	// A length prefix bigger than maxFrameLen, with no real payload behind
	// it — must be rejected outright, not attempt a huge allocation.
	putUint32(&buf, 0xFFFFFFFF)
	if _, _, err := ReadFrame(bufio.NewReader(&buf)); err == nil {
		t.Error("ReadFrame accepted an absurd length prefix")
	}
}

func TestReadFrameOnTruncatedStreamErrors(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, FrameJSON, []byte("hello"))
	truncated := buf.Bytes()[:4] // just the length prefix, no body
	if _, _, err := ReadFrame(bufio.NewReader(bytes.NewReader(truncated))); err == nil {
		t.Error("ReadFrame accepted a truncated frame")
	}
}

func sampleCells(n int) []cell.Cell {
	cells := make([]cell.Cell, n)
	for i := range cells {
		cells[i] = cell.Cell{
			Rune: rune('A' + i%26),
			FG:   cell.RGB(byte(i), byte(i*2), byte(i*3)),
			BG:   cell.RGB(byte(255-i), 0, 0),
			Attr: cell.Attr(i % 4),
		}
	}
	return cells
}

func TestEncodeDecodeDiffFrameRoundTrip(t *testing.T) {
	orig := DiffFrame{
		Cols: 120, Rows: 40,
		Windows: []DiffWindow{
			{
				ID: "w1", Title: "Terminal", X: 2, Y: 3, W: 40, H: 20, Z: 1,
				Focused: true, Maximized: false, Kind: "pty",
				Cols: 38, Rows: 18, Cells: sampleCells(38 * 18),
			},
			{
				ID: "w2", Title: "Settings", X: 10, Y: 5, W: 30, H: 15, Z: 2,
				Focused: false, Maximized: true, Kind: "app",
				Cols: 28, Rows: 13, Cells: nil, // unchanged this frame
			},
		},
	}

	data := EncodeDiffFrame(orig)
	got, err := DecodeDiffFrame(data)
	if err != nil {
		t.Fatalf("DecodeDiffFrame: %v", err)
	}

	if got.Cols != orig.Cols || got.Rows != orig.Rows {
		t.Errorf("Cols/Rows = %d/%d, want %d/%d", got.Cols, got.Rows, orig.Cols, orig.Rows)
	}
	if len(got.Windows) != 2 {
		t.Fatalf("len(Windows) = %d, want 2", len(got.Windows))
	}

	w1 := got.Windows[0]
	if w1.ID != "w1" || w1.Title != "Terminal" || w1.X != 2 || w1.Y != 3 || w1.W != 40 || w1.H != 20 || w1.Z != 1 {
		t.Errorf("w1 metadata = %+v, want matching orig", w1)
	}
	if !w1.Focused || w1.Maximized {
		t.Errorf("w1 flags = focused=%v maximized=%v, want focused=true maximized=false", w1.Focused, w1.Maximized)
	}
	if w1.Kind != "pty" {
		t.Errorf("w1.Kind = %q, want pty", w1.Kind)
	}
	if len(w1.Cells) != 38*18 {
		t.Fatalf("w1 cell count = %d, want %d", len(w1.Cells), 38*18)
	}
	for i, c := range w1.Cells {
		want := orig.Windows[0].Cells[i]
		if c != want {
			t.Fatalf("w1.Cells[%d] = %+v, want %+v", i, c, want)
		}
	}

	w2 := got.Windows[1]
	if w2.ID != "w2" || !w2.Maximized || w2.Focused {
		t.Errorf("w2 metadata/flags = %+v, want maximized=true focused=false", w2)
	}
	if w2.Cells != nil {
		t.Errorf("w2.Cells = %v, want nil (unchanged this frame)", w2.Cells)
	}
}

func TestEncodeDecodeDiffFrameEmptyWindowList(t *testing.T) {
	orig := DiffFrame{Cols: 80, Rows: 24}
	got, err := DecodeDiffFrame(EncodeDiffFrame(orig))
	if err != nil {
		t.Fatalf("DecodeDiffFrame: %v", err)
	}
	if got.Cols != 80 || got.Rows != 24 || len(got.Windows) != 0 {
		t.Errorf("got = %+v, want Cols=80 Rows=24 no windows", got)
	}
}

func TestEncodeDecodeAudioChunkRoundTrip(t *testing.T) {
	samples := []int16{0, 1, -1, 32767, -32768, 12345, -12345}
	got := DecodeAudioChunk(EncodeAudioChunk(samples))
	if len(got) != len(samples) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(samples))
	}
	for i, s := range samples {
		if got[i] != s {
			t.Errorf("got[%d] = %d, want %d", i, got[i], s)
		}
	}
}

func TestEncodeAudioChunkEmpty(t *testing.T) {
	if got := DecodeAudioChunk(EncodeAudioChunk(nil)); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestDiffFrameIsMuchSmallerThanUnchangedJSON(t *testing.T) {
	// The whole point: an unchanged window should cost almost nothing on
	// a repeat frame, versus JSON re-encoding its full cell grid every
	// time regardless of whether anything changed.
	cells := sampleCells(80 * 24)
	changed := EncodeDiffFrame(DiffFrame{Cols: 80, Rows: 24, Windows: []DiffWindow{
		{ID: "w1", Kind: "pty", Cols: 80, Rows: 24, Cells: cells},
	}})
	unchanged := EncodeDiffFrame(DiffFrame{Cols: 80, Rows: 24, Windows: []DiffWindow{
		{ID: "w1", Kind: "pty", Cols: 80, Rows: 24, Cells: nil},
	}})
	if len(unchanged) >= len(changed)/10 {
		t.Errorf("unchanged frame = %d bytes, changed frame = %d bytes — expected the unchanged frame to be a small fraction of the changed one", len(unchanged), len(changed))
	}
}
