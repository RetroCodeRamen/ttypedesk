package client

import (
	"testing"

	"github.com/RetroCodeRamen/ttypedesk/internal/server"
)

func TestHitResizeEdgesNilAndMaximized(t *testing.T) {
	if e := hitResizeEdges(nil, 5, 5); e != 0 {
		t.Errorf("hitResizeEdges(nil, ...) = %d, want 0", e)
	}
	w := &server.Window{X: 0, Y: 0, W: 20, H: 10, Maximized: true}
	if e := hitResizeEdges(w, 0, 0); e != 0 {
		t.Errorf("hitResizeEdges(maximized, ...) = %d, want 0", e)
	}
}

func TestHitResizeEdgesCorners(t *testing.T) {
	// Window spans X:[0,20) Y:[0,10) -> right edge col 19, bottom row 9.
	w := &server.Window{X: 0, Y: 0, W: 20, H: 10}
	cases := []struct {
		name string
		x, y int
		want int
	}{
		{"top-left corner", 0, 0, edgeN | edgeW},
		{"top-right corner", 19, 0, edgeN | edgeE},
		{"bottom-left corner", 0, 9, edgeS | edgeW},
		{"bottom-right corner", 19, 9, edgeS | edgeE},
		{"left edge mid", 0, 5, edgeW},
		{"right edge mid", 19, 5, edgeE},
		{"bottom edge mid", 10, 9, edgeS},
		{"mid title bar (not a resize grip)", 10, 0, 0},
		{"interior (no edge)", 10, 5, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hitResizeEdges(w, c.x, c.y); got != c.want {
				t.Errorf("hitResizeEdges(%d,%d) = %d, want %d", c.x, c.y, got, c.want)
			}
		})
	}
}

func TestGeomFromEdgeDragEachEdge(t *testing.T) {
	// Window starts at (10,10) size 30x20; drag start at (10,10), moved to (15,17): dx=5, dy=7.
	const ox, oy, ow, oh = 10, 10, 30, 20
	const sx, sy, mx, my = 10, 10, 15, 17

	cases := []struct {
		name                       string
		edges                      int
		wantX, wantY, wantW, wantH int
	}{
		{"east grows width", edgeE, ox, oy, ow + 5, oh},
		{"south grows height", edgeS, ox, oy, ow, oh + 7},
		{"west moves+shrinks width", edgeW, ox + 5, oy, ow - 5, oh},
		{"north moves+shrinks height", edgeN, ox, oy + 7, ow, oh - 7},
		{"southeast corner", edgeS | edgeE, ox, oy, ow + 5, oh + 7},
		{"northwest corner", edgeN | edgeW, ox + 5, oy + 7, ow - 5, oh - 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nx, ny, nw, nh := geomFromEdgeDrag(c.edges, ox, oy, ow, oh, sx, sy, mx, my)
			if nx != c.wantX || ny != c.wantY || nw != c.wantW || nh != c.wantH {
				t.Errorf("geomFromEdgeDrag = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					nx, ny, nw, nh, c.wantX, c.wantY, c.wantW, c.wantH)
			}
		})
	}
}

func TestGeomFromEdgeDragClampsMinSize(t *testing.T) {
	const ox, oy, ow, oh = 10, 10, 30, 20
	// Drag the east edge far to the left, well past the 12-col minimum.
	nx, ny, nw, nh := geomFromEdgeDrag(edgeE, ox, oy, ow, oh, 40, 30, 0, 30)
	if nw != 12 {
		t.Errorf("width = %d, want clamped to 12", nw)
	}
	if nx != ox || ny != oy {
		t.Errorf("east-edge drag should not move origin: got (%d,%d), want (%d,%d)", nx, ny, ox, oy)
	}
	if nh != oh {
		t.Errorf("height should be untouched by an east-only drag, got %d", nh)
	}

	// West edge dragged past the window's right side: width clamps to 12,
	// and x is pulled back in so the window doesn't invert.
	nx2, _, nw2, _ := geomFromEdgeDrag(edgeW, ox, oy, ow, oh, 10, 10, 100, 10)
	if nw2 != 12 {
		t.Errorf("width = %d, want clamped to 12", nw2)
	}
	if nx2 != ox+ow-12 {
		t.Errorf("x = %d, want %d (right edge held in place)", nx2, ox+ow-12)
	}

	// South edge dragged above the window's top: height clamps to 5.
	_, _, _, nh3 := geomFromEdgeDrag(edgeS, ox, oy, ow, oh, 10, 10, 10, -100)
	if nh3 != 5 {
		t.Errorf("height = %d, want clamped to 5", nh3)
	}

	// North edge dragged below the window's bottom: height clamps to 5,
	// y pulled back so the window doesn't invert.
	_, ny4, _, nh4 := geomFromEdgeDrag(edgeN, ox, oy, ow, oh, 10, 10, 10, 100)
	if nh4 != 5 {
		t.Errorf("height = %d, want clamped to 5", nh4)
	}
	if ny4 != oy+oh-5 {
		t.Errorf("y = %d, want %d (bottom edge held in place)", ny4, oy+oh-5)
	}
}

func TestGeomFromEdgeDragNoEdgesIsNoop(t *testing.T) {
	nx, ny, nw, nh := geomFromEdgeDrag(0, 10, 10, 30, 20, 0, 0, 50, 50)
	if nx != 10 || ny != 10 || nw != 30 || nh != 20 {
		t.Errorf("geomFromEdgeDrag with no edges = (%d,%d,%d,%d), want unchanged (10,10,30,20)", nx, ny, nw, nh)
	}
}
