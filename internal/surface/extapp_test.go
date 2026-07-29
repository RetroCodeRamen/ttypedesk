package surface

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
type fakeHost struct {
	mu       sync.Mutex
	titles   []string
	notifies []string
	launches []string
	closed   bool
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
func (h *fakeHost) WindowID() string                                          { return "w1" }
func (h *fakeHost) SaveCredential(key string, value []byte) error             { return nil }
func (h *fakeHost) LoadCredential(key string) ([]byte, error)                 { return nil, os.ErrNotExist }
func (h *fakeHost) PlayAudio(pcm <-chan []int16) (uiapp.AudioPlayback, error) { return nil, nil }
func (h *fakeHost) PickFile(startDir string, extensions []string, onResult func(path string, ok bool)) {
	if onResult != nil {
		onResult("", false)
	}
}

func (h *fakeHost) isClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
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

func TestNewExtAppReturnsErrorForMissingBinary(t *testing.T) {
	if _, err := NewExtApp("w1", "/no/such/extapp-binary-xyz", nil, 40, 12); err == nil {
		t.Fatal("NewExtApp succeeded with a nonexistent binary")
	}
}
