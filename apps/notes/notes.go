package notes

import (
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// App is a simple multi-line notepad.
type App struct {
	lines  []string
	cx, cy int
	scroll int
}

func New() *App {
	return &App{lines: []string{""}}
}

func (a *App) Init(ctx *uiapp.Context) error { return nil }

func (a *App) Close() error { return nil }

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventResize:
		return nil
	case uiapp.EventKey:
		return a.key(e)
	case uiapp.EventMouse:
		if e.Action == "press" && e.Y > 0 {
			a.cy = e.Y - 1 + a.scroll
			if a.cy < 0 {
				a.cy = 0
			}
			if a.cy >= len(a.lines) {
				a.cy = len(a.lines) - 1
			}
			a.cx = e.X
			if a.cx > len([]rune(a.lines[a.cy])) {
				a.cx = len([]rune(a.lines[a.cy]))
			}
		}
	}
	return nil
}

func (a *App) key(e uiapp.Event) error {
	ensure := func() {
		for len(a.lines) <= a.cy {
			a.lines = append(a.lines, "")
		}
	}
	ensure()
	line := []rune(a.lines[a.cy])
	switch e.Key {
	case "Enter":
		rest := string(line[a.cx:])
		a.lines[a.cy] = string(line[:a.cx])
		a.lines = append(a.lines[:a.cy+1], append([]string{rest}, a.lines[a.cy+1:]...)...)
		a.cy++
		a.cx = 0
	case "Backspace":
		if a.cx > 0 {
			a.lines[a.cy] = string(append(line[:a.cx-1], line[a.cx:]...))
			a.cx--
		} else if a.cy > 0 {
			prev := []rune(a.lines[a.cy-1])
			a.cx = len(prev)
			a.lines[a.cy-1] = string(prev) + a.lines[a.cy]
			a.lines = append(a.lines[:a.cy], a.lines[a.cy+1:]...)
			a.cy--
		}
	case "Left":
		if a.cx > 0 {
			a.cx--
		} else if a.cy > 0 {
			a.cy--
			a.cx = len([]rune(a.lines[a.cy]))
		}
	case "Right":
		if a.cx < len(line) {
			a.cx++
		} else if a.cy+1 < len(a.lines) {
			a.cy++
			a.cx = 0
		}
	case "Up":
		if a.cy > 0 {
			a.cy--
			if a.cx > len([]rune(a.lines[a.cy])) {
				a.cx = len([]rune(a.lines[a.cy]))
			}
		}
	case "Down":
		if a.cy+1 < len(a.lines) {
			a.cy++
			if a.cx > len([]rune(a.lines[a.cy])) {
				a.cx = len([]rune(a.lines[a.cy]))
			}
		}
	case "Home":
		a.cx = 0
	case "End":
		a.cx = len(line)
	default:
		if e.Rune != 0 && !e.Ctrl {
			a.lines[a.cy] = string(append(line[:a.cx], append([]rune{e.Rune}, line[a.cx:]...)...))
			a.cx++
		}
	}
	return nil
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0xAA)
	cv.FillRect(0, 0, cols, rows, bg)
	cv.DrawText(0, 0, " Notes — type freely ", cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)

	bodyRows := rows - 1
	if bodyRows < 1 {
		return nil
	}
	if a.cy < a.scroll {
		a.scroll = a.cy
	}
	if a.cy >= a.scroll+bodyRows {
		a.scroll = a.cy - bodyRows + 1
	}
	for i := 0; i < bodyRows; i++ {
		li := a.scroll + i
		text := ""
		if li < len(a.lines) {
			text = a.lines[li]
		}
		// pad line
		runes := []rune(text)
		for x := 0; x < cols; x++ {
			ch := ' '
			if x < len(runes) {
				ch = runes[x]
			}
			cellFG, cellBG := fg, bg
			if li == a.cy && x == a.cx {
				cellFG, cellBG = bg, fg
			}
			cv.DrawText(x, i+1, string(ch), cellFG, cellBG, 0)
		}
	}
	return nil
}
