package client

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ttypedesk/ttypedesk/internal/clip"
	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/palette"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uwidth"
)

func (c *Client) openPalette() {
	c.closeFind()
	c.closeStartMenu()
	c.centerOpen = false
	c.paletteOpen = true
	c.paletteQuery = ""
	c.paletteSel = 0
	c.refreshPaletteHits()
	c.layoutDirty = true
}

func (c *Client) closePalette() {
	c.paletteOpen = false
	c.paletteQuery = ""
	c.paletteHits = nil
	c.paletteSel = 0
	c.palettePending = ""
	c.layoutDirty = true
}

func (c *Client) togglePalette() {
	if c.paletteOpen {
		c.closePalette()
		return
	}
	c.openPalette()
}

func (c *Client) refreshPaletteHits() {
	env := c.paletteEnv()
	c.paletteHits = palette.Search(env)
	if c.paletteSel >= len(c.paletteHits) {
		c.paletteSel = len(c.paletteHits) - 1
	}
	if c.paletteSel < 0 {
		c.paletteSel = 0
	}
}

func (c *Client) paletteEnv() palette.Env {
	wins := c.srv.Windows()
	pw := make([]palette.Win, 0, len(wins))
	for _, w := range wins {
		pw = append(pw, palette.Win{ID: w.ID, Title: w.Title, Minimized: w.Minimized})
	}
	icons := make([]palette.Icon, 0, len(c.cfg.DesktopIcons))
	for _, ic := range c.cfg.DesktopIcons {
		icons = append(icons, palette.Icon{Label: ic.Label, Action: ic.Action, Glyph: ic.Icon})
	}
	progs := make([]palette.Program, 0, len(c.cfg.Programs))
	for _, p := range c.cfg.Programs {
		progs = append(progs, palette.Program{
			ID: p.ID, Name: p.Name, Action: config.ProgramAction(p.ID), Icon: p.Icon,
		})
	}
	recipes := make([]palette.Recipe, 0, len(c.cfg.Recipes))
	for _, r := range c.cfg.Recipes {
		recipes = append(recipes, palette.Recipe{Match: r.Match, Action: r.Action, Confirm: r.Confirm})
	}
	max := c.cfg.Palette.MaxResults
	if max <= 0 {
		max = 12
	}
	return palette.Env{
		Query:      c.paletteQuery,
		Max:        max,
		Windows:    pw,
		Icons:      icons,
		Programs:   progs,
		Recipes:    recipes,
		History:    c.paletteHist,
		AsciiIcons: c.cfg.Palette.ASCIIIcons,
		Launch: func(action string) error {
			return c.srv.LaunchAction(action)
		},
		OpenPath: func(path string) error {
			return c.srv.OpenPath(path)
		},
		Focus: func(id string) {
			c.srv.Focus(id)
			c.layoutDirty = true
		},
		OpenFind: func(q string) {
			c.closePalette()
			c.openFind()
			c.findQuery = q
			c.runFind(true)
		},
		Quit: func() { c.quit = true },
		Notify: func(title, body string) {
			if c.notify != nil {
				c.notify.Post(title, body, "⌘", "palette")
			}
		},
		CopyText: func(text string) {
			clip.Set(text)
		},
		ApplyTheme: func(id string) {
			if !c.cfg.ApplyThemePack(id) {
				return
			}
			_ = config.Save(c.cfg)
			c.srv.SetConfig(c.cfg)
			c.ApplyConfig(c.cfg)
			name := c.cfg.Theme.Name
			if name == "" {
				name = id
			}
			if c.notify != nil {
				c.notify.Post(name+" theme", "Colors + wallpaper applied", "🪟", "palette")
			}
		},
		Confirm: func(prompt, action string) bool {
			if c.palettePending == action {
				c.palettePending = ""
				return true
			}
			c.palettePending = action
			if c.notify != nil {
				c.notify.Post("Confirm", prompt+" — Enter again to run", "⚠", "palette")
			}
			return false
		},
	}
}

func (c *Client) rememberPaletteQuery(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	out := []string{q}
	for _, h := range c.paletteHist {
		if h == q {
			continue
		}
		out = append(out, h)
		if len(out) >= 20 {
			break
		}
	}
	c.paletteHist = out
}

func (c *Client) handlePaletteKey(e *tcell.EventKey) bool {
	if !c.paletteOpen {
		return false
	}
	switch e.Key() {
	case tcell.KeyEscape:
		c.closePalette()
		return true
	case tcell.KeyUp:
		if c.paletteSel > 0 {
			c.paletteSel--
			c.layoutDirty = true
		}
		return true
	case tcell.KeyDown:
		if c.paletteSel+1 < len(c.paletteHits) {
			c.paletteSel++
			c.layoutDirty = true
		}
		return true
	case tcell.KeyEnter:
		c.runPaletteSel()
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		r := []rune(c.paletteQuery)
		if len(r) > 0 {
			c.paletteQuery = string(r[:len(r)-1])
			c.paletteSel = 0
			c.refreshPaletteHits()
			c.layoutDirty = true
		}
		return true
	case tcell.KeyCtrlU:
		c.paletteQuery = ""
		c.paletteSel = 0
		c.refreshPaletteHits()
		c.layoutDirty = true
		return true
	default:
		if e.Key() == tcell.KeyRune && e.Modifiers()&(tcell.ModAlt|tcell.ModCtrl) == 0 {
			c.paletteQuery += string(e.Rune())
			c.paletteSel = 0
			c.palettePending = ""
			c.refreshPaletteHits()
			c.layoutDirty = true
			return true
		}
	}
	return true
}

func (c *Client) runPaletteSel() {
	if c.paletteSel < 0 || c.paletteSel >= len(c.paletteHits) {
		return
	}
	hit := c.paletteHits[c.paletteSel]
	if hit.Subtitle == "recent" {
		c.paletteQuery = hit.Title
		c.paletteSel = 0
		c.refreshPaletteHits()
		c.layoutDirty = true
		return
	}
	pendingBefore := c.palettePending
	if hit.Run != nil {
		hit.Run()
	}
	// Confirm recipes: first Enter arms pending and keeps palette open.
	if c.palettePending != "" && c.palettePending != pendingBefore {
		c.layoutDirty = true
		return
	}
	c.rememberPaletteQuery(c.paletteQuery)
	palette.SaveHistory(c.paletteHist, 20)
	c.closePalette()
	c.layoutDirty = true
}

func (c *Client) drawPalette() {
	if !c.paletteOpen {
		return
	}
	sw, sh := c.screen.Size()
	dx0, dy0, dx1, dy1 := c.desktopRect()
	w := 56
	if w > dx1-dx0-2 {
		w = dx1 - dx0 - 2
	}
	if w < 28 {
		w = 28
	}
	maxRows := 14
	h := 3 + len(c.paletteHits)
	if h > maxRows {
		h = maxRows
	}
	if h < 5 {
		h = 5
	}
	x := dx0 + (dx1-dx0-w)/2
	y := dy0 + (dy1-dy0-h)/3
	if x < dx0 {
		x = dx0
	}
	if y < dy0 {
		y = dy0
	}

	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0xAA)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	border := cell.RGB(0x00, 0x00, 0x00)

	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			c.set(x+col, y+row, ' ', fg, bg, 0)
		}
	}
	// border
	for col := 0; col < w; col++ {
		c.set(x+col, y, '─', border, bg, 0)
		c.set(x+col, y+h-1, '─', border, bg, 0)
	}
	for row := 0; row < h; row++ {
		c.set(x, y+row, '│', border, bg, 0)
		c.set(x+w-1, y+row, '│', border, bg, 0)
	}
	c.set(x, y, '┌', border, bg, 0)
	c.set(x+w-1, y, '┐', border, bg, 0)
	c.set(x, y+h-1, '└', border, bg, 0)
	c.set(x+w-1, y+h-1, '┘', border, bg, 0)

	c.drawString(x+2, y, " Command palette ", cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)
	prompt := "> " + c.paletteQuery + "█"
	c.drawString(x+2, y+1, uwidth.Truncate(prompt, w-4), fg, bg, cell.AttrBold)

	listTop := y + 2
	listH := h - 3
	for i := 0; i < listH; i++ {
		if i >= len(c.paletteHits) {
			break
		}
		hit := c.paletteHits[i]
		line := hit.Icon + " " + hit.Title
		if hit.Subtitle != "" {
			line += "  —  " + hit.Subtitle
		}
		f, b := fg, bg
		if i == c.paletteSel {
			f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
		}
		for col := 1; col < w-1; col++ {
			c.set(x+col, listTop+i, ' ', f, b, 0)
		}
		c.drawString(x+2, listTop+i, uwidth.Truncate(line, w-4), f, b, 0)
	}
	_ = sw
	_ = sh
}
