package surface

import (
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
)

// InputEvent is a normalized input event for surfaces.
type InputEvent struct {
	Kind   string // key, mouse
	Rune   rune
	Key    string
	Ctrl   bool
	Alt    bool
	Shift  bool
	Bytes  []byte
	X, Y   int
	Button int
	Action string
}

// Surface produces a cell grid (PTY, native app, or graphical).
type Surface interface {
	ID() string
	Kind() string
	Size() (cols, rows int)
	Resize(cols, rows int)
	HandleInput(InputEvent) error
	ProduceDiff() cell.Diff
	Snapshot() []cell.Cell
	Title() string
	Close() error
}

// CursorProvider is implemented by surfaces that expose a text cursor.
type CursorProvider interface {
	Cursor() (x, y int, visible bool)
}

// MouseModeProvider reports whether the guest wants mouse events.
type MouseModeProvider interface {
	MouseMode() int
}

// BellProvider reports terminal BEL / attention requests.
type BellProvider interface {
	TakeBell() bool
}

// ScrollbackProvider exposes scrollback for a window chrome scrollbar.
// Offset is UI-style: 0 = top of history, Max = live bottom.
type ScrollbackProvider interface {
	ScrollUIState() (offset, content, viewport int)
	SetScrollUIOffset(offset int)
	HasScrollback() bool
}

// ScrollbackSearchProvider searches scrollback and jumps the viewport.
type ScrollbackSearchProvider interface {
	SearchScrollback(query string, towardOlder bool) (found bool, matches int)
}
