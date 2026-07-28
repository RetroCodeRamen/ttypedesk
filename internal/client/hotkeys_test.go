package client

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestMatchBindingNilAndEmpty(t *testing.T) {
	e := tcell.NewEventKey(tcell.KeyF3, 0, tcell.ModNone)
	if matchBinding("", e) {
		t.Error(`matchBinding("", e) = true, want false`)
	}
	if matchBinding("f3", nil) {
		t.Error(`matchBinding("f3", nil) = true, want false`)
	}
}

func TestMatchBindingFunctionKeys(t *testing.T) {
	if !matchBinding("f3", tcell.NewEventKey(tcell.KeyF3, 0, tcell.ModNone)) {
		t.Error(`"f3" should match KeyF3`)
	}
	if matchBinding("f3", tcell.NewEventKey(tcell.KeyF4, 0, tcell.ModNone)) {
		t.Error(`"f3" should not match KeyF4`)
	}
	// Case-insensitive binding text.
	if !matchBinding("F10", tcell.NewEventKey(tcell.KeyF10, 0, tcell.ModNone)) {
		t.Error(`"F10" should match KeyF10 (binding text is lowercased)`)
	}
}

func TestMatchBindingModifiers(t *testing.T) {
	cases := []struct {
		binding string
		key     tcell.Key
		ch      rune
		mod     tcell.ModMask
		want    bool
	}{
		{"ctrl+shift+f", tcell.KeyRune, 'f', tcell.ModCtrl | tcell.ModShift, true},
		{"ctrl+shift+f", tcell.KeyRune, 'f', tcell.ModCtrl, false},  // missing shift
		{"ctrl+shift+f", tcell.KeyRune, 'f', tcell.ModShift, false}, // missing ctrl
		{"alt+/", tcell.KeyRune, '/', tcell.ModAlt, true},
		{"alt+/", tcell.KeyRune, '/', tcell.ModNone, false},
		{"space", tcell.KeyRune, ' ', tcell.ModNone, true},
		{"esc", tcell.KeyEscape, 0, tcell.ModNone, true},
		{"escape", tcell.KeyEscape, 0, tcell.ModNone, true},
		{"enter", tcell.KeyEnter, 0, tcell.ModNone, true},
		{"tab", tcell.KeyTab, 0, tcell.ModNone, true},
	}
	for _, c := range cases {
		e := tcell.NewEventKey(c.key, c.ch, c.mod)
		if got := matchBinding(c.binding, e); got != c.want {
			t.Errorf("matchBinding(%q, key=%v ch=%q mod=%v) = %v, want %v", c.binding, c.key, c.ch, c.mod, got, c.want)
		}
	}
}

func TestMatchBindingSpecialCasedLetters(t *testing.T) {
	// Real terminals report Ctrl+Q as the KeyCtrlQ key identity with no
	// separate modifier bit (there's no way to distinguish "Ctrl held" from
	// "this control byte" at the wire level) — matching tcell's own
	// normalization, confirmed via tcell.NewEventKey(KeyCtrlQ, 0, ModCtrl)
	// still reporting Key()==KeyCtrlQ regardless.
	if !matchBinding("q", tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModNone)) {
		t.Error(`"q" should match KeyCtrlQ`)
	}
	if !matchBinding("q", tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone)) {
		t.Error(`"q" should match a plain 'q' rune`)
	}
}

// Note: matchBinding also has a defensive branch rejecting an unshifted
// single-letter binding when Shift is reported alongside a KeyRune event.
// tcell.NewEventKey (the only public EventKey constructor — its fields are
// unexported) itself normalizes a lone Shift+letter down to ModNone
// ("Windows reports ModShift for shifted keys... harmonize this", see
// tcell's key.go), so that branch isn't reachable through the public API
// and isn't tested directly here.

func TestMatchBindingDefaultSingleCharCase(t *testing.T) {
	// "x" isn't one of the explicitly special-cased letters — falls through
	// to the default single-char branch, matching either case of the rune.
	if !matchBinding("x", tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone)) {
		t.Error(`"x" should match lowercase 'x'`)
	}
	if !matchBinding("x", tcell.NewEventKey(tcell.KeyRune, 'X', tcell.ModNone)) {
		t.Error(`"x" should match uppercase 'X'`)
	}
	if matchBinding("x", tcell.NewEventKey(tcell.KeyRune, 'y', tcell.ModNone)) {
		t.Error(`"x" should not match 'y'`)
	}
}

func TestMatchBindingUnknownKeyPart(t *testing.T) {
	if matchBinding("nonsense-key-name", tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone)) {
		t.Error("an unrecognized multi-char binding should never match")
	}
}
