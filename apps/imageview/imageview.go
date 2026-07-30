package imageview

import (
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/RetroCodeRamen/ttypedesk/internal/gfx"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// App shows a half-block encoded image (or a gradient demo).
type App struct {
	path  string
	img   image.Image
	offX  int
	offY  int
	cells []cell.Cell
	cols  int
	rows  int
}

func New(path string) *App {
	return &App{path: path}
}

func NewDemo() *App {
	return &App{img: demoImage()}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.cols, a.rows = ctx.Cols, ctx.Rows
	if a.img == nil {
		if a.path != "" {
			f, err := os.Open(a.path)
			if err != nil {
				a.img = demoImage()
			} else {
				defer f.Close()
				img, _, err := image.Decode(f)
				if err != nil {
					a.img = demoImage()
				} else {
					a.img = img
				}
			}
		} else {
			a.img = demoImage()
		}
	}
	a.reencode()
	return nil
}

func (a *App) reencode() {
	a.cells = gfx.EncodeHalfBlock(a.img, a.cols, a.rows, a.offX, a.offY)
}

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventResize:
		a.cols, a.rows = e.Cols, e.Rows
		a.reencode()
	case uiapp.EventKey:
		switch e.Key {
		case "Left":
			a.offX -= 4
		case "Right":
			a.offX += 4
		case "Up":
			a.offY -= 4
		case "Down":
			a.offY += 4
		default:
			switch e.Rune {
			case 'h':
				a.offX -= 4
			case 'l':
				a.offX += 4
			case 'k':
				a.offY -= 4
			case 'j':
				a.offY += 4
			}
		}
		if a.offX < 0 {
			a.offX = 0
		}
		if a.offY < 0 {
			a.offY = 0
		}
		a.reencode()
	}
	return nil
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	if cols != a.cols || rows != a.rows {
		a.cols, a.rows = cols, rows
		a.reencode()
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			i := y*cols + x
			if i < len(a.cells) {
				c := a.cells[i]
				cv.DrawText(x, y, string(c.Rune), c.FG, c.BG, 0)
			}
		}
	}
	// Status line overlay
	if rows > 0 {
		msg := fmt.Sprintf(" Image Viewer  arrows/hjkl pan  off=%d,%d ", a.offX, a.offY)
		bg := cell.RGB(0x00, 0x00, 0x00)
		fg := cell.RGB(0xFF, 0xFF, 0x00)
		cv.DrawText(0, rows-1, msg, fg, bg, 0)
	}
	return nil
}

func (a *App) Close() error { return nil }

func demoImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 160, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / 159),
				G: uint8((y * 255) / 99),
				B: uint8(((x + y) * 255) / 258),
				A: 255,
			})
		}
	}
	// Draw a simple "TTYPE" mark
	for x := 20; x < 140; x++ {
		img.Set(x, 40, color.RGBA{255, 255, 255, 255})
		img.Set(x, 60, color.RGBA{255, 255, 255, 255})
	}
	for y := 40; y < 60; y++ {
		img.Set(80, y, color.RGBA{255, 255, 255, 255})
	}
	return img
}
