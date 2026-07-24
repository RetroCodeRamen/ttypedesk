// Package appstore is the Start ▸ App Store window: a generic engine that
// fetches a catalog (index.json) from one or more configured GitHub repos
// (config.Config.AppSources), lets the user install an entry (runs its
// install script in a terminal window), and once an entry's Detect check
// passes, registers its launchers into Start ▸ Programs. No app-specific
// logic lives here — see https://github.com/RetroCodeRamen/ttypedesk-apps
// for the catalog format and the first entry (carbonyl "Internet").
package appstore

import (
	"fmt"
	"strings"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
	"github.com/ttypedesk/ttypedesk/pkg/uwidth"
)

type status int

const (
	statusNotInstalled status = iota
	statusInstalling
	statusInstalled
	statusError
)

type row struct {
	src    config.AppSource
	entry  RemoteEntry
	state  status
	errMsg string
}

type loadResult struct {
	rows []row
	err  string
}

type installResult struct {
	idx int
	err string
}

// App is the App Store window.
type App struct {
	cfg     config.Config
	onSave  func(config.Config)
	ctx     *uiapp.Context
	rows    []row
	sel     int
	loading bool
	loadErr string
	closing bool

	// confirmIdx is the row index awaiting a second Enter/click to trust
	// its source and proceed; -1 when nothing is pending.
	confirmIdx int

	loadCh    chan loadResult
	installCh chan installResult
}

func New(cfg config.Config, onSave func(config.Config)) *App {
	return &App{
		cfg:        cfg,
		onSave:     onSave,
		loading:    true,
		confirmIdx: -1,
		loadCh:     make(chan loadResult, 1),
		installCh:  make(chan installResult, 4),
	}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	sources := append([]config.AppSource{}, a.cfg.AppSources...)
	go a.loadCatalog(sources)
	ctx.StartTimer(2 * time.Second)
	return nil
}

func (a *App) Close() error { return nil }

func (a *App) Hooks() uiapp.Hooks {
	return uiapp.Hooks{
		OnInit: func(ctx *uiapp.Context) {
			ctx.SetTitle("App Store")
		},
	}
}

func (a *App) WantsClose() bool { return a.closing }

// loadCatalog runs on its own goroutine — a network fetch must never block
// the surface's Handle/Draw call path, which serializes the whole window.
// It only touches the copied sources slice and the two channels/ctx set
// once before this goroutine starts, never App fields directly.
func (a *App) loadCatalog(sources []config.AppSource) {
	var rows []row
	var errs []string
	for _, src := range sources {
		entries, err := fetchCatalog(src)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		for _, e := range entries {
			rows = append(rows, row{src: src, entry: e})
		}
	}
	res := loadResult{rows: rows}
	if len(rows) == 0 && len(errs) > 0 {
		res.err = strings.Join(errs, "; ")
	}
	a.loadCh <- res
	if a.ctx != nil {
		a.ctx.MarkDirty()
	}
}

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventKey:
		return a.key(e)
	case uiapp.EventMouse:
		return a.mouse(e)
	case uiapp.EventTimer:
		a.onTimer()
	}
	return nil
}

func (a *App) onTimer() {
	select {
	case res := <-a.loadCh:
		a.loading = false
		a.rows = res.rows
		a.loadErr = res.err
		a.confirmIdx = -1
		for i := range a.rows {
			if detectEntry(a.rows[i].entry) {
				a.rows[i].state = statusInstalled
				if registerEntry(&a.cfg, a.rows[i].entry) {
					a.persist()
				}
			}
		}
	default:
	}

	select {
	case res := <-a.installCh:
		if res.err != "" && res.idx >= 0 && res.idx < len(a.rows) {
			a.rows[res.idx].state = statusError
			a.rows[res.idx].errMsg = res.err
		}
	default:
	}

	for i := range a.rows {
		if a.rows[i].state != statusInstalling {
			continue
		}
		if detectEntry(a.rows[i].entry) {
			a.rows[i].state = statusInstalled
			if registerEntry(&a.cfg, a.rows[i].entry) {
				a.persist()
			}
			if a.ctx != nil {
				a.ctx.Notify("App Store", a.rows[i].entry.Name+" "+registrationNotice(a.rows[i].entry), "🛍")
			}
		}
	}
}

func (a *App) persist() {
	if a.onSave != nil {
		a.onSave(a.cfg)
	}
}

func (a *App) key(e uiapp.Event) error {
	n := len(a.rows)
	switch e.Key {
	case "Up":
		if a.sel > 0 {
			a.sel--
			a.confirmIdx = -1
		}
	case "Down":
		if a.sel+1 < n {
			a.sel++
			a.confirmIdx = -1
		}
	case "Enter":
		a.requestInstall()
	case "Escape":
		if a.confirmIdx != -1 {
			a.confirmIdx = -1
		} else {
			a.closing = true
		}
	default:
		if e.Rune == ' ' {
			a.requestInstall()
		}
	}
	return nil
}

func (a *App) mouse(e uiapp.Event) error {
	if e.Action == "drag" || e.Action == "release" {
		return nil
	}
	if e.Y < 3 {
		return nil
	}
	idx := e.Y - 3
	if idx < 0 || idx >= len(a.rows) {
		return nil
	}
	if idx == a.sel {
		a.requestInstall()
	} else {
		a.sel = idx
		a.confirmIdx = -1
	}
	return nil
}

// requestInstall activates the selected row, gating on source trust first.
// Install scripts run unsandboxed and may invoke sudo, so a source that
// hasn't been trusted yet gets one warning: the first Enter/click arms
// confirmIdx instead of installing, and a second Enter/click on the same
// row trusts the source (persisted to config) before proceeding.
func (a *App) requestInstall() {
	if a.sel < 0 || a.sel >= len(a.rows) {
		return
	}
	r := a.rows[a.sel]
	if r.state == statusInstalling || r.state == statusInstalled {
		return
	}
	if !r.src.Trusted && a.confirmIdx != a.sel {
		a.confirmIdx = a.sel
		return
	}
	a.confirmIdx = -1
	a.trustSource(a.sel)
	a.activate()
}

// trustSource marks the row's source trusted, both on the row (so the
// warning doesn't reappear this session) and in cfg.AppSources (so it
// persists). No-op if already trusted.
func (a *App) trustSource(idx int) {
	if idx < 0 || idx >= len(a.rows) || a.rows[idx].src.Trusted {
		return
	}
	a.rows[idx].src.Trusted = true
	for i := range a.cfg.AppSources {
		src := &a.cfg.AppSources[i]
		if src.Repo == a.rows[idx].src.Repo && src.Branch == a.rows[idx].src.Branch {
			src.Trusted = true
			break
		}
	}
	a.persist()
}

// activate installs the selected row if it's eligible (not already
// installed or mid-install). The actual network fetch + pty launch runs on
// its own goroutine for the same reason loadCatalog does.
func (a *App) activate() {
	if a.sel < 0 || a.sel >= len(a.rows) {
		return
	}
	r := &a.rows[a.sel]
	if r.state == statusInstalling || r.state == statusInstalled {
		return
	}
	r.state = statusInstalling
	r.errMsg = ""
	idx := a.sel
	src, entry, ctx := r.src, r.entry, a.ctx
	go func() {
		if err := installEntry(ctx, src, entry); err != nil {
			a.installCh <- installResult{idx: idx, err: err.Error()}
			if ctx != nil {
				ctx.MarkDirty()
			}
		}
	}()
}

// registrationNotice is the completion toast text — called out specially
// when an entry's install swaps a desktop-wide role (e.g. default file
// manager) rather than just adding a Start ▸ Programs entry.
func registrationNotice(e RemoteEntry) string {
	for _, w := range e.Register {
		if strings.EqualFold(w.SetRole, "filemgr") {
			return "installed — now your default file manager"
		}
	}
	return "installed — added to Start ▸ Programs"
}

func statusLabel(s status) string {
	switch s {
	case statusInstalling:
		return "Installing…"
	case statusInstalled:
		return "Installed"
	case statusError:
		return "Error — Enter to retry"
	default:
		return "Not installed — Enter to install"
	}
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	cv.FillRect(0, 0, cols, rows, bg)
	cv.DrawText(0, 0, " App Store ", cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)
	cv.DrawText(1, 1, "Install extra apps from your configured sources", fg, bg, 0)

	switch {
	case a.loading:
		cv.DrawText(1, 3, "Fetching catalog…", fg, bg, 0)
	case len(a.rows) == 0:
		msg := "(no apps available)"
		if a.loadErr != "" {
			msg = "Couldn't reach a source: " + a.loadErr
		}
		cv.DrawText(1, 3, uwidth.Truncate(msg, cols-2), fg, bg, 0)
	default:
		for i, r := range a.rows {
			y := 3 + i
			if y >= rows-2 {
				break
			}
			f, b := fg, bg
			if i == a.sel {
				f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
			}
			icon := r.entry.Icon
			if icon == "" {
				icon = "📦"
			}
			icon = uwidth.ASCIIIcon(icon, a.cfg.Palette.ASCIIIcons)
			line := fmt.Sprintf("%s %-20s [%s]", icon, r.entry.Name, statusLabel(r.state))
			if !r.src.Trusted && r.state != statusInstalled {
				line += "  " + uwidth.ASCIIIcon("⚠", a.cfg.Palette.ASCIIIcons) + " unverified source"
			}
			cv.DrawText(1, y, uwidth.Truncate(line, cols-2), f, b, 0)
		}
	}

	help := "↑↓ select  Enter install  Esc close"
	if a.sel >= 0 && a.sel < len(a.rows) {
		r := a.rows[a.sel]
		switch {
		case a.confirmIdx == a.sel:
			help = uwidth.ASCIIIcon("⚠", a.cfg.Palette.ASCIIIcons) + " " + r.src.Repo + " isn't trusted — install script runs unsandboxed, may use sudo. Enter again to trust & install, Esc to cancel."
		case r.state == statusInstalling:
			help = r.entry.Name + ": installing — see the new terminal window"
		case r.state == statusInstalled:
			help = r.entry.Name + ": installed"
		case r.state == statusError:
			help = r.entry.Name + ": " + r.errMsg
		default:
			if r.entry.Description != "" {
				help = r.entry.Description
			}
		}
	}
	if rows > 0 {
		cv.DrawText(0, rows-1, " "+uwidth.Truncate(help, cols-1), cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
	}
	return nil
}
