package audiocap

import (
	"os/exec"
	"testing"
	"time"
)

// requirePulse skips the test if there's no reachable PulseAudio/PipeWire
// server — real hardware/daemon dependency, not something to fake.
func requirePulse(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pactl"); err != nil {
		t.Skip("pactl not on PATH")
	}
	if err := exec.Command("pactl", "info").Run(); err != nil {
		t.Skip("no reachable PulseAudio/PipeWire server")
	}
}

func TestCaptureFailsClearlyWithoutParec(t *testing.T) {
	if Available() {
		t.Skip("parec is on PATH in this environment — covered by TestCaptureReceivesRealAudio instead")
	}
	if _, err := Capture(nil); err == nil {
		t.Error("Capture succeeded with no parec on PATH")
	}
}

// TestCaptureReceivesRealAudio plays a real tone into the default sink via
// ffmpeg|paplay and asserts our parec-backed Capture actually reads
// non-silent PCM back off its monitor — a genuine subprocess-to-subprocess
// round trip rather than a mocked reader, matching how internal/ffdecode's
// own tests exercise a real ffmpeg.
func TestCaptureReceivesRealAudio(t *testing.T) {
	if !Available() {
		t.Skip("parec not on PATH")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	if _, err := exec.LookPath("paplay"); err != nil {
		t.Skip("paplay not on PATH")
	}
	requirePulse(t)

	events := make(chan Event, 1)
	stream, err := Capture(events)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	defer stream.Stop()

	play := exec.Command("bash", "-c",
		"ffmpeg -f lavfi -i sine=frequency=440:duration=5 -f wav -loglevel error pipe:1 | paplay")
	if err := play.Start(); err != nil {
		t.Skip("could not start ffmpeg|paplay tone generator: " + err.Error())
	}
	defer func() {
		if play.Process != nil {
			_ = play.Process.Kill()
		}
	}()

	deadline := time.After(15 * time.Second)
	for {
		select {
		case samples, ok := <-stream.PCM:
			if !ok {
				t.Fatal("PCM channel closed before receiving audio")
			}
			for _, s := range samples {
				if s != 0 {
					return // got real, non-silent audio — test passes
				}
			}
		case ev := <-events:
			t.Fatalf("capture ended unexpectedly: %v", ev.Err)
		case <-deadline:
			t.Fatal("timed out waiting for non-silent audio from the monitor source")
		}
	}
}
