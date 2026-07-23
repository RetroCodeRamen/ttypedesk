package mgrprog

import (
	"fmt"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
	"github.com/ttypedesk/ttypedesk/pkg/uwidth"
)

// App lists custom programs and removes them from Start / desktop.
type App struct {
	cfg     config.Config
	onSave  func(config.Config)
	ctx     *uiapp.Context
	sel     int
	confirm bool // confirm delete on selected
	status  string
	closing bool
}

func New(cfg config.Config, onSave func(config.Config)) *App {
	return &App{cfg: cfg, onSave: onSave}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	return nil
}

func (a *App) Close() error { return nil }

func (a *App) Hooks() uiapp.Hooks {
	return uiapp.Hooks{
		OnInit: func(ctx *uiapp.Context) {
			ctx.SetTitle("Manage Programs")
		},
	}
}

func (a *App) WantsClose() bool { return a.closing }

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventKey:
		return a.key(e)
	case uiapp.EventMouse:
		return a.mouse(e)
	}
	return nil
}

func (a *App) key(e uiapp.Event) error {
	n := len(a.cfg.Programs)
	if a.confirm {
		switch e.Key {
		case "Enter", "y", "Y":
			a.doRemove()
			a.confirm = false
		case "Escape", "n", "N":
			a.confirm = false
			a.status = "Cancelled"
		}
		if e.Rune == 'y' || e.Rune == 'Y' {
			a.doRemove()
			a.confirm = false
		}
		if e.Rune == 'n' || e.Rune == 'N' {
			a.confirm = false
			a.status = "Cancelled"
		}
		return nil
	}

	switch e.Key {
	case "Up":
		if a.sel > 0 {
			a.sel--
		}
	case "Down":
		if a.sel+1 < n {
			a.sel++
		}
	case "Delete", "Backspace":
		if n > 0 {
			a.confirm = true
		}
	case "Enter":
		if n > 0 {
			a.confirm = true
		}
	case "Escape":
		a.closing = true
	}
	if e.Rune == 'd' || e.Rune == 'D' {
		if n > 0 {
			a.confirm = true
		}
	}
	return nil
}

func (a *App) mouse(e uiapp.Event) error {
	if e.Action == "drag" || e.Action == "release" {
		return nil
	}
	if a.confirm {
		// bottom confirm row: Y / N regions
		if e.Y >= 0 {
			a.doRemove()
			a.confirm = false
		}
		return nil
	}
	// list starts at y=2
	if e.Y < 2 {
		return nil
	}
	idx := e.Y - 2
	if idx < 0 || idx >= len(a.cfg.Programs) {
		// click Remove button area
		if e.Y == 2+len(a.cfg.Programs)+1 || e.Y >= 10 {
			if len(a.cfg.Programs) > 0 {
				a.confirm = true
			}
		}
		return nil
	}
	if idx == a.sel {
		a.confirm = true
	} else {
		a.sel = idx
	}
	return nil
}

func (a *App) doRemove() {
	if a.sel < 0 || a.sel >= len(a.cfg.Programs) {
		return
	}
	p := a.cfg.Programs[a.sel]
	name := p.Name
	if !a.cfg.RemoveProgram(p.ID) {
		a.status = "Remove failed"
		return
	}
	if a.onSave != nil {
		a.onSave(a.cfg)
	}
	if a.ctx != nil {
		a.ctx.Notify("Manage Programs", "Removed "+name, "🗑")
	}
	a.status = "Removed " + name
	if a.sel >= len(a.cfg.Programs) && a.sel > 0 {
		a.sel--
	}
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	cv.FillRect(0, 0, cols, rows, bg)
	cv.DrawText(0, 0, " Manage Programs ", cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)
	cv.DrawText(1, 1, "Custom Start-menu apps — Delete/Enter to remove", fg, bg, 0)

	if len(a.cfg.Programs) == 0 {
		cv.DrawText(1, 3, "(no custom programs)", fg, bg, 0)
	} else {
		for i, p := range a.cfg.Programs {
			y := 2 + i
			if y >= rows-3 {
				break
			}
			f, b := fg, bg
			if i == a.sel {
				f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
			}
			folder := "Programs"
			if p.MenuFolder() == config.MenuSystem {
				folder = "System"
			}
			icon := p.Icon
			if icon == "" {
				icon = "🚀"
			}
			line := fmt.Sprintf("%s %s  [%s]  %s", icon, p.Name, folder, p.Command)
			cv.DrawText(1, y, uwidth.Truncate(line, cols-2), f, b, 0)
		}
	}

	help := "↑↓ select  Del/Enter remove  Esc close"
	if a.confirm && a.sel >= 0 && a.sel < len(a.cfg.Programs) {
		help = fmt.Sprintf("Remove %q?  Enter/Y=yes  Esc/N=no", a.cfg.Programs[a.sel].Name)
	} else if a.status != "" {
		help = a.status
	}
	if rows > 0 {
		cv.DrawText(0, rows-1, " "+help, cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
	}
	return nil
}
