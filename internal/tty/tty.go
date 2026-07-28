package tty

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Handle wraps a PTY-backed process. File/Cmd are exported for existing
// callers, but Close can run concurrently with a blocked Read/Write in the
// PTY read-loop goroutine (that's how "Close unblocks readers immediately"
// works — closing the fd out from under a blocking Read is intentional), so
// every access to the fields themselves goes through mu.
//
// Read/Write only hold mu for the pointer capture, then call the os.File
// method unlocked — safe because os.File's Read/Write/Close are themselves
// mutually synchronized via the runtime's internal poll.FD refcounting.
// SetSize is different: it goes through creack/pty's Setsize, which calls
// File.Fd() — and Fd() explicitly bypasses that poll.FD protection (per the
// os.File docs), so it's not safe against a concurrent Close at the OS
// level. SetSize and the Close side that actually closes the fd both hold
// mu for their whole call, not just the pointer swap, to cover that gap.
type Handle struct {
	Cmd  *exec.Cmd
	File *os.File

	mu sync.Mutex
}

// Spawn starts cmd in a PTY of the given size with truecolor-friendly env.
func Spawn(command string, args []string, cols, rows int) (*Handle, error) {
	c := exec.Command(command, args...)
	c.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)
	f, err := ptyStart(c, cols, rows)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}
	return &Handle{Cmd: c, File: f}, nil
}

func (h *Handle) SetSize(cols, rows int) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.File == nil {
		return os.ErrClosed
	}
	return ptySetsize(h.File, cols, rows)
}

func (h *Handle) Write(p []byte) (int, error) {
	h.mu.Lock()
	f := h.File
	h.mu.Unlock()
	if f == nil {
		return 0, os.ErrClosed
	}
	return f.Write(p)
}

func (h *Handle) Read(p []byte) (int, error) {
	h.mu.Lock()
	f := h.File
	h.mu.Unlock()
	if f == nil {
		return 0, os.ErrClosed
	}
	return f.Read(p)
}

// Close unblocks readers immediately, then reaps the child without hanging.
func (h *Handle) Close() error {
	h.mu.Lock()
	f := h.File
	h.File = nil
	cmd := h.Cmd
	if f != nil {
		_ = f.Close()
	}
	h.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		proc := cmd.Process
		_ = proc.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(150 * time.Millisecond):
			_ = proc.Kill()
			go func() { _ = cmd.Wait() }()
		}
	}
	return nil
}
