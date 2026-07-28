package bridge

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jezek/xgb"
)

// pickDisplay finds an unused X display number by checking for the absence
// of its Unix socket. Not fully race-free against another process grabbing
// the same number between the check and Xvfb's own bind, but Xvfb exits
// immediately if the display is already taken, which startXvfb surfaces as
// an error the caller can retry.
func pickDisplay() (int, error) {
	for n := 50; n < 200; n++ {
		if _, err := os.Stat(filepath.Join("/tmp/.X11-unix", "X"+strconv.Itoa(n))); os.IsNotExist(err) {
			return n, nil
		}
	}
	return 0, fmt.Errorf("no free X display found in :50-:199")
}

// startXvfb launches a virtual framebuffer X server at the given pixel size
// and waits until it's accepting connections.
func startXvfb(display, w, h int) (*exec.Cmd, error) {
	if _, err := exec.LookPath("Xvfb"); err != nil {
		return nil, fmt.Errorf("Xvfb not found on PATH — install it (e.g. apt install xvfb) to use bridged GUI apps")
	}
	geom := fmt.Sprintf("%dx%dx24", w, h)
	cmd := exec.Command("Xvfb", fmt.Sprintf(":%d", display), "-screen", "0", geom, "-nolisten", "tcp")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Xvfb: %w", err)
	}
	if err := waitDisplayReady(display, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	return cmd, nil
}

func waitDisplayReady(display int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf(":%d", display)
	for time.Now().Before(deadline) {
		conn, err := xgb.NewConnDisplay(addr)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("Xvfb on display %s did not become ready within %s", addr, timeout)
}

// connectRetry opens the real, kept connection to display. Xvfb accepting
// a connection (what waitDisplayReady checks) doesn't always mean it's
// ready to service a *second* one moments later — observed as an
// occasional "connection reset by peer" on the very next connect — so this
// retries a few times rather than treating one reset as fatal.
func connectRetry(display, attempts int, delay time.Duration) (*xgb.Conn, error) {
	addr := fmt.Sprintf(":%d", display)
	var lastErr error
	for i := 0; i < attempts; i++ {
		conn, err := xgb.NewConnDisplay(addr)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		time.Sleep(delay)
	}
	return nil, lastErr
}

// startGuest launches the bridged GUI command against display.
func startGuest(display int, command string) (*exec.Cmd, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=:%d", display))
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start guest %q: %w", command, err)
	}
	return cmd, nil
}
