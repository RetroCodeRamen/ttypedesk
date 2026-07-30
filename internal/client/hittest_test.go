package client

import (
	"testing"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
	"github.com/RetroCodeRamen/ttypedesk/internal/server"
	"github.com/gdamore/tcell/v2"
)

// newHitTestClient builds a Client backed by a tcell.SimulationScreen (no
// real terminal needed, same pattern as internal/attach/attach_test.go) and
// a real in-memory server.Server (server.New needs no I/O either — same
// pattern as internal/server/server_test.go), docked to the given side.
func newHitTestClient(t *testing.T, dock string, cols, rows int) (*Client, *server.Server) {
	t.Helper()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(sim.Fini)
	sim.SetSize(cols, rows)

	cfg := config.Default()
	cfg.Taskbar.Dock = dock
	srv := server.New(cfg)
	srv.SetHostSize(cols, rows)

	return &Client{screen: sim, srv: srv, cfg: cfg, subOpen: -1}, srv
}

func TestOnStartButtonAllDockSides(t *testing.T) {
	for _, dock := range []string{"top", "bottom", "left", "right"} {
		t.Run(dock, func(t *testing.T) {
			c, _ := newHitTestClient(t, dock, 100, 30)
			row := c.taskbarRow()
			col0 := c.taskbarCol0()
			switch dock {
			case "top", "bottom":
				if !c.onStartButton(0, row) {
					t.Errorf("(0,%d) should be the Start button on a %s dock", row, dock)
				}
				if c.onStartButton(9, row) {
					t.Errorf("(9,%d) should be past the Start button on a %s dock", row, dock)
				}
			case "left", "right":
				if !c.onStartButton(col0, 0) {
					t.Errorf("(%d,0) should be the Start button on a %s dock", col0, dock)
				}
				if c.onStartButton(col0, 1) {
					t.Errorf("(%d,1) should be past the Start button on a %s dock", col0, dock)
				}
			}
		})
	}
}

func TestDesktopRectExcludesTaskbarAllSides(t *testing.T) {
	cases := []struct {
		dock string
	}{{"top"}, {"bottom"}, {"left"}, {"right"}}
	for _, c := range cases {
		t.Run(c.dock, func(t *testing.T) {
			cl, _ := newHitTestClient(t, c.dock, 100, 30)
			x0, y0, x1, y1 := cl.desktopRect()
			if x1 <= x0 || y1 <= y0 {
				t.Fatalf("desktopRect() = (%d,%d,%d,%d), degenerate", x0, y0, x1, y1)
			}
			th := cl.taskbarThickness()
			switch c.dock {
			case "top":
				if y0 != th {
					t.Errorf("top dock: y0 = %d, want %d", y0, th)
				}
			case "bottom":
				if y1 != 30-th {
					t.Errorf("bottom dock: y1 = %d, want %d", y1, 30-th)
				}
			case "left":
				if x0 != th {
					t.Errorf("left dock: x0 = %d, want %d", x0, th)
				}
			case "right":
				if x1 != 100-th {
					t.Errorf("right dock: x1 = %d, want %d", x1, 100-th)
				}
			}
		})
	}
}

func TestHitBellAndHitClockBoundaries(t *testing.T) {
	for _, dock := range []string{"top", "bottom", "left", "right"} {
		t.Run(dock, func(t *testing.T) {
			c, _ := newHitTestClient(t, dock, 100, 30)
			tl := c.trayLayout()

			if !c.hitBell(tl.bellX, tl.bellY) {
				t.Errorf("hitBell(%d,%d) should be true at the bell's own origin", tl.bellX, tl.bellY)
			}
			if c.hitBell(tl.bellX+tl.bellW, tl.bellY) {
				t.Error("hitBell should be false just past the bell's right edge")
			}

			if !c.hitClock(tl.clockX, tl.clockY) {
				t.Errorf("hitClock(%d,%d) should be true at the clock's own origin", tl.clockX, tl.clockY)
			}
		})
	}
}

func TestHitTaskbarButtonMapsToWindowID(t *testing.T) {
	for _, dock := range []string{"top", "bottom", "left", "right"} {
		t.Run(dock, func(t *testing.T) {
			c, srv := newHitTestClient(t, dock, 100, 30)
			winA, err := srv.CreateApp("clock", "Clock")
			if err != nil {
				t.Fatalf("CreateApp: %v", err)
			}
			winB, err := srv.CreateApp("notes", "Notes")
			if err != nil {
				t.Fatalf("CreateApp: %v", err)
			}

			layout := c.taskbarButtonLayout()
			if len(layout) != 2 {
				t.Fatalf("taskbarButtonLayout() len = %d, want 2", len(layout))
			}

			var gotA, gotB bool
			for _, b := range layout {
				var x, y int
				if c.isVerticalDock() {
					x, y = c.taskbarCol0(), b.start
				} else {
					x, y = b.start, c.taskbarRow()
				}
				id := c.hitTaskbarButton(x, y)
				if id != b.id {
					t.Errorf("hitTaskbarButton(%d,%d) = %q, want %q", x, y, id, b.id)
				}
				switch id {
				case winA.ID:
					gotA = true
				case winB.ID:
					gotB = true
				}
			}
			if !gotA || !gotB {
				t.Errorf("expected to hit both window buttons, gotA=%v gotB=%v", gotA, gotB)
			}
		})
	}
}

func TestHitDesktopIconIndex(t *testing.T) {
	c, _ := newHitTestClient(t, "top", 100, 30)
	c.cfg.ShowDesktopIcons = true
	c.cfg.DesktopIcons = []config.DesktopIcon{
		{Label: "Alpha", Icon: "📁", X: 2, Y: 2},
		{Label: "Beta", Icon: "📁", X: 2, Y: 6},
	}

	if got := c.hitDesktopIconIndex(2, 2); got != 0 {
		t.Errorf("hitDesktopIconIndex(icon glyph of #0) = %d, want 0", got)
	}
	if got := c.hitDesktopIconIndex(2, 3); got != 0 {
		t.Errorf("hitDesktopIconIndex(label row of #0) = %d, want 0", got)
	}
	if got := c.hitDesktopIconIndex(2, 6); got != 1 {
		t.Errorf("hitDesktopIconIndex(icon glyph of #1) = %d, want 1", got)
	}
	if got := c.hitDesktopIconIndex(50, 50); got != -1 {
		t.Errorf("hitDesktopIconIndex(empty desktop area) = %d, want -1", got)
	}

	c.cfg.ShowDesktopIcons = false
	if got := c.hitDesktopIconIndex(2, 2); got != -1 {
		t.Errorf("hitDesktopIconIndex with icons hidden = %d, want -1", got)
	}
}
