// Package bridge implements the GUI-TUI App Bridge's DisplayNest backend:
// arbitrary X11 GUI applications, nested in an off-screen Xvfb, captured
// and encoded into the same half-block cell representation every other
// graphical surface uses (see internal/gfx), with input remapped back in
// via XTest. See docs/gui-bridge.md.
package bridge

import (
	"fmt"
	"math"
	"os/exec"
	"strings"
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

// atspiTickEvery is how many raster capture ticks pass between AT-SPI text
// re-walks — text layout changes far less often than pixels, and each walk
// costs a handful of D-Bus round-trips per node, so re-walking at the full
// captureFPS would be wasteful. At captureFPS=10 this is a ~3.3fps text
// refresh, plenty for anything that isn't scrolling text mid-keystroke.
const atspiTickEvery = 3

// atspiMaxDepth/atspiMaxNodes bound one walkText call so a pathologically
// large accessible tree can't stall the capture loop.
const (
	atspiMaxDepth = 40
	atspiMaxNodes = 2000
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

	// AT-SPI text overlay — optional/best-effort. a11y is nil (raster-only,
	// no error) whenever dbus-daemon/at-spi2-registryd aren't available or
	// setup otherwise fails; see docs/gui-bridge.md for which apps this
	// actually helps (native GTK/Qt) versus doesn't (Electron apps don't
	// expose real text over AT-SPI — confirmed empirically, not assumed).
	busCmd       *exec.Cmd
	registrydCmd *exec.Cmd
	a11y         *atspiClient

	mu        sync.Mutex
	cols      int
	rows      int
	cells     []cell.Cell
	textNodes []textNode // latest AT-SPI walk result; nil if a11y is nil
	dirty     bool
	title     string
	closed    bool

	stop chan struct{}
	wg   sync.WaitGroup
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

	// AT-SPI text overlay setup — best-effort. Must happen before the guest
	// launches so Chromium/Electron-style toolkits that only build a tree
	// when an AT client is already listening have a chance to see one
	// (doesn't help Cursor specifically, see docs/gui-bridge.md, but costs
	// nothing to still try for whatever guest command is actually passed).
	busCmd, registrydCmd, a11y, busAddr := setupATSPI(id)

	guestCmd, err := startGuest(display, command, busAddr)
	if err != nil {
		if a11y != nil {
			a11y.close()
		}
		killWait(registrydCmd)
		killWait(busCmd)
		conn.Close()
		killWait(xvfbCmd)
		return nil, err
	}

	b := &BridgeSurface{
		id:           id,
		command:      command,
		display:      display,
		conn:         conn,
		root:         screen.Root,
		byteOrd:      setup.ImageByteOrder,
		inj:          inj,
		capW:         capW,
		capH:         capH,
		busCmd:       busCmd,
		registrydCmd: registrydCmd,
		a11y:         a11y,
		cols:         cols,
		rows:         rows,
		cells:        make([]cell.Cell, cols*rows),
		dirty:        true,
		title:        command,
		stop:         make(chan struct{}),
		xvfbCmd:      xvfbCmd,
		guest:        guestCmd,
	}
	b.wg.Add(1)
	go b.captureLoop()
	if a11y != nil {
		b.wg.Add(1)
		go b.atspiLoop()
	}
	slog.Info("bridge surface started id=%s display=:%d cmd=%q a11y=%v", id, display, command, a11y != nil)
	return b, nil
}

func (b *BridgeSurface) captureLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(time.Second / captureFPS)
	defer ticker.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
			b.mu.Lock()
			w, h, cols, rows := b.capW, b.capH, b.cols, b.rows
			b.mu.Unlock()
			img, err := captureFrame(b.conn, b.root, w, h, b.byteOrd)
			if err != nil {
				slog.Warn("bridge id=%s capture: %v", b.id, err)
				continue
			}
			cells := gfx.EncodeHalfBlockFit(img, cols, rows, "stretch", 0, 0)
			b.mu.Lock()
			// Composite whatever text AT-SPI last found on top of the fresh
			// raster — atspiLoop refreshes b.textNodes on its own, slower
			// cadence, independently, so this never blocks on a D-Bus walk.
			if len(b.textNodes) > 0 {
				compositeText(cells, cols, rows, b.textNodes, w, h)
			}
			b.cells = cells
			b.dirty = true
			b.mu.Unlock()
		}
	}
}

// atspiLoop periodically re-walks the guest's accessible tree and stashes
// the result for captureLoop to composite — on its own goroutine/ticker so
// a slow walk (D-Bus round-trip per node) never stalls raster capture.
func (b *BridgeSurface) atspiLoop() {
	defer b.wg.Done()
	period := (time.Second / captureFPS) * atspiTickEvery
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
			nodes := b.a11y.walkText(atspiMaxDepth, atspiMaxNodes)
			b.mu.Lock()
			b.textNodes = nodes
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

// compositeText overwrites cells covered by each text node's bounding box
// with real characters, in place — the inverse of cellToPixel's math,
// mapping a pixel rect back to a cell rect. Half-block cells all use the
// same glyph, so this is the only place actual runes other than '▄' ever
// appear in a bridge window; it keeps the raster background color (so the
// overlay doesn't fight the app's own theme) and picks a contrasting
// foreground by luminance.
func compositeText(cells []cell.Cell, cols, rows int, nodes []textNode, capW, capH int) {
	if cols < 1 || rows < 1 || capW < 1 || capH < 1 {
		return
	}
	for _, n := range nodes {
		x0 := clampInt(int(float64(n.X)/float64(capW)*float64(cols)), 0, cols)
		y0 := clampInt(int(float64(n.Y)/float64(capH)*float64(rows)), 0, rows)
		x1 := clampInt(int(math.Ceil(float64(int(n.X)+int(n.W))/float64(capW)*float64(cols))), 0, cols)
		y1 := clampInt(int(math.Ceil(float64(int(n.Y)+int(n.H))/float64(capH)*float64(rows))), 0, rows)
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		runes := []rune(strings.Join(strings.Fields(n.Text), " "))
		i := 0
		for row := y0; row < y1 && i < len(runes); row++ {
			for col := x0; col < x1 && i < len(runes); col++ {
				idx := row*cols + col
				if idx >= 0 && idx < len(cells) {
					bg := cells[idx].BG
					cells[idx] = cell.Cell{Rune: runes[i], FG: contrastColor(bg), BG: bg}
				}
				i++
			}
		}
	}
}

// contrastColor picks black or white by background luminance, so overlaid
// text stays legible regardless of the guest app's own color scheme.
func contrastColor(bg cell.Color) cell.Color {
	lum := 0.299*float64(bg.R) + 0.587*float64(bg.G) + 0.114*float64(bg.B)
	if lum > 128 {
		return cell.RGB(0, 0, 0)
	}
	return cell.RGB(0xFF, 0xFF, 0xFF)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
	b.wg.Wait()

	if b.inj != nil {
		b.inj.close()
	}
	killWait(b.guest)
	if b.conn != nil {
		b.conn.Close()
	}
	killWait(b.xvfbCmd)
	if b.a11y != nil {
		b.a11y.close()
	}
	killWait(b.registrydCmd)
	killWait(b.busCmd)
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
