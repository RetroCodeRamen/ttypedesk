package client

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

func (c *Client) matchHK(action string, e *tcell.EventKey) bool {
	return matchBinding(c.cfg.ResolveHotkey(action), e)
}

// matchBinding parses bindings like "ctrl+shift+f", "f3", "alt+/", "alt+space", "ctrl+q".
func matchBinding(binding string, e *tcell.EventKey) bool {
	binding = strings.ToLower(strings.TrimSpace(binding))
	if binding == "" || e == nil {
		return false
	}
	wantCtrl := strings.Contains(binding, "ctrl+")
	wantAlt := strings.Contains(binding, "alt+")
	wantShift := strings.Contains(binding, "shift+")
	haveCtrl := e.Modifiers()&tcell.ModCtrl != 0
	haveAlt := e.Modifiers()&tcell.ModAlt != 0
	haveShift := e.Modifiers()&tcell.ModShift != 0
	if wantCtrl != haveCtrl || wantAlt != haveAlt {
		return false
	}
	if wantShift && !haveShift {
		return false
	}

	keyPart := binding
	for _, p := range []string{"ctrl+", "alt+", "shift+"} {
		keyPart = strings.ReplaceAll(keyPart, p, "")
	}
	keyPart = strings.TrimSpace(keyPart)

	if !wantShift && haveShift {
		if len(keyPart) == 1 && keyPart[0] >= 'a' && keyPart[0] <= 'z' {
			return false
		}
	}

	switch keyPart {
	case "f1":
		return e.Key() == tcell.KeyF1
	case "f2":
		return e.Key() == tcell.KeyF2
	case "f3":
		return e.Key() == tcell.KeyF3
	case "f4":
		return e.Key() == tcell.KeyF4
	case "f5":
		return e.Key() == tcell.KeyF5
	case "f6":
		return e.Key() == tcell.KeyF6
	case "f7":
		return e.Key() == tcell.KeyF7
	case "f8":
		return e.Key() == tcell.KeyF8
	case "f9":
		return e.Key() == tcell.KeyF9
	case "f10":
		return e.Key() == tcell.KeyF10
	case "f11":
		return e.Key() == tcell.KeyF11
	case "f12":
		return e.Key() == tcell.KeyF12
	case "tab":
		return e.Key() == tcell.KeyTab
	case "space":
		return e.Key() == tcell.KeyRune && e.Rune() == ' '
	case "esc", "escape":
		return e.Key() == tcell.KeyEscape
	case "enter":
		return e.Key() == tcell.KeyEnter
	case "/":
		return e.Key() == tcell.KeyRune && e.Rune() == '/'
	case "q":
		return e.Key() == tcell.KeyCtrlQ || (e.Key() == tcell.KeyRune && (e.Rune() == 'q' || e.Rune() == 'Q'))
	case "w":
		return e.Key() == tcell.KeyCtrlW || (e.Key() == tcell.KeyRune && (e.Rune() == 'w' || e.Rune() == 'W'))
	case "m":
		return e.Key() == tcell.KeyCtrlM || (e.Key() == tcell.KeyRune && (e.Rune() == 'm' || e.Rune() == 'M'))
	case "p":
		return e.Key() == tcell.KeyCtrlP || (e.Key() == tcell.KeyRune && (e.Rune() == 'p' || e.Rune() == 'P'))
	case "c":
		return e.Key() == tcell.KeyRune && (e.Rune() == 'c' || e.Rune() == 'C')
	case "v":
		return e.Key() == tcell.KeyRune && (e.Rune() == 'v' || e.Rune() == 'V')
	case "f":
		return e.Key() == tcell.KeyRune && (e.Rune() == 'f' || e.Rune() == 'F')
	default:
		if len(keyPart) == 1 {
			r := rune(keyPart[0])
			up := r
			if r >= 'a' && r <= 'z' {
				up = r - 'a' + 'A'
			}
			return e.Key() == tcell.KeyRune && (e.Rune() == r || e.Rune() == up)
		}
	}
	return false
}
