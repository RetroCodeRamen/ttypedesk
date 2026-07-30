package attach

import (
	"bufio"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
	"github.com/RetroCodeRamen/ttypedesk/internal/proto"
	"github.com/RetroCodeRamen/ttypedesk/internal/server"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/gdamore/tcell/v2"
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

func TestPaintDiffFrame(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)

	frame := proto.DiffFrame{
		Cols: 80, Rows: 24,
		Windows: []proto.DiffWindow{
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
	cache := make(map[string][]cell.Cell)
	paintDiffFrame(screen, frame, cache)

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

func TestPaintDiffFrameReusesCacheWhenCellsOmitted(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	cache := make(map[string][]cell.Cell)

	first := proto.DiffFrame{Cols: 80, Rows: 24, Windows: []proto.DiffWindow{
		{ID: "w1", X: 0, Y: 1, W: 4, H: 3, Cols: 2, Rows: 1, Cells: []cell.Cell{{Rune: 'X'}, {Rune: 'Y'}}},
	}}
	paintDiffFrame(screen, first, cache)

	// Second frame omits Cells entirely (unchanged) — content must still
	// be there, painted from the cache.
	second := proto.DiffFrame{Cols: 80, Rows: 24, Windows: []proto.DiffWindow{
		{ID: "w1", X: 0, Y: 1, W: 4, H: 3, Cols: 2, Rows: 1, Cells: nil},
	}}
	paintDiffFrame(screen, second, cache)

	r, _, _, _ := screen.GetContent(1, 2)
	if r != 'X' {
		t.Errorf("cached content (0,0) rune = %q, want 'X'", r)
	}
	r, _, _, _ = screen.GetContent(2, 2)
	if r != 'Y' {
		t.Errorf("cached content (1,0) rune = %q, want 'Y'", r)
	}
}

func TestPaintDiffFramePrunesCacheForClosedWindows(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	cache := make(map[string][]cell.Cell)

	first := proto.DiffFrame{Cols: 80, Rows: 24, Windows: []proto.DiffWindow{
		{ID: "w1", Cols: 1, Rows: 1, Cells: []cell.Cell{{Rune: 'X'}}},
	}}
	paintDiffFrame(screen, first, cache)
	if _, ok := cache["w1"]; !ok {
		t.Fatal("cache does not contain w1 after its first frame")
	}

	// w1 is gone from the second frame (closed host-side) — its cache
	// entry should be pruned, not linger forever.
	second := proto.DiffFrame{Cols: 80, Rows: 24}
	paintDiffFrame(screen, second, cache)
	if _, ok := cache["w1"]; ok {
		t.Error("cache still contains w1 after it disappeared from a frame — should have been pruned")
	}
}

func TestCellsEqual(t *testing.T) {
	a := []cell.Cell{{Rune: 'A'}, {Rune: 'B'}}
	b := []cell.Cell{{Rune: 'A'}, {Rune: 'B'}}
	c := []cell.Cell{{Rune: 'A'}, {Rune: 'X'}}
	if !cellsEqual(a, b) {
		t.Error("cellsEqual(a, b) = false, want true for identical content")
	}
	if cellsEqual(a, c) {
		t.Error("cellsEqual(a, c) = true, want false for differing content")
	}
	if cellsEqual(a, nil) {
		t.Error("cellsEqual(a, nil) = true, want false for differing length")
	}
	if !cellsEqual(nil, nil) {
		t.Error("cellsEqual(nil, nil) = false, want true")
	}
}

func TestBuildDiffFrameOmitsUnchangedOnSecondCallAndPrunesClosed(t *testing.T) {
	cache := make(map[string][]cell.Cell)
	snap1 := proto.Snapshot{Cols: 80, Rows: 24, Windows: []proto.SnapshotWindow{
		{ID: "w1", Cols: 1, Rows: 1, Cells: []cell.Cell{{Rune: 'A'}}},
	}}
	f1 := buildDiffFrame(snap1, cache)
	if len(f1.Windows) != 1 || f1.Windows[0].Cells == nil {
		t.Fatalf("first buildDiffFrame call should include cells: %+v", f1.Windows)
	}

	// Same content again: cells should be omitted the second time.
	f2 := buildDiffFrame(snap1, cache)
	if f2.Windows[0].Cells != nil {
		t.Errorf("second buildDiffFrame call with unchanged content should omit Cells, got %v", f2.Windows[0].Cells)
	}

	// Window closes: cache entry should be pruned.
	snap2 := proto.Snapshot{Cols: 80, Rows: 24}
	_ = buildDiffFrame(snap2, cache)
	if _, ok := cache["w1"]; ok {
		t.Error("cache still contains w1 after it closed — should have been pruned")
	}

	// A window reopening with the same ID after being pruned must be
	// treated as new (cells included), not skipped as "unchanged".
	f3 := buildDiffFrame(snap1, cache)
	if f3.Windows[0].Cells == nil {
		t.Error("a reopened window (same ID, pruned from cache) should include cells again, not be treated as unchanged")
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

	r := bufio.NewReader(conn)

	typ, payload, err := proto.ReadFrame(r)
	if err != nil {
		t.Fatalf("read hello frame: %v", err)
	}
	if typ != proto.FrameJSON {
		t.Fatalf("hello frame type = %d, want FrameJSON", typ)
	}
	hello, err := proto.Decode(payload)
	if err != nil {
		t.Fatalf("decode hello: %v", err)
	}
	if hello.Type != proto.TypeAttach {
		t.Fatalf("hello.Type = %q, want %q", hello.Type, proto.TypeAttach)
	}

	typ, payload, err = proto.ReadFrame(r)
	if err != nil {
		t.Fatalf("read diff frame: %v", err)
	}
	if typ != proto.FrameDiff {
		t.Fatalf("frame type = %d, want FrameDiff", typ)
	}
	diff, err := proto.DecodeDiffFrame(payload)
	if err != nil {
		t.Fatalf("decode diff frame: %v", err)
	}
	if len(diff.Windows) != 2 {
		t.Fatalf("diff frame has %d windows, want 2", len(diff.Windows))
	}
	for _, w := range diff.Windows {
		if w.Cells == nil {
			t.Errorf("window %q on the first frame sent to a new connection should include cells", w.ID)
		}
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
