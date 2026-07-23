package config

import "strings"

// Hotkey action IDs (keys in Hotkeys map).
const (
	HKQuit       = "quit"
	HKFind       = "find"
	HKFindAlt    = "find_alt"
	HKFindLegacy = "find_legacy"
	HKStartMenu       = "start_menu"
	HKStartMenuAlt    = "start_menu_alt"
	HKStartMenuLegacy = "start_menu_legacy"
	HKClose           = "close"
	HKMinimize   = "minimize"
	HKCopy       = "copy"
	HKPaste      = "paste"
	HKCycleFocus = "cycle_focus"
	HKPalette    = "palette"
	HKPaletteAlt = "palette_alt"
)

// DefaultHotkeys are Desk-level bindings (not forwarded into PTYs when matched).
func DefaultHotkeys() map[string]string {
	return map[string]string{
		HKQuit:       "ctrl+q",
		HKFind:       "f3",
		HKFindAlt:    "alt+/",
		HKFindLegacy: "ctrl+shift+f",
		HKStartMenu:       "f10",
		HKStartMenuAlt:    "ctrl+esc",
		HKStartMenuLegacy: "alt+space",
		HKClose:           "ctrl+w",
		HKMinimize:   "ctrl+m",
		HKCopy:       "ctrl+shift+c",
		HKPaste:      "ctrl+shift+v",
		HKCycleFocus: "alt+tab",
		HKPalette:    "ctrl+space",
		HKPaletteAlt: "ctrl+p",
	}
}

// HotkeyLabel is a human label for Settings.
func HotkeyLabel(action string) string {
	switch action {
	case HKQuit:
		return "Quit Desk"
	case HKFind:
		return "Find in scrollback"
	case HKFindAlt:
		return "Find (alternate)"
	case HKFindLegacy:
		return "Find (legacy Ctrl+Shift+F)"
	case HKStartMenu:
		return "Start menu"
	case HKStartMenuAlt:
		return "Start menu (alternate)"
	case HKStartMenuLegacy:
		return "Start menu (legacy Alt+Space)"
	case HKClose:
		return "Close window"
	case HKMinimize:
		return "Minimize window"
	case HKCopy:
		return "Copy selection"
	case HKPaste:
		return "Paste clipboard"
	case HKCycleFocus:
		return "Cycle window focus"
	case HKPalette:
		return "Command palette"
	case HKPaletteAlt:
		return "Command palette (alternate)"
	default:
		return action
	}
}

// HotkeyActions is the Settings order.
func HotkeyActions() []string {
	return []string{
		HKPalette, HKPaletteAlt, HKFind, HKFindAlt, HKFindLegacy,
		HKStartMenu, HKStartMenuAlt, HKStartMenuLegacy,
		HKClose, HKMinimize, HKCopy, HKPaste, HKCycleFocus, HKQuit,
	}
}

// ResolveHotkey returns the binding for action, or default if unset.
func (c Config) ResolveHotkey(action string) string {
	if c.Hotkeys != nil {
		if v, ok := c.Hotkeys[action]; ok && strings.TrimSpace(v) != "" {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	if d, ok := DefaultHotkeys()[action]; ok {
		return d
	}
	return ""
}
