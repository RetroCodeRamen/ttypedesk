package wallpaper

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"sync"

	"github.com/RetroCodeRamen/ttypedesk/assets"
	"github.com/RetroCodeRamen/ttypedesk/internal/config"
	"github.com/RetroCodeRamen/ttypedesk/internal/gfx"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Cache holds a resized half-block wallpaper grid.
type Cache struct {
	mu     sync.Mutex
	path   string
	fit    string
	cols   int
	rows   int
	img    image.Image
	cells  []cell.Cell
	loaded bool
}

// Ensure returns cells for the desktop field (cols × rows), reencoding on change.
func (c *Cache) Ensure(wp config.Wallpaper, cols, rows int) []cell.Cell {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cols < 1 || rows < 1 {
		return nil
	}
	fit := wp.Fit
	if fit == "" {
		fit = "cover"
	}
	path := ResolvePath(wp.Path)
	needLoad := !c.loaded || c.path != path
	if needLoad {
		c.path = path
		c.img = nil
		c.loaded = false
		if img := loadImage(path); img != nil {
			c.img = img
			c.loaded = true
		}
	}
	if c.img == nil {
		c.cells = nil
		c.cols, c.rows = cols, rows
		c.fit = fit
		return nil
	}
	if c.cells != nil && c.cols == cols && c.rows == rows && c.fit == fit && !needLoad {
		return c.cells
	}
	c.fit = fit
	c.cols, c.rows = cols, rows
	c.cells = gfx.EncodeHalfBlockFit(c.img, cols, rows, fit, 0, 0)
	return c.cells
}

// Invalidate forces reload on next Ensure.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	c.loaded = false
	c.img = nil
	c.cells = nil
	c.mu.Unlock()
}

// ResolvePath maps builtin tokens and empty path to a loadable key.
func ResolvePath(path string) string {
	if path == "" {
		return assets.BuiltinBliss
	}
	if assets.IsBuiltin(path) {
		return path
	}
	return path
}

func loadImage(path string) image.Image {
	if data := assets.BuiltinBytes(path); data != nil {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil
		}
		return img
	}
	// Prefer user/shipped Wallpaper/bliss.jpg when path points there relative.
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), path))
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", path))
		}
		if wd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(wd, path))
		}
	}
	for _, p := range candidates {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		img, _, err := image.Decode(f)
		_ = f.Close()
		if err == nil {
			return img
		}
	}
	return nil
}
