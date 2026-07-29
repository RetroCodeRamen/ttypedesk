package server

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/session"
	"github.com/ttypedesk/ttypedesk/internal/surface"
)

// newTestServer returns a Server sized like a typical host terminal, using
// the default (top-docked) taskbar.
func newTestServer() *Server {
	s := New(config.Default())
	s.SetHostSize(120, 40)
	return s
}

func TestCreateAppUnknown(t *testing.T) {
	s := newTestServer()
	if _, err := s.CreateApp("does-not-exist", "Nope"); err == nil {
		t.Fatal("expected error for unknown app name")
	}
	if len(s.Windows()) != 0 {
		t.Fatalf("unknown app should not leave a window behind, got %d", len(s.Windows()))
	}
}

func TestCreateAppAndFocus(t *testing.T) {
	s := newTestServer()
	win, err := s.CreateApp("clock", "Clock")
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if win.Title != "Clock" {
		t.Fatalf("Title = %q, want Clock", win.Title)
	}
	if s.Focused() != win.ID {
		t.Fatalf("Focused() = %q, want new window %q", s.Focused(), win.ID)
	}
	if got := s.Get(win.ID); got != win {
		t.Fatalf("Get(%q) = %v, want %v", win.ID, got, win)
	}
	if len(s.Windows()) != 1 {
		t.Fatalf("Windows() len = %d, want 1", len(s.Windows()))
	}
}

func TestFocusReordersZAndOrder(t *testing.T) {
	s := newTestServer()
	a, _ := s.CreateApp("clock", "Clock")
	b, _ := s.CreateApp("notes", "Notes")
	// b was created last, so it's focused and on top.
	if s.Focused() != b.ID {
		t.Fatalf("Focused() = %q, want %q", s.Focused(), b.ID)
	}
	s.Focus(a.ID)
	if s.Focused() != a.ID {
		t.Fatalf("Focused() after Focus(a) = %q, want %q", s.Focused(), a.ID)
	}
	order := s.Windows()
	if len(order) != 2 || order[len(order)-1].ID != a.ID {
		t.Fatalf("Windows() order = %v, want a.ID last (top)", ids(order))
	}
	if a.Z <= b.Z {
		t.Fatalf("a.Z = %d should be > b.Z = %d after focusing a", a.Z, b.Z)
	}
}

func TestFocusOrCreateAppReusesWindow(t *testing.T) {
	s := newTestServer()
	first, err := s.CreateApp("clock", "Clock")
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	other, _ := s.CreateApp("notes", "Notes")
	s.Focus(other.ID) // steal focus so FocusOrCreateApp has to switch it back

	if err := s.FocusOrCreateApp("clock", "Clock"); err != nil {
		t.Fatalf("FocusOrCreateApp: %v", err)
	}
	if len(s.Windows()) != 2 {
		t.Fatalf("FocusOrCreateApp should reuse the window, got %d windows", len(s.Windows()))
	}
	if s.Focused() != first.ID {
		t.Fatalf("Focused() = %q, want existing clock window %q", s.Focused(), first.ID)
	}
}

func TestCloseWindowRefocusesTopmost(t *testing.T) {
	s := newTestServer()
	a, _ := s.CreateApp("clock", "Clock")
	b, _ := s.CreateApp("notes", "Notes")

	s.CloseWindow(b.ID)
	if s.Get(b.ID) != nil {
		t.Fatal("closed window still reachable via Get")
	}
	if s.Focused() != a.ID {
		t.Fatalf("Focused() after closing top window = %q, want %q", s.Focused(), a.ID)
	}
	if len(s.Windows()) != 1 {
		t.Fatalf("Windows() len = %d, want 1", len(s.Windows()))
	}

	s.CloseWindow(a.ID)
	if s.Focused() != "" {
		t.Fatalf("Focused() after closing last window = %q, want empty", s.Focused())
	}

	// Closing an id that was never a window is a silent no-op.
	s.CloseWindow("bogus")
}

func TestMoveClampsToDesktopInset(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")

	s.Move(win.ID, -5, -5)
	// Default dock is "top": inset is (top=1, left=0), so x clamps to 0, y to 1.
	if win.X != 0 || win.Y != 1 {
		t.Fatalf("Move clamped to (%d,%d), want (0,1)", win.X, win.Y)
	}

	s.Move(win.ID, 10, 10)
	if win.X != 10 || win.Y != 10 {
		t.Fatalf("Move in-bounds = (%d,%d), want (10,10)", win.X, win.Y)
	}
}

func TestMoveIgnoredWhenMaximized(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")
	s.ToggleMaximize(win.ID)
	x, y := win.X, win.Y
	s.Move(win.ID, 50, 50)
	if win.X != x || win.Y != y {
		t.Fatalf("Move mutated a maximized window: (%d,%d) -> (%d,%d)", x, y, win.X, win.Y)
	}
}

func TestResizeWindowEnforcesMinimum(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")

	s.ResizeWindow(win.ID, 1, 1)
	if win.W != 12 || win.H != 5 {
		t.Fatalf("ResizeWindow floor = %dx%d, want 12x5", win.W, win.H)
	}
}

func TestSetGeometryClampsToHostBounds(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")

	// Host is 120x40; ask for a window that overshoots the right/bottom edge.
	s.SetGeometry(win.ID, 100, 35, 40, 20)
	if win.X+win.W > 120 {
		t.Fatalf("window right edge %d exceeds host width 120", win.X+win.W)
	}
	if win.Y+win.H > 40 {
		t.Fatalf("window bottom edge %d exceeds host height 40", win.Y+win.H)
	}
}

func TestToggleMaximizeRoundTrips(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")
	origX, origY, origW, origH := win.X, win.Y, win.W, win.H

	s.ToggleMaximize(win.ID)
	if !win.Maximized {
		t.Fatal("ToggleMaximize did not set Maximized")
	}
	top, bottom, left, right := s.desktopInset()
	if win.X != left || win.Y != top || win.W != 120-left-right || win.H != 40-top-bottom {
		t.Fatalf("maximized geometry = (%d,%d %dx%d), want full desktop", win.X, win.Y, win.W, win.H)
	}

	s.ToggleMaximize(win.ID)
	if win.Maximized {
		t.Fatal("ToggleMaximize did not clear Maximized")
	}
	if win.X != origX || win.Y != origY || win.W != origW || win.H != origH {
		t.Fatalf("restored geometry = (%d,%d %dx%d), want (%d,%d %dx%d)",
			win.X, win.Y, win.W, win.H, origX, origY, origW, origH)
	}
}

func TestSnapTileAndRestore(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")
	origX, origY, origW, origH := win.X, win.Y, win.W, win.H

	s.Snap(win.ID, "left")
	x0, y0, x1, y1 := s.desktopBounds()
	dw, dh := x1-x0, y1-y0
	if win.X != x0 || win.Y != y0 || win.W != dw/2 || win.H != dh {
		t.Fatalf("left-snap geometry = (%d,%d %dx%d), want (%d,%d %dx%d)",
			win.X, win.Y, win.W, win.H, x0, y0, dw/2, dh)
	}

	// Snapping the same region again restores the pre-snap geometry.
	s.Snap(win.ID, "left")
	if win.X != origX || win.Y != origY || win.W != origW || win.H != origH {
		t.Fatalf("restored geometry = (%d,%d %dx%d), want (%d,%d %dx%d)",
			win.X, win.Y, win.W, win.H, origX, origY, origW, origH)
	}
}

func TestSnapUnknownRegionIsNoop(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")
	x, y, w, h := win.X, win.Y, win.W, win.H
	s.Snap(win.ID, "diagonal")
	if win.X != x || win.Y != y || win.W != w || win.H != h {
		t.Fatal("Snap with an unknown region mutated window geometry")
	}
}

func TestToggleMinimizeMovesFocusToNextVisible(t *testing.T) {
	s := newTestServer()
	a, _ := s.CreateApp("clock", "Clock")
	b, _ := s.CreateApp("notes", "Notes")

	s.ToggleMinimize(b.ID) // b was focused/topmost
	if !b.Minimized {
		t.Fatal("ToggleMinimize did not set Minimized")
	}
	if s.Focused() != a.ID {
		t.Fatalf("Focused() after minimizing top window = %q, want %q", s.Focused(), a.ID)
	}

	s.ToggleMinimize(b.ID)
	if b.Minimized {
		t.Fatal("ToggleMinimize did not clear Minimized")
	}
	if s.Focused() != b.ID {
		t.Fatalf("Focused() after un-minimizing = %q, want %q", s.Focused(), b.ID)
	}
}

func TestNudgeMovesAndResizes(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")
	x, y, w, h := win.X, win.Y, win.W, win.H

	s.Nudge(win.ID, 3, 4, 5, 6)
	if win.X != x+3 || win.Y != y+4 {
		t.Fatalf("Nudge position = (%d,%d), want (%d,%d)", win.X, win.Y, x+3, y+4)
	}
	if win.W != w+5 || win.H != h+6 {
		t.Fatalf("Nudge size = %dx%d, want %dx%d", win.W, win.H, w+5, h+6)
	}
}

func TestNudgeIgnoredWhenMaximizedOrMinimized(t *testing.T) {
	s := newTestServer()
	win, _ := s.CreateApp("clock", "Clock")
	s.ToggleMaximize(win.ID)
	x, y, w, h := win.X, win.Y, win.W, win.H
	s.Nudge(win.ID, 1, 1, 1, 1)
	if win.X != x || win.Y != y || win.W != w || win.H != h {
		t.Fatal("Nudge mutated a maximized window")
	}

	s.ToggleMaximize(win.ID) // back to normal
	s.ToggleMinimize(win.ID)
	x, y, w, h = win.X, win.Y, win.W, win.H
	s.Nudge(win.ID, 1, 1, 1, 1)
	if win.X != x || win.Y != y || win.W != w || win.H != h {
		t.Fatal("Nudge mutated a minimized window")
	}
}

func TestLaunchActionKnownAndUnknown(t *testing.T) {
	s := newTestServer()
	if err := s.LaunchAction("notes"); err != nil {
		t.Fatalf("LaunchAction(notes): %v", err)
	}
	if len(s.Windows()) != 1 {
		t.Fatalf("Windows() len = %d, want 1", len(s.Windows()))
	}
	if err := s.LaunchAction("totally-bogus-action"); err == nil {
		t.Fatal("expected error launching an unregistered action/app name")
	}
}

func TestCaptureAndRestoreSessionRoundTrips(t *testing.T) {
	s := newTestServer()
	a, _ := s.CreateApp("clock", "Clock")
	b, _ := s.CreateApp("notes", "Notes")
	s.SetGeometry(a.ID, 5, 5, 30, 10)
	s.ToggleMaximize(b.ID)
	s.Focus(a.ID)

	st := s.CaptureSession()
	if len(st.Windows) != 2 {
		t.Fatalf("CaptureSession() len = %d, want 2", len(st.Windows))
	}

	s2 := newTestServer()
	n := s2.RestoreSession(st)
	if n != 2 {
		t.Fatalf("RestoreSession() restored %d windows, want 2", n)
	}
	if len(s2.Windows()) != 2 {
		t.Fatalf("Windows() after restore len = %d, want 2", len(s2.Windows()))
	}
	var sawMaximized bool
	for _, w := range s2.Windows() {
		if w.Maximized {
			sawMaximized = true
		}
	}
	if !sawMaximized {
		t.Fatal("restored session lost the maximized window")
	}
}

func TestCaptureSessionSkipsTransientDialogs(t *testing.T) {
	s := newTestServer()
	s.CreateApp("appstore", "App Store")
	st := s.CaptureSession()
	if len(st.Windows) != 0 {
		t.Fatalf("CaptureSession() should skip transient dialogs, got %d entries", len(st.Windows))
	}
}

func TestRestoreSessionSkipsBlankActions(t *testing.T) {
	s := newTestServer()
	n := s.RestoreSession(session.State{Windows: []session.Entry{{Action: ""}}})
	if n != 0 {
		t.Fatalf("RestoreSession restored %d windows for a blank action, want 0", n)
	}
}

func TestCloseAllClearsWindows(t *testing.T) {
	s := newTestServer()
	s.CreateApp("clock", "Clock")
	s.CreateApp("notes", "Notes")
	s.CloseAll()
	if len(s.Windows()) != 0 {
		t.Fatalf("Windows() after CloseAll len = %d, want 0", len(s.Windows()))
	}
	if s.Focused() != "" {
		t.Fatalf("Focused() after CloseAll = %q, want empty", s.Focused())
	}
}

func TestSnapGeomRegions(t *testing.T) {
	cases := []struct {
		region                     string
		wantX, wantY, wantW, wantH int
	}{
		{"left", 0, 0, 50, 100},
		{"right", 50, 0, 50, 100},
		{"top", 0, 0, 100, 50},
		{"bottom", 0, 50, 100, 50},
		{"unknown", 0, 0, 0, 0},
	}
	for _, c := range cases {
		x, y, w, h := snapGeom(c.region, 0, 0, 100, 100)
		if x != c.wantX || y != c.wantY || w != c.wantW || h != c.wantH {
			t.Errorf("snapGeom(%q) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
				c.region, x, y, w, h, c.wantX, c.wantY, c.wantW, c.wantH)
		}
	}
}

func TestNear(t *testing.T) {
	cases := []struct {
		a, b, tol int
		want      bool
	}{
		{10, 10, 0, true},
		{10, 11, 1, true},
		{10, 12, 1, false},
		{10, 8, 2, true},
	}
	for _, c := range cases {
		if got := near(c.a, c.b, c.tol); got != c.want {
			t.Errorf("near(%d,%d,%d) = %v, want %v", c.a, c.b, c.tol, got, c.want)
		}
	}
}

func TestOpenFileManagerDefaultsToBuiltinFiles(t *testing.T) {
	s := newTestServer()
	win, err := s.openFileManager("")
	if err != nil {
		t.Fatalf("openFileManager: %v", err)
	}
	defer s.CloseWindow(win.ID)
	if win.Kind != "app" {
		t.Fatalf("Kind = %q, want %q (built-in Files)", win.Kind, "app")
	}
}

func TestOpenFileManagerFallsBackWhenRoleProgramMissing(t *testing.T) {
	s := newTestServer()
	cfg := s.Config()
	cfg.Roles.FileMgr = "prog:does-not-exist"
	s.SetConfig(cfg)

	win, err := s.openFileManager("")
	if err != nil {
		t.Fatalf("openFileManager: %v", err)
	}
	defer s.CloseWindow(win.ID)
	if win.Kind != "app" {
		t.Fatalf("Kind = %q, want %q (fallback to built-in Files)", win.Kind, "app")
	}
}

func TestOpenFileManagerUsesRoleProgram(t *testing.T) {
	s := newTestServer()
	cfg := s.Config()
	cfg.Programs = append(cfg.Programs, config.Program{ID: "appstore-superfile", Name: "SuperFile", Command: "true"})
	cfg.Roles.FileMgr = "prog:appstore-superfile"
	s.SetConfig(cfg)

	win, err := s.openFileManager(t.TempDir())
	if err != nil {
		t.Fatalf("openFileManager: %v", err)
	}
	defer s.CloseWindow(win.ID)
	if win.Kind != "pty" {
		t.Fatalf("Kind = %q, want %q (external program)", win.Kind, "pty")
	}
	if win.Launch != "prog:appstore-superfile" {
		t.Fatalf("Launch = %q, want %q", win.Launch, "prog:appstore-superfile")
	}
}

func TestLaunchActionFilesRoutesThroughFileManagerRole(t *testing.T) {
	s := newTestServer()
	cfg := s.Config()
	cfg.Programs = append(cfg.Programs, config.Program{ID: "appstore-superfile", Name: "SuperFile", Command: "true"})
	cfg.Roles.FileMgr = "prog:appstore-superfile"
	s.SetConfig(cfg)

	if err := s.LaunchAction("files"); err != nil {
		t.Fatalf("LaunchAction(files): %v", err)
	}
	wins := s.Windows()
	if len(wins) != 1 || wins[0].Launch != "prog:appstore-superfile" {
		t.Fatalf("Windows() = %v, want a single prog:appstore-superfile window", ids(wins))
	}
}

// TestLaunchActionBridgeCreatesWindow confirms the "bridge:<cmd>"
// LaunchAction prefix, the "bridge" createLocked case, and CreateBridge all
// actually wire together — internal/bridge's own tests cover BridgeSurface
// in isolation, this covers the server-level plumbing. Needs a real (if
// headless) X server, like internal/bridge's tests.
func TestLaunchActionBridgeCreatesWindow(t *testing.T) {
	if _, err := exec.LookPath("Xvfb"); err != nil {
		t.Skip("Xvfb not on PATH, skipping (apt install xvfb to run this locally)")
	}
	if _, err := exec.LookPath("xclock"); err != nil {
		t.Skip("xclock not on PATH, skipping (apt install x11-apps to run this locally)")
	}

	s := newTestServer()
	if err := s.LaunchAction("bridge:xclock"); err != nil {
		t.Fatalf("LaunchAction(bridge:xclock): %v", err)
	}
	wins := s.Windows()
	if len(wins) != 1 {
		t.Fatalf("Windows() len = %d, want 1", len(wins))
	}
	win := wins[0]
	defer s.CloseWindow(win.ID)
	if win.Kind != "bridge" {
		t.Fatalf("Kind = %q, want bridge", win.Kind)
	}
	if win.Launch != "bridge:xclock" {
		t.Fatalf("Launch = %q, want bridge:xclock", win.Launch)
	}
	if win.Title != "xclock" {
		t.Fatalf("Title = %q, want xclock", win.Title)
	}
}

func TestCreateAppAmpRendersWithoutCrashing(t *testing.T) {
	s := newTestServer()
	win, err := s.CreateApp("amp", "Amp")
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	defer s.CloseWindow(win.ID)

	// Exercise Draw (via ProduceDiff/Snapshot) and a few keys/mouse events
	// through the real AppSurface stack — AppSurface isolates a genuine
	// panic into a crashed-window state rather than failing this test
	// directly, so also assert the window didn't quietly crash.
	_ = win.Surface.ProduceDiff()
	cells := win.Surface.Snapshot()
	if len(cells) == 0 {
		t.Fatal("Snapshot() returned no cells")
	}
	for _, key := range []string{"Up", "Down", "Enter"} {
		if err := win.Surface.HandleInput(surface.InputEvent{Kind: "key", Key: key}); err != nil {
			t.Fatalf("HandleInput %s: %v", key, err)
		}
	}
	for _, r := range []rune{' ', 'n', 'p', 's'} {
		if err := win.Surface.HandleInput(surface.InputEvent{Kind: "key", Rune: r}); err != nil {
			t.Fatalf("HandleInput rune %q: %v", r, err)
		}
	}
	if title := win.Surface.Title(); strings.Contains(title, "crashed") {
		t.Fatalf("Title() = %q — Amp crashed during basic interaction", title)
	}
}

func TestCreateFilePickerOpensWindow(t *testing.T) {
	s := newTestServer()
	home := t.TempDir()
	win, err := s.CreateFilePicker(home, nil, func(string, bool) {})
	if err != nil {
		t.Fatalf("CreateFilePicker: %v", err)
	}
	defer s.CloseWindow(win.ID)
	if win.Kind != "app" {
		t.Fatalf("Kind = %q, want app", win.Kind)
	}
	if win.Title != "Open File" {
		t.Fatalf("Title = %q, want Open File", win.Title)
	}
}

func TestCreateFilePickerEscCancels(t *testing.T) {
	s := newTestServer()
	home := t.TempDir()
	var gotPath string
	var gotOK, called bool
	win, err := s.CreateFilePicker(home, nil, func(path string, ok bool) {
		called, gotPath, gotOK = true, path, ok
	})
	if err != nil {
		t.Fatalf("CreateFilePicker: %v", err)
	}
	defer s.CloseWindow(win.ID)

	if err := win.Surface.HandleInput(surface.InputEvent{Kind: "key", Key: "Escape"}); err != nil {
		t.Fatalf("HandleInput Escape: %v", err)
	}
	if !called {
		t.Fatal("onResult was never called after Escape")
	}
	if gotOK {
		t.Errorf("onResult ok = true, want false (cancelled)")
	}
	if gotPath != "" {
		t.Errorf("onResult path = %q, want empty", gotPath)
	}
}

func TestCreateFilePickerPicksFile(t *testing.T) {
	s := newTestServer()
	home := t.TempDir()
	if err := os.WriteFile(home+"/pick-me.txt", []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var gotPath string
	var gotOK bool
	win, err := s.CreateFilePicker(home, nil, func(path string, ok bool) {
		gotPath, gotOK = path, ok
	})
	if err != nil {
		t.Fatalf("CreateFilePicker: %v", err)
	}
	defer s.CloseWindow(win.ID)

	// The picker lists ".." first (a temp dir always has a distinct parent),
	// then sorted dirs, then sorted files — "pick-me.txt" is row 1 here.
	if err := win.Surface.HandleInput(surface.InputEvent{Kind: "key", Key: "Down"}); err != nil {
		t.Fatalf("HandleInput Down: %v", err)
	}
	if err := win.Surface.HandleInput(surface.InputEvent{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatalf("HandleInput Enter: %v", err)
	}
	if !gotOK {
		t.Fatal("onResult ok = false, want true (a file was picked)")
	}
	if gotPath != home+"/pick-me.txt" {
		t.Errorf("onResult path = %q, want %q", gotPath, home+"/pick-me.txt")
	}
}

func ids(ws []*Window) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.ID
	}
	return out
}
