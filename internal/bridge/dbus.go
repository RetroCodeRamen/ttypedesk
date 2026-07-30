package bridge

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/slog"
)

// setupATSPI best-effort spins up a private session bus + at-spi2-registryd
// + AT-SPI client for the text overlay. Any failure is non-fatal — it logs
// a warning and returns a11y=nil, and the bridge just runs raster-only, the
// same way it always has. Returns the started processes (nil if that step
// was never reached) so the caller can clean them up regardless of where
// setup stopped, plus the session bus address for the guest process's env.
func setupATSPI(id string) (busCmd, registrydCmd *exec.Cmd, a11y *atspiClient, busAddr string) {
	busCmd, busAddr, err := startSessionBus()
	if err != nil {
		slog.Warn("bridge id=%s: AT-SPI text overlay unavailable (%v) — continuing raster-only", id, err)
		return nil, nil, nil, ""
	}
	registrydCmd, err = startRegistryd(busAddr)
	if err != nil {
		slog.Warn("bridge id=%s: AT-SPI text overlay unavailable (%v) — continuing raster-only", id, err)
		killWait(busCmd)
		return nil, nil, nil, ""
	}
	client, err := connectATSPI(busAddr)
	if err != nil {
		slog.Warn("bridge id=%s: AT-SPI text overlay unavailable (%v) — continuing raster-only", id, err)
		killWait(registrydCmd)
		killWait(busCmd)
		return nil, nil, nil, ""
	}
	if err := client.enable(); err != nil {
		// Non-fatal even here — enabling is what unlocks Electron-style
		// trees, but native GTK/Qt apps build their tree regardless.
		slog.Warn("bridge id=%s: org.a11y.Status enable failed (%v) — text overlay may be limited", id, err)
	}
	return busCmd, registrydCmd, client, busAddr
}

// startSessionBus launches a private D-Bus session bus (not the host's —
// one per BridgeSurface, same isolation model as its dedicated Xvfb) and
// returns its address, read off the process's own stdout.
func startSessionBus() (*exec.Cmd, string, error) {
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		return nil, "", fmt.Errorf("dbus-daemon not found on PATH")
	}
	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("dbus-daemon stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start dbus-daemon: %w", err)
	}
	addrCh := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		if sc.Scan() {
			addrCh <- sc.Text()
		}
		close(addrCh)
	}()
	select {
	case addr, ok := <-addrCh:
		if !ok || addr == "" {
			killWait(cmd)
			return nil, "", fmt.Errorf("dbus-daemon didn't print an address")
		}
		return cmd, addr, nil
	case <-time.After(5 * time.Second):
		killWait(cmd)
		return nil, "", fmt.Errorf("dbus-daemon did not start within 5s")
	}
}

// startRegistryd launches at-spi2-registryd against busAddr — required
// before any app on that bus will register an accessible tree.
func startRegistryd(busAddr string) (*exec.Cmd, error) {
	regBin := findRegistryd()
	if regBin == "" {
		return nil, fmt.Errorf("at-spi2-registryd not found (checked PATH and /usr/libexec) — install at-spi2-core")
	}
	cmd := exec.Command(regBin, "--use-gnome-session=no")
	cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS="+busAddr)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start at-spi2-registryd: %w", err)
	}
	// No readiness signal on stdout to wait on (unlike Xvfb/dbus-daemon) —
	// give it a moment to register org.a11y.atspi.Registry before the
	// caller starts issuing D-Bus calls against it.
	time.Sleep(300 * time.Millisecond)
	return cmd, nil
}

// findRegistryd checks PATH first, then the common libexec locations
// distros actually install it to (it's an internal daemon, not meant to be
// run interactively, so it's rarely on PATH).
func findRegistryd() string {
	if p, err := exec.LookPath("at-spi2-registryd"); err == nil {
		return p
	}
	for _, p := range []string{
		"/usr/libexec/at-spi2-registryd",
		"/usr/lib/at-spi2-core/at-spi2-registryd",
		"/usr/lib/x86_64-linux-gnu/at-spi2-core/at-spi2-registryd",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
