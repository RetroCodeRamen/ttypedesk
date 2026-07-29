package bridge

import (
	"testing"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

func TestCompositeTextPlacesCharactersInMappedRect(t *testing.T) {
	const cols, rows = 10, 4
	const capW, capH = 100, 80 // 10px per col, 20px per row

	cells := make([]cell.Cell, cols*rows)
	for i := range cells {
		cells[i] = cell.Cell{Rune: '▄', BG: cell.RGB(10, 20, 30)}
	}

	// Node spans pixel (20,20)-(60,40) -> cell (2,1)-(6,2): 2 rows won't
	// happen since height 20px = exactly 1 row, so this covers row 1 only,
	// cols 2..5.
	nodes := []textNode{{Text: "AB", X: 20, Y: 20, W: 40, H: 20}}
	compositeText(cells, cols, rows, nodes, capW, capH)

	if got := cells[1*cols+2].Rune; got != 'A' {
		t.Errorf("cell(2,1) rune = %q, want 'A'", got)
	}
	if got := cells[1*cols+3].Rune; got != 'B' {
		t.Errorf("cell(3,1) rune = %q, want 'B'", got)
	}
	// Untouched cells keep the original raster glyph.
	if got := cells[0*cols+0].Rune; got != '▄' {
		t.Errorf("untouched cell(0,0) rune = %q, want unchanged '▄'", got)
	}
}

func TestCompositeTextKeepsRasterBackground(t *testing.T) {
	const cols, rows = 5, 5
	const capW, capH = 50, 50
	bg := cell.RGB(1, 2, 3)
	cells := make([]cell.Cell, cols*rows)
	for i := range cells {
		cells[i] = cell.Cell{Rune: '▄', BG: bg}
	}
	compositeText(cells, cols, rows, []textNode{{Text: "X", X: 0, Y: 0, W: 10, H: 10}}, capW, capH)
	if cells[0].BG != bg {
		t.Errorf("overlaid cell BG = %v, want unchanged raster BG %v", cells[0].BG, bg)
	}
	if cells[0].Rune != 'X' {
		t.Errorf("overlaid cell Rune = %q, want 'X'", cells[0].Rune)
	}
}

func TestCompositeTextIgnoresDegenerateAndOutOfRangeNodes(t *testing.T) {
	const cols, rows = 5, 5
	const capW, capH = 50, 50
	cells := make([]cell.Cell, cols*rows)
	for i := range cells {
		cells[i] = cell.Cell{Rune: '▄'}
	}
	nodes := []textNode{
		// X/Y/W/H land on exact cell boundaries here so the floor/ceil
		// pixel->cell mapping can't round a zero-size box into a 1-cell
		// span (it legitimately can for a box straddling a cell boundary,
		// which isn't the degenerate case this test wants to isolate).
		{Text: "zero-size", X: 0, Y: 0, W: 0, H: 0},
		{Text: "negative", X: -100, Y: -100, W: -5, H: -5},
	}
	compositeText(cells, cols, rows, nodes, capW, capH)
	for i, c := range cells {
		if c.Rune != '▄' {
			t.Fatalf("cell %d was modified by a degenerate node: rune=%q", i, c.Rune)
		}
	}
}

func TestCompositeTextCollapsesWhitespace(t *testing.T) {
	const cols, rows = 20, 5
	const capW, capH = 200, 100
	cells := make([]cell.Cell, cols*rows)
	compositeText(cells, cols, rows, []textNode{{Text: "a\n  b   c", X: 0, Y: 0, W: 200, H: 20}}, capW, capH)
	var got []rune
	for i := 0; i < cols; i++ {
		if cells[i].Rune != 0 {
			got = append(got, cells[i].Rune)
		}
	}
	if string(got) != "a b c" {
		t.Errorf("got %q, want whitespace-collapsed %q", string(got), "a b c")
	}
}

func TestContrastColorPicksReadableForeground(t *testing.T) {
	if got := contrastColor(cell.RGB(255, 255, 255)); got != cell.RGB(0, 0, 0) {
		t.Errorf("contrastColor(white) = %v, want black", got)
	}
	if got := contrastColor(cell.RGB(0, 0, 0)); got != cell.RGB(0xFF, 0xFF, 0xFF) {
		t.Errorf("contrastColor(black) = %v, want white", got)
	}
}

func TestClampInt(t *testing.T) {
	cases := []struct{ v, lo, hi, want int }{
		{5, 0, 10, 5},
		{-5, 0, 10, 0},
		{15, 0, 10, 10},
		{0, 0, 10, 0},
		{10, 0, 10, 10},
	}
	for _, c := range cases {
		if got := clampInt(c.v, c.lo, c.hi); got != c.want {
			t.Errorf("clampInt(%d,%d,%d) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
		}
	}
}
