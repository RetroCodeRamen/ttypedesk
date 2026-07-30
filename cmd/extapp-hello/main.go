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
	"math"
	"os"
	"sync"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/proto"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
)

func main() {
	var mu sync.Mutex // guards all state below and drawLocked's send
	cols, rows := 40, 12
	windowID := ""
	clicks := 0
	credStatus := "press c"
	clipStatus := "press v"
	pickStatus := "press f"
	audioOn := false
	var audioStop chan struct{}

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

	// pending correlates a request/response pair (credential save/load,
	// clipboard get, file picker) by the ReqID sendReq generates — the
	// reply arrives on the same NDJSON reader as everything else, tagged
	// with that ID, matched and removed when it lands. Only ever touched
	// from the main loop below, which already holds mu throughout, so no
	// separate lock.
	pending := map[string]func(proto.Envelope){}
	reqSeq := 0
	sendReq := func(typ proto.MessageType, payload any, onReply func(proto.Envelope)) {
		reqSeq++
		id := fmt.Sprintf("r%d", reqSeq)
		if onReply != nil {
			pending[id] = onReply
		}
		data, err := proto.EncodeReq(typ, windowID, id, payload)
		if err != nil {
			return
		}
		outMu.Lock()
		_, _ = out.Write(data)
		_ = out.WriteByte('\n')
		_ = out.Flush()
		outMu.Unlock()
	}
	dispatchReply := func(env proto.Envelope) {
		if env.ReqID == "" {
			return
		}
		if fn := pending[env.ReqID]; fn != nil {
			delete(pending, env.ReqID)
			fn(env)
		}
	}

	// drawLocked must be called with mu held — it reads all the state
	// above and sends the result, so callers on different goroutines (the
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
		put(1, 8, "Cred  (c): "+credStatus)
		put(1, 9, "Clip  (v): "+clipStatus)
		put(1, 10, "Pick  (f): "+pickStatus)
		audioLabel := "off"
		if audioOn {
			audioLabel = "on — streaming a 440Hz tone"
		}
		put(1, 11, "Audio (a): "+audioLabel)
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
			switch {
			case p.Key == "Escape" || p.Rune == 'q' || p.Rune == 'Q':
				mu.Unlock()
				send(proto.TypeCloseWindow, nil)
				mu.Lock()
			case p.Rune == 'c' || p.Rune == 'C':
				credStatus = "saving..."
				drawLocked()
				sendReq(proto.TypeSaveCredential,
					proto.SaveCredentialRequest{Key: "extapp-hello-demo", Value: []byte("hello from extapp-hello")},
					func(reply proto.Envelope) {
						r, _ := proto.DecodePayload[proto.CredentialSavedResponse](reply)
						if r.Err != "" {
							credStatus = "save failed: " + r.Err
							drawLocked()
							return
						}
						credStatus = "saved, loading back..."
						drawLocked()
						sendReq(proto.TypeLoadCredential, proto.LoadCredentialRequest{Key: "extapp-hello-demo"},
							func(reply2 proto.Envelope) {
								r2, _ := proto.DecodePayload[proto.CredentialLoadedResponse](reply2)
								if r2.Err != "" {
									credStatus = "load failed: " + r2.Err
								} else {
									credStatus = "round-trip OK: " + string(r2.Value)
								}
								drawLocked()
							})
					})
			case p.Rune == 'v' || p.Rune == 'V':
				clipStatus = "setting..."
				drawLocked()
				send(proto.TypeClipboardSet, proto.ClipboardSetRequest{Text: "hello from extapp-hello"})
				sendReq(proto.TypeClipboardGet, nil, func(reply proto.Envelope) {
					r, _ := proto.DecodePayload[proto.ClipboardValueResponse](reply)
					clipStatus = "now: " + r.Text
					drawLocked()
				})
			case p.Rune == 'f' || p.Rune == 'F':
				pickStatus = "picking..."
				drawLocked()
				sendReq(proto.TypePickFile, proto.PickFileRequest{StartDir: "/tmp"}, func(reply proto.Envelope) {
					r, _ := proto.DecodePayload[proto.FilePickedResponse](reply)
					if r.Ok {
						pickStatus = "picked: " + r.Path
					} else {
						pickStatus = "cancelled"
					}
					drawLocked()
				})
			case p.Rune == 'a' || p.Rune == 'A':
				if audioOn {
					close(audioStop)
					audioOn = false
					send(proto.TypeStopAudio, nil)
				} else {
					audioOn = true
					audioStop = make(chan struct{})
					send(proto.TypePlayAudio, nil)
					go streamSineTone(audioStop, send)
				}
				drawLocked()
			default:
				drawLocked()
			}
		case proto.TypeMouse:
			p, _ := proto.DecodePayload[proto.MouseEvent](env)
			if p.Action == "press" {
				clicks++
				drawLocked()
			}
		case proto.TypeCredentialSaved, proto.TypeCredentialLoaded, proto.TypeFilePicked, proto.TypeClipboardValue:
			dispatchReply(env)
		}
		mu.Unlock()
	}
	// Stdin closed (the host closes it as the first step of tearing down
	// this window) — exit cleanly rather than waiting to be killed.
}

// streamSineTone generates a continuous 440Hz tone and streams it as
// audio_chunk messages, paced to roughly real-time so the host's playback
// buffer never gets flooded — until stop is closed. send is safe to call
// from any goroutine (see main's outMu).
func streamSineTone(stop <-chan struct{}, send func(proto.MessageType, any)) {
	const sampleRate = 48000
	const channels = 2
	const chunkFrames = 1024
	phase := 0.0
	step := 2 * math.Pi * 440 / float64(sampleRate)
	t := time.NewTicker(chunkFrames * time.Second / sampleRate)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		buf := make([]int16, chunkFrames*channels)
		for i := 0; i < chunkFrames; i++ {
			v := int16(math.Sin(phase) * 8000) // modest volume, not full-scale
			phase += step
			for c := 0; c < channels; c++ {
				buf[i*channels+c] = v
			}
		}
		send(proto.TypeAudioChunk, proto.AudioChunkPayload{PCM: proto.EncodeAudioChunk(buf)})
	}
}
