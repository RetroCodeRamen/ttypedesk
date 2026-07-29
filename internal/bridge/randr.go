package bridge

import (
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
)

// mmPerPixel approximates a 96dpi screen (25.4mm / 96px ≈ 0.2646mm/px) —
// RANDR requires a physical size in millimeters, but Xvfb has no real
// display to measure. This is cosmetic (DPI reporting) only; nothing else
// in the bridge uses physical units.
const mmPerPixel = 0.2646

// initRandr enables the RANDR extension on conn, if the server has it.
// Xvfb ships it by default, but this is treated as optional — callers must
// handle a non-nil error by falling back to the existing fixed-resolution-
// plus-rescale behavior (EncodeHalfBlockFit already handles any size).
func initRandr(conn *xgb.Conn) error {
	return randr.Init(conn)
}

// setScreenSize resizes the live Xvfb screen to w x h via RANDR.
//
// This is more than a single request: a plain RRSetScreenSize alone fails
// with BadMatch (confirmed empirically, both ways — grow and shrink)
// whenever it doesn't match the screen's current CRTC/output mode, because
// nothing has told the CRTC its rectangle is changing too. The real
// recipe (the same one the `xrandr` CLI performs) is: create a mode of the
// requested size, attach it to the output, switch the output's CRTC to
// that mode (which itself resizes the CRTC's rectangle), and only then
// set the screen size to match.
//
// Every call creates a fresh RandR mode and leaves the previous one
// attached but unused on the server — a small, bounded leak for the life
// of this connection (the whole point is this runs rarely, debounced,
// after a resize settles, not per-frame) that disappears entirely when
// the bridge's dedicated Xvfb process is killed on Close.
func setScreenSize(conn *xgb.Conn, root xproto.Window, w, h int) error {
	if w < 1 || h < 1 {
		return fmt.Errorf("invalid screen size %dx%d", w, h)
	}

	res, err := randr.GetScreenResources(conn, root).Reply()
	if err != nil {
		return fmt.Errorf("GetScreenResources: %w", err)
	}
	if len(res.Crtcs) == 0 || len(res.Outputs) == 0 {
		return fmt.Errorf("no CRTC/output exposed by this X server")
	}
	crtc := res.Crtcs[0]
	output := res.Outputs[0]

	name := fmt.Sprintf("ttypedesk-%dx%d", w, h)
	mode := randr.ModeInfo{
		Width:      uint16(w),
		Height:     uint16(h),
		DotClock:   uint32(w) * uint32(h) * 60,
		HsyncStart: uint16(w),
		HsyncEnd:   uint16(w + 1),
		Htotal:     uint16(w + 2),
		VsyncStart: uint16(h),
		VsyncEnd:   uint16(h + 1),
		Vtotal:     uint16(h + 2),
		NameLen:    uint16(len(name)),
	}
	cm, err := randr.CreateMode(conn, root, mode, name).Reply()
	if err != nil {
		return fmt.Errorf("CreateMode: %w", err)
	}

	if err := randr.AddOutputModeChecked(conn, output, cm.Mode).Check(); err != nil {
		return fmt.Errorf("AddOutputMode: %w", err)
	}

	// SetCrtcConfig's Timestamp must be the CRTC's own last-known
	// timestamp (from GetCrtcInfo) — confirmed empirically: passing
	// xproto.TimeCurrentTime (the usual "don't care" wildcard value, 0)
	// here gets rejected with BadValue, unlike most other X requests that
	// accept it.
	ci, err := randr.GetCrtcInfo(conn, crtc, res.ConfigTimestamp).Reply()
	if err != nil {
		return fmt.Errorf("GetCrtcInfo: %w", err)
	}

	mmW := uint32(float64(w) * mmPerPixel)
	mmH := uint32(float64(h) * mmPerPixel)
	if mmW < 1 {
		mmW = 1
	}
	if mmH < 1 {
		mmH = 1
	}
	setScreen := func() error {
		return randr.SetScreenSizeChecked(conn, root, uint16(w), uint16(h), mmW, mmH).Check()
	}
	setCrtc := func() error {
		_, err := randr.SetCrtcConfig(conn, crtc, ci.Timestamp, res.ConfigTimestamp,
			0, 0, cm.Mode, randr.RotationRotate0, []randr.Output{output}).Reply()
		return err
	}

	// The CRTC's rectangle can never exceed the *current* screen
	// rectangle at any intermediate step (confirmed empirically: this is
	// what a plain single-request resize actually runs into) — so which
	// request has to go first depends on direction. Shrinking: shrink the
	// CRTC first (it still fits inside the still-large screen), then
	// shrink the screen to match. Growing: grow the screen first (a
	// no-op for the CRTC, which is still smaller than the new screen),
	// then grow the CRTC to fill it.
	if w > int(ci.Width) || h > int(ci.Height) {
		if err := setScreen(); err != nil {
			return fmt.Errorf("SetScreenSize (grow): %w", err)
		}
		if err := setCrtc(); err != nil {
			return fmt.Errorf("SetCrtcConfig (grow): %w", err)
		}
		return nil
	}
	if err := setCrtc(); err != nil {
		return fmt.Errorf("SetCrtcConfig (shrink): %w", err)
	}
	if err := setScreen(); err != nil {
		return fmt.Errorf("SetScreenSize (shrink): %w", err)
	}
	return nil
}
