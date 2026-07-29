package surface

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ttypedesk/ttypedesk/internal/slog"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// TitleSink receives title changes from the App SDK Host.
type TitleSink func(title string)

// CloseSink is called when an app requests window close via Host.
type CloseSink func()

// AppSurface hosts an in-process uiapp.App.
// Panics inside the app are isolated: the window shows an error screen
// and the rest of the desktop keeps running.
//
// IMPORTANT: SetTitle / MarkDirty must never take s.mu — Handle/Draw hold s.mu
// and apps call those (e.g. Files reload → SetTitle) which used to deadlock
// the entire desktop with no panic log.
type AppSurface struct {
	id     string
	app    uiapp.App
	canvas *uiapp.Canvas
	ctx    *uiapp.Context
	cols   int
	rows   int

	mu       sync.Mutex // draw / handle / lifecycle
	titleMu  sync.Mutex // title + sinks only
	title    string
	onTitle  TitleSink
	onClose  CloseSink
	dirty    atomic.Bool
	closed   bool
	started  bool
	crashed  bool
	crashMsg string
}

func NewAppSurface(id, title string, app uiapp.App, cols, rows int) (*AppSurface, error) {
	if cols < 1 {
		cols = 40
	}
	if rows < 1 {
		rows = 12
	}
	s := &AppSurface{
		id:     id,
		title:  title,
		app:    app,
		cols:   cols,
		rows:   rows,
		canvas: uiapp.NewCanvas(cols, rows),
	}
	s.dirty.Store(true)
	s.ctx = uiapp.NewContext(id, cols, rows, func() { s.markDirty() })
	if hp, ok := app.(uiapp.HookProvider); ok {
		s.ctx.SetHooks(hp.Hooks())
	}
	slog.Debug("app surface created id=%s title=%q %dx%d", id, title, cols, rows)
	return s, nil
}

// BindHost attaches shell services, then initializes the app (so Host works in Init/hooks).
// Must not hold Server.mu across this call — apps may OpenPath / Save config.
func (s *AppSurface) BindHost(host uiapp.Host, onTitle TitleSink, onClose CloseSink) error {
	s.titleMu.Lock()
	s.onTitle = onTitle
	s.onClose = onClose
	s.titleMu.Unlock()

	s.mu.Lock()
	s.ctx.SetHost(&hostBridge{inner: host, surf: s})
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.mu.Unlock()

	slog.Info("app init id=%s title=%q", s.id, s.Title())
	var initErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in app Init id=%s: %v", s.id, r)
				s.markCrashed(fmt.Sprintf("Init panic: %v", r))
				initErr = fmt.Errorf("app init panic: %v", r)
			}
		}()
		initErr = s.app.Init(s.ctx)
	}()
	if initErr != nil {
		slog.Error("app Init failed id=%s: %v", s.id, initErr)
		if !s.isCrashed() {
			return initErr
		}
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in OnInit id=%s: %v", s.id, r)
				s.markCrashed(fmt.Sprintf("OnInit panic: %v", r))
			}
		}()
		if !s.isCrashed() {
			s.ctx.RunHook("init", s.cols, s.rows, true)
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.safeDrawLocked()
	s.dirty.Store(true)
	slog.Debug("app ready id=%s crashed=%v", s.id, s.crashed)
	return nil
}

func (s *AppSurface) isCrashed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.crashed
}

func (s *AppSurface) markCrashed(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.crashed {
		return
	}
	s.crashed = true
	s.crashMsg = msg
	s.dirty.Store(true)
	slog.Error("app crashed id=%s: %s (window isolated)", s.id, msg)
}

type hostBridge struct {
	inner uiapp.Host
	surf  *AppSurface
}

func (b *hostBridge) Notify(title, body, icon string) { b.inner.Notify(title, body, icon) }
func (b *hostBridge) Launch(action string) error      { return b.inner.Launch(action) }
func (b *hostBridge) OpenPath(path string) error      { return b.inner.OpenPath(path) }
func (b *hostBridge) WindowID() string                { return b.inner.WindowID() }

func (b *hostBridge) SaveCredential(key string, value []byte) error {
	return b.inner.SaveCredential(key, value)
}

func (b *hostBridge) LoadCredential(key string) ([]byte, error) {
	return b.inner.LoadCredential(key)
}

func (b *hostBridge) PickFile(startDir string, extensions []string, onResult func(path string, ok bool)) {
	b.inner.PickFile(startDir, extensions, onResult)
}

func (b *hostBridge) PlayAudio(pcm <-chan []int16) (uiapp.AudioPlayback, error) {
	return b.inner.PlayAudio(pcm)
}

func (b *hostBridge) SetTitle(title string) {
	// Never take AppSurface.mu — may be called from Handle/Draw while mu is held.
	b.surf.titleMu.Lock()
	b.surf.title = title
	sink := b.surf.onTitle
	b.surf.titleMu.Unlock()
	if sink != nil {
		sink(title)
	}
}

func (b *hostBridge) RequestClose() {
	slog.Info("app RequestClose id=%s", b.surf.id)
	b.surf.titleMu.Lock()
	sink := b.surf.onClose
	b.surf.titleMu.Unlock()
	if sink != nil {
		sink()
	}
}

func (s *AppSurface) markDirty() {
	s.dirty.Store(true)
}

func (s *AppSurface) ID() string   { return s.id }
func (s *AppSurface) Kind() string { return "app" }

func (s *AppSurface) Title() string {
	s.titleMu.Lock()
	t := s.title
	s.titleMu.Unlock()
	s.mu.Lock()
	crashed := s.crashed
	s.mu.Unlock()
	if crashed {
		return t + " [crashed]"
	}
	return t
}

func (s *AppSurface) MouseMode() int { return 1 }

func (s *AppSurface) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

func (s *AppSurface) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	s.cols, s.rows = cols, rows
	s.canvas.Resize(cols, rows)
	s.ctx.SetSize(cols, rows)
	if s.started && !s.crashed {
		s.safeHandleLocked(uiapp.Event{Kind: uiapp.EventResize, Cols: cols, Rows: rows})
		s.safeHookLocked("resize", cols, rows, false)
	}
	s.safeDrawLocked()
	s.dirty.Store(true)
}

func (s *AppSurface) maybeCloseLocked() {
	if s.crashed {
		return
	}
	cr, ok := s.app.(interface{ WantsClose() bool })
	if !ok || !cr.WantsClose() {
		return
	}
	s.titleMu.Lock()
	sink := s.onClose
	s.titleMu.Unlock()
	if sink == nil {
		return
	}
	slog.Info("app WantsClose id=%s", s.id)
	s.mu.Unlock()
	sink()
	s.mu.Lock()
}

func (s *AppSurface) HandleInput(ev InputEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return nil
	}
	if s.crashed {
		if ev.Kind == "key" && (ev.Key == "Escape" || ev.Rune == 'q' || ev.Rune == 'Q') {
			s.titleMu.Lock()
			sink := s.onClose
			s.titleMu.Unlock()
			if sink != nil {
				s.mu.Unlock()
				sink()
				s.mu.Lock()
			}
		}
		return nil
	}
	ue := uiapp.Event{Kind: uiapp.EventKey, Rune: ev.Rune, Key: ev.Key, Ctrl: ev.Ctrl, Alt: ev.Alt, Shift: ev.Shift}
	if ev.Kind == "mouse" {
		ue.Kind = uiapp.EventMouse
		ue.X, ue.Y = ev.X, ev.Y
		ue.Button = ev.Button
		ue.Action = ev.Action
	}
	if ev.Kind == "scroll" {
		ue.Kind = uiapp.EventMouse
		ue.X, ue.Y = ev.X, ev.Y
		ue.Button = ev.Button
		ue.Action = "wheel"
	}
	s.safeHandleLocked(ue)
	s.maybeCloseLocked()
	s.safeDrawLocked()
	s.dirty.Store(true)
	return nil
}

func (s *AppSurface) ProduceDiff() cell.Diff {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return cell.Diff{}
	}
	if !s.crashed {
		for _, e := range s.ctx.DrainTimers() {
			s.safeHandleLocked(e)
			s.safeHookLocked("timer", s.cols, s.rows, false)
		}
		s.maybeCloseLocked()
		s.safeDrawLocked()
	} else {
		s.drawCrashLocked()
	}
	if !s.dirty.Load() {
		return cell.Diff{}
	}
	s.dirty.Store(false)
	cells := s.canvas.Cells()
	return cell.FullGridDiff(s.cols, s.rows, cells)
}

func (s *AppSurface) Snapshot() []cell.Cell {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.safeDrawLocked()
	}
	return s.canvas.Cells()
}

func (s *AppSurface) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	started := s.started
	crashed := s.crashed
	s.mu.Unlock()

	slog.Debug("app Close id=%s", s.id)
	if started && !crashed {
		func() {
			defer func() { _ = recover() }()
			s.ctx.RunHook("close", s.cols, s.rows, false)
		}()
	}
	s.ctx.Stop()
	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in app Close id=%s: %v", s.id, r)
			}
		}()
		_ = s.app.Close()
	}()
	return nil
}

func (s *AppSurface) NotifyFocus(focused bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.crashed {
		return
	}
	kind := uiapp.EventBlur
	if focused {
		kind = uiapp.EventFocus
	}
	s.safeHandleLocked(uiapp.Event{Kind: kind, Focused: focused})
	s.safeHookLocked("focus", s.cols, s.rows, focused)
	s.dirty.Store(true)
}

func (s *AppSurface) safeHandleLocked(e uiapp.Event) {
	if s.crashed {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.crashed = true
			s.crashMsg = fmt.Sprintf("%v", r)
			s.dirty.Store(true)
			slog.Error("panic in app Handle id=%s: %v", s.id, r)
		}
	}()
	if err := s.app.Handle(e); err != nil {
		slog.Warn("app Handle error id=%s: %v", s.id, err)
	}
}

func (s *AppSurface) safeHookLocked(name string, cols, rows int, focused bool) {
	if s.crashed {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.crashed = true
			s.crashMsg = fmt.Sprintf("hook %s: %v", name, r)
			s.dirty.Store(true)
			slog.Error("panic in app hook %s id=%s: %v", name, s.id, r)
		}
	}()
	s.ctx.RunHook(name, cols, rows, focused)
}

func (s *AppSurface) safeDrawLocked() {
	if s.crashed {
		s.drawCrashLocked()
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.crashed = true
			s.crashMsg = fmt.Sprintf("Draw: %v", r)
			s.dirty.Store(true)
			slog.Error("panic in app Draw id=%s: %v", s.id, r)
			s.drawCrashLocked()
		}
	}()
	_ = s.app.Draw(s.canvas)
	if h := s.ctx.Hooks(); h.OnDraw != nil {
		h.OnDraw(s.ctx, s.canvas)
	}
}

func (s *AppSurface) drawCrashLocked() {
	bg := cell.RGB(0x80, 0x00, 0x00)
	fg := cell.RGB(0xFF, 0xFF, 0xFF)
	s.canvas.FillRect(0, 0, s.cols, s.rows, bg)
	s.canvas.DrawText(1, 1, "Application crashed", fg, bg, cell.AttrBold)
	msg := s.crashMsg
	if msg == "" {
		msg = "(unknown)"
	}
	s.canvas.DrawText(1, 3, truncate(msg, s.cols-2), fg, bg, 0)
	s.canvas.DrawText(1, 5, "Esc or Q to close this window", fg, bg, 0)
	s.canvas.DrawText(1, 6, "Desktop is still running.", fg, bg, 0)
	s.dirty.Store(true)
}

func truncate(s string, n int) string {
	if n < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
