package client

import (
	"github.com/gdamore/tcell/v2"
	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/slog"
	"github.com/ttypedesk/ttypedesk/pkg/uwidth"
)

// startMenuItem is one row in the Start menu (leaf or submenu).
type startMenuItem struct {
	Label string
	Icon  string
	Sub   []startMenuItem // non-nil => submenu flyout
	Do    func()          // leaf action
}

func (c *Client) buildStartMenu() {
	launch := func(action string) func() {
		return func() {
			_ = c.srv.LaunchAction(action)
			c.layoutDirty = true
		}
	}
	programs := []startMenuItem{
		{Label: "Notes", Icon: c.iconGlyph("📝"), Do: launch("notes")},
		{Label: "Calendar", Icon: c.iconGlyph("📅"), Do: launch("calendar")},
		{Label: "Clock", Icon: c.iconGlyph("🕐"), Do: launch("clock")},
		{Label: "Image Viewer", Icon: c.iconGlyph("🖼"), Do: launch("image")},
		{Label: "Amp", Icon: c.iconGlyph("🎵"), Do: launch("amp")},
		{Label: "Vid", Icon: c.iconGlyph("🎬"), Do: launch("vid")},
	}
	system := []startMenuItem{
		{Label: "Terminal", Icon: c.iconGlyph("💻"), Do: launch("terminal")},
		{Label: "Files", Icon: c.iconGlyph("📁"), Do: launch("files")},
		{Label: "Settings", Icon: c.iconGlyph("⚙️"), Do: launch("settings")},
		{Label: "Manual", Icon: c.iconGlyph("📖"), Do: launch("manual")},
		{Label: "System folder", Icon: c.iconGlyph("📂"), Do: launch("files:system")},
		{Label: "App Store", Icon: c.iconGlyph("🛍"), Do: launch("appstore")},
		{Label: "Add Program…", Icon: c.iconGlyph("➕"), Do: launch("addprog")},
		{Label: "Manage Programs…", Icon: c.iconGlyph("🗑"), Do: launch("mgrprog")},
	}
	for _, p := range c.cfg.Programs {
		p := p
		icon := p.Icon
		if icon == "" {
			icon = "🚀"
		}
		item := startMenuItem{
			Label: p.Name,
			Icon:  c.iconGlyph(icon),
			Do:    launch(config.ProgramAction(p.ID)),
		}
		if p.MenuFolder() == config.MenuSystem {
			// Insert before Add/Manage at the end.
			insertAt := len(system) - 2
			if insertAt < 0 {
				insertAt = len(system)
			}
			system = append(system[:insertAt], append([]startMenuItem{item}, system[insertAt:]...)...)
		} else {
			programs = append(programs, item)
		}
	}
	c.menuRoot = []startMenuItem{
		{Label: "Programs", Icon: "▶", Sub: programs},
		{Label: "System", Icon: "▶", Sub: system},
		{Label: "Quit", Icon: "⏻", Do: func() { c.quit = true }},
	}
	c.menuIdx = 0
	c.subIdx = 0
	c.subOpen = -1
}

func (c *Client) closeStartMenu() {
	c.menuOpen = false
	c.subOpen = -1
	c.layoutDirty = true
}

func (c *Client) openStartMenu() {
	c.buildStartMenu()
	c.menuOpen = true
	c.centerOpen = false
	c.menuIdx = 0
	c.subOpen = -1
	c.layoutDirty = true
}

func (c *Client) toggleStartMenu() {
	if c.cfg.Palette.StartOpensPalette {
		c.togglePalette()
		return
	}
	if c.menuOpen {
		c.closeStartMenu()
	} else {
		c.openStartMenu()
	}
}

func (c *Client) menuRootRect() (x, y, w, h int) {
	w = 22
	h = len(c.menuRoot) + 2
	dx0, dy0, dx1, dy1 := c.desktopRect()
	// Anchor against the Start button / dock edge, then grow into the desktop.
	switch c.dockSide() {
	case "bottom":
		x = dx0
		y = dy1 - h // sit just above the taskbar
	case "left":
		x = dx0 // flush with desktop left (right of vertical bar)
		y = dy0
	case "right":
		x = dx1 - w // flush with desktop right (left of vertical bar)
		y = dy0
	default: // top
		x = dx0
		y = dy0 // just below the taskbar
	}
	return c.clampMenuPanel(x, y, w, h)
}

func (c *Client) menuFlyoutRect() (x, y, w, h int, ok bool) {
	if c.subOpen < 0 || c.subOpen >= len(c.menuRoot) {
		return 0, 0, 0, 0, false
	}
	sub := c.menuRoot[c.subOpen].Sub
	if len(sub) == 0 {
		return 0, 0, 0, 0, false
	}
	rx, ry, rw, rh := c.menuRootRect()
	dx0, dy0, dx1, dy1 := c.desktopRect()
	w = 24
	h = len(sub) + 2

	// Prefer opening away from the taskbar.
	switch c.dockSide() {
	case "right":
		x = rx - w + 1 // open left
	case "left":
		x = rx + rw - 1 // open right
	default:
		x = rx + rw - 1 // open right (horizontal docks)
	}

	// Align to the parent row, then keep the panel inside the desktop.
	y = ry + 1 + c.subOpen
	switch c.dockSide() {
	case "bottom":
		// Grow upward from the parent row / root bottom so we never cover the bar.
		if y+h > dy1 {
			y = dy1 - h
		}
		// Prefer bottom-aligning tall flyouts with the root panel.
		if h > rh && ry+rh-h >= dy0 {
			y = ry + rh - h
		}
	case "top":
		if y < dy0 {
			y = dy0
		}
		if y+h > dy1 {
			y = dy1 - h
		}
	default: // left / right — full height desktop
		if y+h > dy1 {
			y = dy1 - h
		}
		if y < dy0 {
			y = dy0
		}
	}
	_ = dx0
	_ = dx1
	x, y, w, h = c.clampMenuPanel(x, y, w, h)
	return x, y, w, h, true
}

// clampMenuPanel keeps a menu panel inside the desktop field (never over the taskbar).
func (c *Client) clampMenuPanel(x, y, w, h int) (int, int, int, int) {
	dx0, dy0, dx1, dy1 := c.desktopRect()
	deskW, deskH := dx1-dx0, dy1-dy0
	if w > deskW {
		w = deskW
	}
	if h > deskH {
		h = deskH
	}
	if x < dx0 {
		x = dx0
	}
	if y < dy0 {
		y = dy0
	}
	if x+w > dx1 {
		x = dx1 - w
	}
	if y+h > dy1 {
		y = dy1 - h
	}
	if x < dx0 {
		x = dx0
	}
	if y < dy0 {
		y = dy0
	}
	return x, y, w, h
}

func (c *Client) handleStartMenuKey(e *tcell.EventKey) {
	switch e.Key() {
	case tcell.KeyEscape:
		if c.subOpen >= 0 {
			c.subOpen = -1
			c.layoutDirty = true
			return
		}
		c.closeStartMenu()
	case tcell.KeyUp:
		if c.subOpen >= 0 {
			if c.subIdx > 0 {
				c.subIdx--
			}
		} else if c.menuIdx > 0 {
			c.menuIdx--
		}
		c.layoutDirty = true
	case tcell.KeyDown:
		if c.subOpen >= 0 {
			if c.subIdx < len(c.menuRoot[c.subOpen].Sub)-1 {
				c.subIdx++
			}
		} else if c.menuIdx < len(c.menuRoot)-1 {
			c.menuIdx++
		}
		c.layoutDirty = true
	case tcell.KeyRight:
		c.openSubmenuAt(c.menuIdx)
	case tcell.KeyLeft:
		if c.subOpen >= 0 {
			c.subOpen = -1
			c.layoutDirty = true
		}
	case tcell.KeyEnter:
		c.activateStartMenu()
	}
}

func (c *Client) openSubmenuAt(idx int) {
	if idx < 0 || idx >= len(c.menuRoot) {
		return
	}
	if len(c.menuRoot[idx].Sub) == 0 {
		return
	}
	c.menuIdx = idx
	c.subOpen = idx
	c.subIdx = 0
	c.layoutDirty = true
}

func (c *Client) activateStartMenu() {
	if c.subOpen >= 0 {
		sub := c.menuRoot[c.subOpen].Sub
		if c.subIdx >= 0 && c.subIdx < len(sub) {
			item := sub[c.subIdx]
			c.closeStartMenu()
			if item.Do != nil {
				item.Do()
			}
		}
		return
	}
	if c.menuIdx < 0 || c.menuIdx >= len(c.menuRoot) {
		return
	}
	item := c.menuRoot[c.menuIdx]
	if len(item.Sub) > 0 {
		c.openSubmenuAt(c.menuIdx)
		return
	}
	c.closeStartMenu()
	if item.Do != nil {
		item.Do()
	}
}

// handleStartMenuMouse returns true if the event was consumed.
func (c *Client) handleStartMenuMouse(x, y int, btn tcell.ButtonMask) bool {
	// Hover / highlight even without click.
	c.hoverStartMenu(x, y)

	if btn&tcell.ButtonPrimary == 0 {
		return true // still consume motion while menu open
	}

	// Toggle Start button
	if c.onStartButton(x, y) {
		c.closeStartMenu()
		return true
	}

	if fx, fy, fw, fh, ok := c.menuFlyoutRect(); ok {
		if x >= fx && x < fx+fw && y >= fy && y < fy+fh {
			idx := y - fy - 1
			if idx >= 0 && idx < len(c.menuRoot[c.subOpen].Sub) {
				c.subIdx = idx
				item := c.menuRoot[c.subOpen].Sub[idx]
				label := item.Label
				c.closeStartMenu()
				slog.Info("start menu click flyout %q", label)
				if item.Do != nil {
					func() {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("panic launching %q: %v", label, r)
							}
						}()
						item.Do()
					}()
				}
			}
			return true
		}
	}

	rx, ry, rw, rh := c.menuRootRect()
	if x >= rx && x < rx+rw && y >= ry && y < ry+rh {
		idx := y - ry - 1
		if idx >= 0 && idx < len(c.menuRoot) {
			c.menuIdx = idx
			item := c.menuRoot[idx]
			if len(item.Sub) > 0 {
				c.openSubmenuAt(idx)
			} else {
				label := item.Label
				c.closeStartMenu()
				slog.Info("start menu click root %q", label)
				if item.Do != nil {
					func() {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("panic launching %q: %v", label, r)
							}
						}()
						item.Do()
					}()
				}
			}
		}
		return true
	}

	c.closeStartMenu()
	return true
}

func (c *Client) hoverStartMenu(x, y int) {
	if fx, fy, fw, fh, ok := c.menuFlyoutRect(); ok {
		if x >= fx && x < fx+fw && y >= fy && y < fy+fh {
			idx := y - fy - 1
			if idx >= 0 && idx < len(c.menuRoot[c.subOpen].Sub) && idx != c.subIdx {
				c.subIdx = idx
				c.layoutDirty = true
			}
			return
		}
	}
	rx, ry, rw, rh := c.menuRootRect()
	if x >= rx && x < rx+rw && y >= ry && y < ry+rh {
		idx := y - ry - 1
		if idx >= 0 && idx < len(c.menuRoot) {
			if idx != c.menuIdx {
				c.menuIdx = idx
				c.layoutDirty = true
			}
			if len(c.menuRoot[idx].Sub) > 0 {
				if c.subOpen != idx {
					c.openSubmenuAt(idx)
				}
			} else if c.subOpen >= 0 {
				c.subOpen = -1
				c.layoutDirty = true
			}
		}
	}
}

func (c *Client) drawMenu() {
	rx, ry, rw, rh := c.menuRootRect()
	c.drawMenuPanel(rx, ry, rw, rh, c.menuRoot, c.menuIdx, true)
	if fx, fy, fw, fh, ok := c.menuFlyoutRect(); ok {
		c.drawMenuPanel(fx, fy, fw, fh, c.menuRoot[c.subOpen].Sub, c.subIdx, false)
	}
}

func (c *Client) drawMenuPanel(x, y, w, h int, items []startMenuItem, sel int, root bool) {
	th := c.cfg.Theme
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			c.set(x+col, y+row, ' ', th.MenuFG, th.MenuBG, 0)
		}
	}
	c.set(x, y, '╔', th.MenuFG, th.MenuBG, 0)
	c.set(x+w-1, y, '╗', th.MenuFG, th.MenuBG, 0)
	c.set(x, y+h-1, '╚', th.MenuFG, th.MenuBG, 0)
	c.set(x+w-1, y+h-1, '╝', th.MenuFG, th.MenuBG, 0)
	for col := 1; col < w-1; col++ {
		c.set(x+col, y, '═', th.MenuFG, th.MenuBG, 0)
		c.set(x+col, y+h-1, '═', th.MenuFG, th.MenuBG, 0)
	}
	for row := 1; row < h-1; row++ {
		c.set(x, y+row, '║', th.MenuFG, th.MenuBG, 0)
		c.set(x+w-1, y+row, '║', th.MenuFG, th.MenuBG, 0)
	}
	for i, item := range items {
		fg, bg := th.MenuFG, th.MenuBG
		if i == sel {
			fg, bg = th.TaskbarFG, th.MenuHighlight
		}
		label := " "
		if item.Icon != "" && !root {
			label += item.Icon + " "
		}
		label += item.Label
		if len(item.Sub) > 0 {
			// pad then ▶
			inner := w - 2
			runes := []rune(label)
			for len(runes) < inner-2 {
				runes = append(runes, ' ')
			}
			if len(runes) > inner-2 {
				runes = runes[:inner-2]
			}
			label = string(runes) + " ▶"
		}
		c.drawString(x+1, y+1+i, uwidth.Truncate(label, w-2), fg, bg, 0)
		// fill rest of row for highlight bar
		lw := uwidth.String(uwidth.Truncate(label, w-2))
		for col := 1 + lw; col < w-1; col++ {
			c.set(x+col, y+1+i, ' ', fg, bg, 0)
		}
	}
}
