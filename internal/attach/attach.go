// Package attach provides a thin Unix-socket snapshot/control attach path.
package attach

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ttypedesk/ttypedesk/internal/proto"
	"github.com/ttypedesk/ttypedesk/internal/server"
	"github.com/ttypedesk/ttypedesk/internal/surface"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

// Serve listens on a Unix socket and streams snapshot JSON lines to clients,
// reading key/mouse input back from them in the other direction.
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

func handleConn(srv *server.Server, conn net.Conn) {
	defer conn.Close()
	hello, _ := proto.Encode(proto.TypeAttach, "", map[string]any{"role": "snapshot"})
	_, _ = conn.Write(append(hello, '\n'))

	go readInput(srv, conn)

	ticker := time.NewTicker(time.Second / 10)
	defer ticker.Stop()
	for range ticker.C {
		snap := srv.Snapshot()
		msg, err := proto.Encode(proto.TypeSnapshot, "", snap)
		if err != nil {
			return
		}
		if _, err := conn.Write(append(msg, '\n')); err != nil {
			return
		}
	}
}

// readInput consumes key/mouse envelopes sent back by an attached client and
// applies them against the live window set. Keys go to whatever window is
// currently focused; mouse presses hit-test the window under the pointer,
// focus it, and forward content-area input the same way the local client
// does for PTY/app surfaces. Window chrome (title-bar drag, resize grips,
// taskbar/Start menu) is intentionally not remoted yet — v1 covers typing
// and interacting with whatever's already focused or on screen.
func readInput(srv *server.Server, conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var downID string
	for scanner.Scan() {
		env, err := proto.Decode(scanner.Bytes())
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

// RunViewer attaches to a socket, paints snapshots, and forwards local
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

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	lines := make(chan []byte, 4)
	go func() {
		for scanner.Scan() {
			b := append([]byte(nil), scanner.Bytes()...)
			select {
			case lines <- b:
			default:
			}
		}
		close(lines)
	}()

	var mouseDown bool
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
		case line, ok := <-lines:
			if !ok {
				return fmt.Errorf("connection closed")
			}
			env, err := proto.Decode(line)
			if err != nil || env.Type != proto.TypeSnapshot {
				continue
			}
			snap, err := proto.DecodePayload[proto.Snapshot](env)
			if err != nil {
				continue
			}
			paintSnapshot(screen, snap)
		}
	}
}

func sendEnvelope(conn net.Conn, typ proto.MessageType, payload any) {
	msg, err := proto.Encode(typ, "", payload)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(msg, '\n'))
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

func paintSnapshot(screen tcell.Screen, snap proto.Snapshot) {
	screen.Clear()
	style := func(fg, bg cell.Color) tcell.Style {
		return tcell.StyleDefault.
			Foreground(tcell.NewRGBColor(int32(fg.R), int32(fg.G), int32(fg.B))).
			Background(tcell.NewRGBColor(int32(bg.R), int32(bg.G), int32(bg.B)))
	}
	for _, w := range snap.Windows {
		for row := 0; row < w.Rows; row++ {
			for col := 0; col < w.Cols; col++ {
				i := row*w.Cols + col
				if i >= len(w.Cells) {
					continue
				}
				c := w.Cells[i]
				x, y := w.X+1+col, w.Y+1+row
				screen.SetContent(x, y, c.Rune, nil, style(c.FG, c.BG))
			}
		}
		// simple title
		title := w.Title
		for i, r := range title {
			screen.SetContent(w.X+1+i, w.Y, r, nil, style(cell.RGB(255, 255, 255), cell.RGB(0, 0, 170)))
		}
	}
	msg := " TTYPE Desk remote attach — Ctrl+Q detach "
	for i, r := range msg {
		screen.SetContent(i, 0, r, nil, style(cell.RGB(0, 0, 0), cell.RGB(200, 200, 200)))
	}
	screen.Show()
}
