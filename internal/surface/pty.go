package surface

import (
	"sync"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/slog"
	"github.com/ttypedesk/ttypedesk/internal/tty"
	"github.com/ttypedesk/ttypedesk/internal/vterm"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

// PtySurface hosts a shell/TUI in a PTY + libvterm.
type PtySurface struct {
	id           string
	defaultTitle string
	term         *vterm.Term
	pty          *tty.Handle
	mu           sync.Mutex
	closed       bool
	done         chan struct{}
	readDone     chan struct{}
}

// PtyOpts configures PTY spawn.
type PtyOpts struct {
	Command    string
	Args       []string
	Cols       int
	Rows       int
	Scrollback int
	Title      string // fallback window title when guest OSC title is empty
}

func NewPtySurface(id, shell string, cols, rows, scrollback int) (*PtySurface, error) {
	return NewPtySurfaceOpts(id, PtyOpts{Command: shell, Cols: cols, Rows: rows, Scrollback: scrollback})
}

func NewPtySurfaceOpts(id string, opts PtyOpts) (*PtySurface, error) {
	if opts.Cols < 1 {
		opts.Cols = 80
	}
	if opts.Rows < 1 {
		opts.Rows = 24
	}
	if opts.Command == "" {
		opts.Command = "/bin/bash"
	}
	defTitle := opts.Title
	if defTitle == "" {
		defTitle = "Terminal"
	}
	term := vterm.New(opts.Rows, opts.Cols, opts.Scrollback)
	h, err := tty.Spawn(opts.Command, opts.Args, opts.Cols, opts.Rows)
	if err != nil {
		term.Close()
		return nil, err
	}
	s := &PtySurface{
		id:           id,
		defaultTitle: defTitle,
		term:         term,
		pty:          h,
		done:         make(chan struct{}),
		readDone:     make(chan struct{}),
	}
	go s.readLoop()
	return s, nil
}

func (s *PtySurface) readLoop() {
	defer close(s.readDone)
	buf := make([]byte, 8192)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.mu.Lock()
			closed := s.closed
			term := s.term
			s.mu.Unlock()
			if !closed && term != nil {
				term.Write(buf[:n])
			}
		}
		if err != nil {
			return
		}
		select {
		case <-s.done:
			return
		default:
		}
	}
}

func (s *PtySurface) ID() string   { return s.id }
func (s *PtySurface) Kind() string { return "pty" }

func (s *PtySurface) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.term == nil {
		return 80, 24
	}
	return s.term.Size()
}

func (s *PtySurface) Resize(cols, rows int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in PtySurface.Resize id=%s: %v", s.id, r)
		}
	}()
	s.mu.Lock()
	term := s.term
	closed := s.closed
	pty := s.pty
	s.mu.Unlock()
	if closed || term == nil {
		return
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	term.Resize(rows, cols)
	if pty != nil {
		_ = pty.SetSize(cols, rows)
	}
}

func (s *PtySurface) Title() string {
	s.mu.Lock()
	term := s.term
	def := s.defaultTitle
	s.mu.Unlock()
	if def == "" {
		def = "Terminal"
	}
	if term == nil {
		return def
	}
	t := term.Title()
	if t == "" {
		t = def
	}
	if term.ScrollOffset() > 0 {
		return t + " [scroll]"
	}
	return t
}

// TakeBell reports whether the guest rang the terminal bell since last check.
func (s *PtySurface) TakeBell() bool {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return false
	}
	return term.TakeBell()
}

func (s *PtySurface) HasScrollback() bool {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return false
	}
	return term.HasScrollback()
}

func (s *PtySurface) ScrollUIState() (offset, content, viewport int) {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return 0, 1, 1
	}
	return term.ScrollUIState()
}

func (s *PtySurface) SetScrollUIOffset(offset int) {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return
	}
	term.SetScrollUIOffset(offset)
}

func (s *PtySurface) SearchScrollback(query string, towardOlder bool) (found bool, matches int) {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return false, 0
	}
	return term.SearchScrollback(query, towardOlder)
}

func (s *PtySurface) ProduceDiff() cell.Diff {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return cell.Diff{}
	}
	return term.ProduceDiff()
}

func (s *PtySurface) Snapshot() []cell.Cell {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return nil
	}
	return term.Snapshot()
}

func (s *PtySurface) Cursor() (x, y int, visible bool) {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return 0, 0, false
	}
	return term.Cursor()
}

func (s *PtySurface) MouseMode() int {
	s.mu.Lock()
	term := s.term
	s.mu.Unlock()
	if term == nil {
		return 0
	}
	return term.MouseMode()
}

func (s *PtySurface) HandleInput(ev InputEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.term == nil {
		return nil
	}
	if len(ev.Bytes) > 0 {
		_, err := s.pty.Write(ev.Bytes)
		return err
	}

	// Scrollback (Shift+wheel / PgUp/PgDn when not alt-screen and mouse mode off for keys)
	if ev.Kind == "scroll" {
		if !s.term.AltScreen() {
			s.term.ScrollBy(ev.Button) // Button carries delta lines
			return nil
		}
		// In alt-screen, forward as mouse wheel if mouse mode on
		if s.term.MouseMode() > 0 {
			btn := 4 // wheel up in libvterm is button 4?
			if ev.Button < 0 {
				btn = 5
			}
			out := s.term.MouseButton(btn, true, ev.Y, ev.X, 0)
			if len(out) > 0 {
				_, _ = s.pty.Write(out)
			}
			out = s.term.MouseButton(btn, false, ev.Y, ev.X, 0)
			if len(out) > 0 {
				_, err := s.pty.Write(out)
				return err
			}
		}
		return nil
	}

	if ev.Kind == "mouse" {
		if s.term.MouseMode() == 0 {
			return nil
		}
		mod := 0
		if ev.Shift {
			mod |= vterm.ModShift
		}
		if ev.Alt {
			mod |= vterm.ModAlt
		}
		if ev.Ctrl {
			mod |= vterm.ModCtrl
		}
		btn := ev.Button
		if btn < 1 {
			btn = 1
		}
		// libvterm buttons are 1-based typically for left=1
		pressed := ev.Action == "press" || ev.Action == "drag"
		var out []byte
		switch ev.Action {
		case "move":
			out = s.term.MouseMove(ev.Y, ev.X, mod)
		case "press", "drag":
			out = s.term.MouseButton(btn, true, ev.Y, ev.X, mod)
		case "release":
			out = s.term.MouseButton(btn, false, ev.Y, ev.X, mod)
		}
		if len(out) > 0 {
			_, err := s.pty.Write(out)
			return err
		}
		_ = pressed
		return nil
	}

	mod := 0
	if ev.Shift {
		mod |= vterm.ModShift
	}
	if ev.Alt {
		mod |= vterm.ModAlt
	}
	if ev.Ctrl {
		mod |= vterm.ModCtrl
	}

	// Shift+PgUp/PgDn → scrollback when not alt screen
	if !s.term.AltScreen() && s.term.MouseMode() == 0 {
		if ev.Key == "PgUp" && ev.Shift {
			s.term.ScrollBy(s.pageLines())
			return nil
		}
		if ev.Key == "PgDn" && ev.Shift {
			s.term.ScrollBy(-s.pageLines())
			return nil
		}
	}

	var out []byte
	switch ev.Key {
	case "Enter":
		out = s.term.KeyboardKey(vterm.KeyEnter, mod)
	case "Tab":
		out = s.term.KeyboardKey(vterm.KeyTab, mod)
	case "Backspace":
		out = s.term.KeyboardKey(vterm.KeyBackspace, mod)
	case "Escape":
		out = s.term.KeyboardKey(vterm.KeyEscape, mod)
	case "Up":
		out = s.term.KeyboardKey(vterm.KeyUp, mod)
	case "Down":
		out = s.term.KeyboardKey(vterm.KeyDown, mod)
	case "Left":
		out = s.term.KeyboardKey(vterm.KeyLeft, mod)
	case "Right":
		out = s.term.KeyboardKey(vterm.KeyRight, mod)
	case "Home":
		out = s.term.KeyboardKey(vterm.KeyHome, mod)
	case "End":
		out = s.term.KeyboardKey(vterm.KeyEnd, mod)
	case "PgUp":
		out = s.term.KeyboardKey(vterm.KeyPageUp, mod)
	case "PgDn":
		out = s.term.KeyboardKey(vterm.KeyPageDown, mod)
	case "Delete":
		out = s.term.KeyboardKey(vterm.KeyDel, mod)
	case "Insert":
		out = s.term.KeyboardKey(vterm.KeyIns, mod)
	default:
		if ev.Rune != 0 {
			if ev.Ctrl && ev.Rune >= 'a' && ev.Rune <= 'z' {
				_, err := s.pty.Write([]byte{byte(ev.Rune - 'a' + 1)})
				return err
			}
			out = s.term.KeyboardUnichar(ev.Rune, mod)
		}
	}
	if len(out) > 0 {
		_, err := s.pty.Write(out)
		return err
	}
	return nil
}

func (s *PtySurface) pageLines() int {
	_, rows := s.term.Size()
	if rows < 2 {
		return 1
	}
	return rows / 2
}

func (s *PtySurface) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	select {
	case <-s.done:
	default:
		close(s.done)
	}
	// Unblock reader first; never free vterm while readLoop may Write.
	_ = s.pty.Close()
	select {
	case <-s.readDone:
	case <-time.After(300 * time.Millisecond):
	}
	if s.term != nil {
		s.term.Close()
		s.term = nil
	}
	return nil
}
