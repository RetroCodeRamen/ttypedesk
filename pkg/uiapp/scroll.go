package uiapp

import "github.com/ttypedesk/ttypedesk/pkg/cell"

// ScrollState tracks a 1D scrollable viewport (row units for vertical bars).
type ScrollState struct {
	Offset   int // first visible content unit
	Content  int // total content units
	Viewport int // visible units
}

func (s *ScrollState) MaxOffset() int {
	m := s.Content - s.Viewport
	if m < 0 {
		return 0
	}
	return m
}

func (s *ScrollState) Clamp() {
	if s.Offset < 0 {
		s.Offset = 0
	}
	if max := s.MaxOffset(); s.Offset > max {
		s.Offset = max
	}
}

func (s *ScrollState) ScrollBy(delta int) {
	s.Offset += delta
	s.Clamp()
}

// EnsureVisible scrolls so index is inside the viewport.
func (s *ScrollState) EnsureVisible(index int) {
	if s.Viewport <= 0 {
		return
	}
	if index < s.Offset {
		s.Offset = index
	} else if index >= s.Offset+s.Viewport {
		s.Offset = index - s.Viewport + 1
	}
	s.Clamp()
}

// ScrollHit is the result of hitting a drawn scrollbar.
type ScrollHit int

const (
	ScrollHitNone ScrollHit = iota
	ScrollHitThumb
	ScrollHitTrackAbove
	ScrollHitTrackBelow
	ScrollHitArrowUp
	ScrollHitArrowDown
)

// ScrollbarStyle controls DrawScrollbar colors.
type ScrollbarStyle struct {
	TrackFG, TrackBG cell.Color
	ThumbFG, ThumbBG cell.Color
	ArrowFG, ArrowBG cell.Color
	ShowArrows       bool
}

// DefaultScrollbarStyle is a DOS-ish gray track with dark thumb.
func DefaultScrollbarStyle() ScrollbarStyle {
	return ScrollbarStyle{
		TrackFG:    cell.RGB(0x80, 0x80, 0x80),
		TrackBG:    cell.RGB(0xA0, 0xA0, 0xA0),
		ThumbFG:    cell.RGB(0x00, 0x00, 0x00),
		ThumbBG:    cell.RGB(0xC0, 0xC0, 0xC0),
		ArrowFG:    cell.RGB(0x00, 0x00, 0x00),
		ArrowBG:    cell.RGB(0xC0, 0xC0, 0xC0),
		ShowArrows: true,
	}
}

// ThumbGeom returns thumb start/height within the track (excluding arrows).
func (s ScrollState) ThumbGeom(trackH int) (thumbY, thumbH int) {
	if trackH < 1 {
		return 0, 0
	}
	if s.Content <= s.Viewport || s.Content <= 0 {
		return 0, trackH
	}
	thumbH = (s.Viewport * trackH) / s.Content
	if thumbH < 1 {
		thumbH = 1
	}
	if thumbH > trackH {
		thumbH = trackH
	}
	maxOff := s.MaxOffset()
	if maxOff <= 0 {
		return 0, thumbH
	}
	travel := trackH - thumbH
	thumbY = (s.Offset * travel) / maxOff
	if thumbY+thumbH > trackH {
		thumbY = trackH - thumbH
	}
	return thumbY, thumbH
}

// DrawScrollbar paints a vertical scrollbar at (x,y) of height h.
func (c *Canvas) DrawScrollbar(x, y, h int, state ScrollState, style ScrollbarStyle) {
	if h < 1 {
		return
	}
	trackY, trackH := y, h
	if style.ShowArrows && h >= 3 {
		c.put(x, y, '▲', style.ArrowFG, style.ArrowBG)
		c.put(x, y+h-1, '▼', style.ArrowFG, style.ArrowBG)
		trackY = y + 1
		trackH = h - 2
	}
	for row := 0; row < trackH; row++ {
		c.put(x, trackY+row, '░', style.TrackFG, style.TrackBG)
	}
	ty, th := state.ThumbGeom(trackH)
	for row := 0; row < th; row++ {
		c.put(x, trackY+ty+row, '█', style.ThumbFG, style.ThumbBG)
	}
}

// HitScrollbar hit-tests a vertical bar drawn with DrawScrollbar.
func HitScrollbar(mx, my, x, y, h int, state ScrollState, showArrows bool) ScrollHit {
	if mx != x || my < y || my >= y+h {
		return ScrollHitNone
	}
	trackY, trackH := y, h
	if showArrows && h >= 3 {
		if my == y {
			return ScrollHitArrowUp
		}
		if my == y+h-1 {
			return ScrollHitArrowDown
		}
		trackY = y + 1
		trackH = h - 2
	}
	if trackH < 1 {
		return ScrollHitNone
	}
	rel := my - trackY
	ty, th := state.ThumbGeom(trackH)
	if rel >= ty && rel < ty+th {
		return ScrollHitThumb
	}
	if rel < ty {
		return ScrollHitTrackAbove
	}
	return ScrollHitTrackBelow
}

// ApplyScrollHit updates state for a click (not drag). Page = Viewport.
func (s *ScrollState) ApplyScrollHit(hit ScrollHit) {
	switch hit {
	case ScrollHitArrowUp:
		s.ScrollBy(-1)
	case ScrollHitArrowDown:
		s.ScrollBy(1)
	case ScrollHitTrackAbove:
		s.ScrollBy(-s.Viewport)
	case ScrollHitTrackBelow:
		s.ScrollBy(s.Viewport)
	}
}
