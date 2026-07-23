// Package cell defines the shared cell framebuffer types for TTYPE Desk.
package cell

// Color is always 24-bit RGB.
type Color struct {
	R, G, B uint8
}

func RGB(r, g, b uint8) Color { return Color{R: r, G: g, B: b} }

func (c Color) Equals(o Color) bool {
	return c.R == o.R && c.G == o.G && c.B == o.B
}

// Attr holds text attributes.
type Attr uint16

const (
	AttrBold Attr = 1 << iota
	AttrUnderline
	AttrItalic
	AttrBlink
	AttrReverse
	AttrStrike
)

// Cell is one terminal cell.
type Cell struct {
	Rune rune
	FG   Color
	BG   Color
	Attr Attr
}

func (c Cell) Equals(o Cell) bool {
	return c.Rune == o.Rune && c.FG.Equals(o.FG) && c.BG.Equals(o.BG) && c.Attr == o.Attr
}

// Rect is an inclusive-start exclusive-end rectangle in cell coordinates.
type Rect struct {
	X, Y, W, H int
}

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Diff is a dirty region of cells.
type Diff struct {
	Rect  Rect
	Cells []Cell // row-major, length W*H
}

// FullGridDiff builds a Diff covering an entire grid.
func FullGridDiff(cols, rows int, cells []Cell) Diff {
	return Diff{
		Rect:  Rect{X: 0, Y: 0, W: cols, H: rows},
		Cells: cells,
	}
}
