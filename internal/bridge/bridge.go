// Package bridge implements the GUI-TUI App Bridge's DisplayNest backend:
// arbitrary X11 GUI applications, nested in an off-screen Xvfb, captured
// and encoded into the same half-block cell representation every other
// graphical surface uses (see internal/gfx), with input remapped back in
// via XTest. See docs/gui-bridge.md.
package bridge

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/ttypedesk/ttypedesk/internal/gfx"
	"github.com/ttypedesk/ttypedesk/internal/slog"
	"github.com/ttypedesk/ttypedesk/internal/surface"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

// captureFPS matches the attach/remote snapshot cadence elsewhere in the
// desktop (internal/attach.Serve) — a reasonable frame budget for a nested
// GUI app without saturating a single core on repeated GetImage calls.
const captureFPS = 10

// xvfbCellW/H are the pixel size Xvfb is launched at, per cell. The exact
// values don't need to match any real font metrics — EncodeHalfBlockFit
// resamples to fit whatever cols/rows the window actually has, so this only
// sets how much source detail is available to resample from.
const (
	xvfbCellW = 9
	xvfbCellH = 18 // 2 half-block rows per cell row, so /2 = 9px per half
)

// BridgeSurface hosts one bridged GUI app. Implements surface.Surface.
type BridgeSurface struct {
	id      string
	command string

	display int
	xvfbCmd *exec.Cmd
	guest   *exec.Cmd
	conn    *xgb.Conn
	root    xproto.Window
	byteOrd byte
	inj     *inputInjector
	capW    int
	capH    int

	mu     sync.Mutex
	cols   int
	rows   int
	cells  []cell.Cell
	dirty  bool
	title  string
	closed bool

	stop chan struct{}
	done chan struct{}
}

// New spawns Xvfb, launches command against it, and starts capturing.
// cols/rows are the window's current content size.
func New(id, command string, cols, rows int) (*BridgeSurface, error) {
	if cols < 1 {
		cols = 40
	}
	if rows < 1 {
		rows = 12
	}
	display, err := pickDisplay()
	if err != nil {
		return nil, err
	}
	capW, capH := cols*xvfbCellW, rows*xvfbCellH
	xvfbCmd, err := startXvfb(display, capW, capH)
	if err != nil {
		return nil, err
	}

	conn, err := connectRetry(display, 5, 50*time.Millisecond)
	if err != nil {
		killWait(xvfbCmd)
		return nil, fmt.Errorf("connect to Xvfb: %w", err)
	}
	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	inj, err := newInputInjector(conn, screen.Root)
	if err != nil {
		conn.Close()
		killWait(xvfbCmd)
		return nil, err
	}

	guestCmd, err := startGuest(display, command)
	if err != nil {
		conn.Close()
		killWait(xvfbCmd)
		return nil, err
	}

	b := &BridgeSurface{
		id:      id,
		command: command,
		display: display,
		conn:    conn,
		root:    screen.Root,
		byteOrd: setup.ImageByteOrder,
		inj:     inj,
		capW:    capW,
		capH:    capH,
		cols:    cols,
		rows:    rows,
		cells:   make([]cell.Cell, cols*rows),
		dirty:   true,
		title:   command,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		xvfbCmd: xvfbCmd,
		guest:   guestCmd,
	}
	go b.captureLoop()
	slog.Info("bridge surface started id=%s display=:%d cmd=%q", id, display, command)
	return b, nil
}

func (b *BridgeSurface) captureLoop() {
	defer close(b.done)
	ticker := time.NewTicker(time.Second / captureFPS)
	defer ticker.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
			b.mu.Lock()
			w, h := b.capW, b.capH
			b.mu.Unlock()
			img, err := captureFrame(b.conn, b.root, w, h, b.byteOrd)
			if err != nil {
				slog.Warn("bridge id=%s capture: %v", b.id, err)
				continue
			}
			b.mu.Lock()
			cells := gfx.EncodeHalfBlockFit(img, b.cols, b.rows, "stretch", 0, 0)
			b.cells = cells
			b.dirty = true
			b.mu.Unlock()
		}
	}
}

func (b *BridgeSurface) ID() string   { return b.id }
func (b *BridgeSurface) Kind() string { return "bridge" }

func (b *BridgeSurface) Size() (cols, rows int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cols, b.rows
}

// Resize updates the cell grid the next capture resamples into. It
// deliberately does not touch Xvfb or the guest process — relaunching Xvfb
// on every window resize would kill the guest app's state for no real
// benefit, since EncodeHalfBlockFit already rescales the captured frame to
// whatever cols/rows the window has.
func (b *BridgeSurface) Resize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	b.mu.Lock()
	b.cols, b.rows = cols, rows
	b.dirty = true
	b.mu.Unlock()
}

func (b *BridgeSurface) Title() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.title
}

func (b *BridgeSurface) MouseMode() int { return 1 }

func (b *BridgeSurface) HandleInput(ev surface.InputEvent) error {
	b.mu.Lock()
	closed, cols, rows := b.closed, b.cols, b.rows
	b.mu.Unlock()
	if closed {
		return nil
	}
	switch ev.Kind {
	case "key":
		ks, ok := namedKeysyms[ev.Key]
		if !ok {
			ks, ok = runeKeysym(ev.Rune)
		}
		if !ok {
			return nil
		}
		return b.inj.sendKey(ks, ev.Ctrl, ev.Alt, ev.Shift)
	case "mouse":
		x, y := cellToPixel(ev.X, ev.Y, cols, rows, b.capW, b.capH)
		b.inj.moveMouse(x, y)
		switch ev.Action {
		case "press":
			b.inj.button(true, xtestButton(ev.Button))
		case "release":
			b.inj.button(false, xtestButton(ev.Button))
		}
		return nil
	case "scroll":
		x, y := cellToPixel(ev.X, ev.Y, cols, rows, b.capW, b.capH)
		b.inj.moveMouse(x, y)
		btn := byte(4) // wheel up
		if ev.Button < 0 {
			btn = 5 // wheel down
		}
		b.inj.button(true, btn)
		b.inj.button(false, btn)
		return nil
	}
	return nil
}

// cellToPixel maps a cell coordinate to the pixel center of that cell in
// the capture surface, proportionally — robust to xvfbCellW/H changing
// without needing to keep this in sync.
func cellToPixel(col, row, cols, rows, capW, capH int) (int16, int16) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	x := (float64(col) + 0.5) / float64(cols) * float64(capW)
	y := (float64(row) + 0.5) / float64(rows) * float64(capH)
	return int16(x), int16(y)
}

func xtestButton(b int) byte {
	if b < 1 {
		return 1
	}
	return byte(b)
}

func (b *BridgeSurface) ProduceDiff() cell.Diff {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.dirty {
		return cell.Diff{}
	}
	b.dirty = false
	return cell.FullGridDiff(b.cols, b.rows, b.cells)
}

func (b *BridgeSurface) Snapshot() []cell.Cell {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]cell.Cell, len(b.cells))
	copy(out, b.cells)
	return out
}

func (b *BridgeSurface) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	close(b.stop)
	<-b.done

	if b.inj != nil {
		b.inj.close()
	}
	killWait(b.guest)
	if b.conn != nil {
		b.conn.Close()
	}
	killWait(b.xvfbCmd)
	slog.Info("bridge surface closed id=%s", b.id)
	return nil
}

func killWait(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}
