package client

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/apps/calendar"
	"github.com/RetroCodeRamen/ttypedesk/apps/manual"
	"github.com/RetroCodeRamen/ttypedesk/internal/clip"
	"github.com/RetroCodeRamen/ttypedesk/internal/config"
	"github.com/RetroCodeRamen/ttypedesk/internal/notify"
	"github.com/RetroCodeRamen/ttypedesk/internal/palette"
	"github.com/RetroCodeRamen/ttypedesk/internal/server"
	"github.com/RetroCodeRamen/ttypedesk/internal/session"
	"github.com/RetroCodeRamen/ttypedesk/internal/slog"
	"github.com/RetroCodeRamen/ttypedesk/internal/surface"
	"github.com/RetroCodeRamen/ttypedesk/internal/wallpaper"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uwidth"
	"github.com/gdamore/tcell/v2"
)

// BootOptions control the initial desktop contents.
type BootOptions struct {
	ExecCommand string
	ExecArgs    []string
	ImagePath   string
	OpenClock   bool
}

// Client renders the DOS-era desktop and dispatches input.
type Client struct {
	screen tcell.Screen
	tty    *deadlineTty
	srv    *server.Server
	cfg    config.Config
	boot   BootOptions
	quit   bool

	menuOpen bool
	menuIdx  int
	subOpen  int // index into menuRoot with open flyout; -1 = none
	subIdx   int
	menuRoot []startMenuItem

	dragID     string
	dragMode   string
	dragOX     int
	dragOY     int
	dragOW     int
	dragOH     int
	dragWX     int // window origin at resize start
	dragWY     int
	dragEdges  int // edgeN|edgeS|edgeE|edgeW
	scrollGrab int // thumb-relative Y when dragMode=="scroll"
	lastClick  time.Time
	lastClickW string

	iconDragIdx    int // -1 = none
	iconDragOX     int
	iconDragOY     int
	iconDragMoved  bool
	iconDragStartX int
	iconDragStartY int

	// content caches: avoid full Snapshot every frame
	caches      map[string]*winCache
	layoutDirty bool
	lastClock   string
	lastFocus   string
	mouseDownID string

	// text selection (Shift+drag, or drag when guest mouse mode is off)
	selecting     bool
	selWin        string
	selX0, selY0  int
	selX1, selY1  int
	hasSel        bool
	copyMode      bool // tmux-like keyboard scrollback selection (F8)
	copySelecting bool // anchor fixed; cursor (selX1/selY1) extends the selection
	cursorOn      bool // blink phase
	lastBlink     time.Time

	wall   wallpaper.Cache
	notify *notify.Service

	banner      *notify.Notice
	bannerUntil time.Time
	centerOpen  bool

	reminded       map[string]time.Time // calendar event id → reminder slot
	lastRemindPoll time.Time

	cfgModTime  time.Time // mtime of config.json we last applied (ours or external)
	lastCfgPoll time.Time

	attention map[string]bool // window id → needs taskbar attention (BEL)

	findOpen   bool
	findWin    string
	findQuery  string
	findStatus string

	paletteOpen    bool
	paletteQuery   string
	paletteSel     int
	paletteHits    []palette.Hit
	paletteHist    []string
	palettePending string // recipe action awaiting second Enter confirm
}

type winCache struct {
	cols, rows int
	cells      []cell.Cell
	title      string
	x, y, w, h int
	z          int
	focused    bool
}

func New(srv *server.Server, cfg config.Config, boot BootOptions) (*Client, error) {
	tty, err := newDeadlineTty()
	if err != nil {
		return nil, err
	}
	s, err := tcell.NewTerminfoScreenFromTty(tty)
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.EnableMouse()
	s.EnablePaste()
	c := &Client{
		screen:      s,
		tty:         tty,
		srv:         srv,
		cfg:         cfg,
		boot:        boot,
		caches:      make(map[string]*winCache),
		layoutDirty: true,
		subOpen:     -1,
		iconDragIdx: -1,
		reminded:    make(map[string]time.Time),
		attention:   make(map[string]bool),
	}
	if info, err := os.Stat(config.Path()); err == nil {
		c.cfgModTime = info.ModTime()
	}
	c.buildStartMenu()
	c.notify = srv.NotifyService()
	if c.notify != nil {
		c.notify.Subscribe(func(n notify.Notice) {
			c.onNotice(n)
		})
	}
	srv.SetConfigListener(func(nc config.Config) {
		c.ApplyConfig(nc)
	})
	return c, nil
}

func (c *Client) onNotice(n notify.Notice) {
	showBanner := c.cfg.Notify.Banners
	if config.OverSSH() && c.cfg.Notify.SSHBadgeOnly {
		showBanner = false
	}
	if showBanner {
		cp := n
		c.banner = &cp
		sec := c.cfg.Notify.DismissSec
		if sec <= 0 {
			sec = 5
		}
		c.bannerUntil = time.Now().Add(time.Duration(sec) * time.Second)
	}
	c.layoutDirty = true
}

// ApplyConfig hot-applies settings, whether saved from the Settings app or
// picked up from an external edit to config.json (see pollConfigReload).
func (c *Client) ApplyConfig(cfg config.Config) {
	c.cfg = cfg
	c.srv.SetDock(cfg.TaskbarDock())
	c.buildStartMenu()
	c.wall.Invalidate()
	c.layoutDirty = true
	if info, err := os.Stat(config.Path()); err == nil {
		c.cfgModTime = info.ModTime()
	}
}

func (c *Client) Close() {
	c.saveSession()
	// Always restore the host terminal FIRST. Teardown of PTYs/libvterm can
	// race or panic; leaving raw/alt-screen mode is what "locks" the session.
	c.finiScreen()
	c.srv.CloseAll()
	// lanchat.Service has real goroutines/sockets (unlike notify.Service,
	// which needs no such hook) — close it after windows are torn down.
	if lc := c.srv.LANChatService(); lc != nil {
		_ = lc.Close()
	}
}

func (c *Client) finiScreen() {
	if c.screen == nil {
		return
	}
	s := c.screen
	c.screen = nil
	func() {
		defer func() { _ = recover() }()
		s.Fini()
	}()
	// Belt-and-suspenders: reset mouse, alt screen, cursor if Fini was partial.
	_, _ = os.Stdout.WriteString("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1015l\x1b[?1005l\x1b[?25h\x1b[?1049l\x1b[0m\x1b[?7h")
}

func (c *Client) Run() error {
	defer c.finiScreen()

	_ = manual.EnsureSystemFolder()
	c.paletteHist = palette.LoadHistory(20)

	w, h := c.screen.Size()
	c.srv.SetHostSize(w, h)

	restored := 0
	if c.cfg.RestoreSession && c.boot.ExecCommand == "" && !c.boot.OpenClock && c.boot.ImagePath == "" {
		if st, err := session.Load(); err == nil && len(st.Windows) > 0 {
			restored = c.srv.RestoreSession(st)
			slog.Info("restored session: %d windows", restored)
		}
	}

	if c.boot.ExecCommand != "" {
		if _, err := c.srv.CreatePtyCmd(c.boot.ExecCommand, c.boot.ExecCommand, c.boot.ExecArgs); err != nil {
			return fmt.Errorf("exec: %w", err)
		}
	} else if restored == 0 && c.cfg.OpenTerminalOnStart {
		if _, err := c.srv.CreatePty("Terminal"); err != nil {
			return fmt.Errorf("spawn terminal: %w", err)
		}
	}
	if c.boot.OpenClock {
		_, _ = c.srv.CreateApp("clock", "Clock")
	}
	if c.boot.ImagePath != "" {
		_, _ = c.srv.CreateGfx("Image Viewer", c.boot.ImagePath)
	}

	// Autostart actions (after session restore / boot flags)
	for _, action := range c.cfg.Autostart {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		if err := c.srv.LaunchAction(action); err != nil {
			slog.Warn("autostart %q: %v", action, err)
		}
	}

	fps := c.cfg.EffectiveFPS()
	if fps <= 0 {
		fps = 30
	}
	if config.OverSSH() {
		fmt.Fprintf(os.Stderr, "TTYPE Desk: SSH session detected — streaming at %d FPS\n", fps)
	}
	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()

	events := make(chan tcell.Event, 64)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in event poller: %v\n%s", r, debug.Stack())
			}
		}()
		for {
			s := c.screen
			if s == nil {
				return
			}
			ev := s.PollEvent()
			if ev == nil {
				return
			}
			select {
			case events <- ev:
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()

	lastSave := time.Now()
	for !c.quit {
		select {
		case ev := <-events:
			if c.screen == nil {
				return nil
			}
			if !slog.Guard("client.handleEvent", func() { c.handleEvent(ev) }) {
				c.layoutDirty = true
			}
		case <-ticker.C:
			if c.screen == nil || c.quit {
				continue
			}
			if !slog.Guard("client.tick", func() {
				if time.Since(c.lastBlink) >= 330*time.Millisecond {
					c.cursorOn = !c.cursorOn
					c.lastBlink = time.Now()
				}
				c.srv.RefreshTitles()
				c.pollCalendarReminders()
				c.pollConfigReload()
				c.draw()
				if c.tty.ConsumeTimedOut() {
					// A write got torn by the deadline: the terminal may
					// have a partial escape sequence sitting in it, and
					// tcell's diff model no longer matches what's really
					// on screen. Force a full repaint to self-heal.
					c.screen.Sync()
				}
				if c.cfg.RestoreSession && time.Since(lastSave) >= 30*time.Second {
					c.saveSession()
					lastSave = time.Now()
				}
			}) {
				c.layoutDirty = true
			}
		}
	}
	c.saveSession()
	return nil
}

func (c *Client) saveSession() {
	if !c.cfg.RestoreSession {
		return
	}
	st := c.srv.CaptureSession()
	if err := session.Save(st); err != nil {
		slog.Warn("session save: %v", err)
	} else {
		slog.Debug("session saved (%d windows)", len(st.Windows))
	}
}

func (c *Client) pollCalendarReminders() {
	if c.notify == nil {
		return
	}
	now := time.Now()
	if !c.lastRemindPoll.IsZero() && now.Sub(c.lastRemindPoll) < 20*time.Second {
		return
	}
	c.lastRemindPoll = now
	lead := time.Duration(c.cfg.Calendar.LeadMin) * time.Minute
	for _, due := range calendar.DueReminders(now, lead, c.reminded) {
		body := due.Event.Start.Format("15:04")
		if due.Event.AllDay {
			body = "All day — " + due.Event.Start.Format("Mon Jan 2")
		}
		c.notify.Post(due.Event.Title, body, "📅", "calendar")
		c.reminded[due.Event.ID] = due.At
		key := due.Event.ID + "|" + due.At.Format("2006-01-02T15:04")
		c.reminded[key] = due.At
	}
}

func (c *Client) handleEvent(ev tcell.Event) {
	switch e := ev.(type) {
	case *tcell.EventResize:
		w, h := e.Size()
		c.srv.SetHostSize(w, h)
		c.layoutDirty = true
	case *tcell.EventKey:
		c.handleKey(e)
	case *tcell.EventMouse:
		c.handleMouse(e)
	case *tcell.EventError:
		// The tty read loop only surfaces this once the underlying pty is
		// truly gone (e.g. the SSH session was torn down). Quit cleanly
		// instead of spinning forever on doomed writes to a dead fd.
		slog.Warn("tty error, shutting down: %v", e.Error())
		c.quit = true
	}
}

func (c *Client) handleKey(e *tcell.EventKey) {
	if c.matchHK(config.HKQuit, e) {
		c.quit = true
		return
	}
	if c.copyMode && c.handleCopyModeKey(e) {
		return
	}
	if c.findOpen && c.handleFindKey(e) {
		return
	}
	if c.paletteOpen && c.handlePaletteKey(e) {
		return
	}
	if c.matchHK(config.HKPalette, e) || c.matchHK(config.HKPaletteAlt, e) {
		c.togglePalette()
		return
	}
	if c.matchHK(config.HKFind, e) || c.matchHK(config.HKFindAlt, e) || c.matchHK(config.HKFindLegacy, e) {
		c.openFind()
		return
	}
	if c.matchHK(config.HKCopyMode, e) {
		c.openCopyMode()
		return
	}
	if c.matchHK(config.HKCopy, e) {
		c.copySelection()
		return
	}
	if c.matchHK(config.HKPaste, e) {
		c.pasteClipboard()
		return
	}
	if c.matchHK(config.HKCycleFocus, e) {
		wins := c.srv.Windows()
		visible := make([]*server.Window, 0, len(wins))
		for _, w := range wins {
			if !w.Minimized {
				visible = append(visible, w)
			}
		}
		if len(visible) == 0 {
			return
		}
		focus := c.srv.Focused()
		idx := 0
		for i, w := range visible {
			if w.ID == focus {
				idx = i
				break
			}
		}
		next := visible[(idx+1)%len(visible)]
		c.srv.Focus(next.ID)
		c.layoutDirty = true
		return
	}
	// Keyboard window manager (Alt+arrows still fixed; Start via hotkey below)
	if e.Modifiers()&tcell.ModAlt != 0 && c.srv.Focused() != "" {
		id := c.srv.Focused()
		shift := e.Modifiers()&tcell.ModShift != 0
		ctrl := e.Modifiers()&tcell.ModCtrl != 0
		if ctrl && !shift {
			switch e.Key() {
			case tcell.KeyLeft:
				c.srv.Snap(id, "left")
				c.layoutDirty = true
				return
			case tcell.KeyRight:
				c.srv.Snap(id, "right")
				c.layoutDirty = true
				return
			case tcell.KeyUp:
				c.srv.Snap(id, "top")
				c.layoutDirty = true
				return
			case tcell.KeyDown:
				c.srv.Snap(id, "bottom")
				c.layoutDirty = true
				return
			}
		}
		switch e.Key() {
		case tcell.KeyLeft:
			if shift {
				c.srv.Nudge(id, 0, 0, -2, 0)
			} else {
				c.srv.Nudge(id, -2, 0, 0, 0)
			}
			c.layoutDirty = true
			return
		case tcell.KeyRight:
			if shift {
				c.srv.Nudge(id, 0, 0, 2, 0)
			} else {
				c.srv.Nudge(id, 2, 0, 0, 0)
			}
			c.layoutDirty = true
			return
		case tcell.KeyUp:
			if shift {
				c.srv.Nudge(id, 0, 0, 0, -1)
			} else {
				c.srv.Nudge(id, 0, -1, 0, 0)
			}
			c.layoutDirty = true
			return
		case tcell.KeyDown:
			if shift {
				c.srv.Nudge(id, 0, 0, 0, 1)
			} else {
				c.srv.Nudge(id, 0, 1, 0, 0)
			}
			c.layoutDirty = true
			return
		case tcell.KeyF4:
			c.srv.CloseWindow(id)
			delete(c.caches, id)
			c.layoutDirty = true
			return
		}
	}
	if c.matchHK(config.HKClose, e) {
		id := c.srv.Focused()
		if id != "" {
			c.srv.CloseWindow(id)
			delete(c.caches, id)
			c.layoutDirty = true
		}
		return
	}
	if c.matchHK(config.HKMinimize, e) {
		id := c.srv.Focused()
		if id != "" {
			c.srv.ToggleMinimize(id)
			c.layoutDirty = true
		}
		return
	}
	if c.matchHK(config.HKStartMenu, e) || c.matchHK(config.HKStartMenuAlt, e) || c.matchHK(config.HKStartMenuLegacy, e) {
		c.toggleStartMenu()
		return
	}
	if c.menuOpen {
		c.handleStartMenuKey(e)
		return
	}
	focus := c.srv.Focused()
	if focus == "" {
		return
	}
	ev := surface.InputEvent{Kind: "key"}
	if e.Modifiers()&tcell.ModCtrl != 0 {
		ev.Ctrl = true
	}
	if e.Modifiers()&tcell.ModAlt != 0 {
		ev.Alt = true
	}
	if e.Modifiers()&tcell.ModShift != 0 {
		ev.Shift = true
	}
	switch e.Key() {
	case tcell.KeyEnter:
		ev.Key = "Enter"
	case tcell.KeyTab:
		ev.Key = "Tab"
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		ev.Key = "Backspace"
	case tcell.KeyEscape:
		ev.Key = "Escape"
	case tcell.KeyUp:
		ev.Key = "Up"
	case tcell.KeyDown:
		ev.Key = "Down"
	case tcell.KeyLeft:
		ev.Key = "Left"
	case tcell.KeyRight:
		ev.Key = "Right"
	case tcell.KeyHome:
		ev.Key = "Home"
	case tcell.KeyEnd:
		ev.Key = "End"
	case tcell.KeyPgUp:
		ev.Key = "PgUp"
	case tcell.KeyPgDn:
		ev.Key = "PgDn"
	case tcell.KeyDelete:
		ev.Key = "Delete"
	case tcell.KeyInsert:
		ev.Key = "Insert"
	case tcell.KeyCtrlC:
		ev.Bytes = []byte{0x03}
	case tcell.KeyCtrlD:
		ev.Bytes = []byte{0x04}
	case tcell.KeyCtrlZ:
		ev.Bytes = []byte{0x1a}
	case tcell.KeyCtrlL:
		ev.Bytes = []byte{0x0c}
	case tcell.KeyCtrlV:
		// plain Ctrl+V → paste into focused surface
		c.pasteClipboard()
		return
	case tcell.KeyRune:
		ev.Rune = e.Rune()
	default:
		return
	}
	c.srv.HandleInput(focus, ev)
}

func (c *Client) copySelection() {
	if !c.hasSel {
		return
	}
	text := c.selectionText()
	if text != "" {
		clip.Set(text)
	}
}

func (c *Client) pasteClipboard() {
	focus := c.srv.Focused()
	if focus == "" {
		return
	}
	text := clip.Get()
	if text == "" {
		return
	}
	c.srv.HandleInput(focus, surface.InputEvent{Kind: "key", Bytes: []byte(text)})
}

func (c *Client) selectionText() string {
	cache := c.caches[c.selWin]
	if cache == nil {
		return ""
	}
	x0, y0, x1, y1 := c.selX0, c.selY0, c.selX1, c.selY1
	if y0 > y1 || (y0 == y1 && x0 > x1) {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	var b strings.Builder
	for y := y0; y <= y1; y++ {
		if y < 0 || y >= cache.rows {
			continue
		}
		start, end := 0, cache.cols-1
		if y == y0 {
			start = x0
		}
		if y == y1 {
			end = x1
		}
		if start < 0 {
			start = 0
		}
		if end >= cache.cols {
			end = cache.cols - 1
		}
		for x := start; x <= end; x++ {
			i := y*cache.cols + x
			if i >= 0 && i < len(cache.cells) {
				r := cache.cells[i].Rune
				if r == 0 {
					r = ' '
				}
				b.WriteRune(r)
			}
		}
		if y != y1 {
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), " \t")
}

func (c *Client) inSelection(winID string, x, y int) bool {
	if !c.hasSel || winID != c.selWin {
		return false
	}
	x0, y0, x1, y1 := c.selX0, c.selY0, c.selX1, c.selY1
	if y0 > y1 || (y0 == y1 && x0 > x1) {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	if y < y0 || y > y1 {
		return false
	}
	start, end := 0, 1<<30
	if y == y0 {
		start = x0
	}
	if y == y1 {
		end = x1
	}
	return x >= start && x <= end
}

func (c *Client) handleMouse(e *tcell.EventMouse) {
	x, y := e.Position()
	btn := e.Buttons()

	if btn&tcell.ButtonMiddle != 0 {
		c.pasteClipboard()
		return
	}

	// Wheel → scrollback / mouse wheel
	if btn&tcell.WheelUp != 0 || btn&tcell.WheelDown != 0 {
		delta := 3
		if btn&tcell.WheelDown != 0 {
			delta = -3
		}
		wins := c.srv.Windows()
		for i := len(wins) - 1; i >= 0; i-- {
			w := wins[i]
			if x > w.X && x < w.X+w.W-1 && y > w.Y && y < w.Y+w.H-1 {
				c.srv.HandleInput(w.ID, surface.InputEvent{
					Kind:   "scroll",
					X:      x - w.X - 1,
					Y:      y - w.Y - 1,
					Button: delta,
				})
				return
			}
		}
		return
	}

	if c.menuOpen {
		_ = c.handleStartMenuMouse(x, y, btn)
		return
	}

	if c.centerOpen && btn&tcell.ButtonPrimary != 0 {
		if c.hitNotifyCenter(x, y) {
			return
		}
		c.centerOpen = false
		c.layoutDirty = true
		// fall through if click was on taskbar tray
	}

	if c.banner != nil && btn&tcell.ButtonPrimary != 0 {
		if c.hitBanner(x, y) {
			c.banner = nil
			c.centerOpen = true
			if c.notify != nil {
				c.notify.MarkAllRead()
			}
			c.layoutDirty = true
			return
		}
	}

	if c.onTaskbar(x, y) && btn&tcell.ButtonPrimary != 0 {
		if c.onStartButton(x, y) {
			c.toggleStartMenu()
			return
		}
		if c.hitBell(x, y) {
			c.centerOpen = !c.centerOpen
			if c.centerOpen && c.notify != nil {
				c.notify.MarkAllRead()
			}
			c.layoutDirty = true
			return
		}
		if c.hitClock(x, y) {
			_ = c.srv.LaunchAction("calendar")
			c.layoutDirty = true
			return
		}
		// Taskbar window buttons
		if id := c.hitTaskbarButton(x, y); id != "" {
			w := c.srv.Get(id)
			if w != nil && w.Minimized {
				c.srv.ToggleMinimize(id)
			} else if c.srv.Focused() == id {
				c.srv.ToggleMinimize(id)
			} else {
				c.srv.Focus(id)
			}
			c.layoutDirty = true
			return
		}
	}

	if c.iconDragIdx >= 0 {
		if btn&tcell.ButtonPrimary == 0 {
			idx := c.iconDragIdx
			moved := c.iconDragMoved
			c.iconDragIdx = -1
			c.iconDragMoved = false
			if moved {
				_ = config.Save(c.cfg)
				c.srv.SetConfig(c.cfg)
			} else if idx >= 0 && idx < len(c.cfg.DesktopIcons) {
				_ = c.srv.LaunchAction(c.cfg.DesktopIcons[idx].Action)
			}
			c.layoutDirty = true
			return
		}
		nx, ny := c.clampIconToDesktop(x-c.iconDragOX, y-c.iconDragOY)
		if idx := c.iconDragIdx; idx >= 0 && idx < len(c.cfg.DesktopIcons) {
			ic := &c.cfg.DesktopIcons[idx]
			if nx != ic.X || ny != ic.Y {
				ic.X, ic.Y = nx, ny
				c.iconDragMoved = true
				c.layoutDirty = true
			}
			if abs(x-c.iconDragStartX) > 1 || abs(y-c.iconDragStartY) > 1 {
				c.iconDragMoved = true
			}
		}
		return
	}

	if c.dragID != "" {
		if btn&tcell.ButtonPrimary == 0 {
			c.dragID = ""
			c.dragMode = ""
			c.dragEdges = 0
			c.scrollGrab = 0
			c.layoutDirty = true
			return
		}
		if c.dragMode == "move" {
			c.srv.Move(c.dragID, x-c.dragOX, y-c.dragOY)
			c.layoutDirty = true
		} else if c.dragMode == "resize" {
			nx, ny, nw, nh := geomFromEdgeDrag(c.dragEdges, c.dragWX, c.dragWY, c.dragOW, c.dragOH, c.dragOX, c.dragOY, x, y)
			c.srv.SetGeometry(c.dragID, nx, ny, nw, nh)
			c.layoutDirty = true
		} else if c.dragMode == "scroll" {
			c.dragScrollThumb(c.dragID, y)
		} else if c.dragMode == "select" {
			w := c.srv.Get(c.dragID)
			if w != nil {
				c.selX1 = x - w.X - 1
				c.selY1 = y - w.Y - 1
				c.hasSel = true
			}
		}
		return
	}

	// Mouse release
	if btn&tcell.ButtonPrimary == 0 {
		if c.selecting {
			c.selecting = false
			if c.hasSel {
				c.copySelection()
			}
			return
		}
		if c.mouseDownID != "" {
			w := c.srv.Get(c.mouseDownID)
			if w != nil {
				c.srv.HandleInput(c.mouseDownID, surface.InputEvent{
					Kind: "mouse", X: x - w.X - 1, Y: y - w.Y - 1, Button: 1, Action: "release",
				})
			}
			c.mouseDownID = ""
		}
		return
	}

	// Desktop icon press (drag or click-to-launch on release)
	if !c.onTaskbar(x, y) {
		onWindow := false
		for _, w := range c.srv.Windows() {
			if w.Minimized {
				continue
			}
			if x >= w.X && x < w.X+w.W && y >= w.Y && y < w.Y+w.H {
				onWindow = true
				break
			}
		}
		if !onWindow {
			if idx := c.hitDesktopIconIndex(x, y); idx >= 0 {
				ic := &c.cfg.DesktopIcons[idx]
				c.iconDragIdx = idx
				c.iconDragOX = x - ic.X
				c.iconDragOY = y - ic.Y
				c.iconDragMoved = false
				c.iconDragStartX = x
				c.iconDragStartY = y
				c.layoutDirty = true
				return
			}
		}
	}

	wins := c.srv.Windows()
	for i := len(wins) - 1; i >= 0; i-- {
		w := wins[i]
		if w.Minimized {
			continue
		}
		if x < w.X || y < w.Y || x >= w.X+w.W || y >= w.Y+w.H {
			continue
		}
		if c.srv.Focused() != w.ID {
			c.srv.Focus(w.ID)
			c.layoutDirty = true
		}

		if y == w.Y && x == w.X+w.W-2 {
			c.srv.CloseWindow(w.ID)
			delete(c.caches, w.ID)
			c.layoutDirty = true
			return
		}
		if y == w.Y && x == w.X+w.W-4 {
			c.srv.ToggleMaximize(w.ID)
			c.layoutDirty = true
			return
		}
		if y == w.Y && x == w.X+w.W-6 {
			c.srv.ToggleMinimize(w.ID)
			c.layoutDirty = true
			return
		}
		// PTY scrollback bar on right border (corners still resize)
		if x == w.X+w.W-1 && y > w.Y && y < w.Y+w.H-1 {
			if c.handleScrollbarPress(w, x, y) {
				return
			}
		}
		// Edge / corner resize (before title move so corners win)
		if edges := hitResizeEdges(w, x, y); edges != 0 {
			c.dragID = w.ID
			c.dragMode = "resize"
			c.dragEdges = edges
			c.dragOX, c.dragOY = x, y
			c.dragWX, c.dragWY = w.X, w.Y
			c.dragOW, c.dragOH = w.W, w.H
			return
		}
		if y == w.Y && x > w.X && x < w.X+w.W-7 {
			// Double-click title → maximize
			now := time.Now()
			if c.lastClickW == w.ID && now.Sub(c.lastClick) < 400*time.Millisecond {
				c.srv.ToggleMaximize(w.ID)
				c.lastClickW = ""
				c.layoutDirty = true
				return
			}
			c.lastClick = now
			c.lastClickW = w.ID
			c.dragID = w.ID
			c.dragMode = "move"
			c.dragOX = x - w.X
			c.dragOY = y - w.Y
			return
		}
		if x > w.X && x < w.X+w.W-1 && y > w.Y && y < w.Y+w.H-1 {
			cx, cy := x-w.X-1, y-w.Y-1
			mouseMode := 0
			if mp, ok := w.Surface.(surface.MouseModeProvider); ok {
				mouseMode = mp.MouseMode()
			}
			// Native apps always receive mouse; PTY only when guest enables mouse mode.
			wantsMouse := mouseMode != 0 || w.Kind == "app"
			shift := e.Modifiers()&tcell.ModShift != 0
			if shift || !wantsMouse {
				c.selecting = true
				c.dragID = w.ID
				c.dragMode = "select"
				c.selWin = w.ID
				c.selX0, c.selY0 = cx, cy
				c.selX1, c.selY1 = cx, cy
				c.hasSel = true
				return
			}
			action := "press"
			if c.mouseDownID == w.ID {
				action = "drag"
			}
			c.mouseDownID = w.ID
			c.srv.HandleInput(w.ID, surface.InputEvent{
				Kind: "mouse", X: cx, Y: cy, Button: 1, Action: action,
			})
		}
		return
	}
}

// taskbarButtonLayout returns window button chips along the taskbar axis.
// Horizontal: start/width are X. Vertical: start/width are Y (height of chip).
func (c *Client) taskbarButtonLayout() []struct {
	id    string
	start int
	width int
} {
	wins := c.srv.Windows()
	if len(wins) == 0 {
		return nil
	}
	if c.isVerticalDock() {
		tray := c.trayLayout()
		startY := 1 // below Start
		endY := tray.bellY
		if endY <= startY {
			return nil
		}
		avail := endY - startY
		btnH := 1
		maxN := avail / btnH
		if maxN < 1 {
			return nil
		}
		out := make([]struct {
			id    string
			start int
			width int
		}, 0, len(wins))
		y := startY
		for i, win := range wins {
			if i >= maxN || y+btnH > endY {
				break
			}
			out = append(out, struct {
				id    string
				start int
				width int
			}{win.ID, y, btnH})
			y += btnH
		}
		return out
	}

	tray := c.trayLayout()
	startX := 9
	endX := tray.bellX - 1
	avail := endX - startX
	if avail < 8 {
		return nil
	}
	btnW := avail / len(wins)
	if btnW > 18 {
		btnW = 18
	}
	if btnW < 6 {
		btnW = 6
	}
	out := make([]struct {
		id    string
		start int
		width int
	}, 0, len(wins))
	x := startX
	for _, win := range wins {
		if x+btnW > endX {
			break
		}
		out = append(out, struct {
			id    string
			start int
			width int
		}{win.ID, x, btnW})
		x += btnW
	}
	return out
}

type trayGeom struct {
	bellX, bellY, bellW, bellH int
	clockX, clockY, clockW     int
}

func (c *Client) trayLayout() trayGeom {
	w, h := c.screen.Size()
	if c.isVerticalDock() {
		c0 := c.taskbarCol0()
		th := c.taskbarThickness()
		bellLabel := c.trayBellLabel()
		bellW := uwidth.String(bellLabel)
		if bellW > th {
			bellW = th
		}
		// Bottom of bar: clock row, bell above it.
		clockY := h - 1
		bellY := h - 2
		if bellY < 1 {
			bellY = 1
		}
		return trayGeom{
			bellX: c0, bellY: bellY, bellW: bellW, bellH: 1,
			clockX: c0, clockY: clockY, clockW: th,
		}
	}
	clockStr := time.Now().Format("15:04:05")
	clockW := len(clockStr)
	bellLabel := c.trayBellLabel()
	bellW := uwidth.String(bellLabel)
	if bellW < 2 {
		bellW = 2
	}
	clockX := w - clockW - 1
	bellX := clockX - 3 - bellW
	if bellX < 9 {
		bellX = 9
	}
	return trayGeom{
		bellX: bellX, bellY: c.taskbarRow(), bellW: bellW, bellH: 1,
		clockX: clockX, clockY: c.taskbarRow(), clockW: clockW,
	}
}

func (c *Client) trayBellLabel() string {
	bellGlyph := c.iconGlyph("🔔")
	unread := 0
	if c.notify != nil {
		unread = c.notify.Unread()
	}
	if unread <= 0 {
		return bellGlyph
	}
	if unread > 9 {
		return bellGlyph + "9+"
	}
	return fmt.Sprintf("%s%d", bellGlyph, unread)
}

func (c *Client) hitBell(x, y int) bool {
	t := c.trayLayout()
	return x >= t.bellX && x < t.bellX+t.bellW && y >= t.bellY && y < t.bellY+t.bellH
}

func (c *Client) hitClock(x, y int) bool {
	t := c.trayLayout()
	if c.isVerticalDock() {
		return x >= t.clockX && x < t.clockX+t.clockW && y == t.clockY
	}
	return y == t.clockY && x >= t.clockX-3 && x < t.clockX+t.clockW
}

func (c *Client) hitTaskbarButton(x, y int) string {
	for _, b := range c.taskbarButtonLayout() {
		if c.isVerticalDock() {
			if y >= b.start && y < b.start+b.width {
				return b.id
			}
			continue
		}
		if x >= b.start && x < b.start+b.width {
			return b.id
		}
	}
	return ""
}

func (c *Client) hitBanner(x, y int) bool {
	if c.banner == nil {
		return false
	}
	bx, by, bw, bh := c.bannerRect()
	return x >= bx && x < bx+bw && y >= by && y < by+bh
}

func (c *Client) bannerRect() (x, y, w, h int) {
	sw, sh := c.screen.Size()
	dx0, dy0, dx1, dy1 := c.desktopRect()
	w, h = 36, 4
	if w > dx1-dx0-1 {
		w = dx1 - dx0 - 1
	}
	if w < 12 {
		w = 12
	}
	x = dx1 - w - 1
	if x < dx0 {
		x = dx0
	}
	switch c.dockSide() {
	case "bottom":
		y = dy1 - h
		if y < dy0 {
			y = dy0
		}
	default:
		y = dy0
	}
	_ = sw
	_ = sh
	return
}

func (c *Client) hitNotifyCenter(x, y int) bool {
	cx, cy, cw, ch := c.centerRect()
	if x < cx || x >= cx+cw || y < cy || y >= cy+ch {
		return false
	}
	// header buttons: dismiss all on last row
	if y == cy+ch-1 {
		if c.notify != nil {
			c.notify.DismissAll()
		}
		c.centerOpen = false
		c.layoutDirty = true
		return true
	}
	// click a notice row to dismiss
	row := y - cy - 2
	if c.notify != nil && row >= 0 {
		items := c.notify.List()
		if row < len(items) {
			c.notify.Dismiss(items[row].ID)
			c.layoutDirty = true
		}
	}
	return true
}

func (c *Client) centerRect() (x, y, w, h int) {
	dx0, dy0, dx1, dy1 := c.desktopRect()
	w = 40
	h = 12
	if w > dx1-dx0-1 {
		w = dx1 - dx0 - 1
	}
	if h > dy1-dy0-1 {
		h = dy1 - dy0 - 1
	}
	if w < 16 {
		w = 16
	}
	if h < 6 {
		h = 6
	}
	x = dx1 - w - 1
	if x < dx0 {
		x = dx0
	}
	switch c.dockSide() {
	case "bottom":
		y = dy1 - h
		if y < dy0 {
			y = dy0
		}
	default:
		y = dy0
	}
	return
}

func (c *Client) style(fg, bg cell.Color, attr cell.Attr) tcell.Style {
	s := tcell.StyleDefault.
		Foreground(tcell.NewRGBColor(int32(fg.R), int32(fg.G), int32(fg.B))).
		Background(tcell.NewRGBColor(int32(bg.R), int32(bg.G), int32(bg.B)))
	if attr&cell.AttrBold != 0 {
		s = s.Bold(true)
	}
	if attr&cell.AttrUnderline != 0 {
		s = s.Underline(true)
	}
	if attr&cell.AttrReverse != 0 {
		s = s.Reverse(true)
	}
	if attr&cell.AttrItalic != 0 {
		s = s.Italic(true)
	}
	return s
}

func (c *Client) set(x, y int, r rune, fg, bg cell.Color, attr cell.Attr) {
	sw, sh := c.screen.Size()
	if x < 0 || y < 0 || x >= sw || y >= sh {
		return
	}
	c.screen.SetContent(x, y, r, nil, c.style(fg, bg, attr))
}

func (c *Client) drawString(x, y int, s string, fg, bg cell.Color, attr cell.Attr) {
	col := x
	for _, r := range s {
		w := uwidth.Rune(r)
		c.set(col, y, r, fg, bg, attr)
		// Clear continuation cells for double-width glyphs (emoji).
		for i := 1; i < w; i++ {
			c.set(col+i, y, ' ', fg, bg, attr)
		}
		col += w
	}
}

// iconGlyph returns icon, substituted for an ASCII stand-in when the
// "disable emoji" setting is on.
func (c *Client) iconGlyph(icon string) string {
	return uwidth.ASCIIIcon(icon, c.cfg.Palette.ASCIIIcons)
}

func (c *Client) drawDesktopIcons() {
	th := c.cfg.Theme
	ascii := c.cfg.Palette.ASCIIIcons
	for _, ic := range c.cfg.DesktopIcons {
		glyph := uwidth.ASCIIIcon(ic.Icon, ascii)
		if glyph == "" {
			glyph = "■"
		}
		c.drawString(ic.X, ic.Y, glyph, th.IconFG, th.DesktopBG, cell.AttrBold)
		c.drawString(ic.X, ic.Y+1, uwidth.Truncate(ic.Label, 12), th.IconLabelFG, th.DesktopBG, 0)
	}
}

func (c *Client) hitDesktopIconIndex(x, y int) int {
	if !c.cfg.ShowDesktopIcons {
		return -1
	}
	ascii := c.cfg.Palette.ASCIIIcons
	for i := range c.cfg.DesktopIcons {
		ic := &c.cfg.DesktopIcons[i]
		iw := uwidth.IconCells(ic.Icon, ascii)
		// icon cell(s) or label row
		if y == ic.Y && x >= ic.X && x < ic.X+iw {
			return i
		}
		lw := uwidth.String(uwidth.Truncate(ic.Label, 12))
		if y == ic.Y+1 && x >= ic.X && x < ic.X+lw {
			return i
		}
	}
	return -1
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (c *Client) syncCaches() {
	wins := c.srv.Windows()
	live := make(map[string]struct{}, len(wins))
	focus := c.srv.Focused()
	for _, win := range wins {
		live[win.ID] = struct{}{}
		func(win *server.Window) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic syncing window %s: %v", win.ID, r)
					c.layoutDirty = true
				}
			}()
			cc, cr := win.ContentSize()
			cache, ok := c.caches[win.ID]
			if !ok || cache.cols != cc || cache.rows != cr {
				cells := win.Surface.Snapshot()
				cache = &winCache{cols: cc, rows: cr, cells: cells}
				c.caches[win.ID] = cache
				c.layoutDirty = true
			} else {
				diff := win.Surface.ProduceDiff()
				if diff.Rect.W > 0 && diff.Rect.H > 0 && len(diff.Cells) > 0 {
					applyDiff(cache, diff)
				}
			}
			cache.title = win.Title
			cache.x, cache.y, cache.w, cache.h = win.X, win.Y, win.W, win.H
			cache.z = win.Z
			cache.focused = win.ID == focus
			if bp, ok := win.Surface.(surface.BellProvider); ok {
				if bp.TakeBell() {
					if win.ID != focus {
						c.attention[win.ID] = true
						c.layoutDirty = true
					}
				}
			}
			if win.ID == focus && c.attention[win.ID] {
				delete(c.attention, win.ID)
				c.layoutDirty = true
			}
		}(win)
	}
	for id := range c.caches {
		if _, ok := live[id]; !ok {
			delete(c.caches, id)
			delete(c.attention, id)
			c.layoutDirty = true
		}
	}
	if focus != c.lastFocus {
		c.lastFocus = focus
		c.layoutDirty = true
	}
}

func applyDiff(cache *winCache, diff cell.Diff) {
	r := diff.Rect
	for row := 0; row < r.H; row++ {
		for col := 0; col < r.W; col++ {
			i := row*r.W + col
			if i >= len(diff.Cells) {
				continue
			}
			x, y := r.X+col, r.Y+row
			if x < 0 || y < 0 || x >= cache.cols || y >= cache.rows {
				continue
			}
			cache.cells[y*cache.cols+x] = diff.Cells[i]
		}
	}
}

func (c *Client) draw() {
	if c.screen == nil {
		return
	}
	c.syncCaches()
	th := c.cfg.Theme
	w, h := c.screen.Size()
	clock := time.Now().Format("15:04:05")
	c.lastClock = clock

	needFull := c.layoutDirty || c.menuOpen || c.hasSel || c.centerOpen || c.banner != nil || c.findOpen || c.paletteOpen
	if needFull {
		c.screen.Clear()
		c.drawDesktop(w, h, th)
		if c.cfg.ShowDesktopIcons {
			c.drawDesktopIcons()
		}
	}

	wins := c.srv.Windows()
	for _, win := range wins {
		if win.Minimized {
			continue
		}
		cache := c.caches[win.ID]
		if cache == nil {
			continue
		}
		c.drawWindow(win, cache, win.ID == c.srv.Focused(), needFull)
	}

	// Taskbar
	c.drawTaskbar(w, h, th, clock)

	if c.banner != nil {
		if time.Now().After(c.bannerUntil) {
			c.banner = nil
		} else {
			c.drawBanner()
		}
	}
	if c.centerOpen {
		c.drawNotifyCenter()
	}
	if c.menuOpen {
		c.drawMenu()
	}
	c.drawFindBar()
	c.drawCopyModeBar()
	c.drawPalette()

	c.layoutDirty = false
	c.screen.Show()
}

func (c *Client) drawTaskbar(w, h int, th config.Theme, clock string) {
	focus := c.srv.Focused()
	tray := c.trayLayout()
	bellLabel := c.trayBellLabel()
	bellBG := th.TaskbarBG
	if c.centerOpen {
		bellBG = th.StartBG
	}

	if c.isVerticalDock() {
		c0 := c.taskbarCol0()
		thk := c.taskbarThickness()
		for y := 0; y < h; y++ {
			for x := 0; x < thk; x++ {
				c.set(c0+x, y, ' ', th.TaskbarFG, th.TaskbarBG, 0)
			}
		}
		// Start chip (top) — same label as horizontal bar
		startLabel := uwidth.Truncate(" Start ", thk)
		for x := 0; x < thk; x++ {
			c.set(c0+x, 0, ' ', th.StartFG, th.StartBG, cell.AttrBold)
		}
		c.drawString(c0, 0, startLabel, th.StartFG, th.StartBG, cell.AttrBold)
		for _, b := range c.taskbarButtonLayout() {
			win := c.srv.Get(b.id)
			if win == nil {
				continue
			}
			label := win.Title
			if label == "" {
				label = "?"
			}
			runes := []rune(label)
			if len(runes) > thk {
				runes = runes[:thk]
			}
			bg, fg := th.MenuBG, th.MenuFG
			if win.ID == focus && !win.Minimized {
				bg, fg = th.TitleFocused, th.TitleFG
			} else if win.Minimized {
				bg = cell.RGB(0x60, 0x60, 0x60)
				fg = th.TaskbarFG
			}
			if c.attention[win.ID] && (time.Now().UnixMilli()/400)%2 == 0 {
				bg = cell.RGB(0xFF, 0xFF, 0x00)
				fg = cell.RGB(0x00, 0x00, 0x00)
			}
			for x := 0; x < thk; x++ {
				ch := ' '
				if x < len(runes) {
					ch = runes[x]
				}
				c.set(c0+x, b.start, ch, fg, bg, 0)
			}
		}
		c.drawString(tray.bellX, tray.bellY, uwidth.Truncate(bellLabel, thk), th.TaskbarFG, bellBG, 0)
		clk := uwidth.Truncate(time.Now().Format("15:04"), thk)
		c.drawString(tray.clockX, tray.clockY, clk, th.TaskbarFG, th.TaskbarBG, 0)
		return
	}

	tby := c.taskbarRow()
	for x := 0; x < w; x++ {
		c.set(x, tby, ' ', th.TaskbarFG, th.TaskbarBG, 0)
	}
	for i, r := range " Start " {
		c.set(i, tby, r, th.StartFG, th.StartBG, cell.AttrBold)
	}
	for _, b := range c.taskbarButtonLayout() {
		win := c.srv.Get(b.id)
		if win == nil {
			continue
		}
		label := uwidth.Truncate(" "+win.Title, b.width)
		bg, fg := th.MenuBG, th.MenuFG
		if win.ID == focus && !win.Minimized {
			bg, fg = th.TitleFocused, th.TitleFG
		} else if win.Minimized {
			bg = cell.RGB(0x60, 0x60, 0x60)
			fg = th.TaskbarFG
		}
		if c.attention[win.ID] && (time.Now().UnixMilli()/400)%2 == 0 {
			bg = cell.RGB(0xFF, 0xFF, 0x00)
			fg = cell.RGB(0x00, 0x00, 0x00)
		}
		for i := 0; i < b.width; i++ {
			c.set(b.start+i, tby, ' ', fg, bg, 0)
		}
		c.drawString(b.start, tby, label, fg, bg, 0)
	}
	c.drawString(tray.bellX, tby, bellLabel, th.TaskbarFG, bellBG, 0)
	sepX := tray.clockX - 3
	if sepX > tray.bellX+tray.bellW-1 {
		c.set(sepX, tby, ' ', th.TaskbarFG, th.TaskbarBG, 0)
		c.set(sepX+1, tby, '|', th.TaskbarFG, th.TaskbarBG, 0)
		c.set(sepX+2, tby, ' ', th.TaskbarFG, th.TaskbarBG, 0)
	}
	for i, r := range clock {
		c.set(tray.clockX+i, tby, r, th.TaskbarFG, th.TaskbarBG, 0)
	}
}

func (c *Client) drawDesktop(w, h int, th config.Theme) {
	_ = w
	_ = h
	x0, y0, x1, y1 := c.desktopRect()
	deskW := x1 - x0
	deskH := y1 - y0
	if deskW < 1 || deskH < 1 {
		return
	}
	if c.cfg.UseImageWallpaper() {
		cells := c.wall.Ensure(c.cfg.Wallpaper, deskW, deskH)
		if len(cells) == deskW*deskH {
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					ch := cells[(y-y0)*deskW+(x-x0)]
					c.set(x, y, ch.Rune, ch.FG, ch.BG, ch.Attr)
				}
			}
			return
		}
	}
	pat := th.PatternRune()
	solid := c.cfg.UseSolidDesktop() || !c.cfg.UsePatternDesktop() || pat == 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if solid {
				c.set(x, y, ' ', th.DesktopBG, th.DesktopBG, 0)
			} else {
				c.set(x, y, pat,
					cell.RGB(th.DesktopBG.R/2, th.DesktopBG.G/2, th.DesktopBG.B/2),
					th.DesktopBG, 0)
			}
		}
	}
}

func (c *Client) drawBanner() {
	if c.banner == nil {
		return
	}
	th := c.cfg.Theme
	bx, by, bw, bh := c.bannerRect()
	bg := th.MenuBG
	fg := th.MenuFG
	hdr := th.TitleFocused
	for row := 0; row < bh; row++ {
		for col := 0; col < bw; col++ {
			c.set(bx+col, by+row, ' ', fg, bg, 0)
		}
	}
	title := c.banner.Title
	if c.banner.Icon != "" {
		title = c.iconGlyph(c.banner.Icon) + " " + title
	}
	c.drawString(bx+1, by, uwidth.Truncate(title, bw-2), th.TitleFG, hdr, cell.AttrBold)
	c.drawString(bx+1, by+1, uwidth.Truncate(c.banner.Body, bw-2), fg, bg, 0)
	if bh > 3 {
		c.drawString(bx+1, by+bh-1, "click: center", cell.RGB(0x60, 0x60, 0x60), bg, 0)
	}
}

func (c *Client) drawNotifyCenter() {
	th := c.cfg.Theme
	cx, cy, cw, ch := c.centerRect()
	bg, fg := th.MenuBG, th.MenuFG
	for row := 0; row < ch; row++ {
		for col := 0; col < cw; col++ {
			c.set(cx+col, cy+row, ' ', fg, bg, 0)
		}
	}
	c.drawString(cx+1, cy, " Notification Center ", th.TitleFG, th.TitleFocused, cell.AttrBold)
	items := []notify.Notice{}
	if c.notify != nil {
		items = c.notify.List()
	}
	if len(items) == 0 {
		c.drawString(cx+2, cy+2, "(no notifications)", fg, bg, 0)
	} else {
		for i, n := range items {
			y := cy + 2 + i
			if y >= cy+ch-2 {
				break
			}
			line := n.Title
			if n.Icon != "" {
				line = c.iconGlyph(n.Icon) + " " + line
			}
			if n.Body != "" {
				line += " — " + n.Body
			}
			c.drawString(cx+1, y, uwidth.Truncate(line, cw-2), fg, bg, 0)
		}
	}
	c.drawString(cx+1, cy+ch-1, " click item=dismiss   bottom=dismiss all ", cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
}

func (c *Client) drawWindow(win *server.Window, cache *winCache, focused, fullChrome bool) {
	th := c.cfg.Theme
	border := th.BorderUnfocused
	titleBG := th.TitleUnfocused
	if focused {
		border = th.BorderFocused
		titleBG = th.TitleFocused
	}
	titleFG := th.TitleFG

	if fullChrome {
		for sy := 0; sy < win.H; sy++ {
			for sx := 0; sx < th.ShadowDX; sx++ {
				c.set(win.X+win.W+sx, win.Y+sy+th.ShadowDY, '█', th.Shadow, th.Shadow, 0)
			}
		}
		for sx := 0; sx < win.W; sx++ {
			for sy := 0; sy < th.ShadowDY; sy++ {
				c.set(win.X+sx+th.ShadowDX, win.Y+win.H+sy, '█', th.Shadow, th.Shadow, 0)
			}
		}
		c.set(win.X, win.Y, '╔', border, titleBG, 0)
		c.set(win.X+win.W-1, win.Y, '╗', border, titleBG, 0)
		c.set(win.X, win.Y+win.H-1, '╚', border, th.WindowBody, 0)
		c.set(win.X+win.W-1, win.Y+win.H-1, '╝', border, th.WindowBody, 0)
		for x := 1; x < win.W-1; x++ {
			c.set(win.X+x, win.Y, '═', border, titleBG, 0)
			c.set(win.X+x, win.Y+win.H-1, '═', border, th.WindowBody, 0)
		}
		for y := 1; y < win.H-1; y++ {
			c.set(win.X, win.Y+y, '║', border, th.WindowBody, 0)
			c.set(win.X+win.W-1, win.Y+y, '║', border, th.WindowBody, 0)
		}
		// Discoverable corner grips on focused windows
		if focused && !win.Maximized {
			grip := cell.RGB(0xFF, 0xFF, 0x00)
			c.set(win.X, win.Y, '╔', grip, titleBG, 0)
			c.set(win.X+win.W-1, win.Y, '╗', grip, titleBG, 0)
			c.set(win.X, win.Y+win.H-1, '╚', grip, th.WindowBody, 0)
			c.set(win.X+win.W-1, win.Y+win.H-1, '◢', grip, th.WindowBody, 0)
		}
		title := " " + win.Title + " "
		maxTitle := win.W - 10
		if maxTitle < 0 {
			maxTitle = 0
		}
		runes := []rune(title)
		if len(runes) > maxTitle {
			runes = runes[:maxTitle]
		}
		for i, r := range runes {
			c.set(win.X+1+i, win.Y, r, titleFG, titleBG, cell.AttrBold)
		}
		if win.W >= 8 {
			c.set(win.X+win.W-6, win.Y, '▬', titleFG, titleBG, 0) // minimize
			c.set(win.X+win.W-4, win.Y, '□', titleFG, titleBG, 0)
			c.set(win.X+win.W-2, win.Y, 'X', titleFG, titleBG, cell.AttrBold)
		}
	}

	cc, cr := win.ContentSize()
	var cx, cy int
	curVis := false
	if focused {
		if cp, ok := win.Surface.(surface.CursorProvider); ok {
			cx, cy, curVis = cp.Cursor()
		}
	}
	for row := 0; row < cr && row < cache.rows; row++ {
		for col := 0; col < cc && col < cache.cols; col++ {
			i := row*cache.cols + col
			ch := cell.Cell{Rune: ' ', FG: th.DefaultFG, BG: th.DefaultBG}
			if i < len(cache.cells) {
				ch = cache.cells[i]
			}
			fg, bg, attr := ch.FG, ch.BG, ch.Attr
			if c.inSelection(win.ID, col, row) {
				fg, bg = bg, fg
				if fg.Equals(bg) {
					fg = cell.RGB(0, 0, 0)
					bg = cell.RGB(0xAA, 0xAA, 0xFF)
				}
			}
			if c.findOpen && c.findWin == win.ID && c.findQuery != "" {
				if findMatchAt(cache.cells, cache.cols, row, col, c.findQuery) {
					fg = cell.RGB(0x00, 0x00, 0x00)
					bg = cell.RGB(0xFF, 0xFF, 0x00)
				}
			}
			if curVis && c.cursorOn && col == cx && row == cy {
				fg, bg = bg, fg
				if ch.Rune == 0 || ch.Rune == ' ' {
					ch.Rune = ' '
				}
			}
			c.set(win.X+1+col, win.Y+1+row, ch.Rune, fg, bg, attr)
		}
	}
	c.drawScrollbackBar(win)
}
