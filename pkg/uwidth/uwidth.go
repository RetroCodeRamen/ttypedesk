package uwidth

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

func Rune(r rune) int {
	w := runewidth.RuneWidth(r)
	if w < 1 {
		return 1
	}
	return w
}

func String(s string) int {
	return runewidth.StringWidth(s)
}

// Truncate returns s fitting in maxCols cells, appending "…" if cut.
func Truncate(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	if String(s) <= maxCols {
		return s
	}
	if maxCols == 1 {
		return "…"
	}
	var b []rune
	w := 0
	for _, r := range s {
		rw := Rune(r)
		if w+rw > maxCols-1 {
			break
		}
		b = append(b, r)
		w += rw
	}
	return string(b) + "…"
}

// Pad pads s with spaces to exactly cols display cells (truncates if longer).
func Pad(s string, cols int) string {
	if cols <= 0 {
		return ""
	}
	s = Truncate(s, cols)
	for String(s) < cols {
		s += " "
	}
	return s
}

// ASCIIIcon returns a single-cell substitute when asciiMode is on and icon looks wide/emoji.
func ASCIIIcon(icon string, asciiMode bool) string {
	if !asciiMode || icon == "" {
		return icon
	}
	if sub, ok := emojiASCII[icon]; ok {
		return sub
	}
	// First rune; if wide, use a block
	for _, r := range icon {
		if Rune(r) > 1 {
			return "#"
		}
		return string(r)
	}
	return "*"
}

// StripForHit returns display width to use for hit-testing an icon glyph.
func IconCells(icon string, asciiMode bool) int {
	g := ASCIIIcon(icon, asciiMode)
	if g == "" {
		return 1
	}
	w := String(g)
	if w < 1 {
		return 1
	}
	return w
}

var emojiASCII = map[string]string{
	"💻": "T", "📁": "F", "🏠": "H", "🗑": "X", "📝": "N", "⚙️": "*",
	"📅": "C", "🕐": "O", "📖": "?", "📂": "D", "🖼": "I", "🔔": "!",
	"🚀": ">", "■": "#", "⌘": "*", "⭐": "*", "🔍": "?", "🔢": "=",
	"🔌": "@", "🗔": "W", "⏻": "Q",
	"🛍": "S", "➕": "+", "🪟": "W", "📄": "P", "⚠": "!", "⚠️": "!", "📦": "B",
	"🦊": "F", "📺": "V", "🗂": "D", "🎨": "A", "🃏": "J", "🎮": "G",
	"♟️": "C", "👻": "P", "📧": "M", "💬": "D", "🍵": "T", "🖵": "S",
	"🔵": "B", "📶": "W", "💾": "F", "🔐": "K",
}

// ContainsWide reports whether s has any double-width rune.
func ContainsWide(s string) bool {
	for _, r := range s {
		if Rune(r) > 1 {
			return true
		}
	}
	return false
}

// FitLabel keeps label within cols, preferring ASCII when needed.
func FitLabel(s string, cols int) string {
	return Truncate(strings.TrimSpace(s), cols)
}
