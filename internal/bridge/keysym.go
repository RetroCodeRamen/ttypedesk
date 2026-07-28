package bridge

import "github.com/jezek/xgb/xproto"

// X11 keysyms for the non-printable keys the desktop's InputEvent.Key names
// map to. Printable runes need no table: X11 keysyms 0x20-0xFF are defined
// to equal their Latin-1/Unicode code point directly.
const (
	xkBackSpace = 0xff08
	xkTab       = 0xff09
	xkReturn    = 0xff0d
	xkEscape    = 0xff1b
	xkDelete    = 0xffff
	xkHome      = 0xff50
	xkLeft      = 0xff51
	xkUp        = 0xff52
	xkRight     = 0xff53
	xkDown      = 0xff54
	xkPageUp    = 0xff55
	xkPageDown  = 0xff56
	xkEnd       = 0xff57
	xkInsert    = 0xff63

	xkShiftL   = 0xffe1
	xkControlL = 0xffe3
	xkAltL     = 0xffe9
)

// namedKeysyms maps the desktop's InputEvent.Key strings (see
// internal/surface.InputEvent, same set PtySurface switches on) to keysyms.
var namedKeysyms = map[string]xproto.Keysym{
	"Enter":     xkReturn,
	"Tab":       xkTab,
	"Backspace": xkBackSpace,
	"Escape":    xkEscape,
	"Up":        xkUp,
	"Down":      xkDown,
	"Left":      xkLeft,
	"Right":     xkRight,
	"Home":      xkHome,
	"End":       xkEnd,
	"PgUp":      xkPageUp,
	"PgDn":      xkPageDown,
	"Delete":    xkDelete,
	"Insert":    xkInsert,
}

// runeKeysym returns the keysym for a printable rune, and false for
// anything outside the directly-mapped Latin-1 range (0x20-0xFF); callers
// fall back to namedKeysyms or drop the event.
func runeKeysym(r rune) (xproto.Keysym, bool) {
	if r >= 0x20 && r <= 0xff {
		return xproto.Keysym(r), true
	}
	return 0, false
}
