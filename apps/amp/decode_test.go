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
// ffmpeg's lavfi source, for tests that need a real, valid audio file
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

// The ffmpeg-subprocess pipeline itself (spawn, backpressure, stop,
// missing-file handling) is internal/ffdecode's own responsibility and
// tested there against real ffmpeg. What's specific to amp's decoder
// wrapper is the visualizer — this confirms updateVis actually gets
// wired up as ffdecode's onChunk hook and produces real, non-flat data
// from a real decoded tone, not just that it type-checks.
func TestDecoderVisualizerPopulatesFromRealAudio(t *testing.T) {
	requireFFmpeg(t)
	path := synthTone(t, 1)

	events := make(chan trackEvent, 2)
	dec, err := startDecode(path, events)
	if err != nil {
		t.Fatalf("startDecode: %v", err)
	}
	defer dec.stop()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case _, ok := <-dec.pcm:
			if !ok {
				t.Fatal("pcm channel closed before the visualizer ever saw non-zero data")
			}
			bars := dec.Vis()
			var anyNonZero bool
			for _, b := range bars {
				if b > 0 {
					anyNonZero = true
					break
				}
			}
			if anyNonZero {
				return // success
			}
		case ev := <-events:
			if ev.Err != nil {
				t.Fatalf("decode error: %v", ev.Err)
			}
		case <-deadline:
			t.Fatal("timed out waiting for the visualizer to reflect real decoded audio")
		}
	}
}
