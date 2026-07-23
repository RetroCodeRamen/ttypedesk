package tty

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Handle wraps a PTY-backed process.
type Handle struct {
	Cmd  *exec.Cmd
	File *os.File
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
	return ptySetsize(h.File, cols, rows)
}

func (h *Handle) Write(p []byte) (int, error) {
	if h.File == nil {
		return 0, os.ErrClosed
	}
	return h.File.Write(p)
}

func (h *Handle) Read(p []byte) (int, error) {
	if h.File == nil {
		return 0, os.ErrClosed
	}
	return h.File.Read(p)
}

// Close unblocks readers immediately, then reaps the child without hanging.
func (h *Handle) Close() error {
	f := h.File
	h.File = nil
	if f != nil {
		_ = f.Close()
	}
	if h.Cmd != nil && h.Cmd.Process != nil {
		proc := h.Cmd.Process
		_ = proc.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = h.Cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(150 * time.Millisecond):
			_ = proc.Kill()
			go func() { _ = h.Cmd.Wait() }()
		}
	}
	return nil
}
