package amp

import "testing"

// These exercise playlist/transport state transitions that don't reach
// playAt (which needs a real *uiapp.Context wired to a Host) — the decode
// pipeline itself (startDecode → real ffmpeg → real PCM) is covered
// end-to-end in decode_test.go instead.

func TestNewStartsWithNoCurrentTrack(t *testing.T) {
	a := New()
	if a.current != -1 {
		t.Errorf("current = %d, want -1 (nothing loaded)", a.current)
	}
	if a.clock == nil {
		t.Fatal("clock is nil")
	}
	if a.clock.Playing() {
		t.Error("a fresh player should start paused")
	}
}

func TestNextWithNothingLoadedStopsRatherThanPanicking(t *testing.T) {
	a := New()
	a.playlist = []string{"a.mp3", "b.mp3"}
	a.next() // current == -1: should just stop(), not index [-1+1]=0 blindly into playAt
	if a.current != -1 {
		t.Errorf("current = %d, want -1", a.current)
	}
}

func TestPrevWithNothingLoadedIsANoop(t *testing.T) {
	a := New()
	a.playlist = []string{"a.mp3"}
	a.prev()
	if a.current != -1 {
		t.Errorf("current = %d, want -1 (prev with nothing loaded shouldn't start playback)", a.current)
	}
}

func TestRemoveSelectedAdjustsCurrentIndex(t *testing.T) {
	a := New()
	a.playlist = []string{"a.mp3", "b.mp3", "c.mp3"}
	a.current = 2 // "c.mp3" is playing
	a.sel = 0     // remove "a.mp3", before current

	a.removeSelected()

	if len(a.playlist) != 2 {
		t.Fatalf("len(playlist) = %d, want 2", len(a.playlist))
	}
	if a.playlist[0] != "b.mp3" || a.playlist[1] != "c.mp3" {
		t.Errorf("playlist = %v, want [b.mp3 c.mp3]", a.playlist)
	}
	if a.current != 1 {
		t.Errorf("current = %d, want 1 (shifted down since an earlier track was removed)", a.current)
	}
}

func TestRemoveSelectedClampsSelPastEnd(t *testing.T) {
	a := New()
	a.playlist = []string{"a.mp3", "b.mp3"}
	a.sel = 1
	a.removeSelected()
	if a.sel != 0 {
		t.Errorf("sel = %d, want 0 (clamped after removing the last row)", a.sel)
	}
}

func TestRemoveSelectedOutOfRangeIsNoop(t *testing.T) {
	a := New()
	a.playlist = []string{"a.mp3"}
	a.sel = 5
	a.removeSelected()
	if len(a.playlist) != 1 {
		t.Errorf("len(playlist) = %d, want 1 (out-of-range sel should be a no-op)", len(a.playlist))
	}
}
