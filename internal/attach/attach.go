// Package attach provides a thin Unix-socket snapshot/control attach path.
package attach

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/audio"
	"github.com/RetroCodeRamen/ttypedesk/internal/audiocap"
	"github.com/RetroCodeRamen/ttypedesk/internal/proto"
	"github.com/RetroCodeRamen/ttypedesk/internal/server"
	"github.com/RetroCodeRamen/ttypedesk/internal/surface"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/gdamore/tcell/v2"
)

// Serve listens on a Unix socket and streams binary cell-diff frames to
// clients, reading key/mouse input back from them in the other
// direction. See internal/proto's FrameJSON/FrameDiff doc comment for
// the wire framing.
func Serve(srv *server.Server, path string) error {
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleConn(srv, conn)
	}
}

// frameWriter serializes WriteFrame calls from the diff ticker and (when
// audio streaming is on) the audio-forwarding goroutine — both write to
// the same conn concurrently otherwise, and net.Conn.Write only promises
// safe concurrent *use*, not that two goroutines' writes can't interleave
// mid-buffer, which would desync the whole length-prefixed framing.
type frameWriter struct {
	mu   sync.Mutex
	conn net.Conn
}

func (w *frameWriter) write(typ byte, payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return proto.WriteFrame(w.conn, typ, payload)
}

func handleConn(srv *server.Server, conn net.Conn) {
	defer conn.Close()
	fw := &frameWriter{conn: conn}
	hello, _ := proto.Encode(proto.TypeAttach, "", map[string]any{"role": "snapshot"})
	if err := fw.write(proto.FrameJSON, hello); err != nil {
		return
	}

	go readInput(srv, conn)

	// Audio streaming is opt-in (Settings → Audio streaming) and decided
	// once per connection at attach time — Mute, unlike Enabled, is
	// checked live every chunk in streamAudio so it takes effect
	// immediately without needing to reattach.
	if srv.Config().AudioStream.Enabled {
		go streamAudio(srv, fw)
	}

	// lastCells is this connection's own view of what it's already been
	// sent, per window — never shared with another attached client or
	// with the local render loop, so multiple simultaneous attachments
	// (or attach alongside local rendering) never fight over what counts
	// as "already sent."
	lastCells := make(map[string][]cell.Cell)
	ticker := time.NewTicker(time.Second / 10)
	defer ticker.Stop()
	for range ticker.C {
		snap := srv.Snapshot()
		frame := buildDiffFrame(snap, lastCells)
		if err := fw.write(proto.FrameDiff, proto.EncodeDiffFrame(frame)); err != nil {
			return
		}
	}
}

// streamAudio captures the host's current audio output and forwards it to
// one attach connection as FrameAudio chunks, until capture fails or the
// connection's diff loop returns (which closes conn, which fails fw.write
// here too — no separate shutdown signal needed). Mute is re-checked on
// every chunk via srv.Config() so toggling it in Settings mid-session
// takes effect immediately: capture keeps running either way, chunks are
// just dropped while muted rather than tearing down and restarting parec.
func streamAudio(srv *server.Server, fw *frameWriter) {
	events := make(chan audiocap.Event, 1)
	stream, err := audiocap.Capture(events)
	if err != nil {
		return
	}
	defer stream.Stop()
	for samples := range stream.PCM {
		if srv.Config().AudioStream.Mute {
			continue
		}
		if err := fw.write(proto.FrameAudio, proto.EncodeAudioChunk(samples)); err != nil {
			return
		}
	}
}

// buildDiffFrame converts a full Snapshot into a DiffFrame for one
// connection: window metadata is always included (cheap), but a
// window's cell grid is only included if it changed since the last
// frame built with this same lastCells cache — which this function
// updates in place, including pruning entries for windows that have
// since closed (unbounded growth over a long attach session otherwise).
func buildDiffFrame(snap proto.Snapshot, lastCells map[string][]cell.Cell) proto.DiffFrame {
	seen := make(map[string]bool, len(snap.Windows))
	f := proto.DiffFrame{Cols: snap.Cols, Rows: snap.Rows}
	for _, w := range snap.Windows {
		seen[w.ID] = true
		dw := proto.DiffWindow{
			ID: w.ID, Title: w.Title,
			X: w.X, Y: w.Y, W: w.W, H: w.H, Z: w.Z,
			Focused: w.Focused, Maximized: w.Maximized, Kind: w.Kind,
			Cols: w.Cols, Rows: w.Rows,
		}
		if !cellsEqual(lastCells[w.ID], w.Cells) {
			dw.Cells = w.Cells
			lastCells[w.ID] = w.Cells
		}
		f.Windows = append(f.Windows, dw)
	}
	for id := range lastCells {
		if !seen[id] {
			delete(lastCells, id)
		}
	}
	return f
}

func cellsEqual(a, b []cell.Cell) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// readInput consumes key/mouse envelopes sent back by an attached client and
// applies them against the live window set. Keys go to whatever window is
// currently focused; mouse presses hit-test the window under the pointer,
// focus it, and forward content-area input the same way the local client
// does for PTY/app surfaces. Window chrome (title-bar drag, resize grips,
// taskbar/Start menu) is intentionally not remoted yet — v1 covers typing
// and interacting with whatever's already focused or on screen.
func readInput(srv *server.Server, conn net.Conn) {
	r := bufio.NewReader(conn)
	var downID string
	for {
		typ, payload, err := proto.ReadFrame(r)
		if err != nil {
			return
		}
		if typ != proto.FrameJSON {
			continue
		}
		env, err := proto.Decode(payload)
		if err != nil {
			continue
		}
		switch env.Type {
		case proto.TypeKey:
			ev, err := proto.DecodePayload[proto.KeyEvent](env)
			if err != nil {
				continue
			}
			focus := srv.Focused()
			if focus == "" {
				continue
			}
			srv.HandleInput(focus, surface.InputEvent{
				Kind: "key", Rune: ev.Rune, Key: ev.Key,
				Ctrl: ev.Ctrl, Alt: ev.Alt, Shift: ev.Shift, Bytes: ev.Bytes,
			})
		case proto.TypeMouse:
			ev, err := proto.DecodePayload[proto.MouseEvent](env)
			if err != nil {
				continue
			}
			dispatchRemoteMouse(srv, ev, &downID)
		}
	}
}

func dispatchRemoteMouse(srv *server.Server, ev proto.MouseEvent, downID *string) {
	if ev.Action == "wheel" {
		for _, w := range srv.Windows() {
			if w.Minimized {
				continue
			}
			if ev.X > w.X && ev.X < w.X+w.W-1 && ev.Y > w.Y && ev.Y < w.Y+w.H-1 {
				srv.HandleInput(w.ID, surface.InputEvent{
					Kind: "scroll", X: ev.X - w.X - 1, Y: ev.Y - w.Y - 1, Button: ev.Button,
				})
				return
			}
		}
		return
	}

	if ev.Action == "release" {
		if *downID == "" {
			return
		}
		w := srv.Get(*downID)
		*downID = ""
		if w == nil {
			return
		}
		srv.HandleInput(w.ID, surface.InputEvent{
			Kind: "mouse", X: ev.X - w.X - 1, Y: ev.Y - w.Y - 1, Button: 1, Action: "release",
		})
		return
	}

	wins := srv.Windows()
	for i := len(wins) - 1; i >= 0; i-- {
		w := wins[i]
		if w.Minimized {
			continue
		}
		if ev.X < w.X || ev.Y < w.Y || ev.X >= w.X+w.W || ev.Y >= w.Y+w.H {
			continue
		}
		if ev.Action == "press" && srv.Focused() != w.ID {
			srv.Focus(w.ID)
		}
		if ev.X > w.X && ev.X < w.X+w.W-1 && ev.Y > w.Y && ev.Y < w.Y+w.H-1 {
			if ev.Action == "press" {
				*downID = w.ID
			}
			srv.HandleInput(w.ID, surface.InputEvent{
				Kind: "mouse", X: ev.X - w.X - 1, Y: ev.Y - w.Y - 1, Button: 1, Action: ev.Action,
				Ctrl: ev.Ctrl, Alt: ev.Alt, Shift: ev.Shift,
			})
		}
		return
	}
}

// RunViewer attaches to a socket, paints diff frames, and forwards local
// keyboard/mouse input back to the host desktop. Ctrl+Q detaches.
func RunViewer(path string) error {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return err
	}
	defer conn.Close()

	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.EnableMouse()

	events := make(chan tcell.Event, 8)
	go func() {
		for {
			ev := screen.PollEvent()
			if ev == nil {
				return
			}
			events <- ev
		}
	}()

	r := bufio.NewReader(conn)
	type wireFrame struct {
		typ     byte
		payload []byte
	}
	frames := make(chan wireFrame, 4)
	go func() {
		for {
			typ, payload, err := proto.ReadFrame(r)
			if err != nil {
				close(frames)
				return
			}
			select {
			case frames <- wireFrame{typ, payload}:
			default:
			}
		}
	}()

	lastCells := make(map[string][]cell.Cell)
	var mouseDown bool
	var audioPCM chan []int16
	var audioFailed bool
	var audioPlayback *audio.Playback
	defer func() {
		if audioPlayback != nil {
			audioPlayback.Stop()
		}
	}()
	for {
		select {
		case ev := <-events:
			switch e := ev.(type) {
			case *tcell.EventKey:
				if e.Key() == tcell.KeyCtrlQ {
					return nil
				}
				if key, ok := remoteKeyEvent(e); ok {
					sendEnvelope(conn, proto.TypeKey, key)
				}
			case *tcell.EventMouse:
				x, y := e.Position()
				btn := e.Buttons()
				mod := e.Modifiers()
				me := proto.MouseEvent{
					X: x, Y: y,
					Ctrl:  mod&tcell.ModCtrl != 0,
					Alt:   mod&tcell.ModAlt != 0,
					Shift: mod&tcell.ModShift != 0,
				}
				switch {
				case btn&tcell.WheelUp != 0:
					me.Action, me.Button = "wheel", 3
					sendEnvelope(conn, proto.TypeMouse, me)
				case btn&tcell.WheelDown != 0:
					me.Action, me.Button = "wheel", -3
					sendEnvelope(conn, proto.TypeMouse, me)
				case btn&tcell.ButtonPrimary != 0:
					me.Button = 1
					if mouseDown {
						me.Action = "drag"
					} else {
						me.Action = "press"
					}
					mouseDown = true
					sendEnvelope(conn, proto.TypeMouse, me)
				case mouseDown:
					mouseDown = false
					me.Action, me.Button = "release", 1
					sendEnvelope(conn, proto.TypeMouse, me)
				}
			}
		case fr, ok := <-frames:
			if !ok {
				return fmt.Errorf("connection closed")
			}
			switch fr.typ {
			case proto.FrameDiff:
				df, err := proto.DecodeDiffFrame(fr.payload)
				if err != nil {
					continue
				}
				paintDiffFrame(screen, df, lastCells)
			case proto.FrameAudio:
				if audioFailed {
					continue
				}
				if audioPCM == nil {
					audioPCM = make(chan []int16, 8)
					pb, err := audio.Play(audioPCM)
					if err != nil {
						audioFailed = true
						audioPCM = nil
						continue
					}
					audioPlayback = pb
				}
				samples := proto.DecodeAudioChunk(fr.payload)
				// Non-blocking: this is a live stream, not a file decode —
				// dropping samples under backpressure keeps the viewer
				// loop (and diff-frame reads) responsive, unlike
				// internal/audio's usual blocking-as-backpressure pattern
				// which is only right when something local is feeding it.
				select {
				case audioPCM <- samples:
				default:
				}
			}
			// FrameJSON here would just be a stray post-hello envelope —
			// nothing to do with it.
		}
	}
}

func sendEnvelope(conn net.Conn, typ proto.MessageType, payload any) {
	msg, err := proto.Encode(typ, "", payload)
	if err != nil {
		return
	}
	_ = proto.WriteFrame(conn, proto.FrameJSON, msg)
}

// remoteKeyEvent maps a tcell key event to a wire KeyEvent, mirroring the
// local client's key handling. Unmapped keys are dropped.
func remoteKeyEvent(e *tcell.EventKey) (proto.KeyEvent, bool) {
	ev := proto.KeyEvent{
		Ctrl:  e.Modifiers()&tcell.ModCtrl != 0,
		Alt:   e.Modifiers()&tcell.ModAlt != 0,
		Shift: e.Modifiers()&tcell.ModShift != 0,
	}
	switch e.Key() {
	case tcell.KeyEnter:
		ev.Key = "Enter"
	case tcell.KeyTab:
		ev.Key = "Tab"
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		ev.Key = "Backspace"
	case tcell.KeyEscape:
		ev.Key = "Escape"
	case tcell.KeyUp:
		ev.Key = "Up"
	case tcell.KeyDown:
		ev.Key = "Down"
	case tcell.KeyLeft:
		ev.Key = "Left"
	case tcell.KeyRight:
		ev.Key = "Right"
	case tcell.KeyHome:
		ev.Key = "Home"
	case tcell.KeyEnd:
		ev.Key = "End"
	case tcell.KeyPgUp:
		ev.Key = "PgUp"
	case tcell.KeyPgDn:
		ev.Key = "PgDn"
	case tcell.KeyDelete:
		ev.Key = "Delete"
	case tcell.KeyInsert:
		ev.Key = "Insert"
	case tcell.KeyCtrlC:
		ev.Bytes = []byte{0x03}
	case tcell.KeyCtrlD:
		ev.Bytes = []byte{0x04}
	case tcell.KeyCtrlZ:
		ev.Bytes = []byte{0x1a}
	case tcell.KeyCtrlL:
		ev.Bytes = []byte{0x0c}
	case tcell.KeyRune:
		ev.Rune = e.Rune()
	default:
		return proto.KeyEvent{}, false
	}
	return ev, true
}

// paintDiffFrame renders one DiffFrame to screen. lastCells is the
// client's own cache, keyed by window ID: a window with Cells == nil
// this frame (unchanged since the last one that included them) reuses
// whatever's cached; a window with real Cells updates the cache. Also
// prunes cache entries for windows no longer present (closed host-side).
func paintDiffFrame(screen tcell.Screen, f proto.DiffFrame, lastCells map[string][]cell.Cell) {
	screen.Clear()
	style := func(fg, bg cell.Color) tcell.Style {
		return tcell.StyleDefault.
			Foreground(tcell.NewRGBColor(int32(fg.R), int32(fg.G), int32(fg.B))).
			Background(tcell.NewRGBColor(int32(bg.R), int32(bg.G), int32(bg.B)))
	}
	seen := make(map[string]bool, len(f.Windows))
	for _, w := range f.Windows {
		seen[w.ID] = true
		cells := w.Cells
		if cells == nil {
			cells = lastCells[w.ID]
		} else {
			lastCells[w.ID] = cells
		}
		for row := 0; row < w.Rows; row++ {
			for col := 0; col < w.Cols; col++ {
				i := row*w.Cols + col
				if i >= len(cells) {
					continue
				}
				c := cells[i]
				x, y := w.X+1+col, w.Y+1+row
				screen.SetContent(x, y, c.Rune, nil, style(c.FG, c.BG))
			}
		}
		title := w.Title
		for i, r := range title {
			screen.SetContent(w.X+1+i, w.Y, r, nil, style(cell.RGB(255, 255, 255), cell.RGB(0, 0, 170)))
		}
	}
	for id := range lastCells {
		if !seen[id] {
			delete(lastCells, id)
		}
	}
	msg := " TTYPE Desk remote attach — Ctrl+Q detach "
	for i, r := range msg {
		screen.SetContent(i, 0, r, nil, style(cell.RGB(0, 0, 0), cell.RGB(200, 200, 200)))
	}
	screen.Show()
}
