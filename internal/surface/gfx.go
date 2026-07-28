package surface

import (
	"image"
	"os"
	"sync"

	"github.com/ttypedesk/ttypedesk/internal/gfx"
	"github.com/ttypedesk/ttypedesk/pkg/cell"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// GfxSurface renders an RGBA image as half-block cells.
type GfxSurface struct {
	id    string
	title string
	img   image.Image
	cols  int
	rows  int
	offX  int
	offY  int
	cells []cell.Cell
	dirty bool
	mu    sync.Mutex
}

func NewGfxSurface(id, title, path string, cols, rows int) (*GfxSurface, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	s := &GfxSurface{
		id:    id,
		title: title,
		img:   img,
		cols:  cols,
		rows:  rows,
		dirty: true,
	}
	s.reencode()
	return s, nil
}

func NewGfxSurfaceFromImage(id, title string, img image.Image, cols, rows int) *GfxSurface {
	s := &GfxSurface{
		id:    id,
		title: title,
		img:   img,
		cols:  cols,
		rows:  rows,
		dirty: true,
	}
	s.reencode()
	return s
}

func (s *GfxSurface) reencode() {
	s.cells = gfx.EncodeHalfBlock(s.img, s.cols, s.rows, s.offX, s.offY)
	s.dirty = true
}

func (s *GfxSurface) ID() string    { return s.id }
func (s *GfxSurface) Kind() string  { return "gfx" }
func (s *GfxSurface) Title() string { return s.title }

func (s *GfxSurface) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

func (s *GfxSurface) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cols, s.rows = cols, rows
	s.reencode()
}

func (s *GfxSurface) HandleInput(ev InputEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch ev.Key {
	case "Left", "h":
		s.offX -= 2
	case "Right", "l":
		s.offX += 2
	case "Up", "k":
		s.offY -= 2
	case "Down", "j":
		s.offY += 2
	default:
		if ev.Rune == 'h' {
			s.offX -= 2
		} else if ev.Rune == 'l' {
			s.offX += 2
		} else if ev.Rune == 'k' {
			s.offY -= 2
		} else if ev.Rune == 'j' {
			s.offY += 2
		}
	}
	if s.offX < 0 {
		s.offX = 0
	}
	if s.offY < 0 {
		s.offY = 0
	}
	s.reencode()
	return nil
}

func (s *GfxSurface) ProduceDiff() cell.Diff {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return cell.Diff{}
	}
	s.dirty = false
	return cell.FullGridDiff(s.cols, s.rows, append([]cell.Cell(nil), s.cells...))
}

func (s *GfxSurface) Snapshot() []cell.Cell {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]cell.Cell(nil), s.cells...)
}

func (s *GfxSurface) Close() error { return nil }
