// Package vid is a terminal-native video player: live frames decoded to
// half-block cells, not a GUI nest (that's the GUI-TUI Bridge's job).
// Decoding is always two ffmpeg subprocesses (video frames + the file's
// own audio track, decoded independently — see the package doc comment
// on internal/ffdecode for why one process per stream rather than a
// single multi-pipe invocation), never a linked decoder library.
package vid

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
	"github.com/RetroCodeRamen/ttypedesk/internal/ffdecode"
	"github.com/RetroCodeRamen/ttypedesk/internal/gfx"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
)

var videoExtensions = []string{"mp4", "mkv", "webm", "avi", "mov", "m4v", "flv", "ogv"}

// cellW/H is the pixel resolution ffmpeg decodes to, per cell — matches
// the GUI-TUI Bridge's own xvfbCellW/H reasoning: no need to match real
// font metrics, EncodeHalfBlockFit resamples to fit whatever cols/rows
// the window actually has, so this only sets how much source detail is
// available to resample from per frame.
const (
	cellW = 8
	cellH = 16
)

// fpsLocal/fpsSSH: decode frame rate. Adaptive in the sense the Bridge's
// perf doc means it — a fixed, deliberately lower budget over SSH, not a
// live-measured one; a real measured-adaptive version is future work, not
// this first pass.
const (
	fpsLocal = 15
	fpsSSH   = 8
)

// seekStep is how far Left/Right jump on each press.
const seekStep = 5 * time.Second

// App is the Vid player.
type App struct {
	ctx        *uiapp.Context
	cols, rows int

	path  string
	fps   int
	clock *uiapp.MediaClock

	video       *ffdecode.VideoStream
	videoEvents chan ffdecode.VideoEvent
	cells       []cell.Cell

	audio       *ffdecode.AudioStream
	audioEvents chan ffdecode.AudioEvent
	playback    uiapp.AudioPlayback

	status string
}

func New() *App {
	return &App{clock: uiapp.NewMediaClock()}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	a.cols, a.rows = ctx.Cols, ctx.Rows
	if !ffdecode.Available() {
		a.status = "ffmpeg not found on PATH — install it to play video (e.g. apt install ffmpeg)"
	}
	// One fixed UI tick, independent of the decode fps: Draw just grabs
	// whatever the newest available frame is each tick, so this only
	// needs to be fast enough that decode's own -r rate is never the
	// bottleneck, not matched to it exactly.
	ctx.StartTimer(33 * time.Millisecond)
	return nil
}

func (a *App) Close() error {
	a.teardown()
	return nil
}

func (a *App) teardown() {
	if a.video != nil {
		a.video.Stop()
		a.video = nil
	}
	if a.audio != nil {
		a.audio.Stop()
		a.audio = nil
	}
	if a.playback != nil {
		a.playback.Stop()
		a.playback = nil
	}
}

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventResize:
		// Deliberately doesn't restart decode at the new resolution —
		// same reasoning as the Bridge's Resize: relaunching ffmpeg on
		// every drag would thrash the pipeline for no real benefit, since
		// EncodeHalfBlockFit already resamples to whatever cols/rows Draw
		// reports. Picked up fresh on the next Open.
		a.cols, a.rows = e.Cols, e.Rows
	case uiapp.EventKey:
		return a.key(e)
	}
	return nil
}

func (a *App) key(e uiapp.Event) error {
	switch e.Key {
	case "Left":
		a.seek(-seekStep)
	case "Right":
		a.seek(seekStep)
	}
	switch e.Rune {
	case 'o', 'O':
		a.openFile()
	case ' ':
		a.togglePause()
	case 's', 'S':
		a.stop()
	}
	return nil
}

func (a *App) openFile() {
	if a.ctx == nil {
		return
	}
	a.ctx.PickFile("", videoExtensions, func(path string, ok bool) {
		if !ok {
			return
		}
		a.playAt(path, 0)
		a.ctx.MarkDirty()
	})
}

// playAt starts (or restarts) decode of path from startAt into the file.
// Used both for opening a fresh file (startAt=0) and for seeking, which
// has no true random-access alternative against a live ffmpeg pipe — it
// just tears down and restarts decode with an input-side -ss.
func (a *App) playAt(path string, startAt time.Duration) {
	wasPlaying := a.clock.Playing() || a.video == nil // default to playing for a freshly opened file
	a.teardown()
	if !ffdecode.Available() {
		a.status = "ffmpeg not found on PATH — install it to play video (e.g. apt install ffmpeg)"
		return
	}
	if startAt < 0 {
		startAt = 0
	}

	fps := fpsLocal
	if config.OverSSH() {
		fps = fpsSSH
	}
	a.fps = fps

	w, h := a.cols*cellW, a.rows*cellH
	a.videoEvents = make(chan ffdecode.VideoEvent, 2)
	video, err := ffdecode.DecodeVideoAt(path, startAt, w, h, fps, a.videoEvents)
	if err != nil {
		a.status = err.Error()
		return
	}
	a.video = video
	a.path = path
	a.clock.SetPosition(startAt)

	a.audioEvents = make(chan ffdecode.AudioEvent, 2)
	if audio, aerr := ffdecode.DecodeAudioAt(path, startAt, nil, a.audioEvents); aerr == nil {
		if pb, perr := a.ctx.PlayAudio(audio.PCM); perr == nil {
			a.audio = audio
			a.playback = pb
		} else {
			audio.Stop()
		}
	}
	// A file with no audio track (or one ffmpeg can't extract for
	// whatever reason) isn't fatal — video-only playback still works,
	// same "degrade the specific feature, not the whole thing" posture
	// as the Bridge's AT-SPI overlay.

	if wasPlaying {
		a.clock.Play()
	} else {
		a.clock.Pause()
		if a.playback != nil {
			a.playback.Pause()
		}
	}
	a.status = "Playing " + filepath.Base(path)
}

func (a *App) togglePause() {
	if a.video == nil {
		if a.path != "" {
			a.playAt(a.path, a.clock.Position())
		}
		return
	}
	if a.clock.Playing() {
		if a.playback != nil {
			a.playback.Pause()
		}
		a.clock.Pause()
		a.status = "Paused"
	} else {
		if a.playback != nil {
			a.playback.Resume()
		}
		a.clock.Play()
		a.status = "Playing " + filepath.Base(a.path)
	}
}

func (a *App) seek(delta time.Duration) {
	if a.path == "" {
		return
	}
	a.playAt(a.path, a.clock.Position()+delta)
}

func (a *App) stop() {
	a.teardown()
	a.path = ""
	a.cells = nil
	a.clock.Pause()
	a.clock.SetPosition(0)
	a.status = "Stopped"
}

// drainEvents applies any pending video/audio decode events — called
// only from Draw, i.e. only ever on the single thread that owns every
// other App field (same contract as apps/calendar/sync.go and
// apps/amp's drainEvents).
func (a *App) drainEvents() {
	if a.videoEvents != nil {
		select {
		case ev := <-a.videoEvents:
			if ev.Err != nil {
				a.status = ev.Err.Error()
				a.stop()
				return
			}
			if ev.Ended {
				a.stop()
				return
			}
		default:
		}
	}
	if a.audioEvents != nil {
		select {
		case <-a.audioEvents:
			// The audio track ending or failing isn't fatal to video —
			// just drop audio and keep playing picture-only. Tracks of
			// slightly different length between video/audio are common
			// enough (container padding, etc.) that treating this as an
			// error would be more annoying than useful.
			if a.playback != nil {
				a.playback.Stop()
				a.playback = nil
			}
			a.audio = nil
		default:
		}
	}
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	a.drainEvents()
	cols, rows := cv.Bounds()
	bg := cell.RGB(0, 0, 0)
	cv.FillRect(0, 0, cols, rows, bg)

	if a.video != nil && a.clock.Playing() {
		select {
		case f, ok := <-a.video.Frames:
			if ok {
				a.cells = gfx.EncodeHalfBlockFit(f.Image(), cols, rows, "contain", 0, 0)
			}
		default:
		}
	}
	if a.cells != nil {
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				i := y*cols + x
				if i < len(a.cells) {
					c := a.cells[i]
					cv.DrawText(x, y, string(c.Rune), c.FG, c.BG, 0)
				}
			}
		}
	} else {
		cv.DrawText(1, 1, "(no video open — press O to open a file)", cell.RGB(0xC0, 0xC0, 0xC0), bg, 0)
	}

	pos := a.clock.Position()
	transport := fmt.Sprintf("%02d:%02d", int(pos.Minutes()), int(pos.Seconds())%60)
	help := " Vid — O=open  Space=play/pause  ←→=seek 5s  S=stop   " + transport
	if a.status != "" {
		help = " " + a.status + "   " + transport
	}
	if rows > 0 {
		cv.DrawText(0, rows-1, help, cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0x00, 0x00, 0x00), 0)
	}
	return nil
}
