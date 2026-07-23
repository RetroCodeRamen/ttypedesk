package client

// dock helpers for top/bottom (horizontal) and left/right (vertical) taskbars.

func (c *Client) dockSide() string {
	return c.cfg.TaskbarDock()
}

func (c *Client) isVerticalDock() bool {
	s := c.dockSide()
	return s == "left" || s == "right"
}

// taskbarThickness is 1 for horizontal docks, 7 for vertical so " Start " fits.
func (c *Client) taskbarThickness() int {
	if c.isVerticalDock() {
		return 7
	}
	return 1
}

// taskbarRow is the screen row of a horizontal taskbar (-1 if vertical).
func (c *Client) taskbarRow() int {
	if c.isVerticalDock() {
		return -1
	}
	_, h := c.screen.Size()
	if c.dockSide() == "bottom" {
		return h - 1
	}
	return 0
}

// taskbarCol0 is the first column of a vertical taskbar (-1 if horizontal).
func (c *Client) taskbarCol0() int {
	if !c.isVerticalDock() {
		return -1
	}
	w, _ := c.screen.Size()
	if c.dockSide() == "right" {
		return w - c.taskbarThickness()
	}
	return 0
}

// desktopRect returns exclusive-end desktop field bounds.
func (c *Client) desktopRect() (x0, y0, x1, y1 int) {
	w, h := c.screen.Size()
	th := c.taskbarThickness()
	switch c.dockSide() {
	case "bottom":
		return 0, 0, w, h - th
	case "left":
		return th, 0, w, h
	case "right":
		return 0, 0, w - th, h
	default: // top
		return 0, th, w, h
	}
}

func (c *Client) desktopYRange() (y0, y1 int) {
	_, y0, _, y1 = c.desktopRect()
	return
}

func (c *Client) desktopXRange() (x0, x1 int) {
	x0, _, x1, _ = c.desktopRect()
	return
}

func (c *Client) onTaskbar(x, y int) bool {
	if c.isVerticalDock() {
		c0 := c.taskbarCol0()
		th := c.taskbarThickness()
		return x >= c0 && x < c0+th
	}
	return y == c.taskbarRow()
}

func (c *Client) onStartButton(x, y int) bool {
	if !c.onTaskbar(x, y) {
		return false
	}
	if c.isVerticalDock() {
		return y == 0
	}
	return x >= 0 && x < 9
}

// clampIconToDesktop keeps a desktop icon inside the desktop field.
func (c *Client) clampIconToDesktop(x, y int) (int, int) {
	x0, y0, x1, y1 := c.desktopRect()
	if x < x0 {
		x = x0
	}
	if x > x1-4 {
		x = x1 - 4
	}
	if x < x0 {
		x = x0
	}
	if y < y0 {
		y = y0
	}
	if y > y1-2 {
		y = y1 - 2
	}
	if y < y0 {
		y = y0
	}
	return x, y
}
