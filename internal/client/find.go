package client

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/RetroCodeRamen/ttypedesk/internal/surface"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uwidth"
	"github.com/gdamore/tcell/v2"
)

func (c *Client) openFind() {
	id := c.srv.Focused()
	if id == "" {
		return
	}
	w := c.srv.Get(id)
	if w == nil {
		return
	}
	if _, ok := w.Surface.(surface.ScrollbackSearchProvider); !ok {
		return
	}
	c.findOpen = true
	c.findWin = id
	c.findStatus = "F3/Alt+/ open · Enter older · Shift+Enter newer · Esc"
	c.layoutDirty = true
}

func (c *Client) closeFind() {
	c.findOpen = false
	c.findWin = ""
	c.findStatus = ""
	c.layoutDirty = true
}

func (c *Client) handleFindKey(e *tcell.EventKey) bool {
	if !c.findOpen {
		return false
	}
	switch e.Key() {
	case tcell.KeyEscape:
		c.closeFind()
		return true
	case tcell.KeyF3:
		// F3 = older, Shift+F3 = newer (works when find bar already open)
		c.runFind(e.Modifiers()&tcell.ModShift == 0)
		return true
	case tcell.KeyEnter:
		towardOlder := e.Modifiers()&tcell.ModShift == 0
		c.runFind(towardOlder)
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		r := []rune(c.findQuery)
		if len(r) > 0 {
			c.findQuery = string(r[:len(r)-1])
			c.findStatus = ""
			c.layoutDirty = true
		}
		return true
	case tcell.KeyCtrlU:
		c.findQuery = ""
		c.findStatus = ""
		c.layoutDirty = true
		return true
	default:
		if e.Key() == tcell.KeyRune && e.Modifiers()&(tcell.ModAlt|tcell.ModCtrl) == 0 {
			if unicode.IsPrint(e.Rune()) {
				c.findQuery += string(e.Rune())
				c.findStatus = ""
				c.layoutDirty = true
			}
			return true
		}
	}
	return true
}

func (c *Client) runFind(towardOlder bool) {
	w := c.srv.Get(c.findWin)
	if w == nil {
		c.closeFind()
		return
	}
	sp, ok := w.Surface.(surface.ScrollbackSearchProvider)
	if !ok {
		c.findStatus = "not a terminal"
		return
	}
	q := strings.TrimSpace(c.findQuery)
	if q == "" {
		c.findStatus = "enter a search string"
		c.layoutDirty = true
		return
	}
	found, n := sp.SearchScrollback(q, towardOlder)
	if !found {
		c.findStatus = "no match"
	} else {
		dir := "older"
		if !towardOlder {
			dir = "newer"
		}
		c.findStatus = fmt.Sprintf("%d hits — %s", n, dir)
	}
	c.layoutDirty = true
}

func (c *Client) drawFindBar() {
	if !c.findOpen {
		return
	}
	w := c.srv.Get(c.findWin)
	if w == nil || w.Minimized {
		return
	}
	y := w.Y + w.H - 1
	x := w.X + 1
	maxW := w.W - 2
	if maxW < 8 {
		return
	}
	label := " Find: " + c.findQuery + "█ "
	if c.findStatus != "" {
		label += " " + c.findStatus
	}
	fg := cell.RGB(0x00, 0x00, 0x00)
	bg := cell.RGB(0xFF, 0xFF, 0x00)
	for i := 0; i < maxW; i++ {
		c.set(x+i, y, ' ', fg, bg, 0)
	}
	c.drawString(x, y, uwidth.Truncate(label, maxW), fg, bg, cell.AttrBold)
}

// findMatchAt reports whether the cell at (row,col) is inside a case-insensitive match of query.
func findMatchAt(cells []cell.Cell, cols, row, col int, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || cols < 1 || row < 0 || col < 0 || col >= cols {
		return false
	}
	qr := []rune(q)
	runes := make([]rune, cols)
	for c := 0; c < cols; c++ {
		i := row*cols + c
		r := ' '
		if i < len(cells) && cells[i].Rune != 0 {
			r = unicode.ToLower(cells[i].Rune)
		} else {
			r = ' '
		}
		runes[c] = r
	}
	for start := 0; start+len(qr) <= cols; start++ {
		ok := true
		for j := range qr {
			if runes[start+j] != qr[j] {
				ok = false
				break
			}
		}
		if ok && col >= start && col < start+len(qr) {
			return true
		}
	}
	return false
}
