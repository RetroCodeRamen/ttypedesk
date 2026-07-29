package ffdecode

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// requireFFmpeg skips a test unless ffmpeg is actually on PATH — like the
// GUI-TUI Bridge's X11-dependent tests, this needs a real external tool
// CI installs but a bare dev machine might not have.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("ffmpeg not on PATH, skipping (apt install ffmpeg to run this locally)")
	}
}

// synthTone generates a short real WAV file (a 440Hz sine tone) via
// ffmpeg's lavfi source — the same tool this package itself shells out
// to, so this exercises DecodeAudio against a real, valid audio file
// rather than a hand-rolled fixture.
func synthTone(t *testing.T, seconds float64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tone.wav")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+strconv.FormatFloat(seconds, 'f', -1, 64),
		"-y", "-loglevel", "error", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg synth failed: %v\n%s", err, out)
	}
	return path
}

func TestDecodeAudioProducesRealAudibleSamples(t *testing.T) {
	requireFFmpeg(t)
	path := synthTone(t, 1)

	events := make(chan AudioEvent, 2)
	var chunkCalls atomic.Int64
	s, err := DecodeAudio(path, func([]int16) { chunkCalls.Add(1) }, events)
	if err != nil {
		t.Fatalf("DecodeAudio: %v", err)
	}
	defer s.Stop()

	var gotNonZero bool
	var totalSamples int
	deadline := time.After(10 * time.Second)
loop:
	for {
		select {
		case samples, ok := <-s.PCM:
			if !ok {
				break loop
			}
			totalSamples += len(samples)
			for _, v := range samples {
				if v != 0 {
					gotNonZero = true
				}
			}
		case ev := <-events:
			if ev.Err != nil {
				t.Fatalf("decode error: %v", ev.Err)
			}
			if ev.Ended {
				break loop
			}
		case <-deadline:
			t.Fatal("timed out waiting for decoded samples — ffmpeg pipeline likely stuck")
		}
	}
	if !gotNonZero {
		t.Error("all decoded samples were zero — expected an audible 440Hz tone")
	}
	if totalSamples < 40000 {
		t.Errorf("totalSamples = %d, want at least ~40000 for a 1s stereo tone at 48kHz", totalSamples)
	}
	if chunkCalls.Load() == 0 {
		t.Error("OnChunk was never called")
	}
}

func TestDecodeAudioMissingFileReportsErrorEvent(t *testing.T) {
	requireFFmpeg(t)
	events := make(chan AudioEvent, 2)
	s, err := DecodeAudio(filepath.Join(t.TempDir(), "does-not-exist.mp3"), nil, events)
	if err != nil {
		return // some ffmpeg builds fail fast at Start(); either outcome is fine
	}
	defer s.Stop()

	select {
	case ev := <-events:
		if ev.Err == nil && !ev.Ended {
			t.Error("expected an error (or at worst an immediate empty-ended) event for a missing input file")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a decode-failure event")
	}
}

func TestDecodeAudioStopUnblocksFeedGoroutine(t *testing.T) {
	requireFFmpeg(t)
	path := synthTone(t, 5) // long enough that we can stop mid-decode

	events := make(chan AudioEvent, 2)
	s, err := DecodeAudio(path, nil, events)
	if err != nil {
		t.Fatalf("DecodeAudio: %v", err)
	}
	// Deliberately never drain s.PCM — the feeder should still be able to
	// exit via stopCh instead of leaking, blocked forever on a send.
	s.Stop()

	done := make(chan struct{})
	go func() {
		for range s.PCM {
			// drain until feed's deferred close(s.PCM) fires
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("feed goroutine did not exit after Stop() — PCM channel never closed")
	}
}

func countSamples(t *testing.T, s *AudioStream, events chan AudioEvent) int {
	t.Helper()
	var n int
	deadline := time.After(10 * time.Second)
	for {
		select {
		case samples, ok := <-s.PCM:
			if !ok {
				return n
			}
			n += len(samples)
		case ev := <-events:
			if ev.Err != nil {
				t.Fatalf("decode error: %v", ev.Err)
			}
			if ev.Ended {
				return n
			}
		case <-deadline:
			t.Fatal("timed out counting samples — ffmpeg pipeline likely stuck")
		}
	}
}

func TestDecodeAudioAtSeekSkipsContent(t *testing.T) {
	requireFFmpeg(t)
	path := synthTone(t, 4)

	fullEvents := make(chan AudioEvent, 2)
	full, err := DecodeAudioAt(path, 0, nil, fullEvents)
	if err != nil {
		t.Fatalf("DecodeAudioAt(seek=0): %v", err)
	}
	fullCount := countSamples(t, full, fullEvents)
	full.Stop()

	seekEvents := make(chan AudioEvent, 2)
	seeked, err := DecodeAudioAt(path, 2*time.Second, nil, seekEvents)
	if err != nil {
		t.Fatalf("DecodeAudioAt(seek=2s): %v", err)
	}
	seekedCount := countSamples(t, seeked, seekEvents)
	seeked.Stop()

	if seekedCount >= fullCount {
		t.Errorf("seeked decode produced %d samples, want fewer than the unseeked %d (seeking 2s into a 4s clip should skip roughly half of it)", seekedCount, fullCount)
	}
}
