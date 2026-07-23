package clock

import (
	"time"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// App is a reference native clock application.
type App struct {
	fmt string
}

func New() *App { return &App{fmt: "15:04:05"} }

func (a *App) Init(ctx *uiapp.Context) error {
	ctx.StartTimer(time.Second)
	return nil
}

func (a *App) Handle(e uiapp.Event) error { return nil }

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0x00, 0x00, 0xAA)
	fg := cell.RGB(0xFF, 0xFF, 0xFF)
	cv.FillRect(0, 0, cols, rows, bg)
	cv.DrawBox(0, 0, cols, rows, fg, bg)
	t := time.Now().Format(a.fmt)
	label := "TTYPE Desk Clock"
	cv.DrawText(2, 1, label, fg, bg, 0)
	if rows >= 4 {
		x := (cols - len(t)) / 2
		if x < 1 {
			x = 1
		}
		cv.DrawText(x, rows/2, t, fg, bg, cell.AttrBold)
	}
	return nil
}

func (a *App) Close() error { return nil }
