package addprog

import (
	"fmt"
	"strings"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
	"github.com/ttypedesk/ttypedesk/pkg/uwidth"
)

// App lets the user register a shell command under Programs or System.
type App struct {
	cfg      config.Config
	onSave   func(config.Config)
	ctx      *uiapp.Context
	name     string // TTYPE Desk display name
	command  string
	iconIdx  int
	icons    []string
	menuSys  bool // false=Programs, true=System
	desktop  bool
	bridge   bool // launch Command via the GUI-TUI Bridge instead of a shell PTY
	field    int  // 0 name, 1 command, 2 icon, 3 menu, 4 desktop, 5 bridge, 6 save, 7 cancel
	editing  bool
	dropOpen bool
	status   string
	dropOff  int
	closing  bool
}

func New(cfg config.Config, onSave func(config.Config)) *App {
	icons := cfg.AvailableIcons()
	if len(icons) == 0 {
		icons = []string{"🚀"}
	}
	return &App{
		cfg:     cfg,
		onSave:  onSave,
		icons:   icons,
		desktop: true,
		field:   0,
	}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	return nil
}

func (a *App) Close() error { return nil }

func (a *App) Hooks() uiapp.Hooks {
	return uiapp.Hooks{
		OnInit: func(ctx *uiapp.Context) {
			ctx.SetTitle("Add Program")
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
	if a.dropOpen {
		switch e.Key {
		case "Escape":
			a.dropOpen = false
		case "Up":
			if a.iconIdx > 0 {
				a.iconIdx--
			}
			a.ensureDropVisible()
		case "Down":
			if a.iconIdx < len(a.icons)-1 {
				a.iconIdx++
			}
			a.ensureDropVisible()
		case "Enter":
			a.dropOpen = false
		}
		return nil
	}

	if a.editing {
		switch e.Key {
		case "Enter", "Escape":
			a.editing = false
		case "Backspace":
			buf := a.editBuf()
			r := []rune(buf)
			if len(r) > 0 {
				a.setEditBuf(string(r[:len(r)-1]))
			}
		default:
			if e.Rune != 0 && !e.Ctrl {
				a.setEditBuf(a.editBuf() + string(e.Rune))
			}
		}
		return nil
	}

	switch e.Key {
	case "Up":
		if a.field > 0 {
			a.field--
		}
	case "Down", "Tab":
		if a.field < 7 {
			a.field++
		}
	case "Enter":
		a.activate()
	case "Escape":
		a.closing = true
	case "Left", "Right":
		switch a.field {
		case 2:
			if e.Key == "Left" && a.iconIdx > 0 {
				a.iconIdx--
			}
			if e.Key == "Right" && a.iconIdx < len(a.icons)-1 {
				a.iconIdx++
			}
		case 3:
			a.menuSys = !a.menuSys
		case 4:
			a.desktop = !a.desktop
		case 5:
			a.bridge = !a.bridge
		}
	default:
		if e.Rune == ' ' {
			a.activate()
		}
	}
	return nil
}

func (a *App) mouse(e uiapp.Event) error {
	if e.Action == "drag" || e.Action == "release" {
		return nil
	}
	if a.dropOpen {
		row := e.Y - 5
		if row >= 0 && row < 6 {
			idx := a.dropOff + row
			if idx >= 0 && idx < len(a.icons) {
				a.iconIdx = idx
				a.dropOpen = false
			}
			return nil
		}
		a.dropOpen = false
		return nil
	}
	switch e.Y {
	case 2:
		a.field = 0
		a.editing = true
	case 3:
		a.field = 1
		a.editing = true
	case 4:
		a.field = 2
		a.dropOpen = true
		a.ensureDropVisible()
	case 5:
		a.field = 3
		a.menuSys = !a.menuSys
	case 6:
		a.field = 4
		a.desktop = !a.desktop
	case 7:
		a.field = 5
		a.bridge = !a.bridge
	case 9:
		if e.X < 16 {
			a.field = 6
			a.save()
		} else {
			a.field = 7
			a.closing = true
		}
	}
	return nil
}

func (a *App) editBuf() string {
	if a.field == 0 {
		return a.name
	}
	return a.command
}

func (a *App) setEditBuf(s string) {
	if a.field == 0 {
		a.name = s
	} else {
		a.command = s
	}
}

func (a *App) activate() {
	switch a.field {
	case 0, 1:
		a.editing = true
	case 2:
		a.dropOpen = !a.dropOpen
		a.ensureDropVisible()
	case 3:
		a.menuSys = !a.menuSys
	case 4:
		a.desktop = !a.desktop
	case 5:
		a.bridge = !a.bridge
	case 6:
		a.save()
	case 7:
		a.closing = true
	}
}

func (a *App) ensureDropVisible() {
	const vis = 6
	if a.iconIdx < a.dropOff {
		a.dropOff = a.iconIdx
	}
	if a.iconIdx >= a.dropOff+vis {
		a.dropOff = a.iconIdx - vis + 1
	}
}

func (a *App) save() {
	name := strings.TrimSpace(a.name)
	cmd := strings.TrimSpace(a.command)
	if name == "" {
		a.status = "Desk name is required (e.g. Task Manager)"
		return
	}
	if cmd == "" {
		if a.bridge {
			a.status = "X11 app command is required (e.g. firefox)"
		} else {
			a.status = "Command is required (e.g. htop)"
		}
		return
	}
	icon := "🚀"
	if a.iconIdx >= 0 && a.iconIdx < len(a.icons) {
		icon = a.icons[a.iconIdx]
	}
	menu := config.MenuPrograms
	if a.menuSys {
		menu = config.MenuSystem
	}
	id := fmt.Sprintf("p%d", time.Now().UnixNano()%1_000_000_000)
	p := config.Program{
		ID:      id,
		Name:    name,
		Command: cmd,
		Icon:    icon,
		Menu:    menu,
		Desktop: a.desktop,
		Bridge:  a.bridge,
	}
	a.cfg.Programs = append(a.cfg.Programs, p)
	a.cfg.SyncProgramDesktop()
	if a.onSave != nil {
		a.onSave(a.cfg)
	}
	if a.ctx != nil {
		folder := "Programs"
		if a.menuSys {
			folder = "System"
		}
		a.ctx.Notify("Add Program", fmt.Sprintf("%s → Start ▸ %s", name, folder), icon)
	}
	a.status = "Saved"
	a.closing = true
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	cv.FillRect(0, 0, cols, rows, bg)
	cv.DrawText(0, 0, " Add Program ", cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)
	cv.DrawText(1, 1, "Desk name is what appears in Start — command is what runs", fg, bg, 0)

	icon := "🚀"
	if a.iconIdx >= 0 && a.iconIdx < len(a.icons) {
		icon = a.icons[a.iconIdx]
	}

	drawField := func(y int, label, value string, field int) {
		f, b := fg, bg
		if a.field == field {
			f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
		}
		line := label + value
		if a.editing && a.field == field {
			line += "█"
		}
		cv.DrawText(1, y, uwidth.Truncate(line, cols-2), f, b, 0)
	}

	drawField(2, "Desk name: ", a.name, 0)
	cmdLabel := "Command:   "
	if a.bridge {
		cmdLabel = "X11 app:   "
	}
	drawField(3, cmdLabel, a.command, 1)

	f, b := fg, bg
	if a.field == 2 {
		f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
	}
	cv.DrawText(1, 4, uwidth.Truncate(fmt.Sprintf("Icon:      %s  (Enter/click unused emoji)", icon), cols-2), f, b, 0)

	f, b = fg, bg
	if a.field == 3 {
		f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
	}
	menuLabel := "Programs"
	if a.menuSys {
		menuLabel = "System"
	}
	cv.DrawText(1, 5, uwidth.Truncate("Start menu: "+menuLabel+"  (←→ or click to toggle)", cols-2), f, b, 0)

	f, b = fg, bg
	if a.field == 4 {
		f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
	}
	desk := "[ ]"
	if a.desktop {
		desk = "[X]"
	}
	cv.DrawText(1, 6, desk+" Add desktop shortcut", f, b, 0)

	f, b = fg, bg
	if a.field == 5 {
		f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
	}
	bridgeBox := "[ ]"
	if a.bridge {
		bridgeBox = "[X]"
	}
	cv.DrawText(1, 7, bridgeBox+" Launch via GUI-TUI Bridge (real X11 app, e.g. firefox, gimp)", f, b, 0)

	cv.DrawButton(1, 9, 12, "Save", fg, cell.RGB(0x00, 0xAA, 0x00), a.field == 6)
	cv.DrawButton(15, 9, 12, "Cancel", fg, cell.RGB(0xAA, 0xAA, 0xAA), a.field == 7)

	if a.dropOpen {
		a.drawDropdown(cv, cols, rows, hi, fg)
	}

	help := "Example: Desk name=Task Manager  Command=htop  Menu=System"
	if a.status != "" {
		help = a.status
	}
	if rows > 0 {
		cv.DrawText(0, rows-1, " "+help, cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
	}
	return nil
}

func (a *App) drawDropdown(cv *uiapp.Canvas, cols, rows int, hi, fg cell.Color) {
	x, y := 12, 5
	w := 18
	h := 8
	if y+h >= rows {
		h = rows - y - 1
	}
	if h < 3 {
		return
	}
	cv.FillRect(x, y, w, h, cell.RGB(0xFF, 0xFF, 0xFF))
	cv.DrawBox(x, y, w, h, fg, cell.RGB(0xFF, 0xFF, 0xFF))
	vis := h - 2
	for i := 0; i < vis; i++ {
		idx := a.dropOff + i
		if idx >= len(a.icons) {
			break
		}
		ff, bb := fg, cell.RGB(0xFF, 0xFF, 0xFF)
		if idx == a.iconIdx {
			ff, bb = cell.RGB(0xFF, 0xFF, 0xFF), hi
		}
		cv.DrawText(x+1, y+1+i, a.icons[idx]+" choose", ff, bb, 0)
	}
}
