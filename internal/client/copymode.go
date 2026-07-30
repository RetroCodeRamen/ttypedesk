package client

import (
	"github.com/RetroCodeRamen/ttypedesk/internal/surface"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uwidth"
	"github.com/gdamore/tcell/v2"
)

// openCopyMode enters tmux-like keyboard scrollback selection on the
// focused window. It reuses the existing mouse-selection state (selWin,
// selX0/Y0, selX1/Y1, hasSel) so rendering and copySelection() need no
// separate code path — the copy-mode cursor *is* selX1/selY1.
func (c *Client) openCopyMode() {
	if c.copyMode || c.findOpen || c.paletteOpen || c.menuOpen {
		return
	}
	id := c.srv.Focused()
	if id == "" {
		return
	}
	w := c.srv.Get(id)
	if w == nil {
		return
	}
	if _, ok := w.Surface.(surface.ScrollbackProvider); !ok {
		return
	}
	x, y := 0, 0
	if cp, ok := w.Surface.(surface.CursorProvider); ok {
		if cx, cy, vis := cp.Cursor(); vis {
			x, y = cx, cy
		}
	}
	c.copyMode = true
	c.copySelecting = false
	c.selWin = id
	c.selX0, c.selY0 = x, y
	c.selX1, c.selY1 = x, y
	c.hasSel = true
	c.layoutDirty = true
}

func (c *Client) closeCopyMode(clearSel bool) {
	c.copyMode = false
	c.copySelecting = false
	if clearSel {
		c.hasSel = false
	}
	c.layoutDirty = true
}

// handleCopyModeKey consumes all keys while copy-mode is active. It returns
// true unconditionally so callers should only invoke it when copyMode is on.
func (c *Client) handleCopyModeKey(e *tcell.EventKey) bool {
	w := c.srv.Get(c.selWin)
	if w == nil {
		c.closeCopyMode(true)
		return true
	}
	sp, ok := w.Surface.(surface.ScrollbackProvider)
	if !ok {
		c.closeCopyMode(true)
		return true
	}
	cache := c.caches[c.selWin]
	cc, cr := 1, 1
	if cache != nil && cache.cols > 0 && cache.rows > 0 {
		cc, cr = cache.cols, cache.rows
	}

	// move steps the cursor by (dx,dy), scrolling the viewport one line at a
	// time when the cursor would leave the top/bottom row.
	move := func(dx, dy int) {
		nx, ny := c.selX1+dx, c.selY1+dy
		if nx < 0 {
			nx = 0
		}
		if nx >= cc {
			nx = cc - 1
		}
		for ny < 0 {
			st := scrollUIState(sp)
			if st.Offset <= 0 {
				ny = 0
				break
			}
			sp.SetScrollUIOffset(st.Offset - 1)
			ny++
		}
		for ny >= cr {
			st := scrollUIState(sp)
			if st.Offset >= st.MaxOffset() {
				ny = cr - 1
				break
			}
			sp.SetScrollUIOffset(st.Offset + 1)
			ny--
		}
		c.selX1, c.selY1 = nx, ny
		if !c.copySelecting {
			c.selX0, c.selY0 = nx, ny
		}
		c.layoutDirty = true
	}
	jumpTop := func() {
		sp.SetScrollUIOffset(0)
		c.selX1, c.selY1 = 0, 0
		if !c.copySelecting {
			c.selX0, c.selY0 = c.selX1, c.selY1
		}
		c.layoutDirty = true
	}
	jumpBottom := func() {
		st := scrollUIState(sp)
		sp.SetScrollUIOffset(st.MaxOffset())
		c.selX1, c.selY1 = 0, cr-1
		if !c.copySelecting {
			c.selX0, c.selY0 = c.selX1, c.selY1
		}
		c.layoutDirty = true
	}
	toggleSelect := func() {
		c.copySelecting = !c.copySelecting
		if c.copySelecting {
			c.selX0, c.selY0 = c.selX1, c.selY1
		}
		c.layoutDirty = true
	}
	copyAndClose := func() {
		c.copySelection()
		c.closeCopyMode(true)
	}

	switch e.Key() {
	case tcell.KeyEscape:
		c.closeCopyMode(true)
		return true
	case tcell.KeyUp:
		move(0, -1)
		return true
	case tcell.KeyDown:
		move(0, 1)
		return true
	case tcell.KeyLeft:
		move(-1, 0)
		return true
	case tcell.KeyRight:
		move(1, 0)
		return true
	case tcell.KeyHome:
		move(-c.selX1, 0)
		return true
	case tcell.KeyEnd:
		move(cc-1-c.selX1, 0)
		return true
	case tcell.KeyPgUp:
		st := scrollUIState(sp)
		sp.SetScrollUIOffset(st.Offset - cr)
		c.layoutDirty = true
		return true
	case tcell.KeyPgDn:
		st := scrollUIState(sp)
		sp.SetScrollUIOffset(st.Offset + cr)
		c.layoutDirty = true
		return true
	case tcell.KeyEnter:
		copyAndClose()
		return true
	case tcell.KeyRune:
		switch e.Rune() {
		case ' ', 'v', 'V':
			toggleSelect()
		case 'y', 'Y':
			copyAndClose()
		case 'q', 'Q':
			c.closeCopyMode(true)
		case 'h':
			move(-1, 0)
		case 'j':
			move(0, 1)
		case 'k':
			move(0, -1)
		case 'l':
			move(1, 0)
		case 'g':
			jumpTop()
		case 'G':
			jumpBottom()
		}
		return true
	}
	return true
}

func (c *Client) drawCopyModeBar() {
	if !c.copyMode {
		return
	}
	w := c.srv.Get(c.selWin)
	if w == nil || w.Minimized {
		return
	}
	y := w.Y + w.H - 1
	x := w.X + 1
	maxW := w.W - 2
	if maxW < 8 {
		return
	}
	label := " COPY MODE "
	if c.copySelecting {
		label += "(selecting) "
	}
	label += "— hjkl/arrows move · g/G top/bottom · Space/v select · Enter/y copy · Esc cancel "
	fg := cell.RGB(0x00, 0x00, 0x00)
	bg := cell.RGB(0x00, 0xFF, 0xFF)
	for i := 0; i < maxW; i++ {
		c.set(x+i, y, ' ', fg, bg, 0)
	}
	c.drawString(x, y, uwidth.Truncate(label, maxW), fg, bg, cell.AttrBold)
}
