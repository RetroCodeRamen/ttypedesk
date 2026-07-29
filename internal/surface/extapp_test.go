package surface

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/audio"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// helloBinary is built once from cmd/extapp-hello — a real reference
// implementation of the protocol (see docs/extapp.md), not a mock, so
// these tests exercise an actual subprocess round trip exactly like
// internal/ffdecode and internal/audiocap's tests do with ffmpeg/parec.
var helloBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "extapp-test-*")
	if err != nil {
		panic(err)
	}
	helloBinary = filepath.Join(dir, "extapp-hello")
	build := exec.Command("go", "build", "-o", helloBinary, "../../cmd/extapp-hello")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(dir)
		panic("building extapp-hello fixture: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeHost records every uiapp.Host call an ExtAppSurface forwards, so
// tests can assert on notify/launch/title/close without a real Server.
// Credentials and clipboard are real in-memory implementations (not just
// no-ops) so round-trip tests through the wire protocol actually prove
// something; PlayAudio goes through the real internal/audio package, same
// as apps/amp and apps/vid's own test fakes, so audio tests exercise a
// genuine oto.Context rather than stopping at "a channel was fed."
type fakeHost struct {
	mu         sync.Mutex
	titles     []string
	notifies   []string
	launches   []string
	closed     bool
	creds      map[string][]byte
	clipboard  string
	pickResult func(startDir string, extensions []string) (path string, ok bool) // nil = cancel
	pickCalls  []pickFileCall
}

type pickFileCall struct {
	startDir   string
	extensions []string
}

func (h *fakeHost) Notify(title, body, icon string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notifies = append(h.notifies, title+": "+body)
}
func (h *fakeHost) Launch(action string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.launches = append(h.launches, action)
	return nil
}
func (h *fakeHost) OpenPath(path string) error { return nil }
func (h *fakeHost) SetTitle(title string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.titles = append(h.titles, title)
}
func (h *fakeHost) RequestClose() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
}
func (h *fakeHost) WindowID() string { return "w1" }

func (h *fakeHost) SaveCredential(key string, value []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.creds == nil {
		h.creds = map[string][]byte{}
	}
	h.creds[key] = append([]byte(nil), value...)
	return nil
}
func (h *fakeHost) LoadCredential(key string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.creds[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), v...), nil
}

func (h *fakeHost) ClipboardGet() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clipboard
}
func (h *fakeHost) ClipboardSet(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clipboard = text
}

func (h *fakeHost) PlayAudio(pcm <-chan []int16) (uiapp.AudioPlayback, error) {
	return audio.Play(pcm)
}

// requireAudioDevice skips unless the shared oto context can actually be
// created — CI runners typically have no real audio hardware at all
// (unlike this sandbox), same treatment apps/amp's own tests give this.
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

func (h *fakeHost) PickFile(startDir string, extensions []string, onResult func(path string, ok bool)) {
	h.mu.Lock()
	h.pickCalls = append(h.pickCalls, pickFileCall{startDir, extensions})
	fn := h.pickResult
	h.mu.Unlock()
	path, ok := "", false
	if fn != nil {
		path, ok = fn(startDir, extensions)
	}
	if onResult != nil {
		onResult(path, ok)
	}
}

func (h *fakeHost) isClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

func (h *fakeHost) clipboardValue() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clipboard
}

var _ uiapp.Host = (*fakeHost)(nil)

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

// gridText reconstructs cells into rows of text for substring assertions.
func gridText(cells []cell.Cell, cols, rows int) string {
	var b strings.Builder
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			i := row*cols + col
			if i >= len(cells) {
				continue
			}
			b.WriteRune(cells[i].Rune)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestExtAppSurfaceDrawsRealSubprocessOutput(t *testing.T) {
	s, err := NewExtApp("w1", helloBinary, nil, 40, 12)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	host := &fakeHost{}
	if err := s.BindHost(host); err != nil {
		t.Fatalf("BindHost: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), 40, 12), "extapp-hello")
	})
	if strings.Contains(s.Title(), "[crashed]") {
		t.Fatalf("surface reported crashed: %s", s.Title())
	}
}

func TestExtAppSurfaceHandleInputMouseClickUpdatesState(t *testing.T) {
	s, err := NewExtApp("w1", helloBinary, nil, 40, 12)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	if err := s.BindHost(&fakeHost{}); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), 40, 12), "Clicks: 0")
	})

	if err := s.HandleInput(InputEvent{Kind: "mouse", Action: "press", X: 5, Y: 5, Button: 1}); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), 40, 12), "Clicks: 1")
	})
}

func TestExtAppSurfaceQKeyRequestsClose(t *testing.T) {
	s, err := NewExtApp("w1", helloBinary, nil, 40, 12)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	host := &fakeHost{}
	if err := s.BindHost(host); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), 40, 12), "extapp-hello")
	})

	if err := s.HandleInput(InputEvent{Kind: "key", Rune: 'q'}); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff() // drains the close_window message and dispatches RequestClose
		return host.isClosed()
	})
}

func TestExtAppSurfaceResizeAppliesNewGrid(t *testing.T) {
	s, err := NewExtApp("w1", helloBinary, nil, 40, 12)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	if err := s.BindHost(&fakeHost{}); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), 40, 12), "extapp-hello")
	})

	s.Resize(60, 20)
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		cols, rows := s.Size()
		return cols == 60 && rows == 20 && len(s.Snapshot()) == 60*20
	})
}

func TestExtAppSurfaceMarksCrashedWhenProcessExits(t *testing.T) {
	s, err := NewExtApp("w1", "sh", []string{"-c", "exit 0"}, 40, 12)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	if err := s.BindHost(&fakeHost{}); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(s.Title(), "[crashed]")
	})
}

// extapp-hello needs more rows than the other tests' 40x12 to fit its
// credential/clipboard/pick/audio status lines (see cmd/extapp-hello's
// drawLocked) without them running off the bottom of the grid.
const wideCols, wideRows = 50, 18

func TestExtAppSurfaceCredentialSaveLoadRoundTrip(t *testing.T) {
	s, err := NewExtApp("w1", helloBinary, nil, wideCols, wideRows)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	host := &fakeHost{}
	if err := s.BindHost(host); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "extapp-hello")
	})

	if err := s.HandleInput(InputEvent{Kind: "key", Rune: 'c'}); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "round-trip OK: hello from extapp-hello")
	})
	// The round trip went through the real save/load path, not just a
	// coincidental string match — confirm the fake credential store
	// actually holds it under the key extapp-hello used.
	host.mu.Lock()
	got, ok := host.creds["extapp-hello-demo"]
	host.mu.Unlock()
	if !ok || string(got) != "hello from extapp-hello" {
		t.Fatalf("fakeHost.creds[extapp-hello-demo] = %q, %v; want %q, true", got, ok, "hello from extapp-hello")
	}
}

func TestExtAppSurfaceClipboardRoundTrip(t *testing.T) {
	s, err := NewExtApp("w1", helloBinary, nil, wideCols, wideRows)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	host := &fakeHost{}
	if err := s.BindHost(host); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "extapp-hello")
	})

	if err := s.HandleInput(InputEvent{Kind: "key", Rune: 'v'}); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "now: hello from extapp-hello")
	})
	if got := host.clipboardValue(); got != "hello from extapp-hello" {
		t.Fatalf("fakeHost clipboard = %q, want %q (proves clipboard_set actually landed, not just clipboard_get echoing something else)", got, "hello from extapp-hello")
	}
}

func TestExtAppSurfacePickFileReturnsChosenPath(t *testing.T) {
	s, err := NewExtApp("w1", helloBinary, nil, wideCols, wideRows)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	host := &fakeHost{pickResult: func(startDir string, extensions []string) (string, bool) {
		return "/tmp/chosen.txt", true
	}}
	if err := s.BindHost(host); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "extapp-hello")
	})

	if err := s.HandleInput(InputEvent{Kind: "key", Rune: 'f'}); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "picked: /tmp/chosen.txt")
	})
	host.mu.Lock()
	calls := host.pickCalls
	host.mu.Unlock()
	if len(calls) != 1 || calls[0].startDir != "/tmp" {
		t.Fatalf("pickCalls = %+v, want one call with startDir=/tmp", calls)
	}
}

func TestExtAppSurfacePickFileCancelled(t *testing.T) {
	s, err := NewExtApp("w1", helloBinary, nil, wideCols, wideRows)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	// No pickResult configured — fakeHost.PickFile defaults to a cancel.
	if err := s.BindHost(&fakeHost{}); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "extapp-hello")
	})

	if err := s.HandleInput(InputEvent{Kind: "key", Rune: 'f'}); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "cancelled")
	})
}

// TestExtAppSurfaceAudioPlaysAndStops drives extapp-hello's own sine-tone
// generator through play_audio/audio_chunk/stop_audio end-to-end against a
// real internal/audio.Playback (fakeHost.PlayAudio), the same bar as
// apps/amp and apps/vid's own audio tests — not just checking that a
// message was received.
func TestExtAppSurfaceAudioPlaysAndStops(t *testing.T) {
	requireAudioDevice(t)
	s, err := NewExtApp("w1", helloBinary, nil, wideCols, wideRows)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	if err := s.BindHost(&fakeHost{}); err != nil {
		t.Fatalf("BindHost: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "extapp-hello")
	})

	if err := s.HandleInput(InputEvent{Kind: "key", Rune: 'a'}); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "Audio (a): on")
	})
	// A real internal/audio.Playback should now exist and be playing.
	waitFor(t, 2*time.Second, func() bool {
		s.mu.Lock()
		pb := s.audioPlayback
		s.mu.Unlock()
		return pb != nil && pb.Playing()
	})

	// Let a couple of real chunks actually flow before stopping.
	time.Sleep(200 * time.Millisecond)

	if err := s.HandleInput(InputEvent{Kind: "key", Rune: 'a'}); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), wideCols, wideRows), "Audio (a): off")
	})
	waitFor(t, 2*time.Second, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.audioPCM == nil && s.audioPlayback == nil
	})
}

func TestNewExtAppReturnsErrorForMissingBinary(t *testing.T) {
	if _, err := NewExtApp("w1", "/no/such/extapp-binary-xyz", nil, 40, 12); err == nil {
		t.Fatal("NewExtApp succeeded with a nonexistent binary")
	}
}
