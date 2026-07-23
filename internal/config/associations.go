package config

import (
	"path/filepath"
	"strings"
)

// FilesCfg controls the Files app.
type FilesCfg struct {
	View          string `json:"view"` // list | grid
	ShowHidden    bool   `json:"show_hidden"`
	Sort          string `json:"sort"` // name | size | mtime
	StartDir      string `json:"start_dir"` // home | last | path
	ConfirmDelete bool   `json:"confirm_delete"`
	LastDir       string `json:"last_dir,omitempty"`
}

// DefaultAssociations maps file extensions (no dot) to launch actions.
// role:editor / role:image expand via Roles.
func DefaultAssociations() map[string]string {
	text := []string{
		"txt", "md", "markdown", "go", "rs", "py", "c", "h", "cc", "cpp", "hpp",
		"java", "js", "ts", "tsx", "jsx", "json", "yaml", "yml", "toml", "xml",
		"html", "css", "sh", "bash", "zsh", "fish", "conf", "cfg", "ini", "log",
		"env", "gitignore", "dockerfile", "makefile", "cmake", "sql", "csv",
		"mod", "sum", "lock",
	}
	m := make(map[string]string, len(text)+8)
	for _, e := range text {
		m[e] = "role:editor"
	}
	m["png"] = "role:image"
	m["jpg"] = "role:image"
	m["jpeg"] = "role:image"
	m["gif"] = "role:image"
	m["webp"] = "role:image"
	m["bmp"] = "role:image"
	return m
}

func DefaultFiles() FilesCfg {
	return FilesCfg{
		View:          "list",
		ShowHidden:    false,
		Sort:          "name",
		StartDir:      "home",
		ConfirmDelete: true,
	}
}

// ExtOf returns the lowercase extension without dot (last suffix).
func ExtOf(path string) string {
	base := filepath.Base(path)
	i := strings.LastIndex(base, ".")
	if i <= 0 || i == len(base)-1 {
		return ""
	}
	return strings.ToLower(base[i+1:])
}

// ResolveRole expands role:name using Roles; unknown roles return empty.
func (c Config) ResolveRole(name string) string {
	switch strings.ToLower(name) {
	case "terminal":
		if c.Roles.Terminal != "" {
			return c.Roles.Terminal
		}
		return "terminal"
	case "editor":
		if c.Roles.Editor != "" {
			return c.Roles.Editor
		}
		return "pty:nano"
	case "filemgr", "files":
		if c.Roles.FileMgr != "" {
			return c.Roles.FileMgr
		}
		return "files"
	case "image", "imageview":
		if c.Roles.Image != "" {
			return c.Roles.Image
		}
		return "image"
	case "browser":
		return c.Roles.Browser
	default:
		return ""
	}
}

// ExpandLaunch resolves role:name and {terminal}/{editor}/{browser}/{filemgr}/{image}
// placeholders in a LaunchAction string. Returns "" if a required role is unset
// (e.g. role:browser with no Roles.Browser).
func (c Config) ExpandLaunch(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return ""
	}
	switch strings.ToLower(action) {
	case "browser":
		action = "role:browser"
	case "editor":
		action = "role:editor"
	}
	if strings.HasPrefix(strings.ToLower(action), "role:") {
		name := action[5:]
		if i := strings.IndexAny(name, " \t"); i >= 0 {
			name = name[:i]
		}
		resolved := c.ResolveRole(name)
		if resolved == "" {
			return ""
		}
		return resolved
	}
	out := action
	for _, ph := range []struct{ token, role string }{
		{"{terminal}", "terminal"},
		{"{editor}", "editor"},
		{"{browser}", "browser"},
		{"{filemgr}", "filemgr"},
		{"{files}", "files"},
		{"{image}", "image"},
	} {
		if !strings.Contains(out, ph.token) {
			continue
		}
		v := c.ResolveRole(ph.role)
		if v == "" {
			return ""
		}
		out = strings.ReplaceAll(out, ph.token, v)
	}
	return out
}

// OpenAction returns the launch action for a file path.
// ok=false means no association (caller should notify).
// Directories are not handled here — callers navigate in Files.
func (c Config) OpenAction(path string) (action string, ok bool) {
	ext := ExtOf(path)
	if ext == "" {
		return "", false
	}
	raw, found := c.Associations[ext]
	if !found || raw == "" {
		return "", false
	}
	action = c.expandAssociation(raw)
	if action == "" {
		return "", false
	}
	return action, true
}

func (c Config) expandAssociation(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "role:") {
		return c.ResolveRole(raw[5:])
	}
	return raw
}

// FormatMissingAssoc is the user-facing notify body for unknown types.
func FormatMissingAssoc(path string) string {
	ext := ExtOf(path)
	if ext == "" {
		return "No default app selected for file type: (none)"
	}
	return "No default app selected for file type: ." + ext
}
