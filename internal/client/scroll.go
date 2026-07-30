package client

import (
	"github.com/RetroCodeRamen/ttypedesk/internal/server"
	"github.com/RetroCodeRamen/ttypedesk/internal/surface"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
)

func scrollBarGeom(win *server.Window) (x, y, h int) {
	return win.X + win.W - 1, win.Y + 1, win.H - 2
}

func scrollUIState(sp surface.ScrollbackProvider) uiapp.ScrollState {
	off, content, viewport := sp.ScrollUIState()
	return uiapp.ScrollState{Offset: off, Content: content, Viewport: viewport}
}

func (c *Client) drawScrollbackBar(win *server.Window) {
	sp, ok := win.Surface.(surface.ScrollbackProvider)
	if !ok || !sp.HasScrollback() {
		return
	}
	x, y, h := scrollBarGeom(win)
	if h < 1 {
		return
	}
	state := scrollUIState(sp)
	style := uiapp.DefaultScrollbarStyle()
	trackY, trackH := y, h
	if h >= 3 {
		c.set(x, y, '▲', style.ArrowFG, style.ArrowBG, 0)
		c.set(x, y+h-1, '▼', style.ArrowFG, style.ArrowBG, 0)
		trackY = y + 1
		trackH = h - 2
	}
	for row := 0; row < trackH; row++ {
		c.set(x, trackY+row, '░', style.TrackFG, style.TrackBG, 0)
	}
	ty, th := state.ThumbGeom(trackH)
	for row := 0; row < th; row++ {
		c.set(x, trackY+ty+row, '█', style.ThumbFG, style.ThumbBG, 0)
	}
}

func (c *Client) handleScrollbarPress(w *server.Window, mx, my int) bool {
	sp, ok := w.Surface.(surface.ScrollbackProvider)
	if !ok || !sp.HasScrollback() {
		return false
	}
	x, y, h := scrollBarGeom(w)
	state := scrollUIState(sp)
	hit := uiapp.HitScrollbar(mx, my, x, y, h, state, h >= 3)
	if hit == uiapp.ScrollHitNone {
		return false
	}
	if hit == uiapp.ScrollHitThumb {
		trackY, trackH := y, h
		if h >= 3 {
			trackY = y + 1
			trackH = h - 2
		}
		ty, _ := state.ThumbGeom(trackH)
		c.dragID = w.ID
		c.dragMode = "scroll"
		c.scrollGrab = my - trackY - ty
		return true
	}
	state.ApplyScrollHit(hit)
	sp.SetScrollUIOffset(state.Offset)
	c.layoutDirty = true
	return true
}

func (c *Client) dragScrollThumb(id string, my int) {
	w := c.srv.Get(id)
	if w == nil {
		return
	}
	sp, ok := w.Surface.(surface.ScrollbackProvider)
	if !ok {
		return
	}
	_, y, h := scrollBarGeom(w)
	state := scrollUIState(sp)
	trackY, trackH := y, h
	if h >= 3 {
		trackY = y + 1
		trackH = h - 2
	}
	_, th := state.ThumbGeom(trackH)
	travel := trackH - th
	if travel < 1 {
		return
	}
	desired := my - trackY - c.scrollGrab
	if desired < 0 {
		desired = 0
	}
	if desired > travel {
		desired = travel
	}
	maxOff := state.MaxOffset()
	off := 0
	if maxOff > 0 {
		off = (desired * maxOff) / travel
	}
	sp.SetScrollUIOffset(off)
	c.layoutDirty = true
}
