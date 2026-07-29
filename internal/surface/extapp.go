package surface

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/proto"
	"github.com/ttypedesk/ttypedesk/internal/slog"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// ExtAppSurface hosts an out-of-process app: any executable speaking the
// NDJSON protocol documented in docs/extapp.md over its own stdin/stdout,
// rather than a Go uiapp.App called in-process. Lets apps be written in
// any language, at the cost of the process-spawn overhead and a stricter
// wire contract (every screen_diff must cover the full grid — see the
// doc). Crash isolation mirrors AppSurface: a child that exits
// unexpectedly gets a crash screen in its own window, same as an
// in-process app panicking, rather than taking anything else down.
type ExtAppSurface struct {
	id  string
	cmd *exec.Cmd

	writeMu sync.Mutex // guards stdin writes only, never taken together with mu
	stdinW  io.WriteCloser

	mu       sync.Mutex
	cols     int
	rows     int
	title    string
	host     uiapp.Host
	cells    []cell.Cell
	closed   bool
	crashed  bool
	crashMsg string
	dirty    atomic.Bool

	msgCh    chan proto.Envelope // decoded messages from the child, drained under mu in ProduceDiff
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewExtApp spawns command (with args) and speaks the out-of-process App
// protocol over its stdin/stdout. The process is running (but not yet
// initialized — see BindHost) when this returns without error.
func NewExtApp(id, command string, args []string, cols, rows int) (*ExtAppSurface, error) {
	if cols < 1 {
		cols = 40
	}
	if rows < 1 {
		rows = 12
	}
	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("extapp: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("extapp: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("extapp: start %s: %w", command, err)
	}

	s := &ExtAppSurface{
		id:     id,
		cmd:    cmd,
		stdinW: stdin,
		cols:   cols,
		rows:   rows,
		title:  command,
		msgCh:  make(chan proto.Envelope, 32),
		stopCh: make(chan struct{}),
	}
	s.dirty.Store(true)
	go s.readLoop(stdout)
	slog.Debug("extapp surface created id=%s command=%q %dx%d", id, command, cols, rows)
	return s, nil
}

// BindHost attaches shell services and sends the child its startup Init
// message. Unlike AppSurface.BindHost, this doesn't wait for the child's
// Ready reply — the child runs in its own process, and blocking window
// creation on an arbitrary external binary responding promptly would let
// one slow or hung app stall the whole desktop. The window just shows a
// blank canvas until the first real screen_diff arrives.
func (s *ExtAppSurface) BindHost(host uiapp.Host) error {
	s.mu.Lock()
	s.host = host
	cols, rows := s.cols, s.rows
	s.mu.Unlock()
	return s.writeEnvelope(proto.TypeInit, proto.InitPayload{WindowID: s.id, Cols: cols, Rows: rows})
}

func (s *ExtAppSurface) writeEnvelope(typ proto.MessageType, payload any) error {
	data, err := proto.Encode(typ, s.id, payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.stdinW.Write(data)
	return err
}

// readLoop decodes NDJSON envelopes from the child's stdout and hands
// them to msgCh for ProduceDiff to apply — never touching s's fields
// directly, exactly like every other background-goroutine result channel
// in this codebase (internal/ffdecode, internal/audiocap). Once stdout
// closes (the child exited), it reaps the process and marks the surface
// crashed unless Close already tore it down deliberately.
func (s *ExtAppSurface) readLoop(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // a full 200x60 grid is well under 1MB of JSON
	for sc.Scan() {
		env, err := proto.Decode(sc.Bytes())
		if err != nil {
			continue // one malformed line isn't worth crashing the window over
		}
		select {
		case s.msgCh <- env:
		case <-s.stopCh:
			return
		}
	}
	// Deliberately doesn't call cmd.Wait itself: Close is the sole place
	// that reaps the process (see its doc comment) — os/exec warns against
	// calling Wait while a read from a pipe it created might still be in
	// flight, and this goroutine has no way to know Close won't race it.
	s.mu.Lock()
	if !s.closed {
		s.markCrashedLocked("process exited unexpectedly")
	}
	s.mu.Unlock()
}

func (s *ExtAppSurface) markCrashedLocked(msg string) {
	if s.crashed {
		return
	}
	s.crashed = true
	s.crashMsg = msg
	s.dirty.Store(true)
	slog.Error("extapp crashed id=%s: %s (window isolated)", s.id, msg)
}

func (s *ExtAppSurface) ID() string   { return s.id }
func (s *ExtAppSurface) Kind() string { return "app" }

func (s *ExtAppSurface) Title() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.crashed {
		return s.title + " [crashed]"
	}
	return s.title
}

func (s *ExtAppSurface) MouseMode() int { return 1 }

func (s *ExtAppSurface) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

func (s *ExtAppSurface) Resize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	s.mu.Lock()
	s.cols, s.rows = cols, rows
	s.mu.Unlock()
	_ = s.writeEnvelope(proto.TypeResize, proto.ResizeEvent{Cols: cols, Rows: rows})
}

func (s *ExtAppSurface) HandleInput(ev InputEvent) error {
	switch ev.Kind {
	case "key":
		return s.writeEnvelope(proto.TypeKey, proto.KeyEvent{
			Rune: ev.Rune, Key: ev.Key, Ctrl: ev.Ctrl, Alt: ev.Alt, Shift: ev.Shift, Bytes: ev.Bytes,
		})
	case "mouse", "scroll":
		action := ev.Action
		if ev.Kind == "scroll" {
			action = "wheel"
		}
		return s.writeEnvelope(proto.TypeMouse, proto.MouseEvent{
			X: ev.X, Y: ev.Y, Button: ev.Button, Action: action,
			Ctrl: ev.Ctrl, Alt: ev.Alt, Shift: ev.Shift,
		})
	}
	return nil
}

// NotifyFocus mirrors AppSurface's optional NotifyFocus(bool), picked up
// by internal/server via the same type assertion.
func (s *ExtAppSurface) NotifyFocus(focused bool) {
	_ = s.writeEnvelope(proto.TypeFocus, proto.FocusEvent{Focused: focused})
}

// ProduceDiff drains messages queued by readLoop and applies them: a
// screen_diff becomes the surface's new cell grid (v1 requires every
// screen_diff to cover the full grid — see docs/extapp.md — so there's no
// partial-rect merge to do), title_changed both updates the local title
// and forwards to the real Host (so the taskbar/window chrome, which
// reads the window's own title field, updates too), and notify/launch/
// open_path/close_window forward straight to Host. RequestClose is
// dispatched in its own goroutine: Close() re-takes this same mu, so
// calling it synchronously here (while mu is already held by this very
// call) would deadlock — the other four Host calls don't re-enter this
// surface, so they're safe to call directly.
func (s *ExtAppSurface) ProduceDiff() cell.Diff {
	s.mu.Lock()
	defer s.mu.Unlock()
	host := s.host
drain:
	for {
		select {
		case env := <-s.msgCh:
			s.applyLocked(env, host)
		default:
			break drain
		}
	}
	if s.crashed {
		return s.drawCrashLocked()
	}
	if !s.dirty.Load() {
		return cell.Diff{}
	}
	s.dirty.Store(false)
	return cell.FullGridDiff(s.cols, s.rows, s.cells)
}

func (s *ExtAppSurface) applyLocked(env proto.Envelope, host uiapp.Host) {
	switch env.Type {
	case proto.TypeReady:
		p, _ := proto.DecodePayload[proto.ReadyPayload](env)
		if p.Err != "" {
			s.markCrashedLocked("Init: " + p.Err)
		}
	case proto.TypeScreenDiff:
		p, err := proto.DecodePayload[proto.ScreenDiffPayload](env)
		if err != nil {
			return
		}
		want := s.cols * s.rows
		if len(p.Diff.Cells) != want {
			slog.Warn("extapp id=%s sent a screen_diff with %d cells, want %d (%dx%d) — ignored",
				s.id, len(p.Diff.Cells), want, s.cols, s.rows)
			return
		}
		s.cells = p.Diff.Cells
		s.dirty.Store(true)
	case proto.TypeTitleChanged:
		p, err := proto.DecodePayload[proto.TitleChanged](env)
		if err != nil {
			return
		}
		s.title = p.Title
		if host != nil {
			host.SetTitle(p.Title)
		}
	case proto.TypeNotify:
		p, err := proto.DecodePayload[proto.NotifyPayload](env)
		if err != nil || host == nil {
			return
		}
		host.Notify(p.Title, p.Body, p.Icon)
	case proto.TypeLaunch:
		p, err := proto.DecodePayload[proto.LaunchPayload](env)
		if err != nil || host == nil {
			return
		}
		if err := host.Launch(p.Action); err != nil {
			slog.Warn("extapp id=%s launch %q failed: %v", s.id, p.Action, err)
		}
	case proto.TypeOpenPath:
		p, err := proto.DecodePayload[proto.OpenPathPayload](env)
		if err != nil || host == nil {
			return
		}
		if err := host.OpenPath(p.Path); err != nil {
			slog.Warn("extapp id=%s open_path %q failed: %v", s.id, p.Path, err)
		}
	case proto.TypeCloseWindow:
		if host != nil {
			go host.RequestClose()
		}
	}
}

func (s *ExtAppSurface) drawCrashLocked() cell.Diff {
	if !s.dirty.Load() {
		return cell.Diff{}
	}
	s.dirty.Store(false)
	bg := cell.RGB(0x80, 0x00, 0x00)
	fg := cell.RGB(0xFF, 0xFF, 0xFF)
	cells := make([]cell.Cell, s.cols*s.rows)
	for i := range cells {
		cells[i] = cell.Cell{Rune: ' ', FG: fg, BG: bg}
	}
	put := func(x, y int, text string) {
		for i, r := range text {
			cx := x + i
			if cx < 0 || cx >= s.cols || y < 0 || y >= s.rows {
				continue
			}
			cells[y*s.cols+cx] = cell.Cell{Rune: r, FG: fg, BG: bg}
		}
	}
	put(1, 1, "Application crashed")
	msg := s.crashMsg
	if msg == "" {
		msg = "(unknown)"
	}
	put(1, 3, truncate(msg, s.cols-2))
	put(1, 5, "Esc or Q to close this window")
	put(1, 6, "Desktop is still running.")
	s.cells = cells
	return cell.FullGridDiff(s.cols, s.rows, cells)
}

func (s *ExtAppSurface) Snapshot() []cell.Cell {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cells == nil {
		return make([]cell.Cell, s.cols*s.rows)
	}
	out := make([]cell.Cell, len(s.cells))
	copy(out, s.cells)
	return out
}

// Close asks the child to exit by closing its stdin (a well-behaved
// process sees EOF on its next read and exits on its own), then gives it
// a couple seconds before killing it outright. Safe to call more than
// once.
func (s *ExtAppSurface) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	s.stopOnce.Do(func() { close(s.stopCh) })

	_ = s.stdinW.Close()
	done := make(chan struct{})
	go func() {
		_ = s.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		<-done
	}
	slog.Debug("extapp Close id=%s", s.id)
	return nil
}
