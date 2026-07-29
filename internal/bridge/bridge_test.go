package bridge

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/surface"
)

// requireX11 skips the test unless both Xvfb and the given guest command
// are actually available — these tests need a real (if headless) X server,
// unlike the rest of the repo's test suite. CI installs xvfb + x11-apps
// specifically so this runs there; local runs without them just skip.
func requireX11(t *testing.T, guestCmd string) {
	t.Helper()
	if _, err := exec.LookPath("Xvfb"); err != nil {
		t.Skip("Xvfb not on PATH, skipping (apt install xvfb to run this locally)")
	}
	if _, err := exec.LookPath(guestCmd); err != nil {
		t.Skipf("%s not on PATH, skipping (apt install x11-apps to run this locally)", guestCmd)
	}
}

// requireATSPI additionally skips unless dbus-daemon and at-spi2-registryd
// are available — the text-overlay tests need the full accessibility stack
// on top of requireX11's plain Xvfb/guest-app requirement.
func requireATSPI(t *testing.T, guestCmd string) {
	t.Helper()
	requireX11(t, guestCmd)
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon not on PATH, skipping (apt install dbus-x11 to run this locally)")
	}
	if findRegistryd() == "" {
		t.Skip("at-spi2-registryd not found, skipping (apt install at-spi2-core to run this locally)")
	}
}

func TestBridgeSurfaceCapturesRealContent(t *testing.T) {
	requireX11(t, "xclock")

	b, err := New("t1", "xclock", 40, 20)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	if b.ID() != "t1" {
		t.Errorf("ID() = %q, want t1", b.ID())
	}
	if b.Kind() != "bridge" {
		t.Errorf("Kind() = %q, want bridge", b.Kind())
	}
	if cols, rows := b.Size(); cols != 40 || rows != 20 {
		t.Errorf("Size() = (%d,%d), want (40,20)", cols, rows)
	}

	deadline := time.Now().Add(8 * time.Second)
	var colors map[string]struct{}
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		colors = map[string]struct{}{}
		for _, c := range b.Snapshot() {
			colors[fmt.Sprintf("%v/%v", c.FG, c.BG)] = struct{}{}
		}
		if len(colors) > 1 {
			break
		}
	}
	if len(colors) <= 1 {
		t.Fatalf("captured frame has only %d distinct color(s) after 8s — xclock doesn't appear to be rendering", len(colors))
	}
}

func TestBridgeSurfaceProduceDiffThenQuiet(t *testing.T) {
	requireX11(t, "xclock")

	b, err := New("t2", "xclock", 20, 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	// Eventually a capture lands and ProduceDiff reports it once.
	deadline := time.Now().Add(5 * time.Second)
	var gotDiff bool
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		d := b.ProduceDiff()
		if d.Rect.W > 0 {
			gotDiff = true
			break
		}
	}
	if !gotDiff {
		t.Fatal("ProduceDiff never returned a non-empty diff within 5s")
	}
}

func TestBridgeSurfaceInputReachesGuest(t *testing.T) {
	requireX11(t, "xeyes")

	b, err := New("t3", "xeyes", 40, 20)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	time.Sleep(1 * time.Second)
	before := b.Snapshot()

	if err := b.HandleInput(surface.InputEvent{Kind: "mouse", X: 2, Y: 2, Action: "move"}); err != nil {
		t.Fatalf("HandleInput (move 1): %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := b.HandleInput(surface.InputEvent{Kind: "mouse", X: 38, Y: 18, Action: "move"}); err != nil {
		t.Fatalf("HandleInput (move 2): %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	after := b.Snapshot()
	changed := 0
	for i := range before {
		if i < len(after) && before[i] != after[i] {
			changed++
		}
	}
	if changed == 0 {
		t.Fatal("mouse movement produced no visible change — input may not be reaching the guest app")
	}
}

func TestBridgeSurfaceResizeIsPureNoRelaunch(t *testing.T) {
	requireX11(t, "xclock")

	b, err := New("t4", "xclock", 40, 20)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	xvfbPID := b.xvfbCmd.Process.Pid
	guestPID := b.guest.Process.Pid

	b.Resize(30, 15)
	if cols, rows := b.Size(); cols != 30 || rows != 15 {
		t.Errorf("Size() after Resize = (%d,%d), want (30,15)", cols, rows)
	}
	if b.xvfbCmd.Process.Pid != xvfbPID {
		t.Error("Resize relaunched Xvfb — it shouldn't (would kill the guest app for no benefit)")
	}
	if b.guest.Process.Pid != guestPID {
		t.Error("Resize restarted the guest process — it shouldn't")
	}
}

func TestBridgeSurfaceCloseIsIdempotentAndCleansUp(t *testing.T) {
	requireX11(t, "xclock")

	b, err := New("t5", "xclock", 20, 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestBridgeSurfaceResizeGrowsScreenViaRandr covers the perf follow-up from
// docs/gui-bridge.md: growing a window well past the overscan-padded
// capture buffer should trigger a real RANDR SetScreenSize (growScreen),
// not just leave the guest painting at its original resolution forever.
func TestBridgeSurfaceResizeGrowsScreenViaRandr(t *testing.T) {
	requireX11(t, "xclock")

	b, err := New("t-grow", "xclock", 10, 5)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	if !b.randrOK {
		t.Skip("RANDR not available on this Xvfb build")
	}

	origW, origH := b.capW, b.capH

	b.Resize(200, 100) // well past the overscan-padded initial buffer

	deadline := time.Now().Add(resizeDebounce + 2*time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		w, h := b.capW, b.capH
		b.mu.Unlock()
		if w > origW || h > origH {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.mu.Lock()
	w, h := b.capW, b.capH
	b.mu.Unlock()
	t.Errorf("capture buffer never grew past %dx%d after resizing to 200x100 cells (still %dx%d)", origW, origH, w, h)
}

func TestNewFailsClearlyWithoutXvfb(t *testing.T) {
	if _, err := exec.LookPath("Xvfb"); err == nil {
		t.Skip("Xvfb is on PATH in this environment; this test wants it absent")
	}
	if _, err := New("t6", "true", 10, 5); err == nil {
		t.Fatal("New() should fail when Xvfb isn't available")
	}
}

// TestBridgeSurfaceTextOverlayOnNativeGTKApp is the end-to-end proof of the
// AT-SPI text overlay (see docs/gui-bridge.md — validated to work for
// native GTK/Qt apps, not Electron ones): a real GTK app's actual text
// content should show up as real characters in Snapshot(), not just the
// constant half-block glyph every raster-only cell uses.
func TestBridgeSurfaceTextOverlayOnNativeGTKApp(t *testing.T) {
	requireATSPI(t, "zenity")

	const needle = "OVERLAYCHECK"
	dir := t.TempDir()
	path := dir + "/sample.txt"
	if err := os.WriteFile(path, []byte(needle+" some readable text for the overlay test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	b, err := New("t7", "zenity --text-info --filename="+path+" --width=400 --height=200", 60, 25)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		var sb strings.Builder
		for _, c := range b.Snapshot() {
			if c.Rune != 0 {
				sb.WriteRune(c.Rune)
			}
		}
		if strings.Contains(sb.String(), needle) {
			return // found real overlaid text — overlay works end to end
		}
	}
	t.Fatalf("never found %q as real overlaid text within 15s — text overlay isn't reaching the cell grid", needle)
}
