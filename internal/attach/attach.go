// Package attach provides a thin Unix-socket snapshot/control attach path.
package attach

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ttypedesk/ttypedesk/internal/proto"
	"github.com/ttypedesk/ttypedesk/internal/server"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

// Serve listens on a Unix socket and streams snapshot JSON lines to clients.
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
	enc := json.NewEncoder(conn)
	// Hello
	hello, _ := proto.Encode(proto.TypeAttach, "", map[string]any{"role": "snapshot"})
	_, _ = conn.Write(append(hello, '\n'))

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
		_ = enc // keep import useful for future bidirectional control
	}
}

// RunViewer attaches to a socket and paints snapshots (read-only).
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

	for {
		select {
		case ev := <-events:
			if e, ok := ev.(*tcell.EventKey); ok {
				if e.Key() == tcell.KeyCtrlQ || e.Key() == tcell.KeyEscape {
					return nil
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
	msg := " TTYPE Desk remote attach (read-only) — Ctrl+Q quit "
	for i, r := range msg {
		screen.SetContent(i, 0, r, nil, style(cell.RGB(0, 0, 0), cell.RGB(200, 200, 200)))
	}
	screen.Show()
}
