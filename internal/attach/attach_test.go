package attach

import (
	"bufio"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/proto"
	"github.com/ttypedesk/ttypedesk/internal/server"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

func TestRemoteKeyEvent(t *testing.T) {
	cases := []struct {
		name    string
		key     tcell.Key
		ch      rune
		mod     tcell.ModMask
		wantOK  bool
		wantKey string
		wantB   []byte
		wantR   rune
	}{
		{"enter", tcell.KeyEnter, 0, tcell.ModNone, true, "Enter", nil, 0},
		{"tab", tcell.KeyTab, 0, tcell.ModNone, true, "Tab", nil, 0},
		{"backspace", tcell.KeyBackspace, 0, tcell.ModNone, true, "Backspace", nil, 0},
		{"backspace2", tcell.KeyBackspace2, 0, tcell.ModNone, true, "Backspace", nil, 0},
		{"escape", tcell.KeyEscape, 0, tcell.ModNone, true, "Escape", nil, 0},
		{"up", tcell.KeyUp, 0, tcell.ModNone, true, "Up", nil, 0},
		{"down", tcell.KeyDown, 0, tcell.ModNone, true, "Down", nil, 0},
		{"left", tcell.KeyLeft, 0, tcell.ModNone, true, "Left", nil, 0},
		{"right", tcell.KeyRight, 0, tcell.ModNone, true, "Right", nil, 0},
		{"home", tcell.KeyHome, 0, tcell.ModNone, true, "Home", nil, 0},
		{"end", tcell.KeyEnd, 0, tcell.ModNone, true, "End", nil, 0},
		{"pgup", tcell.KeyPgUp, 0, tcell.ModNone, true, "PgUp", nil, 0},
		{"pgdn", tcell.KeyPgDn, 0, tcell.ModNone, true, "PgDn", nil, 0},
		{"delete", tcell.KeyDelete, 0, tcell.ModNone, true, "Delete", nil, 0},
		{"insert", tcell.KeyInsert, 0, tcell.ModNone, true, "Insert", nil, 0},
		{"ctrl-c", tcell.KeyCtrlC, 0, tcell.ModNone, true, "", []byte{0x03}, 0},
		{"ctrl-d", tcell.KeyCtrlD, 0, tcell.ModNone, true, "", []byte{0x04}, 0},
		{"ctrl-z", tcell.KeyCtrlZ, 0, tcell.ModNone, true, "", []byte{0x1a}, 0},
		{"ctrl-l", tcell.KeyCtrlL, 0, tcell.ModNone, true, "", []byte{0x0c}, 0},
		{"rune", tcell.KeyRune, 'q', tcell.ModNone, true, "", nil, 'q'},
		{"unmapped-F1", tcell.KeyF1, 0, tcell.ModNone, false, "", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := tcell.NewEventKey(c.key, c.ch, c.mod)
			ev, ok := remoteKeyEvent(e)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if ev.Key != c.wantKey {
				t.Errorf("Key = %q, want %q", ev.Key, c.wantKey)
			}
			if ev.Rune != c.wantR {
				t.Errorf("Rune = %q, want %q", ev.Rune, c.wantR)
			}
			if string(ev.Bytes) != string(c.wantB) {
				t.Errorf("Bytes = %v, want %v", ev.Bytes, c.wantB)
			}
		})
	}
}

func TestRemoteKeyEventModifiers(t *testing.T) {
	e := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModCtrl|tcell.ModAlt|tcell.ModShift)
	ev, ok := remoteKeyEvent(e)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !ev.Ctrl || !ev.Alt || !ev.Shift {
		t.Errorf("modifiers = %+v, want all set", ev)
	}
}

// newDispatchServer builds a server with two non-overlapping windows for
// exercising dispatchRemoteMouse's hit-testing/focus logic in isolation.
func newDispatchServer(t *testing.T) (s *server.Server, a, b *server.Window) {
	t.Helper()
	s = server.New(config.Default())
	s.SetHostSize(120, 40)
	winA, err := s.CreateApp("clock", "Clock")
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	winB, err := s.CreateApp("notes", "Notes")
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	s.SetGeometry(winA.ID, 2, 2, 20, 10)
	s.SetGeometry(winB.ID, 30, 2, 20, 10)
	s.Focus(winA.ID) // deterministic starting focus, independent of creation order
	return s, winA, winB
}

func TestDispatchRemoteMousePressFocusesAndArmsDrag(t *testing.T) {
	s, _, b := newDispatchServer(t)
	var downID string

	// A point strictly inside b's content area (b.X=30,Y=2,W=20,H=10).
	dispatchRemoteMouse(s, proto.MouseEvent{X: 32, Y: 4, Action: "press"}, &downID)

	if s.Focused() != b.ID {
		t.Fatalf("Focused() = %q, want %q", s.Focused(), b.ID)
	}
	if downID != b.ID {
		t.Fatalf("downID = %q, want %q", downID, b.ID)
	}
}

func TestDispatchRemoteMouseBorderClickFocusesButNoDrag(t *testing.T) {
	s, _, b := newDispatchServer(t)
	var downID string

	// Exactly on b's left border: inside the outer hit box but not the
	// strict content-interior box, so chrome stays local-only.
	dispatchRemoteMouse(s, proto.MouseEvent{X: b.X, Y: b.Y + 2, Action: "press"}, &downID)

	if s.Focused() != b.ID {
		t.Fatalf("Focused() = %q, want %q (border click should still focus)", s.Focused(), b.ID)
	}
	if downID != "" {
		t.Fatalf("downID = %q, want empty (border click should not arm content drag)", downID)
	}
}

func TestDispatchRemoteMouseOutsideAnyWindowIsNoop(t *testing.T) {
	s, a, _ := newDispatchServer(t)
	var downID string

	dispatchRemoteMouse(s, proto.MouseEvent{X: 200, Y: 200, Action: "press"}, &downID)

	if s.Focused() != a.ID {
		t.Fatalf("Focused() = %q, want unchanged %q", s.Focused(), a.ID)
	}
	if downID != "" {
		t.Fatalf("downID = %q, want empty", downID)
	}
}

func TestDispatchRemoteMouseReleaseClearsDownID(t *testing.T) {
	s, _, b := newDispatchServer(t)
	downID := b.ID

	dispatchRemoteMouse(s, proto.MouseEvent{X: 32, Y: 4, Action: "release"}, &downID)

	if downID != "" {
		t.Fatalf("downID after release = %q, want empty", downID)
	}
}

func TestDispatchRemoteMouseReleaseWithNoDownIDIsNoop(t *testing.T) {
	s, a, _ := newDispatchServer(t)
	downID := ""

	dispatchRemoteMouse(s, proto.MouseEvent{X: 32, Y: 4, Action: "release"}, &downID)

	if downID != "" {
		t.Fatalf("downID = %q, want empty", downID)
	}
	if s.Focused() != a.ID {
		t.Fatalf("Focused() = %q, want unchanged %q", s.Focused(), a.ID)
	}
}

func TestDispatchRemoteMouseSkipsMinimizedWindow(t *testing.T) {
	s := server.New(config.Default())
	s.SetHostSize(120, 40)
	bottom, _ := s.CreateApp("clock", "Clock")
	top, _ := s.CreateApp("notes", "Notes")
	// Overlap them completely; top was created last so it's topmost.
	s.SetGeometry(bottom.ID, 5, 5, 20, 10)
	s.SetGeometry(top.ID, 5, 5, 20, 10)
	s.ToggleMinimize(top.ID)
	var downID string

	dispatchRemoteMouse(s, proto.MouseEvent{X: 10, Y: 8, Action: "press"}, &downID)

	if s.Focused() != bottom.ID {
		t.Fatalf("Focused() = %q, want %q (minimized top window should be skipped)", s.Focused(), bottom.ID)
	}
}

func TestDispatchRemoteMouseWheelDoesNotAffectFocusOrDrag(t *testing.T) {
	s, a, _ := newDispatchServer(t)
	downID := ""

	dispatchRemoteMouse(s, proto.MouseEvent{X: 10, Y: 6, Action: "wheel", Button: 3}, &downID)

	if s.Focused() != a.ID {
		t.Fatalf("Focused() = %q, want unchanged %q", s.Focused(), a.ID)
	}
	if downID != "" {
		t.Fatalf("downID = %q, want empty", downID)
	}
}

func TestDispatchRemoteMouseDragKeepsExistingDownID(t *testing.T) {
	s, _, b := newDispatchServer(t)
	downID := b.ID

	dispatchRemoteMouse(s, proto.MouseEvent{X: 33, Y: 5, Action: "drag"}, &downID)

	if downID != b.ID {
		t.Fatalf("downID after drag = %q, want unchanged %q", downID, b.ID)
	}
}

func TestPaintSnapshot(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)

	snap := proto.Snapshot{
		Cols: 80, Rows: 24,
		Windows: []proto.SnapshotWindow{
			{
				ID: "w1", Title: "Hi", X: 2, Y: 3, W: 5, H: 4, Cols: 3, Rows: 2,
				Cells: []cell.Cell{
					{Rune: 'A', FG: cell.RGB(1, 2, 3), BG: cell.RGB(4, 5, 6)},
					{Rune: 'B'}, {Rune: 'C'},
					{Rune: 'D'}, {Rune: 'E'}, {Rune: 'F'},
				},
			},
		},
	}
	paintSnapshot(screen, snap)

	// Content cell (0,0) of the window lands at (X+1, Y+1).
	r, _, _, _ := screen.GetContent(3, 4)
	if r != 'A' {
		t.Errorf("content (0,0) rune = %q, want 'A'", r)
	}
	r, _, _, _ = screen.GetContent(5, 4)
	if r != 'C' {
		t.Errorf("content (2,0) rune = %q, want 'C'", r)
	}
	r, _, _, _ = screen.GetContent(3, 5)
	if r != 'D' {
		t.Errorf("content (0,1) rune = %q, want 'D'", r)
	}

	// Title bar is drawn at row Y starting at X+1.
	r, _, _, _ = screen.GetContent(3, 3)
	if r != 'H' {
		t.Errorf("title[0] rune = %q, want 'H'", r)
	}
	r, _, _, _ = screen.GetContent(4, 3)
	if r != 'i' {
		t.Errorf("title[1] rune = %q, want 'i'", r)
	}

	// Status line is drawn across row 0.
	r, _, _, _ = screen.GetContent(1, 0)
	if r != 'T' {
		t.Errorf("status line rune = %q, want 'T'", r)
	}
}

func dialWithRetry(t *testing.T, path string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("could not dial %s: %v", path, lastErr)
	return nil
}

func TestServeEndToEnd(t *testing.T) {
	s := server.New(config.Default())
	s.SetHostSize(120, 40)
	winA, _ := s.CreateApp("clock", "Clock")
	winB, _ := s.CreateApp("notes", "Notes")
	s.SetGeometry(winA.ID, 2, 2, 20, 10)
	s.SetGeometry(winB.ID, 30, 2, 20, 10)
	s.Focus(winA.ID)

	sockPath := filepath.Join(t.TempDir(), "attach.sock")
	go func() { _ = Serve(s, sockPath) }()

	conn := dialWithRetry(t, sockPath)
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	if !scanner.Scan() {
		t.Fatalf("no hello line: %v", scanner.Err())
	}
	hello, err := proto.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode hello: %v", err)
	}
	if hello.Type != proto.TypeAttach {
		t.Fatalf("hello.Type = %q, want %q", hello.Type, proto.TypeAttach)
	}

	if !scanner.Scan() {
		t.Fatalf("no snapshot line: %v", scanner.Err())
	}
	snapEnv, err := proto.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapEnv.Type != proto.TypeSnapshot {
		t.Fatalf("snapEnv.Type = %q, want %q", snapEnv.Type, proto.TypeSnapshot)
	}
	snap, err := proto.DecodePayload[proto.Snapshot](snapEnv)
	if err != nil {
		t.Fatalf("decode snapshot payload: %v", err)
	}
	if len(snap.Windows) != 2 {
		t.Fatalf("snapshot has %d windows, want 2", len(snap.Windows))
	}

	// Send a mouse press over window B's content and confirm the host's
	// focus follows the remote client's input round trip.
	sendEnvelope(conn, proto.TypeMouse, proto.MouseEvent{X: 32, Y: 4, Action: "press", Button: 1})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.Focused() != winB.ID {
		time.Sleep(10 * time.Millisecond)
	}
	if s.Focused() != winB.ID {
		t.Fatalf("Focused() = %q, want %q after remote press", s.Focused(), winB.ID)
	}
}
