package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

// Theme holds chrome colors. Shipped packs live in themes.go (XP, Scarlet, Bumble, Bubble, Sprout).
type Theme struct {
	Name            string     `json:"name,omitempty"` // e.g. "XP", "Scarlet"
	DesktopBG       cell.Color `json:"desktop_bg"`
	DesktopPattern  string     `json:"desktop_pattern"` // one char / emoji; empty = solid
	TaskbarBG       cell.Color `json:"taskbar_bg"`
	TaskbarFG       cell.Color `json:"taskbar_fg"`
	StartBG         cell.Color `json:"start_bg"`
	StartFG         cell.Color `json:"start_fg"`
	MenuBG          cell.Color `json:"menu_bg"`
	MenuFG          cell.Color `json:"menu_fg"`
	MenuHighlight   cell.Color `json:"menu_highlight"`
	TitleFocused    cell.Color `json:"title_focused"`
	TitleUnfocused  cell.Color `json:"title_unfocused"`
	TitleFG         cell.Color `json:"title_fg"`
	BorderFocused   cell.Color `json:"border_focused"`
	BorderUnfocused cell.Color `json:"border_unfocused"`
	WindowBody      cell.Color `json:"window_body"`
	Shadow          cell.Color `json:"shadow"`
	ShadowDX        int        `json:"shadow_dx"`
	ShadowDY        int        `json:"shadow_dy"`
	DefaultFG       cell.Color `json:"default_fg"`
	DefaultBG       cell.Color `json:"default_bg"`
	IconFG          cell.Color `json:"icon_fg"`
	IconLabelFG     cell.Color `json:"icon_label_fg"`
}

func (t Theme) PatternRune() rune {
	if t.DesktopPattern == "" {
		return 0
	}
	for _, r := range t.DesktopPattern {
		return r
	}
	return 0
}

// DefaultTheme returns the XP theme (Windows XP–inspired chrome).
func DefaultTheme() Theme {
	return XPTheme()
}

// DesktopIcon is a free-form desktop shortcut (icon glyph is optional / user-chosen).
type DesktopIcon struct {
	Label  string `json:"label"`
	Icon   string `json:"icon,omitempty"` // emoji or short text — not bound to action type
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Action string `json:"action"` // terminal | notes | clock | settings | image | files | pty:<cmd> | app:<name>
}

// Roles map logical app slots to launch actions.
type Roles struct {
	Terminal string `json:"terminal,omitempty"`
	Editor   string `json:"editor,omitempty"`
	Browser  string `json:"browser,omitempty"`
	FileMgr  string `json:"filemgr,omitempty"`
	Image    string `json:"image,omitempty"`
}

// Wallpaper configures the desktop background.
type Wallpaper struct {
	Mode    string `json:"mode"`     // solid | pattern | image
	Path    string `json:"path"`     // image path when mode=image
	Fit     string `json:"fit"`      // cover | contain | stretch
	SSHMode string `json:"ssh_mode"` // solid | keep — override when over SSH
}

// NotifyCfg controls system notification UI.
type NotifyCfg struct {
	Banners      bool `json:"banners"`
	DismissSec   int  `json:"dismiss_sec"`
	SSHBadgeOnly bool `json:"ssh_badge_only"`
	Persist      bool `json:"persist"`  // write history to notifications.json
	MaxHist      int  `json:"max_hist"` // cap persisted/in-memory list
}

// TaskbarCfg controls taskbar docking.
type TaskbarCfg struct {
	Dock string `json:"dock"` // top | bottom | left | right
}

// Recipe is a command-palette phrase → LaunchAction.
type Recipe struct {
	Match   string `json:"match"`
	Action  string `json:"action"`
	Confirm bool   `json:"confirm,omitempty"`
}

// PaletteCfg tunes the command palette.
type PaletteCfg struct {
	MaxResults       int  `json:"max_results"`
	StartOpensPalette bool `json:"start_opens_palette"` // Start button / Start hotkey opens palette
	ASCIIIcons       bool `json:"ascii_icons"`          // draw ASCII substitutes for emoji icons
}

// AppSource is a catalog repo the App Store fetches index.json from.
type AppSource struct {
	Repo   string `json:"repo"`             // "owner/repo"
	Branch string `json:"branch,omitempty"` // defaults to "main" if empty
}

// MinScrollback is the smallest scrollback depth (in lines) any terminal
// window is allowed to have, regardless of what a user config requests.
const MinScrollback = 1000

// Config is the application configuration.
type Config struct {
	FPS                 int           `json:"fps"`
	SSHFPS              int           `json:"ssh_fps"`
	Shell               string        `json:"shell"`
	Scrollback          int           `json:"scrollback"`
	ShowDesktopIcons    bool          `json:"show_desktop_icons"`
	SolidDesktopOnSSH   bool          `json:"solid_desktop_on_ssh"`
	RestoreSession      bool          `json:"restore_session"`
	OpenTerminalOnStart bool          `json:"open_terminal_on_start"`
	Autostart           []string      `json:"autostart"` // LaunchAction strings after session restore
	Theme               Theme         `json:"theme"`
	Taskbar             TaskbarCfg        `json:"taskbar"`
	DesktopIcons        []DesktopIcon     `json:"desktop_icons"`
	Programs            []Program         `json:"programs"`
	Roles               Roles             `json:"roles"`
	Associations        map[string]string `json:"associations"`
	Files               FilesCfg          `json:"files"`
	Wallpaper           Wallpaper         `json:"wallpaper"`
	Notify              NotifyCfg         `json:"notify"`
	Hotkeys             map[string]string `json:"hotkeys"`
	Palette             PaletteCfg        `json:"palette"`
	Recipes             []Recipe          `json:"recipes"`
	AppSources          []AppSource       `json:"app_sources"`
}

// TaskbarDock returns normalized dock side.
func (c Config) TaskbarDock() string {
	switch strings.ToLower(c.Taskbar.Dock) {
	case "bottom", "left", "right":
		return strings.ToLower(c.Taskbar.Dock)
	default:
		return "top"
	}
}

func DefaultDesktopIcons() []DesktopIcon {
	return []DesktopIcon{
		{Label: "Terminal", Icon: "💻", X: 2, Y: 2, Action: "terminal"},
		{Label: "Files", Icon: "📁", X: 2, Y: 6, Action: "files"},
		{Label: "Home", Icon: "🏠", X: 2, Y: 10, Action: "files:home"},
		{Label: "Trash", Icon: "🗑", X: 2, Y: 14, Action: "files:trash"},
		{Label: "Notes", Icon: "📝", X: 16, Y: 2, Action: "notes"},
		{Label: "Settings", Icon: "⚙️", X: 16, Y: 6, Action: "settings"},
	}
}

func Default() Config {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return Config{
		FPS:               30,
		SSHFPS:            15,
		Shell:             shell,
		Scrollback:        2000,
		ShowDesktopIcons:    true,
		SolidDesktopOnSSH:   true,
		RestoreSession:      true,
		OpenTerminalOnStart: false,
		Autostart:           nil,
		Theme:               DefaultTheme(),
		Taskbar:             TaskbarCfg{Dock: "top"},
		DesktopIcons:        DefaultDesktopIcons(),
		Roles: Roles{
			Terminal: "terminal",
			Editor:   "pty:nano",
			FileMgr:  "files",
			Image:    "image",
		},
		Associations: DefaultAssociations(),
		Files:        DefaultFiles(),
		Wallpaper: Wallpaper{
			Mode:    "image",
			Path:    "builtin:bliss",
			Fit:     "cover",
			SSHMode: "solid",
		},
		Notify: NotifyCfg{
			Banners:      true,
			DismissSec:   5,
			SSHBadgeOnly: false,
			Persist:      true,
			MaxHist:      50,
		},
		Hotkeys: DefaultHotkeys(),
		Palette: PaletteCfg{MaxResults: 12},
		Recipes: []Recipe{
			{Match: "wifi connect", Action: "pty:nmtui"},
			{Match: "install doom", Action: "pty:bash -lc sudo apt install chocolate-doom", Confirm: true},
		},
		AppSources: []AppSource{
			{Repo: "RetroCodeRamen/ttypedesk-apps", Branch: "main"},
		},
	}
}

func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "ttypedesk.json"
	}
	return filepath.Join(home, ".config", "ttypedesk", "config.json")
}

// OverSSH reports whether we appear to be running inside an SSH session.
func OverSSH() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != ""
}

// EffectiveFPS returns the frame rate to use (env override > ssh/local defaults).
func (c Config) EffectiveFPS() int {
	if v := os.Getenv("TTYPEDESK_FPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if OverSSH() {
		if c.SSHFPS > 0 {
			return c.SSHFPS
		}
		return 15
	}
	if c.FPS > 0 {
		return c.FPS
	}
	return 30
}

func (c Config) UseSolidDesktop() bool {
	if OverSSH() && c.SolidDesktopOnSSH {
		return true
	}
	if OverSSH() && c.Wallpaper.SSHMode == "solid" {
		return true
	}
	return c.Wallpaper.Mode == "solid"
}

// UseImageWallpaper reports whether to blit a half-block image wallpaper.
func (c Config) UseImageWallpaper() bool {
	if c.Wallpaper.Mode != "image" || c.Wallpaper.Path == "" {
		return false
	}
	if c.UseSolidDesktop() {
		return false
	}
	if OverSSH() && c.Wallpaper.SSHMode == "solid" {
		return false
	}
	return true
}

// UsePatternDesktop reports whether to draw the theme pattern (not image/solid).
func (c Config) UsePatternDesktop() bool {
	if c.UseImageWallpaper() || c.UseSolidDesktop() {
		return false
	}
	mode := c.Wallpaper.Mode
	if mode == "" || mode == "pattern" {
		return c.Theme.PatternRune() != 0
	}
	return false
}

func Load() (Config, error) {
	cfg := Default()
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = Save(cfg)
			return cfg, nil
		}
		return cfg, err
	}
	hadNotify := strings.Contains(string(data), `"notify"`)
	hadNotifyPersist := strings.Contains(string(data), `"persist"`)
	hadWallpaper := strings.Contains(string(data), `"wallpaper"`)
	hadRestore := strings.Contains(string(data), `"restore_session"`)
	hadAssoc := strings.Contains(string(data), `"associations"`)
	hadFiles := strings.Contains(string(data), `"files"`)
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}
	if !hadNotify {
		cfg.Notify = Default().Notify
	} else if !hadNotifyPersist {
		cfg.Notify.Persist = true
	}
	if !hadWallpaper {
		cfg.Wallpaper = Default().Wallpaper
	}
	if !hadRestore {
		cfg.RestoreSession = true
		cfg.OpenTerminalOnStart = false
	}
	if !hadAssoc {
		cfg.Associations = DefaultAssociations()
	}
	if !hadFiles {
		cfg.Files = DefaultFiles()
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	if c.FPS <= 0 {
		c.FPS = 30
	}
	if c.SSHFPS <= 0 {
		c.SSHFPS = 15
	}
	if c.Shell == "" {
		c.Shell = Default().Shell
	}
	if c.Scrollback <= 0 {
		c.Scrollback = 2000
	}
	if c.Scrollback < MinScrollback {
		c.Scrollback = MinScrollback
	}
	if c.DesktopIcons == nil {
		c.DesktopIcons = DefaultDesktopIcons()
	}
	if c.Programs == nil {
		c.Programs = []Program{}
	}
	for i := range c.Programs {
		if c.Programs[i].Menu != MenuSystem {
			c.Programs[i].Menu = MenuPrograms
		}
	}
	c.SyncProgramDesktop()
	if c.Wallpaper.Mode == "" {
		c.Wallpaper.Mode = "image"
	}
	if c.Wallpaper.Path == "" && c.Wallpaper.Mode == "image" {
		c.Wallpaper.Path = "builtin:bliss"
	}
	if c.Wallpaper.Fit == "" {
		c.Wallpaper.Fit = "cover"
	}
	if c.Wallpaper.SSHMode == "" {
		c.Wallpaper.SSHMode = "solid"
	}
	if c.Theme.Name == "" {
		c.Theme.Name = ThemeXP
	}
	if c.Taskbar.Dock == "" {
		c.Taskbar.Dock = "top"
	}
	if c.Associations == nil {
		c.Associations = DefaultAssociations()
	}
	if c.Roles.Editor == "" {
		c.Roles.Editor = "pty:nano"
	}
	if c.Roles.Image == "" {
		c.Roles.Image = "image"
	}
	if c.Roles.FileMgr == "" {
		c.Roles.FileMgr = "files"
	}
	if c.Files.View == "" {
		c.Files.View = "list"
	}
	if c.Files.Sort == "" {
		c.Files.Sort = "name"
	}
	if c.Files.StartDir == "" {
		c.Files.StartDir = "home"
	}
	// ConfirmDelete defaults true; zero-value false only if user never had files section —
	// treat missing section as DefaultFiles via Load merge when needed.
	if c.Notify.DismissSec <= 0 {
		c.Notify.DismissSec = 5
	}
	if c.Notify.MaxHist <= 0 {
		c.Notify.MaxHist = 50
	}
	if c.Hotkeys == nil {
		c.Hotkeys = DefaultHotkeys()
	} else {
		for k, v := range DefaultHotkeys() {
			if _, ok := c.Hotkeys[k]; !ok {
				c.Hotkeys[k] = v
			}
		}
		// Hosts (Guake, etc.) often steal Alt+Space — move primary Start to F10.
		if strings.EqualFold(strings.TrimSpace(c.Hotkeys[HKStartMenu]), "alt+space") {
			c.Hotkeys[HKStartMenu] = "f10"
			c.Hotkeys[HKStartMenuLegacy] = "alt+space"
		}
	}
	if c.Palette.MaxResults <= 0 {
		c.Palette.MaxResults = 12
	}
	// Banners default true when unset in old configs: zero-value false is OK
	// if user never had notify section — treat missing as enabled via Load merge.
	if c.Theme.DesktopPattern == "" && c.Theme.DesktopBG.R == 0 && c.Theme.DesktopBG.G == 0 && c.Theme.DesktopBG.B == 0 {
		// leave; user may want solid black
	}
}

func Save(cfg Config) error {
	cfg.normalize()
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
