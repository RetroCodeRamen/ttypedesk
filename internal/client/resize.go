package client

import "github.com/RetroCodeRamen/ttypedesk/internal/server"

// resize edge bitflags
const (
	edgeN = 1 << iota
	edgeS
	edgeE
	edgeW
)

// hitResizeEdges returns which border grips contain (x,y).
// Title-bar buttons and mid-title move zone are excluded by the caller;
// on the top row only corners (with left/right) count as resize.
func hitResizeEdges(w *server.Window, x, y int) int {
	if w == nil || w.Maximized {
		return 0
	}
	left := x == w.X
	right := x == w.X+w.W-1
	top := y == w.Y
	bot := y == w.Y+w.H-1
	if !left && !right && !top && !bot {
		return 0
	}
	var e int
	if top {
		e |= edgeN
	}
	if bot {
		e |= edgeS
	}
	if left {
		e |= edgeW
	}
	if right {
		e |= edgeE
	}
	// Mid title bar is for dragging the window, not N-edge resize.
	if top && !left && !right {
		return 0
	}
	return e
}

// geomFromEdgeDrag computes new window geometry from a drag.
// sx,sy = mouse at drag start; mx,my = current mouse; ox,oy,ow,oh = window at start.
func geomFromEdgeDrag(edges, ox, oy, ow, oh, sx, sy, mx, my int) (nx, ny, nw, nh int) {
	nx, ny, nw, nh = ox, oy, ow, oh
	dx, dy := mx-sx, my-sy
	if edges&edgeE != 0 {
		nw = ow + dx
	}
	if edges&edgeS != 0 {
		nh = oh + dy
	}
	if edges&edgeW != 0 {
		nx = ox + dx
		nw = ow - dx
	}
	if edges&edgeN != 0 {
		ny = oy + dy
		nh = oh - dy
	}
	if nw < 12 {
		if edges&edgeW != 0 {
			nx = ox + ow - 12
		}
		nw = 12
	}
	if nh < 5 {
		if edges&edgeN != 0 {
			ny = oy + oh - 5
		}
		nh = 5
	}
	return nx, ny, nw, nh
}
