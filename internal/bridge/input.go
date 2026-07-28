package bridge

import (
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// inputInjector sends synthetic keyboard/mouse input into an X server via
// XTest. XTest's FakeInput takes a keycode, not a keysym, so injecting an
// arbitrary character means temporarily remapping a scratch keycode to the
// keysym we want first — the same trick tools like xdotool use, since it
// avoids needing to know or alter the guest's real keyboard layout.
type inputInjector struct {
	conn    *xgb.Conn
	root    xproto.Window
	scratch xproto.Keycode // remapped to whatever keysym we're about to send

	// Modifier keycodes, looked up once from the existing keymap — Xvfb's
	// default layout already has these, so no remap needed for them.
	shiftKC, ctrlKC, altKC xproto.Keycode
}

func newInputInjector(conn *xgb.Conn, root xproto.Window) (*inputInjector, error) {
	if err := xtest.Init(conn); err != nil {
		return nil, fmt.Errorf("xtest init: %w", err)
	}
	setup := xproto.Setup(conn)
	min, max := setup.MinKeycode, setup.MaxKeycode
	reply, err := xproto.GetKeyboardMapping(conn, min, byte(int(max)-int(min)+1)).Reply()
	if err != nil {
		return nil, fmt.Errorf("get keyboard mapping: %w", err)
	}
	inj := &inputInjector{conn: conn, root: root}
	per := int(reply.KeysymsPerKeycode)
	if per > 0 {
		for i, ks := range reply.Keysyms {
			kc := xproto.Keycode(int(min) + i/per)
			switch ks {
			case xkShiftL:
				inj.shiftKC = kc
			case xkControlL:
				inj.ctrlKC = kc
			case xkAltL:
				inj.altKC = kc
			}
		}
	}
	// Reserve the highest keycode as our scratch slot for arbitrary keysyms.
	inj.scratch = max
	return inj, nil
}

func (inj *inputInjector) fake(typ byte, detail xproto.Keycode) {
	_ = xtest.FakeInputChecked(inj.conn, typ, byte(detail), 0, inj.root, 0, 0, 0).Check()
}

func (inj *inputInjector) fakeButton(typ byte, button byte) {
	_ = xtest.FakeInputChecked(inj.conn, typ, button, 0, inj.root, 0, 0, 0).Check()
}

// sendKey presses and releases keysym, wrapped in whatever modifier keys
// are requested (sent as real key press/release pairs — XTest has no
// separate modifier-mask field, the server tracks modifiers from actual
// key state).
func (inj *inputInjector) sendKey(ks xproto.Keysym, ctrl, alt, shift bool) error {
	if err := xproto.ChangeKeyboardMappingChecked(inj.conn, 1, inj.scratch, 1, []xproto.Keysym{ks}).Check(); err != nil {
		return fmt.Errorf("remap scratch keycode: %w", err)
	}
	if ctrl && inj.ctrlKC != 0 {
		inj.fake(xproto.KeyPress, inj.ctrlKC)
	}
	if alt && inj.altKC != 0 {
		inj.fake(xproto.KeyPress, inj.altKC)
	}
	if shift && inj.shiftKC != 0 {
		inj.fake(xproto.KeyPress, inj.shiftKC)
	}
	inj.fake(xproto.KeyPress, inj.scratch)
	inj.fake(xproto.KeyRelease, inj.scratch)
	if shift && inj.shiftKC != 0 {
		inj.fake(xproto.KeyRelease, inj.shiftKC)
	}
	if alt && inj.altKC != 0 {
		inj.fake(xproto.KeyRelease, inj.altKC)
	}
	if ctrl && inj.ctrlKC != 0 {
		inj.fake(xproto.KeyRelease, inj.ctrlKC)
	}
	return nil
}

func (inj *inputInjector) moveMouse(x, y int16) {
	_ = xtest.FakeInputChecked(inj.conn, xproto.MotionNotify, 0, 0, inj.root, x, y, 0).Check()
}

func (inj *inputInjector) button(pressed bool, button byte) {
	typ := byte(xproto.ButtonRelease)
	if pressed {
		typ = xproto.ButtonPress
	}
	inj.fakeButton(typ, button)
}

func (inj *inputInjector) close() {
	if inj == nil || inj.conn == nil {
		return
	}
	// Best-effort: put the scratch keycode's mapping back to "nothing".
	_ = xproto.ChangeKeyboardMappingChecked(inj.conn, 1, inj.scratch, 1, []xproto.Keysym{0}).Check()
}
