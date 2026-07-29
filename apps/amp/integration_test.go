package amp

import (
	"os"
	"testing"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/audio"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// fakeHost wires only PlayAudio to the real internal/audio package —
// everything else is a no-op. This is the one place amp's own tests reach
// past startDecode to prove App.playAt really drives real playback
// end-to-end (App -> Context -> Host.PlayAudio -> internal/audio -> a real
// oto.Context), not just that each piece works in isolation.
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

// failingAudioHost is fakeHost but PlayAudio always errors — for testing
// playAt's own error-recovery path without needing to actually break real
// audio playback.
type failingAudioHost struct{ fakeHost }

func (failingAudioHost) PlayAudio(pcm <-chan []int16) (uiapp.AudioPlayback, error) {
	return nil, errAudioFixture
}

var errAudioFixture = fixtureErr("no audio device")

type fixtureErr string

func (e fixtureErr) Error() string { return string(e) }

// requireAudioDevice skips unless the shared oto context can actually be
// created — CI runners typically have no real audio hardware at all
// (unlike this sandbox, which happens to have some backend available),
// so this needs the same graceful-skip treatment as requireFFmpeg rather
// than failing the build over something no CI audio driver can fix.
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

func TestAppPlayAtDrivesRealPlaybackEndToEnd(t *testing.T) {
	requireFFmpeg(t)
	requireAudioDevice(t)
	path := synthTone(t, 1)

	ctx := uiapp.NewContext("test", 60, 20, func() {})
	ctx.SetHost(fakeHost{})

	a := New()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer a.Close()

	a.playlist = []string{path}
	a.playAt(0)
	if a.status != "" && a.current == -1 {
		t.Fatalf("playAt failed: %s", a.status)
	}
	if a.current != 0 {
		t.Fatalf("current = %d, want 0 after playAt", a.current)
	}
	if !a.clock.Playing() {
		t.Error("clock is not playing after playAt")
	}

	// Let real audio actually flow for a moment, then drain the "track
	// ended" event the same way Draw would.
	time.Sleep(300 * time.Millisecond)
	a.togglePause()
	if a.clock.Playing() {
		t.Error("togglePause() while playing should pause")
	}
	a.togglePause()
	if !a.clock.Playing() {
		t.Error("togglePause() while paused should resume")
	}

	a.stop()
	if a.current != -1 {
		t.Errorf("current = %d, want -1 after stop()", a.current)
	}
	if a.clock.Playing() {
		t.Error("clock should be paused after stop()")
	}
}

// TestPlayAtResetsCurrentWhenPlayAudioFailsAfterAnotherTrackWasPlaying is a
// regression test: playAt used to leave a.current pointing at whatever
// was playing before, even though teardown() had already torn it down,
// if the *new* track's PlayAudio call failed — the UI would then still
// show a "now playing" track that wasn't actually playing.
func TestPlayAtResetsCurrentWhenPlayAudioFailsAfterAnotherTrackWasPlaying(t *testing.T) {
	requireFFmpeg(t)
	path := synthTone(t, 1)

	ctx := uiapp.NewContext("test", 60, 20, func() {})
	ctx.SetHost(failingAudioHost{})

	a := New()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer a.Close()

	a.playlist = []string{path, path}
	a.current = 0 // simulate "track 0 was already playing" without a real prior PlayAudio call

	a.playAt(1)

	if a.current != -1 {
		t.Errorf("current = %d, want -1 — PlayAudio failed, teardown already ran, nothing is actually playing", a.current)
	}
}
