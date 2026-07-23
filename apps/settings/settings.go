package settings

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/slog"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// App is the TTYPE Desk Settings control panel.
type App struct {
	cfg       config.Config
	onSave    func(config.Config)
	onNotify  func(title, body string)
	onClearN  func()
	ctx       *uiapp.Context
	page      int // 0 menu, 1 appearance, 2 terminal, 3 desktop, 4 notify, 5 default apps, 6 apps, 7 input, 8 advanced, 9 about
	sel       int
	editing   bool
	editBuf   string
	status    string
	hkEdit    string // action id while editing a hotkey
}

func New(cfg config.Config, onSave func(config.Config), onNotify func(title, body string), onClearNotices ...func()) *App {
	a := &App{cfg: cfg, onSave: onSave, onNotify: onNotify, page: 0}
	if len(onClearNotices) > 0 {
		a.onClearN = onClearNotices[0]
	}
	return a
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	return nil
}
func (a *App) Close() error { return nil }

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventKey:
		return a.key(e)
	case uiapp.EventMouse:
		return a.mouse(e)
	}
	return nil
}

func (a *App) mouse(e uiapp.Event) error {
	if e.Action == "drag" || e.Action == "release" {
		return nil
	}
	if a.editing {
		return nil
	}
	// List rows start at y=2
	if e.Y < 2 {
		return nil
	}
	idx := e.Y - 2
	lines := a.lines()
	if idx < 0 || idx >= len(lines) {
		return nil
	}
	a.sel = idx
	a.activate()
	return nil
}

func (a *App) key(e uiapp.Event) error {
	if a.editing {
		switch e.Key {
		case "Enter":
			a.commitEdit()
			a.editing = false
			a.editBuf = ""
		case "Escape":
			a.editing = false
			a.editBuf = ""
			a.hkEdit = ""
		case "Backspace":
			r := []rune(a.editBuf)
			if len(r) > 0 {
				a.editBuf = string(r[:len(r)-1])
			}
		default:
			if e.Rune != 0 && !e.Ctrl {
				a.editBuf += string(e.Rune)
			}
		}
		return nil
	}

	switch e.Key {
	case "Escape":
		if a.page != 0 {
			a.page = 0
			a.sel = 0
		}
	case "Up":
		if a.sel > 0 {
			a.sel--
		}
	case "Down":
		a.sel++
	case "Enter":
		a.activate()
	default:
		if e.Rune == 's' || e.Rune == 'S' {
			a.save()
		}
	}
	return nil
}

func (a *App) activate() {
	switch a.page {
	case 0:
		items := 10
		if a.sel >= items {
			a.sel = items - 1
		}
		switch a.sel {
		case 0:
			a.page, a.sel = 1, 0
		case 1:
			a.page, a.sel = 2, 0
		case 2:
			a.page, a.sel = 3, 0
		case 3:
			a.page, a.sel = 4, 0
		case 4:
			a.page, a.sel = 5, 0
		case 5:
			a.page, a.sel = 6, 0
		case 6:
			a.page, a.sel = 7, 0
		case 7:
			a.page, a.sel = 8, 0
		case 8:
			a.page, a.sel = 9, 0
		case 9:
			a.save()
		}
	case 1: // appearance
		switch a.sel {
		case 0:
			a.cfg.Wallpaper.Mode = "pattern"
			a.cfg.Theme.DesktopPattern = "░"
			a.status = "Wallpaper: pattern ░"
		case 1:
			a.cfg.Wallpaper.Mode = "solid"
			a.cfg.Theme.DesktopPattern = ""
			a.status = "Wallpaper: solid"
		case 2:
			a.cfg.Wallpaper.Mode = "image"
			a.cfg.Wallpaper.Path = "builtin:bliss"
			a.cfg.Wallpaper.Fit = "cover"
			a.status = "Wallpaper: Bliss (XP default)"
		case 3:
			a.editing = true
			a.editBuf = a.cfg.Wallpaper.Path
			if strings.HasPrefix(a.editBuf, "builtin:") {
				a.editBuf = ""
			}
			a.status = "Custom image path — Enter to set mode=image"
		case 4:
			if a.ctx != nil {
				_ = a.ctx.Launch("files")
			}
			a.status = "Files: select image, press W or toolbar Wall"
		case 5:
			fits := []string{"cover", "contain", "stretch"}
			cur := a.cfg.Wallpaper.Fit
			next := "cover"
			for i, f := range fits {
				if f == cur {
					next = fits[(i+1)%len(fits)]
					break
				}
			}
			a.cfg.Wallpaper.Fit = next
			a.status = "Fit: " + next
		case 6:
			switch a.cfg.TaskbarDock() {
			case "top":
				a.cfg.Taskbar.Dock = "bottom"
			case "bottom":
				a.cfg.Taskbar.Dock = "left"
			case "left":
				a.cfg.Taskbar.Dock = "right"
			default:
				a.cfg.Taskbar.Dock = "top"
			}
			a.status = "Taskbar: " + a.cfg.Taskbar.Dock
		default:
			packs := config.ThemePacks()
			idx := a.sel - 7
			if idx >= 0 && idx < len(packs) {
				p := packs[idx]
				if a.cfg.ApplyThemePack(p.ID) {
					if a.onSave != nil {
						a.onSave(a.cfg)
					}
					a.status = fmt.Sprintf("%s theme — %s (applied)", p.Name, p.Tagline)
				}
			}
		}
	case 2: // terminal
		switch a.sel {
		case 0:
			a.editing = true
			a.editBuf = strconv.Itoa(a.cfg.FPS)
			a.status = "Edit local FPS — Enter to apply"
		case 1:
			a.editing = true
			a.editBuf = strconv.Itoa(a.cfg.SSHFPS)
			a.status = "Edit SSH FPS — Enter to apply"
		case 2:
			a.editing = true
			a.editBuf = a.cfg.Shell
			a.status = "Edit shell — Enter to apply"
		case 3:
			a.editing = true
			a.editBuf = strconv.Itoa(a.cfg.Scrollback)
			a.status = "Edit scrollback — Enter to apply"
		}
	case 3: // desktop
		switch a.sel {
		case 0:
			a.cfg.ShowDesktopIcons = !a.cfg.ShowDesktopIcons
			a.status = fmt.Sprintf("Desktop icons: %v", a.cfg.ShowDesktopIcons)
		case 1:
			a.cfg.SolidDesktopOnSSH = !a.cfg.SolidDesktopOnSSH
			a.status = fmt.Sprintf("Solid desktop on SSH: %v", a.cfg.SolidDesktopOnSSH)
		case 2:
			if a.cfg.Wallpaper.SSHMode == "solid" {
				a.cfg.Wallpaper.SSHMode = "keep"
			} else {
				a.cfg.Wallpaper.SSHMode = "solid"
			}
			a.status = "Wallpaper SSH mode: " + a.cfg.Wallpaper.SSHMode
		case 3:
			a.cfg.RestoreSession = !a.cfg.RestoreSession
			a.status = fmt.Sprintf("Restore last session: %v", a.cfg.RestoreSession)
		case 4:
			a.cfg.OpenTerminalOnStart = !a.cfg.OpenTerminalOnStart
			a.status = fmt.Sprintf("Open terminal on start: %v", a.cfg.OpenTerminalOnStart)
		case 5:
			a.editing = true
			a.editBuf = strings.Join(a.cfg.Autostart, ", ")
			a.status = "Autostart actions (comma-separated) — e.g. terminal, notes, calendar"
		case 6:
			a.cfg.DesktopIcons = config.DefaultDesktopIcons()
			a.status = "Reset desktop icons"
		}
	case 4: // notifications
		switch a.sel {
		case 0:
			a.cfg.Notify.Banners = !a.cfg.Notify.Banners
			a.status = fmt.Sprintf("Banners: %v", a.cfg.Notify.Banners)
		case 1:
			a.editing = true
			a.editBuf = strconv.Itoa(a.cfg.Notify.DismissSec)
			a.status = "Banner dismiss seconds"
		case 2:
			a.cfg.Notify.SSHBadgeOnly = !a.cfg.Notify.SSHBadgeOnly
			a.status = fmt.Sprintf("SSH badge-only: %v", a.cfg.Notify.SSHBadgeOnly)
		case 3:
			a.cfg.Notify.Persist = !a.cfg.Notify.Persist
			a.status = fmt.Sprintf("Persist history: %v", a.cfg.Notify.Persist)
		case 4:
			a.editing = true
			a.editBuf = strconv.Itoa(a.cfg.Notify.MaxHist)
			a.status = "Max history entries (1–500)"
		case 5:
			if a.onNotify != nil {
				a.onNotify("TTYPE Desk", "Test notification from Settings")
				a.status = "Posted test notification"
			} else {
				a.status = "Notify service unavailable"
			}
		case 6:
			if a.onClearN != nil {
				a.onClearN()
				a.status = "Notification history cleared"
			} else {
				a.status = "Clear unavailable"
			}
		}
	case 5: // default apps / roles
		switch a.sel {
		case 0:
			editors := []string{"pty:nano", "pty:nvim", "pty:vim", "pty:emacs"}
			cur := a.cfg.Roles.Editor
			next := editors[0]
			for i, e := range editors {
				if e == cur {
					next = editors[(i+1)%len(editors)]
					break
				}
			}
			a.cfg.Roles.Editor = next
			a.status = "Default editor: " + next
		case 1:
			a.editing = true
			a.editBuf = strings.TrimPrefix(a.cfg.Roles.Editor, "pty:")
			if a.editBuf == a.cfg.Roles.Editor {
				a.editBuf = a.cfg.Roles.Editor
			}
			a.status = "Custom editor command — Enter (stored as pty:<cmd>)"
		case 2:
			a.cfg.Roles.Image = "image"
			a.status = "Image app: Image Viewer"
		case 3:
			browsers := []string{"", "pty:lynx", "pty:w3m", "pty:elinks"}
			cur := a.cfg.Roles.Browser
			next := browsers[0]
			for i, b := range browsers {
				if b == cur {
					next = browsers[(i+1)%len(browsers)]
					break
				}
			}
			a.cfg.Roles.Browser = next
			if next == "" {
				a.status = "Browser role: (unset)"
			} else {
				a.status = "Browser role: " + next
			}
		case 4:
			a.editing = true
			a.editBuf = strings.TrimPrefix(a.cfg.Roles.Browser, "pty:")
			if a.editBuf == a.cfg.Roles.Browser {
				a.editBuf = a.cfg.Roles.Browser
			}
			a.status = "Custom browser command — Enter (pty:<cmd>), empty clears"
		case 5:
			a.cfg.Associations = config.DefaultAssociations()
			a.status = "Reset file associations to defaults"
		}
	case 6: // apps / Start menu
		switch a.sel {
		case 0:
			if a.ctx != nil {
				_ = a.ctx.Launch("addprog")
			}
			a.status = "Opening Add Program…"
		case 1:
			if a.ctx != nil {
				_ = a.ctx.Launch("mgrprog")
			}
			a.status = "Opening Manage Programs…"
		case 2:
			n := len(a.cfg.Programs)
			if n == 0 {
				a.status = "No custom programs yet — Add Program…"
			} else {
				names := make([]string, 0, n)
				for _, p := range a.cfg.Programs {
					names = append(names, p.Name)
				}
				a.status = fmt.Sprintf("%d program(s): %s", n, strings.Join(names, ", "))
			}
		case 3:
			if a.cfg.Roles.Terminal == "" || a.cfg.Roles.Terminal == "terminal" {
				a.cfg.Roles.Terminal = "pty:" + a.cfg.Shell
				if a.cfg.Roles.Terminal == "pty:" {
					a.cfg.Roles.Terminal = "pty:bash"
				}
			} else {
				a.cfg.Roles.Terminal = "terminal"
			}
			a.status = "Terminal role: " + a.cfg.Roles.Terminal
		case 4:
			a.cfg.Roles.FileMgr = "files"
			a.status = "File manager role: files"
		case 5:
			if a.ctx != nil {
				_ = a.ctx.Launch("files:system")
			}
			a.status = "Opening System folder…"
		}
	case 7: // input / hotkeys
		actions := config.HotkeyActions()
		switch {
		case a.sel < len(actions):
			act := actions[a.sel]
			a.editing = true
			a.hkEdit = act
			a.editBuf = a.cfg.ResolveHotkey(act)
			a.status = "Binding e.g. f3, alt+/, ctrl+shift+f — Enter apply, Esc cancel"
		case a.sel == len(actions):
			a.cfg.Palette.StartOpensPalette = !a.cfg.Palette.StartOpensPalette
			if a.cfg.Palette.StartOpensPalette {
				a.status = "Start opens command palette"
			} else {
				a.status = "Start opens Start menu"
			}
		case a.sel == len(actions)+1:
			a.cfg.Palette.ASCIIIcons = !a.cfg.Palette.ASCIIIcons
			a.status = fmt.Sprintf("ASCII icon substitutes: %v", a.cfg.Palette.ASCIIIcons)
		case a.sel == len(actions)+2:
			a.cfg.Hotkeys = config.DefaultHotkeys()
			a.status = "Hotkeys reset to defaults"
		}
	case 8: // advanced
		switch a.sel {
		case 0:
			if a.ctx != nil {
				_ = a.ctx.OpenPath(slogPath())
				a.status = "Opening log…"
			}
		case 1:
			a.status = "Log: " + slogPath()
		case 2:
			a.status = "Config: " + config.Path()
		case 3:
			a.status = "Notifications file: ~/.config/ttypedesk/notifications.json"
		case 4:
			a.status = fmt.Sprintf("Effective FPS: %d", a.cfg.EffectiveFPS())
		}
	}
}

func (a *App) commitEdit() {
	switch a.page {
	case 1:
		if a.sel == 3 {
			a.cfg.Wallpaper.Path = a.editBuf
			if a.editBuf != "" {
				a.cfg.Wallpaper.Mode = "image"
				a.status = "Wallpaper image set"
			}
		}
	case 2:
		switch a.sel {
		case 0:
			if n, err := strconv.Atoi(a.editBuf); err == nil && n > 0 && n <= 120 {
				a.cfg.FPS = n
				a.status = fmt.Sprintf("FPS=%d", n)
			}
		case 1:
			if n, err := strconv.Atoi(a.editBuf); err == nil && n > 0 && n <= 60 {
				a.cfg.SSHFPS = n
				a.status = fmt.Sprintf("SSH FPS=%d", n)
			}
		case 2:
			if a.editBuf != "" {
				a.cfg.Shell = a.editBuf
				a.status = "Shell updated"
			}
		case 3:
			if n, err := strconv.Atoi(a.editBuf); err == nil && n >= 0 {
				if n < config.MinScrollback {
					n = config.MinScrollback
				}
				a.cfg.Scrollback = n
				a.status = fmt.Sprintf("Scrollback=%d", n)
			}
		}
	case 3:
		if a.sel == 5 {
			parts := strings.Split(a.editBuf, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			a.cfg.Autostart = out
			if len(out) == 0 {
				a.status = "Autostart cleared"
			} else {
				a.status = fmt.Sprintf("Autostart: %d action(s)", len(out))
			}
		}
	case 4:
		if a.sel == 1 {
			if n, err := strconv.Atoi(a.editBuf); err == nil && n > 0 && n <= 60 {
				a.cfg.Notify.DismissSec = n
				a.status = fmt.Sprintf("Dismiss=%ds", n)
			}
		}
		if a.sel == 4 {
			if n, err := strconv.Atoi(a.editBuf); err == nil && n >= 1 && n <= 500 {
				a.cfg.Notify.MaxHist = n
				a.status = fmt.Sprintf("Max history=%d", n)
			}
		}
	case 5:
		if a.sel == 1 && a.editBuf != "" {
			cmd := strings.TrimSpace(a.editBuf)
			if !strings.HasPrefix(cmd, "pty:") && !strings.HasPrefix(cmd, "prog:") {
				cmd = "pty:" + cmd
			}
			a.cfg.Roles.Editor = cmd
			a.status = "Editor: " + cmd
		}
		if a.sel == 4 {
			cmd := strings.TrimSpace(a.editBuf)
			if cmd == "" {
				a.cfg.Roles.Browser = ""
				a.status = "Browser role cleared"
			} else {
				if !strings.HasPrefix(cmd, "pty:") && !strings.HasPrefix(cmd, "prog:") && !strings.HasPrefix(cmd, "role:") {
					cmd = "pty:" + cmd
				}
				a.cfg.Roles.Browser = cmd
				a.status = "Browser: " + cmd
			}
		}
	case 7:
		if a.hkEdit != "" {
			if a.cfg.Hotkeys == nil {
				a.cfg.Hotkeys = config.DefaultHotkeys()
			}
			b := strings.ToLower(strings.TrimSpace(a.editBuf))
			if b == "" {
				delete(a.cfg.Hotkeys, a.hkEdit)
				a.status = "Cleared " + a.hkEdit + " (uses default)"
			} else {
				a.cfg.Hotkeys[a.hkEdit] = b
				a.status = config.HotkeyLabel(a.hkEdit) + " = " + b
			}
			a.hkEdit = ""
		}
	}
}

func (a *App) save() {
	if a.onSave != nil {
		a.onSave(a.cfg)
	}
	a.status = "Saved " + config.Path()
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	cv.FillRect(0, 0, cols, rows, bg)
	title := " Settings "
	if config.OverSSH() {
		title = " Settings  (SSH mode) "
	}
	cv.DrawText(0, 0, title, cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)

	lines := a.lines()
	if a.sel >= len(lines) {
		a.sel = len(lines) - 1
	}
	if a.sel < 0 {
		a.sel = 0
	}
	for i, line := range lines {
		y := 2 + i
		if y >= rows-2 {
			break
		}
		f, b := fg, bg
		if i == a.sel {
			f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
		}
		if a.editing && i == a.sel {
			line = line + " › " + a.editBuf + "█"
		}
		cv.DrawText(1, y, line, f, b, 0)
	}
	help := "↑↓ Enter  Esc=back  S=save"
	if a.status != "" {
		help = a.status
	}
	if rows > 1 {
		cv.DrawText(0, rows-1, " "+help, cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0x00, 0x00, 0x00), 0)
	}
	return nil
}

func (a *App) lines() []string {
	on := map[bool]string{true: "ON", false: "OFF"}
	switch a.page {
	case 0:
		return []string{
			"Appearance",
			"Terminal & performance",
			"Desktop",
			"Notifications",
			"Default apps",
			"Apps",
			"Input (hotkeys)",
			"Advanced",
			"About",
			"Save configuration",
		}
	case 1:
		path := a.cfg.Wallpaper.Path
		pathLabel := path
		switch path {
		case "", "builtin:bliss":
			pathLabel = "builtin:bliss (Bliss)"
		case "builtin:scarlet":
			pathLabel = "builtin:scarlet (Scarlet)"
		case "builtin:bumble":
			pathLabel = "builtin:bumble (Bumble)"
		case "builtin:bubble":
			pathLabel = "builtin:bubble (Bubble)"
		case "builtin:sprout":
			pathLabel = "builtin:sprout (Sprout)"
		}
		themeName := a.cfg.Theme.Name
		if themeName == "" {
			themeName = config.ThemeXP
		}
		out := []string{
			"Wallpaper: pattern ░",
			"Wallpaper: solid color",
			"Wallpaper: Bliss (XP default)",
			fmt.Sprintf("Wallpaper image path: %s", pathLabel),
			"Browse wallpaper in Files…",
			fmt.Sprintf("Image fit: %s", a.cfg.Wallpaper.Fit),
			fmt.Sprintf("Taskbar position: %s", a.cfg.TaskbarDock()),
		}
		for _, p := range config.ThemePacks() {
			mark := " "
			if themeName == p.Name {
				mark = "*"
			}
			out = append(out, fmt.Sprintf("%s Theme: %s — %s", mark, p.Name, p.Tagline))
		}
		return out
	case 2:
		return []string{
			fmt.Sprintf("Local FPS: %d", a.cfg.FPS),
			fmt.Sprintf("SSH FPS: %d (effective now: %d)", a.cfg.SSHFPS, a.cfg.EffectiveFPS()),
			fmt.Sprintf("Shell: %s", a.cfg.Shell),
			fmt.Sprintf("Scrollback: %d", a.cfg.Scrollback),
		}
	case 3:
		auto := strings.Join(a.cfg.Autostart, ", ")
		if auto == "" {
			auto = "(none)"
		}
		return []string{
			fmt.Sprintf("Show desktop icons: %s", on[a.cfg.ShowDesktopIcons]),
			fmt.Sprintf("Solid desktop over SSH: %s", on[a.cfg.SolidDesktopOnSSH]),
			fmt.Sprintf("Wallpaper SSH mode: %s", a.cfg.Wallpaper.SSHMode),
			fmt.Sprintf("Restore last session: %s", on[a.cfg.RestoreSession]),
			fmt.Sprintf("Open terminal on start: %s", on[a.cfg.OpenTerminalOnStart]),
			fmt.Sprintf("Autostart: %s", auto),
			"Reset default desktop icons",
		}
	case 4:
		return []string{
			fmt.Sprintf("Show banners: %s", on[a.cfg.Notify.Banners]),
			fmt.Sprintf("Banner dismiss seconds: %d", a.cfg.Notify.DismissSec),
			fmt.Sprintf("SSH badge-only: %s", on[a.cfg.Notify.SSHBadgeOnly]),
			fmt.Sprintf("Persist history: %s", on[a.cfg.Notify.Persist]),
			fmt.Sprintf("Max history: %d", a.cfg.Notify.MaxHist),
			"Send test notification",
			"Clear notification history",
		}
	case 5:
		ed := a.cfg.Roles.Editor
		if ed == "" {
			ed = "pty:nano"
		}
		img := a.cfg.Roles.Image
		if img == "" {
			img = "image"
		}
		br := a.cfg.Roles.Browser
		if br == "" {
			br = "(unset — cycle lynx/w3m/elinks)"
		}
		nAssoc := len(a.cfg.Associations)
		return []string{
			fmt.Sprintf("Default editor: %s  (Enter cycles nano/nvim/vim/emacs)", ed),
			"Custom editor command…",
			fmt.Sprintf("Image app: %s", img),
			fmt.Sprintf("Browser role: %s  (Enter cycles)", br),
			"Custom browser command…",
			fmt.Sprintf("Reset associations (%d entries) to defaults", nAssoc),
		}
	case 6:
		nProg := len(a.cfg.Programs)
		term := a.cfg.Roles.Terminal
		if term == "" {
			term = "terminal"
		}
		fm := a.cfg.Roles.FileMgr
		if fm == "" {
			fm = "files"
		}
		return []string{
			"Add Program…",
			"Manage Programs…",
			fmt.Sprintf("Custom programs: %d  (Enter lists names)", nProg),
			fmt.Sprintf("Terminal role: %s  (Enter toggles builtin/shell)", term),
			fmt.Sprintf("File manager role: %s", fm),
			"Open System folder…",
		}
	case 7:
		out := make([]string, 0, len(config.HotkeyActions())+3)
		for _, act := range config.HotkeyActions() {
			out = append(out, fmt.Sprintf("%s: %s", config.HotkeyLabel(act), a.cfg.ResolveHotkey(act)))
		}
		out = append(out,
			fmt.Sprintf("Start opens command palette: %s", on[a.cfg.Palette.StartOpensPalette]),
			fmt.Sprintf("ASCII icon substitutes: %s", on[a.cfg.Palette.ASCIIIcons]),
			"Reset hotkeys to defaults",
		)
		return out
	case 8:
		ssh := "no"
		if config.OverSSH() {
			ssh = "yes"
		}
		return []string{
			"Open log file…",
			"Log path: " + slogPath(),
			"Config path: " + config.Path(),
			"Notifications: ~/.config/ttypedesk/notifications.json",
			fmt.Sprintf("Effective FPS: %d  (SSH: %s)", a.cfg.EffectiveFPS(), ssh),
		}
	default:
		ssh := "no"
		if config.OverSSH() {
			ssh = "yes"
		}
		return []string{
			"TTYPE Desk — DOS-era terminal desktop",
			"Themes: XP, Scarlet, Bumble, Bubble, Sprout",
			"Config: " + config.Path(),
			fmt.Sprintf("Over SSH: %s", ssh),
			"User guide: Start → System → Manual",
			"On-disk copy: ~/.config/ttypedesk/System/",
			"Icons are user-chosen glyphs (emoji optional)",
		}
	}
}

func slogPath() string {
	return slog.Path()
}
