// Package uiapp is the in-process App SDK for TTYPE Desk.
//
// Native apps implement App and receive a Context with Host services
// (notifications, launch, title, close) plus optional lifecycle Hooks.
// Custom shell programs registered via Add Program run in PTY windows and
// do not use this SDK — the SDK is for first-party / rich UI apps that need
// more than a terminal.
package uiapp

import (
	"os"
	"sync"
	"time"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uwidth"
)

// App is a first-class windowed application (no PTY).
type App interface {
	Init(ctx *Context) error
	Handle(Event) error
	Draw(*Canvas) error
	Close() error
}

// HookProvider is an optional interface: apps may return lifecycle hooks.
type HookProvider interface {
	Hooks() Hooks
}

// Hooks are optional lifecycle callbacks (beyond Handle events).
type Hooks struct {
	OnInit   func(ctx *Context)
	OnClose  func(ctx *Context)
	OnFocus  func(ctx *Context, focused bool)
	OnResize func(ctx *Context, cols, rows int)
	OnTimer  func(ctx *Context)
	OnDraw   func(ctx *Context, cv *Canvas) // after Draw, for overlays
}

// Host is the desktop shell API exposed to native apps.
type Host interface {
	Notify(title, body, icon string)
	Launch(action string) error
	OpenPath(path string) error
	SetTitle(title string)
	RequestClose()
	WindowID() string

	// SaveCredential/LoadCredential persist a secret (OAuth2 tokens, a
	// session token, …) under ~/.config/ttypedesk/credentials/ — never in
	// config.json or git. LoadCredential's error wraps os.ErrNotExist when
	// nothing has been saved under key yet.
	SaveCredential(key string, value []byte) error
	LoadCredential(key string) ([]byte, error)

	// PickFile opens a small modal file browser rooted at startDir,
	// restricted to the given extensions if any are given (without the
	// leading dot; empty means no filter). onResult fires exactly once,
	// asynchronously, with ok=false if the user cancels.
	PickFile(startDir string, extensions []string, onResult func(path string, ok bool))

	// PlayAudio streams pcm (interleaved int16 samples, at the fixed
	// internal/audio.SampleRate/Channels — decode to that rate/channel
	// count, there is no per-call resampling) to the shared audio output.
	// The returned stop func halts playback and releases the player.
	PlayAudio(pcm <-chan []int16) (stop func(), err error)
}

// Event kinds.
const (
	EventKey    = "key"
	EventMouse  = "mouse"
	EventResize = "resize"
	EventFocus  = "focus"
	EventBlur   = "blur"
	EventTimer  = "timer"
	EventHook   = "hook" // Data carries hook name for generic listeners
)

// Event delivered to apps.
type Event struct {
	Kind    string
	Rune    rune
	Key     string
	Ctrl    bool
	Alt     bool
	Shift   bool
	X, Y    int
	Button  int
	Action  string
	Cols    int
	Rows    int
	Focused bool
	Name    string // for EventHook
}

// Context provides window services to an app.
type Context struct {
	WindowID string
	Cols     int
	Rows     int

	mu      sync.Mutex
	timers  []timer
	onDirty func()
	stopCh  chan struct{}
	pending []Event
	host    Host
	hooks   Hooks
}

type timer struct {
	every time.Duration
	next  time.Time
	id    int
}

func NewContext(windowID string, cols, rows int, onDirty func()) *Context {
	return &Context{
		WindowID: windowID,
		Cols:     cols,
		Rows:     rows,
		onDirty:  onDirty,
		stopCh:   make(chan struct{}),
	}
}

// SetHost attaches shell services (notify, launch, …).
func (c *Context) SetHost(h Host) {
	c.mu.Lock()
	c.host = h
	c.mu.Unlock()
}

// SetHooks registers lifecycle hooks (usually from HookProvider).
func (c *Context) SetHooks(h Hooks) {
	c.mu.Lock()
	c.hooks = h
	c.mu.Unlock()
}

func (c *Context) Host() Host {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host
}

func (c *Context) Hooks() Hooks {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hooks
}

// Notify posts a system notification (no-op if host unset).
func (c *Context) Notify(title, body, icon string) {
	if h := c.Host(); h != nil {
		h.Notify(title, body, icon)
	}
}

// OpenPath opens a file/directory with the desk default-app associations.
func (c *Context) OpenPath(path string) error {
	if h := c.Host(); h != nil {
		return h.OpenPath(path)
	}
	return nil
}

// Launch runs a desktop action (e.g. "terminal", "prog:id", "pty:htop").
func (c *Context) Launch(action string) error {
	if h := c.Host(); h != nil {
		return h.Launch(action)
	}
	return nil
}

// SetTitle updates the window title bar.
func (c *Context) SetTitle(title string) {
	if h := c.Host(); h != nil {
		h.SetTitle(title)
	}
}

// RequestClose asks the shell to close this window.
func (c *Context) RequestClose() {
	if h := c.Host(); h != nil {
		h.RequestClose()
	}
}

// SaveCredential persists a secret under key (no-op if host unset).
func (c *Context) SaveCredential(key string, value []byte) error {
	if h := c.Host(); h != nil {
		return h.SaveCredential(key, value)
	}
	return nil
}

// LoadCredential reads back a secret saved under key.
func (c *Context) LoadCredential(key string) ([]byte, error) {
	if h := c.Host(); h != nil {
		return h.LoadCredential(key)
	}
	return nil, os.ErrNotExist
}

// PickFile opens a modal file browser; see Host.PickFile.
func (c *Context) PickFile(startDir string, extensions []string, onResult func(path string, ok bool)) {
	if h := c.Host(); h != nil {
		h.PickFile(startDir, extensions, onResult)
		return
	}
	if onResult != nil {
		onResult("", false)
	}
}

// PlayAudio streams pcm to the shared audio output; see Host.PlayAudio.
func (c *Context) PlayAudio(pcm <-chan []int16) (stop func(), err error) {
	if h := c.Host(); h != nil {
		return h.PlayAudio(pcm)
	}
	return func() {}, nil
}

func (c *Context) SetSize(cols, rows int) {
	c.mu.Lock()
	c.Cols, c.Rows = cols, rows
	c.mu.Unlock()
}

// Size returns the current canvas size (thread-safe).
func (c *Context) Size() (cols, rows int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Cols, c.Rows
}

// MarkDirty requests a redraw on the next frame.
func (c *Context) MarkDirty() {
	c.mu.Lock()
	fn := c.onDirty
	c.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// StartTimer requests periodic EventTimer events.
func (c *Context) StartTimer(every time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := len(c.timers) + 1
	c.timers = append(c.timers, timer{every: every, next: time.Now().Add(every), id: id})
}

func (c *Context) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

// DrainTimers returns due timer events and marks dirty.
func (c *Context) DrainTimers() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	var out []Event
	for i := range c.timers {
		t := &c.timers[i]
		if !now.Before(t.next) {
			out = append(out, Event{Kind: EventTimer})
			t.next = now.Add(t.every)
			if c.onDirty != nil {
				c.onDirty()
			}
		}
	}
	return out
}

// RunHook invokes a named lifecycle hook if set.
func (c *Context) RunHook(name string, cols, rows int, focused bool) {
	h := c.Hooks()
	switch name {
	case "init":
		if h.OnInit != nil {
			h.OnInit(c)
		}
	case "close":
		if h.OnClose != nil {
			h.OnClose(c)
		}
	case "focus":
		if h.OnFocus != nil {
			h.OnFocus(c, focused)
		}
	case "resize":
		if h.OnResize != nil {
			h.OnResize(c, cols, rows)
		}
	case "timer":
		if h.OnTimer != nil {
			h.OnTimer(c)
		}
	}
}

// Canvas is a retained-mode cell grid.
type Canvas struct {
	cols, rows int
	cells      []cell.Cell
	fg, bg     cell.Color
}

func NewCanvas(cols, rows int) *Canvas {
	c := &Canvas{
		fg: cell.RGB(0x00, 0x00, 0x00),
		bg: cell.RGB(0xC0, 0xC0, 0xC0),
	}
	c.Resize(cols, rows)
	return c
}

func (c *Canvas) Resize(cols, rows int) {
	c.cols, c.rows = cols, rows
	c.cells = make([]cell.Cell, cols*rows)
	for i := range c.cells {
		c.cells[i] = cell.Cell{Rune: ' ', FG: c.fg, BG: c.bg}
	}
}

func (c *Canvas) Bounds() (cols, rows int) { return c.cols, c.rows }

func (c *Canvas) SetPalette(fg, bg cell.Color) {
	c.fg, c.bg = fg, bg
}

func (c *Canvas) Cells() []cell.Cell {
	out := make([]cell.Cell, len(c.cells))
	copy(out, c.cells)
	return out
}

func (c *Canvas) FillRect(x, y, w, h int, bg cell.Color) {
	for row := y; row < y+h && row < c.rows; row++ {
		if row < 0 {
			continue
		}
		for col := x; col < x+w && col < c.cols; col++ {
			if col < 0 {
				continue
			}
			c.cells[row*c.cols+col] = cell.Cell{Rune: ' ', FG: c.fg, BG: bg}
		}
	}
}

func (c *Canvas) DrawText(x, y int, text string, fg, bg cell.Color, attr cell.Attr) {
	col := x
	for _, r := range text {
		w := uwidth.Rune(r)
		if col < 0 || y < 0 || y >= c.rows {
			col += w
			continue
		}
		if col < c.cols {
			c.cells[y*c.cols+col] = cell.Cell{Rune: r, FG: fg, BG: bg, Attr: attr}
		}
		for i := 1; i < w && col+i < c.cols; i++ {
			c.cells[y*c.cols+col+i] = cell.Cell{Rune: ' ', FG: fg, BG: bg, Attr: attr}
		}
		col += w
	}
}

// DrawIcon draws an optional icon glyph then a label (width-aware).
func (c *Canvas) DrawIcon(x, y int, icon, label string, fg, bg cell.Color, attr cell.Attr) {
	if icon != "" {
		c.DrawText(x, y, icon+" ", fg, bg, attr)
		x += uwidth.String(icon) + 1
	}
	c.DrawText(x, y, label, fg, bg, attr)
}

func (c *Canvas) DrawBox(x, y, w, h int, fg, bg cell.Color) {
	if w < 2 || h < 2 {
		return
	}
	c.put(x, y, '╔', fg, bg)
	c.put(x+w-1, y, '╗', fg, bg)
	c.put(x, y+h-1, '╚', fg, bg)
	c.put(x+w-1, y+h-1, '╝', fg, bg)
	for col := x + 1; col < x+w-1; col++ {
		c.put(col, y, '═', fg, bg)
		c.put(col, y+h-1, '═', fg, bg)
	}
	for row := y + 1; row < y+h-1; row++ {
		c.put(x, row, '║', fg, bg)
		c.put(x+w-1, row, '║', fg, bg)
	}
}

// DrawButton draws a simple clickable-looking button.
func (c *Canvas) DrawButton(x, y, w int, label string, fg, bg cell.Color, active bool) {
	if w < 3 {
		w = 3
	}
	fill := bg
	if active {
		fill = cell.RGB(0x00, 0x00, 0xAA)
		fg = cell.RGB(0xFF, 0xFF, 0xFF)
	}
	c.FillRect(x, y, w, 1, fill)
	c.DrawText(x+1, y, uwidth.Truncate(label, w-2), fg, fill, 0)
}

func (c *Canvas) put(x, y int, r rune, fg, bg cell.Color) {
	if x < 0 || y < 0 || x >= c.cols || y >= c.rows {
		return
	}
	c.cells[y*c.cols+x] = cell.Cell{Rune: r, FG: fg, BG: bg}
}
