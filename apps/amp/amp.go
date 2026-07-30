// Package amp is a Winamp-flavored music player: open local files,
// playlist, play/pause/stop/next/prev, a small peak-amplitude visualizer.
// Decoding is always via an ffmpeg subprocess (a soft runtime dependency —
// nothing else in the desktop needs it) into raw PCM, never a linked
// decoder library, keeping the desktop itself free of format-specific
// code. See internal/audio for playback and pkg/uiapp.MediaClock for
// transport timing, both built for this in Phase 2 of the same effort.
package amp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
)

var audioExtensions = []string{"mp3", "flac", "wav", "ogg", "m4a", "aac", "opus", "wma"}

// App is the Amp player.
type App struct {
	ctx *uiapp.Context

	playlist []string
	current  int // index into playlist of the loaded/playing track, -1 if none
	sel      int // playlist cursor (navigation, independent of current)

	clock    *uiapp.MediaClock
	playback uiapp.AudioPlayback
	dec      *decoder
	events   chan trackEvent

	status string
}

func New() *App {
	return &App{current: -1, clock: uiapp.NewMediaClock()}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	if !ffmpegAvailable() {
		a.status = "ffmpeg not found on PATH — install it to play audio (e.g. apt install ffmpeg)"
	}
	return nil
}

func (a *App) Close() error {
	a.teardown()
	return nil
}

func (a *App) teardown() {
	if a.dec != nil {
		a.dec.stop()
		a.dec = nil
	}
	if a.playback != nil {
		a.playback.Stop()
		a.playback = nil
	}
}

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventKey:
		return a.key(e)
	case uiapp.EventMouse:
		return a.mouse(e)
	}
	return nil
}

func (a *App) mouse(e uiapp.Event) error {
	if e.Action != "press" {
		return nil
	}
	if e.Y == 0 {
		return nil
	}
	idx := e.Y - 1
	if idx < 0 || idx >= len(a.playlist) {
		return nil
	}
	a.playAt(idx)
	return nil
}

func (a *App) key(e uiapp.Event) error {
	switch e.Key {
	case "Up":
		if a.sel > 0 {
			a.sel--
		}
	case "Down":
		if a.sel < len(a.playlist)-1 {
			a.sel++
		}
	case "Enter":
		if len(a.playlist) > 0 {
			a.playAt(a.sel)
		}
	}
	switch e.Rune {
	case 'o', 'O':
		a.openFiles()
	case ' ':
		a.togglePause()
	case 'n', 'N':
		a.next()
	case 'p', 'P':
		a.prev()
	case 's', 'S':
		a.stop()
	case 'd', 'D':
		a.removeSelected()
	}
	return nil
}

func (a *App) openFiles() {
	if a.ctx == nil {
		return
	}
	a.ctx.PickFile("", audioExtensions, func(path string, ok bool) {
		if !ok {
			return
		}
		a.playlist = append(a.playlist, path)
		if a.current == -1 {
			a.sel = len(a.playlist) - 1
		}
		a.status = "Added " + filepath.Base(path)
		a.ctx.MarkDirty()
	})
}

func (a *App) removeSelected() {
	if a.sel < 0 || a.sel >= len(a.playlist) {
		return
	}
	if a.sel == a.current {
		a.stop()
	}
	a.playlist = append(a.playlist[:a.sel], a.playlist[a.sel+1:]...)
	if a.current > a.sel {
		a.current--
	}
	if a.sel >= len(a.playlist) {
		a.sel = len(a.playlist) - 1
	}
}

func (a *App) playAt(idx int) {
	if idx < 0 || idx >= len(a.playlist) {
		return
	}
	a.teardown()
	path := a.playlist[idx]
	a.events = make(chan trackEvent, 2)
	dec, err := startDecode(path, a.events)
	if err != nil {
		a.status = err.Error()
		a.current = -1
		return
	}
	pb, err := a.ctx.PlayAudio(dec.pcm)
	if err != nil {
		a.status = "amp: " + err.Error()
		dec.stop()
		a.current = -1 // teardown() above already tore down whatever was playing before
		return
	}
	a.dec = dec
	a.playback = pb
	a.current = idx
	a.sel = idx
	a.clock.SetPosition(0)
	a.clock.Play()
	a.status = "Playing " + filepath.Base(path)
}

func (a *App) togglePause() {
	if a.playback == nil {
		if len(a.playlist) > 0 {
			a.playAt(a.sel)
		}
		return
	}
	if a.clock.Playing() {
		a.playback.Pause()
		a.clock.Pause()
		a.status = "Paused"
	} else {
		a.playback.Resume()
		a.clock.Play()
		a.status = "Playing " + filepath.Base(a.playlist[a.current])
	}
}

func (a *App) stop() {
	a.teardown()
	a.current = -1
	a.clock.Pause()
	a.clock.SetPosition(0)
	a.status = "Stopped"
}

func (a *App) next() {
	if a.current == -1 || a.current+1 >= len(a.playlist) {
		a.stop()
		return
	}
	a.playAt(a.current + 1)
}

func (a *App) prev() {
	if a.current <= 0 {
		if a.current == 0 {
			a.playAt(0)
		}
		return
	}
	a.playAt(a.current - 1)
}

// drainEvents applies any trackEvent posted by the current decoder's feed
// goroutine — called only from Draw, i.e. only ever on the single thread
// that owns every other App field.
func (a *App) drainEvents() {
	if a.events == nil {
		return
	}
	for {
		select {
		case ev := <-a.events:
			if ev.Err != nil {
				a.status = ev.Err.Error()
				a.stop()
				return
			}
			if ev.Ended {
				a.next()
				return
			}
		default:
			return
		}
	}
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	a.drainEvents()
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	green := cell.RGB(0x00, 0xAA, 0x00)
	cv.FillRect(0, 0, cols, rows, bg)
	cv.DrawText(0, 0, " Amp — O=open  Space=play/pause  N/P=next/prev  S=stop  D=remove ", cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)

	visRows := 3
	playlistRows := rows - 1 - visRows - 1
	if playlistRows < 1 {
		playlistRows = rows - 2
		visRows = 0
	}
	for i := 0; i < playlistRows && i < len(a.playlist); i++ {
		y := 1 + i
		f, b := fg, bg
		if i == a.sel {
			f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
		}
		label := filepath.Base(a.playlist[i])
		marker := "  "
		if i == a.current {
			marker = "▶ "
		}
		cv.FillRect(0, y, cols, 1, b)
		cv.DrawText(1, y, marker+label, f, b, 0)
	}
	if len(a.playlist) == 0 {
		cv.DrawText(1, 1, "(empty — press O to open a file)", fg, bg, 0)
	}

	if visRows > 0 {
		visY := rows - 1 - visRows
		if a.dec != nil {
			bars := a.dec.Vis()
			barW := cols / visBars
			if barW < 1 {
				barW = 1
			}
			for i, amp := range bars {
				h := int(amp * float64(visRows))
				x := i * barW
				for row := 0; row < visRows; row++ {
					c := bg
					if visRows-row <= h {
						c = green
					}
					cv.FillRect(x, visY+row, barW-1, 1, c)
				}
			}
		}
	}

	pos := a.clock.Position()
	transport := fmt.Sprintf("%02d:%02d", int(pos.Minutes()), int(pos.Seconds())%60)
	if a.current >= 0 && a.current < len(a.playlist) {
		transport += "  " + filepath.Base(a.playlist[a.current])
	}
	help := transport
	if a.status != "" {
		help = a.status + "   " + transport
	}
	if rows > 0 {
		cv.DrawText(0, rows-1, " "+strings.TrimSpace(help), cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0x00, 0x00, 0x00), 0)
	}
	return nil
}
