package config

import (
	"strings"

	"github.com/ttypedesk/ttypedesk/assets"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

// Theme pack IDs / display names.
const (
	ThemeXP      = "XP"
	ThemeScarlet = "Scarlet"
	ThemeBumble  = "Bumble"
	ThemeBubble  = "Bubble"
	ThemeSprout  = "Sprout"
)

// ThemePack pairs chrome colors with a wallpaper.
type ThemePack struct {
	ID        string // xp, scarlet, bumble, bubble, sprout
	Name      string // display name
	Tagline   string // short fun blurb
	Wallpaper string // builtin:…
	Theme     Theme
}

// ThemePacks returns the shipped theme packs (XP + wallpaper-matched palettes).
func ThemePacks() []ThemePack {
	return []ThemePack{
		{
			ID: "xp", Name: ThemeXP, Tagline: "classic Bliss hills",
			Wallpaper: assets.BuiltinBliss, Theme: XPTheme(),
		},
		{
			ID: "scarlet", Name: ThemeScarlet, Tagline: "red & black heat",
			Wallpaper: assets.BuiltinScarlet, Theme: ScarletTheme(),
		},
		{
			ID: "bumble", Name: ThemeBumble, Tagline: "yellow black white hive",
			Wallpaper: assets.BuiltinBumble, Theme: BumbleTheme(),
		},
		{
			ID: "bubble", Name: ThemeBubble, Tagline: "layers of blue",
			Wallpaper: assets.BuiltinBubble, Theme: BubbleTheme(),
		},
		{
			ID: "sprout", Name: ThemeSprout, Tagline: "green face, blue & yellow pops",
			Wallpaper: assets.BuiltinSprout, Theme: SproutTheme(),
		},
	}
}

// LookupThemePack matches by ID or display name (case-insensitive).
func LookupThemePack(idOrName string) (ThemePack, bool) {
	key := strings.ToLower(strings.TrimSpace(idOrName))
	for _, p := range ThemePacks() {
		if key == p.ID || key == strings.ToLower(p.Name) {
			return p, true
		}
	}
	return ThemePack{}, false
}

// NextThemePack returns the pack after the current theme name (wraps).
func NextThemePack(currentName string) ThemePack {
	packs := ThemePacks()
	cur := strings.ToLower(strings.TrimSpace(currentName))
	for i, p := range packs {
		if cur == p.ID || cur == strings.ToLower(p.Name) {
			return packs[(i+1)%len(packs)]
		}
	}
	return packs[0]
}

// ApplyThemePack sets chrome colors and wallpaper for a pack.
func (c *Config) ApplyThemePack(idOrName string) bool {
	p, ok := LookupThemePack(idOrName)
	if !ok {
		return false
	}
	c.Theme = p.Theme
	c.Wallpaper.Mode = "image"
	c.Wallpaper.Path = p.Wallpaper
	c.Wallpaper.Fit = "cover"
	return true
}

// ApplyXPTheme sets XP chrome colors and Bliss wallpaper (image/cover).
func (c *Config) ApplyXPTheme() {
	_ = c.ApplyThemePack("xp")
}

// XPTheme is the classic TTYPE Desk look: XP blues/greens + Bliss wallpaper by default.
func XPTheme() Theme {
	return Theme{
		Name:            ThemeXP,
		DesktopBG:       cell.RGB(0x00, 0x55, 0xAA),
		DesktopPattern:  "░",
		TaskbarBG:       cell.RGB(0x00, 0x00, 0xAA),
		TaskbarFG:       cell.RGB(0xFF, 0xFF, 0xFF),
		StartBG:         cell.RGB(0x00, 0xAA, 0x00),
		StartFG:         cell.RGB(0xFF, 0xFF, 0xFF),
		MenuBG:          cell.RGB(0xC0, 0xC0, 0xC0),
		MenuFG:          cell.RGB(0x00, 0x00, 0x00),
		MenuHighlight:   cell.RGB(0x00, 0x00, 0xAA),
		TitleFocused:    cell.RGB(0x00, 0x00, 0xAA),
		TitleUnfocused:  cell.RGB(0x80, 0x80, 0x80),
		TitleFG:         cell.RGB(0xFF, 0xFF, 0xFF),
		BorderFocused:   cell.RGB(0xFF, 0xFF, 0xFF),
		BorderUnfocused: cell.RGB(0xA0, 0xA0, 0xA0),
		WindowBody:      cell.RGB(0x00, 0x00, 0x00),
		Shadow:          cell.RGB(0x00, 0x00, 0x00),
		ShadowDX:        2,
		ShadowDY:        1,
		DefaultFG:       cell.RGB(0xCC, 0xCC, 0xCC),
		DefaultBG:       cell.RGB(0x00, 0x00, 0x00),
		IconFG:          cell.RGB(0xFF, 0xFF, 0xFF),
		IconLabelFG:     cell.RGB(0xFF, 0xFF, 0xCC),
	}
}

// ScarletTheme — red & black heat (Red wallpaper).
func ScarletTheme() Theme {
	return Theme{
		Name:            ThemeScarlet,
		DesktopBG:       cell.RGB(0x1A, 0x00, 0x00),
		DesktopPattern:  "",
		TaskbarBG:       cell.RGB(0x00, 0x00, 0x00),
		TaskbarFG:       cell.RGB(0xFF, 0xCC, 0xCC),
		StartBG:         cell.RGB(0xCC, 0x00, 0x00),
		StartFG:         cell.RGB(0xFF, 0xFF, 0xFF),
		MenuBG:          cell.RGB(0x1A, 0x1A, 0x1A),
		MenuFG:          cell.RGB(0xFF, 0xCC, 0xCC),
		MenuHighlight:   cell.RGB(0xAA, 0x00, 0x00),
		TitleFocused:    cell.RGB(0x99, 0x00, 0x00),
		TitleUnfocused:  cell.RGB(0x40, 0x20, 0x20),
		TitleFG:         cell.RGB(0xFF, 0xFF, 0xFF),
		BorderFocused:   cell.RGB(0xFF, 0x33, 0x33),
		BorderUnfocused: cell.RGB(0x66, 0x22, 0x22),
		WindowBody:      cell.RGB(0x00, 0x00, 0x00),
		Shadow:          cell.RGB(0x00, 0x00, 0x00),
		ShadowDX:        2,
		ShadowDY:        1,
		DefaultFG:       cell.RGB(0xEE, 0xCC, 0xCC),
		DefaultBG:       cell.RGB(0x00, 0x00, 0x00),
		IconFG:          cell.RGB(0xFF, 0xFF, 0xFF),
		IconLabelFG:     cell.RGB(0xFF, 0xAA, 0xAA),
	}
}

// BumbleTheme — yellow / black / white hive (honeycomb wallpaper).
func BumbleTheme() Theme {
	return Theme{
		Name:            ThemeBumble,
		DesktopBG:       cell.RGB(0x0A, 0x08, 0x00),
		DesktopPattern:  "",
		TaskbarBG:       cell.RGB(0x00, 0x00, 0x00),
		TaskbarFG:       cell.RGB(0xFF, 0xDD, 0x00),
		StartBG:         cell.RGB(0xFF, 0xCC, 0x00),
		StartFG:         cell.RGB(0x00, 0x00, 0x00),
		MenuBG:          cell.RGB(0xFF, 0xF8, 0xE7),
		MenuFG:          cell.RGB(0x00, 0x00, 0x00),
		MenuHighlight:   cell.RGB(0xFF, 0xCC, 0x00),
		TitleFocused:    cell.RGB(0x00, 0x00, 0x00),
		TitleUnfocused:  cell.RGB(0x55, 0x44, 0x00),
		TitleFG:         cell.RGB(0xFF, 0xDD, 0x00),
		BorderFocused:   cell.RGB(0xFF, 0xDD, 0x00),
		BorderUnfocused: cell.RGB(0x66, 0x66, 0x66),
		WindowBody:      cell.RGB(0x00, 0x00, 0x00),
		Shadow:          cell.RGB(0x00, 0x00, 0x00),
		ShadowDX:        2,
		ShadowDY:        1,
		DefaultFG:       cell.RGB(0xEE, 0xEE, 0xCC),
		DefaultBG:       cell.RGB(0x00, 0x00, 0x00),
		IconFG:          cell.RGB(0xFF, 0xFF, 0xFF),
		IconLabelFG:     cell.RGB(0xFF, 0xFF, 0xFF),
	}
}

// BubbleTheme — stacked blues (BlueBubble wallpaper).
func BubbleTheme() Theme {
	return Theme{
		Name:            ThemeBubble,
		DesktopBG:       cell.RGB(0x00, 0x33, 0x66),
		DesktopPattern:  "",
		TaskbarBG:       cell.RGB(0x00, 0x1A, 0x44),
		TaskbarFG:       cell.RGB(0xCC, 0xEE, 0xFF),
		StartBG:         cell.RGB(0x33, 0xAA, 0xFF),
		StartFG:         cell.RGB(0xFF, 0xFF, 0xFF),
		MenuBG:          cell.RGB(0xE8, 0xF4, 0xFF),
		MenuFG:          cell.RGB(0x00, 0x22, 0x44),
		MenuHighlight:   cell.RGB(0x00, 0x88, 0xCC),
		TitleFocused:    cell.RGB(0x00, 0x66, 0xAA),
		TitleUnfocused:  cell.RGB(0x44, 0x66, 0x88),
		TitleFG:         cell.RGB(0xFF, 0xFF, 0xFF),
		BorderFocused:   cell.RGB(0x66, 0xCC, 0xFF),
		BorderUnfocused: cell.RGB(0x44, 0x66, 0x88),
		WindowBody:      cell.RGB(0x00, 0x11, 0x22),
		Shadow:          cell.RGB(0x00, 0x00, 0x22),
		ShadowDX:        2,
		ShadowDY:        1,
		DefaultFG:       cell.RGB(0xCC, 0xDD, 0xEE),
		DefaultBG:       cell.RGB(0x00, 0x11, 0x22),
		IconFG:          cell.RGB(0xFF, 0xFF, 0xFF),
		IconLabelFG:     cell.RGB(0xCC, 0xEE, 0xFF),
	}
}

// SproutTheme — green face with blue & yellow accents.
func SproutTheme() Theme {
	return Theme{
		Name:            ThemeSprout,
		DesktopBG:       cell.RGB(0x1A, 0x55, 0x30),
		DesktopPattern:  "",
		TaskbarBG:       cell.RGB(0x0D, 0x33, 0x20),
		TaskbarFG:       cell.RGB(0xDD, 0xFF, 0xEE),
		StartBG:         cell.RGB(0xFF, 0xCC, 0x00),
		StartFG:         cell.RGB(0x00, 0x33, 0x00),
		MenuBG:          cell.RGB(0xE8, 0xFF, 0xE8),
		MenuFG:          cell.RGB(0x00, 0x30, 0x20),
		MenuHighlight:   cell.RGB(0x22, 0x77, 0xBB),
		TitleFocused:    cell.RGB(0x22, 0x88, 0x44),
		TitleUnfocused:  cell.RGB(0x55, 0x77, 0x66),
		TitleFG:         cell.RGB(0xFF, 0xFF, 0xFF),
		BorderFocused:   cell.RGB(0xFF, 0xDD, 0x44),
		BorderUnfocused: cell.RGB(0x55, 0x88, 0x66),
		WindowBody:      cell.RGB(0x00, 0x1A, 0x0D),
		Shadow:          cell.RGB(0x00, 0x11, 0x00),
		ShadowDX:        2,
		ShadowDY:        1,
		DefaultFG:       cell.RGB(0xCC, 0xEE, 0xCC),
		DefaultBG:       cell.RGB(0x00, 0x1A, 0x0D),
		IconFG:          cell.RGB(0xFF, 0xFF, 0xFF),
		IconLabelFG:     cell.RGB(0xFF, 0xFF, 0xAA),
	}
}
