package server

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ttypedesk/ttypedesk/apps/addprog"
	"github.com/ttypedesk/ttypedesk/apps/calendar"
	"github.com/ttypedesk/ttypedesk/apps/clock"
	"github.com/ttypedesk/ttypedesk/apps/files"
	"github.com/ttypedesk/ttypedesk/apps/imageview"
	"github.com/ttypedesk/ttypedesk/apps/manual"
	"github.com/ttypedesk/ttypedesk/apps/mgrprog"
	"github.com/ttypedesk/ttypedesk/apps/notes"
	"github.com/ttypedesk/ttypedesk/apps/settings"
	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/notify"
	"github.com/ttypedesk/ttypedesk/internal/proto"
	"github.com/ttypedesk/ttypedesk/internal/session"
	"github.com/ttypedesk/ttypedesk/internal/slog"
	"github.com/ttypedesk/ttypedesk/internal/surface"
)

// Window is a floating managed window.
type Window struct {
	ID          string
	Title       string
	TitlePinned bool
	Launch      string // recreate action: terminal, notes, prog:id, …
	X, Y        int
	W, H        int // outer size including chrome
	Z           int
	Maximized   bool
	Minimized   bool
	Restore     struct{ X, Y, W, H int }
	Surface     surface.Surface
	Kind        string
}

// content size inside chrome: title(1)+borders(2) horizontal, borders+title
func (w *Window) ContentSize() (cols, rows int) {
	cols = w.W - 2
	rows = w.H - 2 // title row is top border area with title
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return
}

// Server owns windows and surfaces.
type Server struct {
	mu       sync.Mutex
	cfg      config.Config
	onConfig func(config.Config)
	notify   *notify.Service
	windows  map[string]*Window
	order    []string // bottom to top z
	focus    string
	nextID   atomic.Uint64
	nextZ    int
	hostCols int
	hostRows int
	dock     string // top | bottom | left | right
}

func New(cfg config.Config) *Server {
	n := notify.New()
	n.Configure(cfg.Notify.Persist, cfg.Notify.MaxHist)
	return &Server{
		cfg:     cfg,
		windows: make(map[string]*Window),
		notify:  n,
		dock:    cfg.TaskbarDock(),
	}
}

func (s *Server) NotifyService() *notify.Service {
	return s.notify
}

func (s *Server) SetNotify(n *notify.Service) {
	s.mu.Lock()
	s.notify = n
	s.mu.Unlock()
}

func (s *Server) SetConfigListener(fn func(config.Config)) {
	s.mu.Lock()
	s.onConfig = fn
	s.mu.Unlock()
}

func (s *Server) SetConfig(cfg config.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.dock = cfg.TaskbarDock()
	n := s.notify
	s.mu.Unlock()
	if n != nil {
		n.Configure(cfg.Notify.Persist, cfg.Notify.MaxHist)
	}
}

func (s *Server) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Server) SetDock(dock string) {
	s.mu.Lock()
	s.dock = dock
	s.mu.Unlock()
}

// desktopInset returns top/bottom/left/right reserved for the taskbar.
func (s *Server) desktopInset() (top, bottom, left, right int) {
	switch s.dock {
	case "bottom":
		return 0, 1, 0, 0
	case "left":
		return 0, 0, 7, 0 // wide enough for " Start "
	case "right":
		return 0, 0, 0, 7
	default:
		return 1, 0, 0, 0
	}
}

// LaunchAction runs a desktop/Start-menu action string.
func (s *Server) LaunchAction(action string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in LaunchAction %q: %v", action, r)
			err = fmt.Errorf("launch panic: %v", r)
		}
	}()
	slog.Info("LaunchAction %q", action)
	err = s.launchAction(action)
	if err != nil {
		slog.Error("LaunchAction %q failed: %v", action, err)
	}
	return err
}

// OpenPath opens a file with default-app associations, or a directory in Files.
func (s *Server) OpenPath(path string) error {
	path = filepath.Clean(path)
	fi, err := os.Stat(path)
	if err != nil {
		if s.notify != nil {
			s.notify.Post("Open", err.Error(), "⚠", "open")
		}
		return err
	}
	if fi.IsDir() {
		return s.LaunchAction("files:" + path)
	}
	action, ok := s.Config().OpenAction(path)
	if !ok {
		msg := config.FormatMissingAssoc(path)
		if s.notify != nil {
			s.notify.Post("No default app", msg, "📄", "open")
		}
		return fmt.Errorf("%s", msg)
	}
	return s.launchWithFile(action, path)
}

func (s *Server) launchWithFile(action, path string) error {
	action = strings.TrimSpace(action)
	switch {
	case action == "image" || action == "imageview":
		_, err := s.CreateGfx(filepath.Base(path), path)
		return err
	case strings.HasPrefix(action, "image:"):
		_, err := s.CreateGfx(filepath.Base(path), action[6:])
		return err
	case strings.HasPrefix(action, "pty:"):
		parts := strings.Fields(action[4:])
		if len(parts) == 0 {
			return fmt.Errorf("empty pty command")
		}
		cmd := parts[0]
		args := append(append([]string{}, parts[1:]...), path)
		title := filepath.Base(path)
		_, err := s.CreatePtyCmd(title, cmd, args)
		return err
	case strings.HasPrefix(action, "prog:"):
		return s.LaunchAction(action)
	case action == "notes":
		_, err := s.CreateApp("notes", "Notes")
		return err
	default:
		// Unknown action — try as launch string; if it's just an editor binary name, wrap as pty
		if !strings.Contains(action, ":") {
			_, err := s.CreatePtyCmd(filepath.Base(path), action, []string{path})
			return err
		}
		return s.LaunchAction(action)
	}
}

func (s *Server) launchAction(action string) error {
	action = strings.TrimSpace(action)
	orig := action
	if expanded := s.Config().ExpandLaunch(action); expanded != action {
		if expanded == "" {
			msg := fmt.Sprintf("Unresolved app role in %q — set it in Settings → Default apps", orig)
			if s.notify != nil {
				s.notify.Post("No default app", msg, "⚠", "launch")
			}
			return fmt.Errorf("%s", msg)
		}
		action = expanded
	} else if strings.HasPrefix(strings.ToLower(action), "role:") {
		msg := fmt.Sprintf("Unresolved app role in %q — set it in Settings → Default apps", orig)
		if s.notify != nil {
			s.notify.Post("No default app", msg, "⚠", "launch")
		}
		return fmt.Errorf("%s", msg)
	}
	switch action {
	case "", "terminal":
		_, err := s.CreatePty("Terminal")
		return err
	case "notes":
		_, err := s.CreateApp("notes", "Notes")
		return err
	case "clock":
		_, err := s.CreateApp("clock", "Clock")
		return err
	case "calendar":
		return s.FocusOrCreateApp("calendar", "Calendar")
	case "settings":
		return s.FocusOrCreateApp("settings", "Settings")
	case "manual", "help":
		return s.FocusOrCreateApp("manual", "Manual")
	case "image", "imageview":
		_, err := s.CreateGfx("Image Viewer", "")
		return err
	case "files", "filemgr":
		_, err := s.CreateApp("files", "Files")
		return err
	case "addprog", "add-program":
		_, err := s.CreateApp("addprog", "Add Program")
		return err
	case "mgrprog", "manage-programs", "removeprog":
		_, err := s.CreateApp("mgrprog", "Manage Programs")
		return err
	default:
		if len(action) > 5 && action[:5] == "open:" {
			return s.OpenPath(action[5:])
		}
		if len(action) > 6 && action[:6] == "files:" {
			p := action[6:]
			switch p {
			case "home", "~":
				p, _ = os.UserHomeDir()
			case "trash":
				home, _ := os.UserHomeDir()
				p = filepath.Join(home, ".local", "share", "Trash", "files")
				_ = os.MkdirAll(p, 0o755)
			case "system", "manual":
				_ = manual.EnsureSystemFolder()
				p = manual.Dir()
			}
			_, err := s.CreateAppPath("files", "Files", p)
			return err
		}
		if len(action) > 6 && action[:6] == "image:" {
			_, err := s.CreateGfx("Image Viewer", action[6:])
			return err
		}
		if len(action) > 5 && action[:5] == "prog:" {
			id := action[5:]
			p := s.Config().FindProgram(id)
			if p == nil {
				return fmt.Errorf("unknown program %q", id)
			}
			_, err := s.CreateProgram(*p)
			return err
		}
		if len(action) > 4 && action[:4] == "pty:" {
			parts := strings.Fields(action[4:])
			if len(parts) == 0 {
				return fmt.Errorf("empty pty command")
			}
			_, err := s.CreatePtyCmd(parts[0], parts[0], parts[1:])
			return err
		}
		if len(action) > 4 && action[:4] == "app:" {
			_, err := s.CreateApp(action[4:], action[4:])
			return err
		}
		_, err := s.CreateApp(action, action)
		return err
	}
}

// FocusOrCreateApp focuses an existing app window by launch id or title, or creates one.
func (s *Server) FocusOrCreateApp(appName, title string) error {
	s.mu.Lock()
	for _, id := range s.order {
		w := s.windows[id]
		if w == nil {
			continue
		}
		if w.Launch == appName || w.Title == title || strings.HasPrefix(w.Title, title+" —") || strings.HasPrefix(w.Title, title+" - ") {
			s.mu.Unlock()
			s.Focus(id)
			return nil
		}
	}
	s.mu.Unlock()
	_, err := s.CreateApp(appName, title)
	return err
}

func (s *Server) SetHostSize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostCols, s.hostRows = cols, rows
}

func (s *Server) Focused() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.focus
}

func (s *Server) Windows() []*Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Window, 0, len(s.order))
	for _, id := range s.order {
		if w, ok := s.windows[id]; ok {
			out = append(out, w)
		}
	}
	return out
}

func (s *Server) Get(id string) *Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.windows[id]
}

func (s *Server) CreatePty(title string) (*Window, error) {
	return s.createPty("terminal", title, s.cfg.Shell, nil, false)
}

// CreatePtyCmd spawns a PTY running command with optional args.
func (s *Server) CreatePtyCmd(title, command string, args []string) (*Window, error) {
	launch := "pty:" + command
	if len(args) > 0 {
		launch = "pty:" + command + " " + strings.Join(args, " ")
	}
	return s.createPty(launch, title, command, args, false)
}

// CreatePtyCmdPinned keeps the desk title (custom registered programs).
func (s *Server) CreatePtyCmdPinned(title, command string, args []string) (*Window, error) {
	return s.createPty("", title, command, args, true) // launch set by caller via CreateProgram
}

// CreateProgram opens a registered program with pinned title and prog: launch key.
func (s *Server) CreateProgram(p config.Program) (*Window, error) {
	shell := s.cfg.Shell
	name := p.Name
	if name == "" {
		name = p.Command
	}
	return s.createPty(config.ProgramAction(p.ID), name, shell, []string{"-c", p.Command}, true)
}

func (s *Server) createPty(launch, title, command string, args []string, pinTitle bool) (*Window, error) {
	s.mu.Lock()
	win, err := s.createLocked("pty", title, "", "", command, args, pinTitle)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if launch != "" {
		s.mu.Lock()
		win.Launch = launch
		s.mu.Unlock()
	} else if pinTitle {
		s.mu.Lock()
		win.Launch = "terminal"
		s.mu.Unlock()
	} else {
		s.mu.Lock()
		win.Launch = "terminal"
		s.mu.Unlock()
	}
	return win, nil
}

func (s *Server) CreateApp(appName, title string) (*Window, error) {
	return s.CreateAppPath(appName, title, "")
}

// CreateAppPath creates a native app with an optional path (Files start dir, etc.).
func (s *Server) CreateAppPath(appName, title, path string) (*Window, error) {
	s.mu.Lock()
	win, err := s.createLocked("app", title, appName, path, "", nil, false)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	win.Launch = appName
	if appName == "files" && path != "" {
		win.Launch = "files:" + path
	}
	if appName == "imageview" {
		win.Launch = "image"
	}
	s.mu.Unlock()
	if err := s.bindNativeApp(win); err != nil {
		slog.Error("BindHost failed id=%s: %v", win.ID, err)
		s.CloseWindow(win.ID)
		return nil, err
	}
	return win, nil
}

func (s *Server) CreateGfx(title, path string) (*Window, error) {
	s.mu.Lock()
	win, err := s.createLocked("gfx", title, "imageview", path, "", nil, false)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if path != "" {
		win.Launch = "image:" + path
	} else {
		win.Launch = "image"
	}
	s.mu.Unlock()
	if err := s.bindNativeApp(win); err != nil {
		slog.Error("BindHost failed id=%s: %v", win.ID, err)
		s.CloseWindow(win.ID)
		return nil, err
	}
	return win, nil
}

func (s *Server) create(kind, title, appName, path string) (*Window, error) {
	s.mu.Lock()
	win, err := s.createLocked(kind, title, appName, path, "", nil, false)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := s.bindNativeApp(win); err != nil {
		s.CloseWindow(win.ID)
		return nil, err
	}
	return win, nil
}

func (s *Server) createLocked(kind, title, appName, path, command string, args []string, pinTitle bool) (*Window, error) {
	id := fmt.Sprintf("w%d", s.nextID.Add(1))
	w, h := 60, 20
	x, y := 4+int(s.nextID.Load()%5)*3, 3+int(s.nextID.Load()%4)*2
	if s.hostCols > 0 && x+w >= s.hostCols {
		x = 2
	}
	if s.hostRows > 0 && y+h >= s.hostRows {
		top, _, _, _ := s.desktopInset()
		y = top + 1
		if y < 1 {
			y = 1
		}
	}

	cc, cr := w-2, h-2
	var surf surface.Surface
	var err error
	switch kind {
	case "pty":
		cmd := command
		if cmd == "" {
			cmd = s.cfg.Shell
		}
		surf, err = surface.NewPtySurfaceOpts(id, surface.PtyOpts{
			Command: cmd, Args: args, Cols: cc, Rows: cr, Scrollback: s.cfg.Scrollback, Title: title,
		})
		if title == "" {
			title = "Terminal"
		}
	case "app":
		switch appName {
		case "clock":
			surf, err = surface.NewAppSurface(id, "Clock", clock.New(), cc, cr)
			title = "Clock"
		case "calendar":
			surf, err = surface.NewAppSurface(id, "Calendar", calendar.New(), cc, cr)
			title = "Calendar"
		case "notes":
			surf, err = surface.NewAppSurface(id, "Notes", notes.New(), cc, cr)
			title = "Notes"
		case "files":
			start := path
			if start == "" {
				fc := s.cfg.Files
				switch fc.StartDir {
				case "last":
					start = fc.LastDir
				case "home", "":
					start, _ = os.UserHomeDir()
				default:
					start = fc.StartDir
				}
			}
			surf, err = surface.NewAppSurface(id, "Files", files.New(start, s.cfg, func(nc config.Config) {
				_ = config.Save(nc)
				s.SetConfig(nc)
				if s.onConfig != nil {
					s.onConfig(nc)
				}
			}), cc, cr)
			title = "Files"
		case "settings":
			cfgSnap := s.cfg
			n := s.notify
			surf, err = surface.NewAppSurface(id, "Settings", settings.New(cfgSnap, func(nc config.Config) {
				_ = config.Save(nc)
				s.SetConfig(nc)
				if s.onConfig != nil {
					s.onConfig(nc)
				}
			}, func(title, body string) {
				if n != nil {
					n.Post(title, body, "🔔", "settings")
				}
			}, func() {
				if n != nil {
					n.DismissAll()
				}
			}), cc, cr)
			title = "Settings"
		case "manual", "help":
			w, h = 72, 22
			cc, cr = w-2, h-2
			_ = manual.EnsureSystemFolder()
			surf, err = surface.NewAppSurface(id, "Manual", manual.New(), cc, cr)
			title = "Manual"
		case "addprog":
			cfgSnap := s.cfg
			w, h = 56, 15
			cc, cr = w-2, h-2
			surf, err = surface.NewAppSurface(id, "Add Program", addprog.New(cfgSnap, func(nc config.Config) {
				_ = config.Save(nc)
				s.SetConfig(nc)
				if s.onConfig != nil {
					s.onConfig(nc)
				}
			}), cc, cr)
			title = "Add Program"
		case "mgrprog":
			cfgSnap := s.cfg
			w, h = 56, 16
			cc, cr = w-2, h-2
			surf, err = surface.NewAppSurface(id, "Manage Programs", mgrprog.New(cfgSnap, func(nc config.Config) {
				_ = config.Save(nc)
				s.SetConfig(nc)
				if s.onConfig != nil {
					s.onConfig(nc)
				}
			}), cc, cr)
			title = "Manage Programs"
		case "imageview":
			app := imageview.NewDemo()
			if path != "" {
				app = imageview.New(path)
			}
			surf, err = surface.NewAppSurface(id, "Image Viewer", app, cc, cr)
			title = "Image Viewer"
		default:
			return nil, fmt.Errorf("unknown app %q", appName)
		}
		kind = "app"
	case "gfx":
		app := imageview.NewDemo()
		if path != "" {
			app = imageview.New(path)
		}
		surf, err = surface.NewAppSurface(id, "Image Viewer", app, cc, cr)
		title = "Image Viewer"
		kind = "app"
	default:
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	if err != nil {
		return nil, err
	}

	s.nextZ++
	win := &Window{
		ID:          id,
		Title:       title,
		TitlePinned: pinTitle,
		X:           x,
		Y:           y,
		W:           w,
		H:           h,
		Z:           s.nextZ,
		Surface:     surf,
		Kind:        kind,
	}
	s.windows[id] = win
	s.order = append(s.order, id)
	s.focus = id
	// BindHost is done by the caller AFTER releasing s.mu (apps may save config / OpenPath).
	slog.Info("window created id=%s kind=%s title=%q %dx%d @%d,%d", id, kind, title, w, h, x, y)
	return win, nil
}

// bindNativeApp initializes AppSurface hosts outside Server.mu.
func (s *Server) bindNativeApp(win *Window) error {
	if win == nil {
		return nil
	}
	as, ok := win.Surface.(*surface.AppSurface)
	if !ok {
		return nil
	}
	hid := win.ID
	return as.BindHost(&appHost{srv: s, id: hid}, func(t string) {
		s.mu.Lock()
		if w := s.windows[hid]; w != nil {
			if !w.TitlePinned {
				w.Title = t
			}
		}
		s.mu.Unlock()
	}, func() {
		go s.CloseWindow(hid)
	})
}

// SetWindowTitle updates a window's title bar text.
func (s *Server) SetWindowTitle(id, title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w, ok := s.windows[id]; ok {
		w.Title = title
	}
}

func (s *Server) CloseWindow(id string) {
	s.mu.Lock()
	w, ok := s.windows[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.windows, id)
	for i, oid := range s.order {
		if oid == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	if s.focus == id {
		s.focus = ""
		if len(s.order) > 0 {
			s.focus = s.order[len(s.order)-1]
		}
	}
	surf := w.Surface
	s.mu.Unlock()
	// Close outside the server lock — apps may save config (SetConfig) in Close.
	if surf != nil {
		_ = surf.Close()
	}
}

func (s *Server) Focus(id string) {
	s.mu.Lock()
	prev := s.focus
	w, ok := s.windows[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	w.Minimized = false
	s.focus = id
	s.nextZ++
	w.Z = s.nextZ
	for i, oid := range s.order {
		if oid == id {
			s.order = append(append(s.order[:i], s.order[i+1:]...), id)
			break
		}
	}
	var prevSurf, nextSurf surface.Surface
	if prev != "" && prev != id {
		if pw := s.windows[prev]; pw != nil {
			prevSurf = pw.Surface
		}
	}
	nextSurf = w.Surface
	s.mu.Unlock()
	if fn, ok := prevSurf.(interface{ NotifyFocus(bool) }); ok {
		fn.NotifyFocus(false)
	}
	if fn, ok := nextSurf.(interface{ NotifyFocus(bool) }); ok {
		fn.NotifyFocus(true)
	}
}

func (s *Server) Move(id string, x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.windows[id]
	if w == nil || w.Maximized {
		return
	}
	top, _, left, _ := s.desktopInset()
	if x < left {
		x = left
	}
	if y < top {
		y = top
	}
	w.X, w.Y = x, y
}

func (s *Server) ResizeWindow(id string, w, h int) {
	s.mu.Lock()
	win := s.windows[id]
	if win == nil || win.Maximized {
		s.mu.Unlock()
		return
	}
	cc, cr := s.setGeomLocked(win, win.X, win.Y, w, h)
	surf := win.Surface
	s.mu.Unlock()
	safeSurfResize(surf, cc, cr)
}

// SetGeometry moves and resizes a window (used for edge/corner drag).
func (s *Server) SetGeometry(id string, x, y, w, h int) {
	s.mu.Lock()
	win := s.windows[id]
	if win == nil || win.Maximized {
		s.mu.Unlock()
		return
	}
	cc, cr := s.setGeomLocked(win, x, y, w, h)
	surf := win.Surface
	s.mu.Unlock()
	safeSurfResize(surf, cc, cr)
}

// setGeomLocked updates window chrome geometry. Caller must Resize the surface OUTSIDE s.mu.
func (s *Server) setGeomLocked(win *Window, x, y, w, h int) (cc, cr int) {
	top, bottom, left, right := s.desktopInset()
	if w < 12 {
		w = 12
	}
	if h < 5 {
		h = 5
	}
	if y < top {
		y = top
	}
	if x < left {
		x = left
	}
	maxW := s.hostCols - left - right
	maxH := s.hostRows - top - bottom
	if maxW < 12 {
		maxW = 12
	}
	if maxH < 5 {
		maxH = 5
	}
	if s.hostCols > 0 && x+w > s.hostCols-right {
		if w <= maxW {
			x = s.hostCols - right - w
		} else {
			w = maxW
			x = left
		}
	}
	if s.hostRows > 0 && y+h > s.hostRows-bottom {
		if h <= maxH {
			y = s.hostRows - bottom - h
		} else {
			h = maxH
			y = top
		}
	}
	win.X, win.Y, win.W, win.H = x, y, w, h
	return win.ContentSize()
}

func safeSurfResize(surf surface.Surface, cc, cr int) {
	if surf == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in Surface.Resize: %v\n%s", r, debug.Stack())
		}
	}()
	surf.Resize(cc, cr)
}

func (s *Server) ToggleMaximize(id string) {
	s.mu.Lock()
	win := s.windows[id]
	if win == nil {
		s.mu.Unlock()
		return
	}
	win.Minimized = false
	if win.Maximized {
		win.X, win.Y, win.W, win.H = win.Restore.X, win.Restore.Y, win.Restore.W, win.Restore.H
		win.Maximized = false
	} else {
		s.saveRestoreLocked(win)
		top, bottom, left, right := s.desktopInset()
		win.X, win.Y = left, top
		win.W = s.hostCols - left - right
		win.H = s.hostRows - top - bottom
		if win.W < 12 {
			win.W = 12
		}
		if win.H < 5 {
			win.H = 5
		}
		win.Maximized = true
	}
	cc, cr := win.ContentSize()
	surf := win.Surface
	s.mu.Unlock()
	safeSurfResize(surf, cc, cr)
}

func (s *Server) saveRestoreLocked(win *Window) {
	if win.Maximized {
		return
	}
	// Keep existing restore when already snapped (half tile).
	if win.Restore.W > 0 && win.Restore.H > 0 {
		dx0, dy0, dx1, dy1 := s.desktopBounds()
		dw, dh := dx1-dx0, dy1-dy0
		// Heuristic: if current size looks like a tile, don't overwrite restore.
		halfW := dw / 2
		halfH := dh / 2
		tiled := (win.W >= halfW-2 && win.W <= halfW+2 && win.H >= dh-2) ||
			(win.H >= halfH-2 && win.H <= halfH+2 && win.W >= dw-2)
		if tiled {
			return
		}
	}
	win.Restore = struct{ X, Y, W, H int }{win.X, win.Y, win.W, win.H}
}

func (s *Server) desktopBounds() (x0, y0, x1, y1 int) {
	top, bottom, left, right := s.desktopInset()
	return left, top, s.hostCols - right, s.hostRows - bottom
}

// Snap tiles the window into a desktop region: left|right|top|bottom.
// Snapping again to the same region restores the previous geometry.
func (s *Server) Snap(id, region string) {
	s.mu.Lock()
	win := s.windows[id]
	if win == nil || win.Minimized {
		s.mu.Unlock()
		return
	}
	x0, y0, x1, y1 := s.desktopBounds()
	dw, dh := x1-x0, y1-y0
	if dw < 12 || dh < 5 {
		s.mu.Unlock()
		return
	}
	region = strings.ToLower(region)
	var cc, cr int
	var surf surface.Surface
	if region == "restore" {
		cc, cr, surf = s.restoreGeomLocked(win)
		s.mu.Unlock()
		safeSurfResize(surf, cc, cr)
		return
	}
	tx, ty, tw, th := snapGeom(region, x0, y0, dw, dh)
	if tw < 12 || th < 5 {
		s.mu.Unlock()
		return
	}
	// Same tile again → restore
	if !win.Maximized && near(win.X, tx, 1) && near(win.Y, ty, 1) && near(win.W, tw, 2) && near(win.H, th, 2) {
		cc, cr, surf = s.restoreGeomLocked(win)
		s.mu.Unlock()
		safeSurfResize(surf, cc, cr)
		return
	}
	s.saveRestoreLocked(win)
	win.Maximized = false
	cc, cr = s.setGeomLocked(win, tx, ty, tw, th)
	surf = win.Surface
	s.mu.Unlock()
	safeSurfResize(surf, cc, cr)
}

func snapGeom(region string, x0, y0, dw, dh int) (x, y, w, h int) {
	switch region {
	case "left":
		return x0, y0, dw / 2, dh
	case "right":
		w = dw - dw/2
		return x0 + dw/2, y0, w, dh
	case "top":
		return x0, y0, dw, dh / 2
	case "bottom":
		h = dh - dh/2
		return x0, y0 + dh/2, dw, h
	default:
		return 0, 0, 0, 0
	}
}

func near(a, b, tol int) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func (s *Server) restoreGeomLocked(win *Window) (cc, cr int, surf surface.Surface) {
	if win.Restore.W < 12 || win.Restore.H < 5 {
		return 0, 0, nil
	}
	rx, ry, rw, rh := win.Restore.X, win.Restore.Y, win.Restore.W, win.Restore.H
	win.Maximized = false
	cc, cr = s.setGeomLocked(win, rx, ry, rw, rh)
	return cc, cr, win.Surface
}

func (s *Server) ToggleMinimize(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	win := s.windows[id]
	if win == nil {
		return
	}
	win.Minimized = !win.Minimized
	if !win.Minimized {
		s.focus = id
		s.nextZ++
		win.Z = s.nextZ
		for i, oid := range s.order {
			if oid == id {
				s.order = append(append(s.order[:i], s.order[i+1:]...), id)
				break
			}
		}
	} else if s.focus == id {
		s.focus = ""
		for i := len(s.order) - 1; i >= 0; i-- {
			w := s.windows[s.order[i]]
			if w != nil && !w.Minimized {
				s.focus = w.ID
				break
			}
		}
	}
}

func (s *Server) Nudge(id string, dx, dy, dw, dh int) {
	s.mu.Lock()
	win := s.windows[id]
	if win == nil || win.Maximized || win.Minimized {
		s.mu.Unlock()
		return
	}
	win.X += dx
	win.Y += dy
	top, _, left, _ := s.desktopInset()
	if win.Y < top {
		win.Y = top
	}
	if win.X < left {
		win.X = left
	}
	var cc, cr int
	var surf surface.Surface
	needResize := false
	if dw != 0 || dh != 0 {
		win.W += dw
		win.H += dh
		if win.W < 12 {
			win.W = 12
		}
		if win.H < 5 {
			win.H = 5
		}
		cc, cr = win.ContentSize()
		surf = win.Surface
		needResize = true
	}
	s.mu.Unlock()
	if needResize {
		safeSurfResize(surf, cc, cr)
	}
}

func (s *Server) HandleInput(id string, ev surface.InputEvent) {
	s.mu.Lock()
	w := s.windows[id]
	s.mu.Unlock()
	if w == nil || w.Surface == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in HandleInput id=%s: %v\n%s", id, r, debug.Stack())
		}
	}()
	_ = w.Surface.HandleInput(ev)
}

func (s *Server) RefreshTitles() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.windows {
		if w.TitlePinned {
			continue
		}
		if t := w.Surface.Title(); t != "" {
			w.Title = t
		}
	}
}

// CaptureSession builds a restorable snapshot of open windows.
func (s *Server) CaptureSession() session.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := session.State{Windows: make([]session.Entry, 0, len(s.order))}
	focus := s.focus
	for _, id := range s.order {
		w := s.windows[id]
		if w == nil || w.Launch == "" {
			continue
		}
		// Skip transient setup dialogs
		switch w.Launch {
		case "addprog", "mgrprog":
			continue
		}
		x, y, ww, hh := w.X, w.Y, w.W, w.H
		if w.Maximized {
			x, y, ww, hh = w.Restore.X, w.Restore.Y, w.Restore.W, w.Restore.H
		}
		st.Windows = append(st.Windows, session.Entry{
			Action:      w.Launch,
			Title:       w.Title,
			X:           x,
			Y:           y,
			W:           ww,
			H:           hh,
			Maximized:   w.Maximized,
			Minimized:   w.Minimized,
			TitlePinned: w.TitlePinned,
			Focused:     w.ID == focus,
		})
	}
	return st
}

// RestoreSession recreates windows from a saved session (geometry applied after create).
func (s *Server) RestoreSession(st session.State) int {
	n := 0
	var focusID string
	for _, e := range st.Windows {
		if e.Action == "" {
			continue
		}
		before := s.Windows()
		if err := s.LaunchAction(e.Action); err != nil {
			slog.Warn("session restore %q: %v", e.Action, err)
			continue
		}
		after := s.Windows()
		var win *Window
		if len(after) > len(before) {
			win = after[len(after)-1]
		} else if len(after) > 0 {
			win = after[len(after)-1]
		}
		if win == nil {
			continue
		}
		if e.W >= 12 && e.H >= 5 {
			s.SetGeometry(win.ID, e.X, e.Y, e.W, e.H)
		}
		if e.Maximized {
			s.ToggleMaximize(win.ID)
		}
		if e.Minimized {
			s.ToggleMinimize(win.ID)
		}
		if e.TitlePinned && e.Title != "" {
			s.SetWindowTitle(win.ID, e.Title)
			s.mu.Lock()
			if w := s.windows[win.ID]; w != nil {
				w.TitlePinned = true
			}
			s.mu.Unlock()
		}
		if e.Focused {
			focusID = win.ID
		}
		n++
	}
	if focusID != "" {
		s.Focus(focusID)
	}
	slog.Info("session restored %d window(s)", n)
	return n
}

func (s *Server) Snapshot() proto.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := proto.Snapshot{Cols: s.hostCols, Rows: s.hostRows}
	for _, id := range s.order {
		w := s.windows[id]
		if w == nil {
			continue
		}
		cc, cr := w.ContentSize()
		sw := proto.SnapshotWindow{
			ID:        w.ID,
			Title:     w.Title,
			X:         w.X,
			Y:         w.Y,
			W:         w.W,
			H:         w.H,
			Z:         w.Z,
			Focused:   w.ID == s.focus,
			Maximized: w.Maximized,
			Kind:      w.Kind,
			Cols:      cc,
			Rows:      cr,
			Cells:     w.Surface.Snapshot(),
		}
		snap.Windows = append(snap.Windows, sw)
	}
	return snap
}

func (s *Server) CloseAll() {
	s.mu.Lock()
	wins := make([]*Window, 0, len(s.windows))
	for _, w := range s.windows {
		wins = append(wins, w)
	}
	s.windows = make(map[string]*Window)
	s.order = nil
	s.focus = ""
	s.mu.Unlock()
	for _, w := range wins {
		_ = w.Surface.Close()
	}
}
