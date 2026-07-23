// Package gfx converts RGBA images to truecolor half-block cell grids.
package gfx

import (
	"image"
	"image/color"
	"strings"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

const halfBlock = '▄'

// EncodeHalfBlock maps an image into a cols×rows cell grid.
// Each cell covers 1×2 image samples (top=bg, bottom=fg) using '▄'.
// offX/offY are source pixel offsets for panning.
func EncodeHalfBlock(img image.Image, cols, rows, offX, offY int) []cell.Cell {
	return EncodeHalfBlockFit(img, cols, rows, "cover", offX, offY)
}

// EncodeHalfBlockFit encodes with fit mode: cover | contain | stretch.
func EncodeHalfBlockFit(img image.Image, cols, rows int, fit string, offX, offY int) []cell.Cell {
	if cols < 1 || rows < 1 || img == nil {
		return make([]cell.Cell, cols*rows)
	}
	b := img.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	out := make([]cell.Cell, cols*rows)
	dstW := float64(cols)
	dstH := float64(rows * 2)

	sx := float64(srcW) / dstW
	sy := float64(srcH) / dstH
	var scaleX, scaleY float64
	var originX, originY float64
	pad := cell.RGB(0, 0, 0)
	letterbox := false

	switch strings.ToLower(fit) {
	case "stretch":
		scaleX, scaleY = sx, sy
	case "contain":
		// Zoom out so full image fits (letterbox).
		scale := sx
		if sy > scale {
			scale = sy
		}
		scaleX, scaleY = scale, scale
		originX = (float64(srcW) - dstW*scaleX) / 2
		originY = (float64(srcH) - dstH*scaleY) / 2
		letterbox = true
	default: // cover — zoom in so dest is filled (crop)
		scale := sx
		if sy < scale {
			scale = sy
		}
		scaleX, scaleY = scale, scale
		originX = (float64(srcW) - dstW*scaleX) / 2
		originY = (float64(srcH) - dstH*scaleY) / 2
	}

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			px := offX + int(originX+float64(x)*scaleX)
			pyTop := offY + int(originY+float64(y*2)*scaleY)
			pyBot := offY + int(originY+float64(y*2+1)*scaleY)
			if letterbox {
				outside := px < b.Min.X || px >= b.Max.X || pyTop < b.Min.Y || pyBot >= b.Max.Y
				if outside {
					out[y*cols+x] = cell.Cell{Rune: ' ', FG: pad, BG: pad}
					continue
				}
			}
			top := sample(img, b, px, pyTop)
			bot := sample(img, b, px, pyBot)
			out[y*cols+x] = cell.Cell{
				Rune: halfBlock,
				FG:   bot,
				BG:   top,
			}
		}
	}
	return out
}

func sample(img image.Image, b image.Rectangle, x, y int) cell.Color {
	if x < b.Min.X {
		x = b.Min.X
	}
	if y < b.Min.Y {
		y = b.Min.Y
	}
	if x >= b.Max.X {
		x = b.Max.X - 1
	}
	if y >= b.Max.Y {
		y = b.Max.Y - 1
	}
	if x < b.Min.X || y < b.Min.Y {
		return cell.RGB(0, 0, 0)
	}
	r, g, b8, _ := img.At(x, y).RGBA()
	return cell.RGB(uint8(r>>8), uint8(g>>8), uint8(b8>>8))
}

// SolidImage creates a simple gradient test image.
func SolidImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 255 / max(1, w-1)),
				G: uint8(y * 255 / max(1, h-1)),
				B: 128,
				A: 255,
			})
		}
	}
	return img
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
