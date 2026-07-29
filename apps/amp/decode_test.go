package amp

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// requireFFmpeg skips a test unless ffmpeg is actually on PATH — like the
// GUI-TUI Bridge's X11-dependent tests, this needs a real external tool
// CI installs but a bare dev machine might not have.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not on PATH, skipping (apt install ffmpeg to run this locally)")
	}
}

// synthTone generates a short real WAV file (a 440Hz sine tone) via
// ffmpeg's lavfi source — the same tool amp itself shells out to, so this
// exercises startDecode against a real, valid audio file rather than a
// hand-rolled fixture.
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

func TestStartDecodeProducesRealAudibleSamples(t *testing.T) {
	requireFFmpeg(t)
	path := synthTone(t, 1)

	events := make(chan trackEvent, 2)
	dec, err := startDecode(path, events)
	if err != nil {
		t.Fatalf("startDecode: %v", err)
	}
	defer dec.stop()

	var gotNonZero bool
	var totalSamples int
	deadline := time.After(10 * time.Second)
loop:
	for {
		select {
		case samples, ok := <-dec.pcm:
			if !ok {
				break loop
			}
			totalSamples += len(samples)
			for _, s := range samples {
				if s != 0 {
					gotNonZero = true
				}
			}
		case ev := <-events:
			if ev.err != nil {
				t.Fatalf("decode error: %v", ev.err)
			}
			if ev.ended {
				break loop
			}
		case <-deadline:
			t.Fatal("timed out waiting for decoded samples — ffmpeg pipeline likely stuck")
		}
	}
	if !gotNonZero {
		t.Error("all decoded samples were zero — expected an audible 440Hz tone")
	}
	// 1 second at 48kHz stereo = 96000 samples; allow slack for ffmpeg's
	// own startup/flush overhead rather than asserting an exact count.
	if totalSamples < 40000 {
		t.Errorf("totalSamples = %d, want at least ~40000 for a 1s stereo tone at 48kHz", totalSamples)
	}
}

func TestStartDecodeMissingFileReportsErrorEvent(t *testing.T) {
	requireFFmpeg(t)
	events := make(chan trackEvent, 2)
	dec, err := startDecode(filepath.Join(t.TempDir(), "does-not-exist.mp3"), events)
	if err != nil {
		// Some ffmpeg builds fail fast at Start(); either outcome is fine.
		return
	}
	defer dec.stop()

	select {
	case ev := <-events:
		if ev.err == nil && !ev.ended {
			t.Error("expected an error (or at worst an immediate empty-ended) event for a missing input file")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a decode-failure event")
	}
}

func TestStartDecodeStopUnblocksFeedGoroutine(t *testing.T) {
	requireFFmpeg(t)
	path := synthTone(t, 5) // long enough that we can stop mid-decode

	events := make(chan trackEvent, 2)
	dec, err := startDecode(path, events)
	if err != nil {
		t.Fatalf("startDecode: %v", err)
	}
	// Deliberately never drain dec.pcm — the feeder should still be able
	// to exit via stopCh instead of leaking, blocked forever on a send.
	dec.stop()

	done := make(chan struct{})
	go func() {
		for range dec.pcm {
			// drain until feed's deferred close(d.pcm) fires
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("feed goroutine did not exit after stop() — pcm channel never closed")
	}
}
