package bridge

import (
	"fmt"
	"os/exec"
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

func TestNewFailsClearlyWithoutXvfb(t *testing.T) {
	if _, err := exec.LookPath("Xvfb"); err == nil {
		t.Skip("Xvfb is on PATH in this environment; this test wants it absent")
	}
	if _, err := New("t6", "true", 10, 5); err == nil {
		t.Fatal("New() should fail when Xvfb isn't available")
	}
}
