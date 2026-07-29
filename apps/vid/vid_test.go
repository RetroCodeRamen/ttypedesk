package vid

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/audio"
	"github.com/ttypedesk/ttypedesk/internal/ffdecode"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if !ffdecode.Available() {
		t.Skip("ffmpeg not on PATH, skipping (apt install ffmpeg to run this locally)")
	}
}

// requireAudioDevice skips unless the shared oto context can actually be
// created — see apps/amp's identical helper for why (CI runners
// typically have no real audio hardware).
func requireAudioDevice(t *testing.T) {
	t.Helper()
	pcm := make(chan []int16)
	close(pcm)
	pb, err := audio.Play(pcm)
	if err != nil {
		t.Skipf("no audio device available, skipping: %v", err)
	}
	pb.Stop()
}

// synthClip generates a short real video file with BOTH a video and an
// audio track via ffmpeg's own lavfi sources — exercises Vid's real
// two-process (video + its own audio) decode path end-to-end.
func synthClip(t *testing.T, seconds int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clip.mp4")
	dur := strconv.Itoa(seconds)
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=duration="+dur+":size=64x64:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+dur,
		"-c:v", "libx264", "-c:a", "aac", "-shortest",
		"-y", "-loglevel", "error", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg synth failed: %v\n%s", err, out)
	}
	return path
}

// fakeHost wires only PlayAudio to the real internal/audio package —
// everything else is a no-op. Mirrors apps/amp's own fakeHost.
type fakeHost struct{}

func (fakeHost) Notify(string, string, string)                 {}
func (fakeHost) Launch(string) error                           { return nil }
func (fakeHost) OpenPath(string) error                         { return nil }
func (fakeHost) SetTitle(string)                               {}
func (fakeHost) RequestClose()                                 {}
func (fakeHost) WindowID() string                              { return "test" }
func (fakeHost) SaveCredential(string, []byte) error           { return nil }
func (fakeHost) LoadCredential(string) ([]byte, error)         { return nil, os.ErrNotExist }
func (fakeHost) PickFile(string, []string, func(string, bool)) {}
func (fakeHost) PlayAudio(pcm <-chan []int16) (uiapp.AudioPlayback, error) {
	return audio.Play(pcm)
}
func (fakeHost) ClipboardGet() string { return "" }
func (fakeHost) ClipboardSet(string)  {}

func newTestApp(t *testing.T) (*App, *uiapp.Context) {
	t.Helper()
	ctx := uiapp.NewContext("test", 70, 22, func() {})
	ctx.SetHost(fakeHost{})
	a := New()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a, ctx
}

func TestNewStartsWithNothingLoaded(t *testing.T) {
	a, _ := newTestApp(t)
	if a.video != nil || a.path != "" {
		t.Error("a fresh App should have no video/path loaded")
	}
	if a.clock.Playing() {
		t.Error("a fresh player should start paused")
	}
}

func TestTogglePauseWithNothingLoadedIsSafe(t *testing.T) {
	a, _ := newTestApp(t)
	a.togglePause() // must not panic
	if a.clock.Playing() {
		t.Error("togglePause with nothing loaded and no path shouldn't start playback")
	}
}

func TestSeekWithNothingLoadedIsANoop(t *testing.T) {
	a, _ := newTestApp(t)
	a.seek(5 * time.Second) // must not panic
	if a.video != nil {
		t.Error("seek with no path loaded shouldn't start decode")
	}
}

func TestStopWithNothingLoadedIsSafe(t *testing.T) {
	a, _ := newTestApp(t)
	a.stop() // must not panic
	if a.clock.Playing() {
		t.Error("stop should leave the clock paused")
	}
}

func TestPlayAtDrivesRealVideoAndAudioEndToEnd(t *testing.T) {
	requireFFmpeg(t)
	requireAudioDevice(t)
	path := synthClip(t, 2)

	a, _ := newTestApp(t)
	a.playAt(path, 0)

	if a.video == nil {
		t.Fatalf("playAt failed: %s", a.status)
	}
	if !a.clock.Playing() {
		t.Error("clock is not playing after playAt")
	}
	if a.playback == nil {
		t.Error("no audio playback started — expected the synthesized clip's audio track to decode")
	}

	// Let real decode actually flow, then confirm a real frame shows up.
	deadline := time.After(10 * time.Second)
	var gotFrame bool
	for !gotFrame {
		select {
		case f, ok := <-a.video.Frames:
			if ok && len(f.Pix) > 0 {
				gotFrame = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for a real decoded video frame")
		}
	}

	a.togglePause()
	if a.clock.Playing() {
		t.Error("togglePause() while playing should pause")
	}
	a.togglePause()
	if !a.clock.Playing() {
		t.Error("togglePause() while paused should resume")
	}

	a.seek(1 * time.Second)
	if a.video == nil {
		t.Fatalf("seek failed: %s", a.status)
	}
	if pos := a.clock.Position(); pos < 1*time.Second {
		t.Errorf("Position() = %v after seeking to 1s, want >= 1s", pos)
	}

	a.stop()
	if a.video != nil || a.path != "" {
		t.Error("stop() should tear down video and clear path")
	}
	if a.clock.Playing() {
		t.Error("clock should be paused after stop()")
	}
}
