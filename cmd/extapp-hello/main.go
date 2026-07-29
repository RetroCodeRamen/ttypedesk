// Command extapp-hello is a minimal, runnable reference implementation of
// TTYPE Desk's out-of-process App SDK protocol (see docs/extapp.md) — a
// real example for anyone writing an app in a language other than Go, and
// the fixture internal/surface's ExtAppSurface tests spawn as a genuine
// subprocess rather than mocking the wire protocol. Launch it from the
// desktop with an action string of "extapp:/path/to/extapp-hello".
package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/proto"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

func main() {
	var mu sync.Mutex // guards cols/rows/clicks/windowID and drawLocked's send
	cols, rows := 40, 12
	windowID := ""
	clicks := 0

	out := bufio.NewWriter(os.Stdout)
	var outMu sync.Mutex
	send := func(typ proto.MessageType, payload any) {
		data, err := proto.Encode(typ, windowID, payload)
		if err != nil {
			return
		}
		outMu.Lock()
		_, _ = out.Write(data)
		_ = out.WriteByte('\n')
		_ = out.Flush()
		outMu.Unlock()
	}

	// drawLocked must be called with mu held — it reads cols/rows/clicks
	// and sends the result, so callers on different goroutines (the
	// ticker below and the main NDJSON-reading loop) can't be allowed to
	// interleave their reads of that state with each other's writes.
	drawLocked := func() {
		grid := make([]cell.Cell, cols*rows)
		fg := cell.RGB(0xFF, 0xFF, 0xFF)
		bg := cell.RGB(0x00, 0x00, 0x80)
		for i := range grid {
			grid[i] = cell.Cell{Rune: ' ', FG: fg, BG: bg}
		}
		put := func(x, y int, text string) {
			for i, r := range text {
				cx := x + i
				if cx < 0 || cx >= cols || y < 0 || y >= rows {
					continue
				}
				grid[y*cols+cx] = cell.Cell{Rune: r, FG: fg, BG: bg}
			}
		}
		put(1, 1, "extapp-hello")
		put(1, 3, "Out-of-process App SDK demo")
		put(1, 5, fmt.Sprintf("Clicks: %d", clicks))
		put(1, 6, time.Now().Format("15:04:05"))
		if rows > 2 {
			put(1, rows-2, "q or Esc closes this window")
		}
		send(proto.TypeScreenDiff, proto.ScreenDiffPayload{Diff: cell.FullGridDiff(cols, rows, grid)})
	}

	// A self-driven redraw independent of any input from the host — shows
	// that a child can push updates on its own schedule (a live clock, an
	// animation, …), not just react to key/mouse/resize messages.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			mu.Lock()
			drawLocked()
			mu.Unlock()
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		env, err := proto.Decode(sc.Bytes())
		if err != nil {
			continue // one malformed line isn't worth crashing over
		}
		mu.Lock()
		switch env.Type {
		case proto.TypeInit:
			p, _ := proto.DecodePayload[proto.InitPayload](env)
			windowID = p.WindowID
			if p.Cols > 0 {
				cols = p.Cols
			}
			if p.Rows > 0 {
				rows = p.Rows
			}
			mu.Unlock()
			send(proto.TypeReady, proto.ReadyPayload{})
			mu.Lock()
			drawLocked()
		case proto.TypeResize:
			p, _ := proto.DecodePayload[proto.ResizeEvent](env)
			if p.Cols > 0 {
				cols = p.Cols
			}
			if p.Rows > 0 {
				rows = p.Rows
			}
			drawLocked()
		case proto.TypeKey:
			p, _ := proto.DecodePayload[proto.KeyEvent](env)
			if p.Key == "Escape" || p.Rune == 'q' || p.Rune == 'Q' {
				mu.Unlock()
				send(proto.TypeCloseWindow, nil)
				mu.Lock()
			} else {
				drawLocked()
			}
		case proto.TypeMouse:
			p, _ := proto.DecodePayload[proto.MouseEvent](env)
			if p.Action == "press" {
				clicks++
				drawLocked()
			}
		}
		mu.Unlock()
	}
	// Stdin closed (the host closes it as the first step of tearing down
	// this window) — exit cleanly rather than waiting to be killed.
}
