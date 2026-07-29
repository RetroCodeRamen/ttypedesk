package config

// Menu folder for Start menu placement.
const (
	MenuPrograms = "programs"
	MenuSystem   = "system"
)

// Program is a user-registered app: usually a shell command run in a PTY
// window, or — if Bridge is set — an X11 GUI app launched through the
// GUI-TUI Bridge (see internal/bridge) instead.
type Program struct {
	ID      string `json:"id"`
	Name    string `json:"name"`    // display name in TTYPE Desk (e.g. "Task Manager")
	Command string `json:"command"` // run via $SHELL -c (e.g. "htop"), or the X11 command if Bridge
	Icon    string `json:"icon"`
	Menu    string `json:"menu"`             // programs | system
	Desktop bool   `json:"desktop"`          // keep a desktop shortcut in sync
	Bridge  bool   `json:"bridge,omitempty"` // launch Command via the GUI-TUI Bridge, not a shell PTY
}

// MenuFolder returns programs or system (default programs).
func (p Program) MenuFolder() string {
	if p.Menu == MenuSystem {
		return MenuSystem
	}
	return MenuPrograms
}

// ProgramAction returns the LaunchAction string for a custom program.
func ProgramAction(id string) string {
	return "prog:" + id
}

// UsedIcons returns emoji/icon glyphs already claimed by programs, desktop icons, or builtins.
func (c Config) UsedIcons() map[string]struct{} {
	used := map[string]struct{}{}
	for _, p := range BuiltinProgramIcons() {
		used[p] = struct{}{}
	}
	for _, p := range c.Programs {
		if p.Icon != "" {
			used[p.Icon] = struct{}{}
		}
	}
	for _, ic := range c.DesktopIcons {
		if ic.Icon != "" {
			used[ic.Icon] = struct{}{}
		}
	}
	return used
}

// BuiltinProgramIcons are reserved by first-party Start menu entries.
func BuiltinProgramIcons() []string {
	return []string{"📝", "📅", "🕐", "🖼", "💻", "📁", "⚙️", "⏻", "➕", "🗑", "🛍"}
}

// IconPalette is the selectable emoji pool for Add Program.
func IconPalette() []string {
	return []string{
		"🚀", "🎮", "🎵", "🎬", "📷", "🎨", "📊", "📈", "📉", "🧩",
		"🛠", "🔧", "🔨", "📦", "📚", "📖", "✏️", "🔍", "🌐", "💬",
		"📧", "🗂", "🗒", "🧮", "🧪", "🔬", "🕹", "🎧", "🎤", "📻",
		"📺", "🖥", "⌨️", "🖱", "💾", "💿", "📀", "📡", "🛰️", "🤖",
		"🦊", "🐱", "🐶", "🐼", "🐨", "🐯", "🦁", "🐸", "🐙", "🦄",
		"🌟", "⭐", "🔥", "💡", "🎯", "🏆", "🎪", "🎭", "🎬", "🪄",
		"🗺", "🧭", "⚓", "✈️", "🚗", "🚂", "🏠", "🏢", "🌴", "🌵",
		"🍎", "🍕", "☕", "🍺", "🎂", "🍪", "🧩", "🧿", "💎", "🔮",
	}
}

// AvailableIcons returns palette entries not in UsedIcons.
func (c Config) AvailableIcons() []string {
	used := c.UsedIcons()
	var out []string
	seen := map[string]struct{}{}
	for _, ic := range IconPalette() {
		if _, ok := used[ic]; ok {
			continue
		}
		if _, ok := seen[ic]; ok {
			continue
		}
		seen[ic] = struct{}{}
		out = append(out, ic)
	}
	return out
}

// FindProgram returns a program by id.
func (c Config) FindProgram(id string) *Program {
	for i := range c.Programs {
		if c.Programs[i].ID == id {
			return &c.Programs[i]
		}
	}
	return nil
}

// RemoveProgram deletes a custom program by id and syncs desktop shortcuts.
func (c *Config) RemoveProgram(id string) bool {
	dst := c.Programs[:0]
	found := false
	for _, p := range c.Programs {
		if p.ID == id {
			found = true
			continue
		}
		dst = append(dst, p)
	}
	if !found {
		return false
	}
	c.Programs = dst
	c.SyncProgramDesktop()
	return true
}

// SyncProgramDesktop ensures DesktopIcons matches Program.Desktop flags.
func (c *Config) SyncProgramDesktop() {
	// Remove stale prog: shortcuts
	keep := c.DesktopIcons[:0]
	for _, ic := range c.DesktopIcons {
		if len(ic.Action) >= 5 && ic.Action[:5] == "prog:" {
			id := ic.Action[5:]
			p := c.FindProgram(id)
			if p == nil || !p.Desktop {
				continue
			}
			// refresh label/icon from program
			ic.Label = p.Name
			ic.Icon = p.Icon
			keep = append(keep, ic)
			continue
		}
		keep = append(keep, ic)
	}
	c.DesktopIcons = keep

	// Add missing shortcuts
	for _, p := range c.Programs {
		if !p.Desktop {
			continue
		}
		action := ProgramAction(p.ID)
		found := false
		for _, ic := range c.DesktopIcons {
			if ic.Action == action {
				found = true
				break
			}
		}
		if found {
			continue
		}
		x, y := c.nextDesktopSlot()
		c.DesktopIcons = append(c.DesktopIcons, DesktopIcon{
			Label:  p.Name,
			Icon:   p.Icon,
			X:      x,
			Y:      y,
			Action: action,
		})
	}
}

func (c Config) nextDesktopSlot() (x, y int) {
	x, y = 2, 2
	occupied := map[[2]int]bool{}
	for _, ic := range c.DesktopIcons {
		occupied[[2]int{ic.X, ic.Y}] = true
	}
	for row := 0; row < 20; row++ {
		yy := 2 + row*4
		if !occupied[[2]int{2, yy}] {
			return 2, yy
		}
	}
	return 16, 2
}
