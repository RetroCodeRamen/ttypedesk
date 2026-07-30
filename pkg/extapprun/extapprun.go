// Package extapprun lets any pkg/uiapp.App run as a standalone
// out-of-process TTYPE Desk app — speaking the NDJSON protocol documented
// in docs/extapp.md over its own stdin/stdout — instead of being linked
// into the main ttypedesk binary. Call Run from main() and nothing else;
// it owns stdin/stdout for the life of the process.
//
// This generalizes cmd/extapp-hello's hand-written protocol state
// machine into something any existing uiapp.App can reuse verbatim: an
// app written against the in-process App SDK needs zero changes to also
// run this way (see cmd/matrixchat, which is exactly apps/matrixchat.New()
// passed to Run — the app package itself never mentions this protocol).
package extapprun

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/proto"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
)

// tickInterval is how often the run loop checks for self-driven redraws
// (a background goroutine called Context.MarkDirty, or a due
// Context.StartTimer) with no incoming host message to react to — the
// wire protocol has no host-driven "tick" (see docs/extapp.md's "What's
// not in v1"), so the child has to poll its own dirty/timer state itself.
const tickInterval = 50 * time.Millisecond

// Run drives app as a standalone extapp binary over the real os.Stdin/
// os.Stdout. Blocks until stdin closes (the host tore the window down —
// see docs/extapp.md's lifecycle) or the app's own Init/Handle/Draw
// causes a crash (in which case a ready{err} is sent and Run returns
// that error, mirroring how an in-process app panicking in Init marks
// the window crashed instead of taking the whole desktop down).
func Run(app uiapp.App) error {
	return run(app, os.Stdin, os.Stdout)
}

func run(app uiapp.App, stdin io.Reader, stdout io.Writer) error {
	r := &runner{app: app}
	return r.run(stdin, stdout)
}

type runner struct {
	app uiapp.App

	outMu sync.Mutex
	out   *bufio.Writer

	windowID string // set once, from the initial `init` envelope

	pendingMu sync.Mutex
	pending   map[string]func(proto.Envelope)
	reqSeq    int

	dirty atomic.Bool
}

func (r *runner) run(stdin io.Reader, stdout io.Writer) error {
	r.out = bufio.NewWriter(stdout)
	r.pending = map[string]func(proto.Envelope){}

	// msgs carries every envelope the driver loop below needs to see
	// (init/resize/key/mouse/focus). Reply-type envelopes (answering a
	// save_credential/load_credential/pick_file/clipboard_get request)
	// are dispatched directly from the reader goroutine instead — the
	// driver loop can be blocked *inside* a synchronous Host call like
	// SaveCredential waiting on exactly that reply, so routing replies
	// through the same channel the driver polls in its normal select
	// would deadlock (nothing left running to dequeue it). Everything
	// else is fire-and-forget from the app's perspective and can safely
	// wait for the driver loop's own pace.
	msgs := make(chan proto.Envelope, 8)
	go r.readLoop(stdin, msgs)

	env, ok := <-msgs
	if !ok {
		return nil // stdin closed before we ever got an init — nothing to run
	}
	if env.Type != proto.TypeInit {
		return fmt.Errorf("extapprun: expected init, got %q", env.Type)
	}
	initP, err := proto.DecodePayload[proto.InitPayload](env)
	if err != nil {
		return fmt.Errorf("extapprun: decoding init: %w", err)
	}
	r.windowID = initP.WindowID
	cols, rows := initP.Cols, initP.Rows
	if cols < 1 {
		cols = 40
	}
	if rows < 1 {
		rows = 12
	}

	ctx := uiapp.NewContext(r.windowID, cols, rows, func() { r.dirty.Store(true) })
	ctx.SetHost(&wireHost{write: r.write, sendReq: r.sendReq})

	if err := r.app.Init(ctx); err != nil {
		r.write(proto.TypeReady, "", proto.ReadyPayload{Err: err.Error()})
		return err
	}
	r.write(proto.TypeReady, "", proto.ReadyPayload{})

	cv := uiapp.NewCanvas(cols, rows)
	r.drawAndSend(cv)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case env, ok := <-msgs:
			if !ok {
				_ = r.app.Close()
				return nil
			}
			r.handle(ctx, cv, env)
			r.drawAndSend(cv)
		case <-ticker.C:
			for _, ev := range ctx.DrainTimers() {
				_ = r.app.Handle(ev)
			}
			if r.dirty.CompareAndSwap(true, false) {
				r.drawAndSend(cv)
			}
		}
	}
}

// handle applies one host-driven envelope to ctx/app — everything
// msgs delivers other than the initial init, already consumed in run.
func (r *runner) handle(ctx *uiapp.Context, cv *uiapp.Canvas, env proto.Envelope) {
	switch env.Type {
	case proto.TypeResize:
		p, err := proto.DecodePayload[proto.ResizeEvent](env)
		if err != nil || p.Cols < 1 || p.Rows < 1 {
			return
		}
		ctx.SetSize(p.Cols, p.Rows)
		cv.Resize(p.Cols, p.Rows)
		_ = r.app.Handle(uiapp.Event{Kind: uiapp.EventResize, Cols: p.Cols, Rows: p.Rows})
	case proto.TypeKey:
		p, err := proto.DecodePayload[proto.KeyEvent](env)
		if err != nil {
			return
		}
		_ = r.app.Handle(uiapp.Event{
			Kind: uiapp.EventKey, Rune: p.Rune, Key: p.Key, Ctrl: p.Ctrl, Alt: p.Alt, Shift: p.Shift,
		})
	case proto.TypeMouse:
		p, err := proto.DecodePayload[proto.MouseEvent](env)
		if err != nil {
			return
		}
		_ = r.app.Handle(uiapp.Event{
			Kind: uiapp.EventMouse, X: p.X, Y: p.Y, Button: p.Button, Action: p.Action,
			Ctrl: p.Ctrl, Alt: p.Alt, Shift: p.Shift,
		})
	case proto.TypeFocus:
		p, err := proto.DecodePayload[proto.FocusEvent](env)
		if err != nil {
			return
		}
		kind := uiapp.EventBlur
		if p.Focused {
			kind = uiapp.EventFocus
		}
		_ = r.app.Handle(uiapp.Event{Kind: kind, Focused: p.Focused})
	}
}

func (r *runner) drawAndSend(cv *uiapp.Canvas) {
	_ = r.app.Draw(cv)
	cols, rows := cv.Bounds()
	r.write(proto.TypeScreenDiff, "", proto.ScreenDiffPayload{Diff: cell.FullGridDiff(cols, rows, cv.Cells())})
}

// readLoop decodes NDJSON envelopes from stdin. Reply envelopes are
// dispatched immediately (see run's comment on msgs for why); everything
// else is queued for the driver loop. Closes msgs on EOF so the driver
// loop can tell "host went away" apart from "nothing to do right now."
func (r *runner) readLoop(stdin io.Reader, msgs chan<- proto.Envelope) {
	defer close(msgs)
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		env, err := proto.Decode(sc.Bytes())
		if err != nil {
			continue // one malformed line isn't worth crashing over
		}
		switch env.Type {
		case proto.TypeCredentialSaved, proto.TypeCredentialLoaded, proto.TypeFilePicked, proto.TypeClipboardValue:
			r.dispatchReply(env)
		default:
			msgs <- env
		}
	}
}

func (r *runner) dispatchReply(env proto.Envelope) {
	if env.ReqID == "" {
		return
	}
	r.pendingMu.Lock()
	fn := r.pending[env.ReqID]
	delete(r.pending, env.ReqID)
	r.pendingMu.Unlock()
	if fn != nil {
		fn(env)
	}
}

func (r *runner) write(typ proto.MessageType, reqID string, payload any) {
	data, err := proto.EncodeReq(typ, r.windowID, reqID, payload)
	if err != nil {
		return
	}
	data = append(data, '\n')
	r.outMu.Lock()
	_, _ = r.out.Write(data)
	_ = r.out.Flush()
	r.outMu.Unlock()
}

// sendReq sends a request/response message, registering onReply to fire
// when the matching reply arrives (see dispatchReply). onReply may be
// nil for a fire-and-forget send that still wants a fresh req_id (none
// of the current message types need that, but costs nothing to support).
func (r *runner) sendReq(typ proto.MessageType, payload any, onReply func(proto.Envelope)) {
	r.pendingMu.Lock()
	r.reqSeq++
	id := fmt.Sprintf("r%d", r.reqSeq)
	if onReply != nil {
		r.pending[id] = onReply
	}
	r.pendingMu.Unlock()
	r.write(typ, id, payload)
}

// wireHost implements uiapp.Host by encoding/decoding the wire protocol
// — the out-of-process mirror of internal/server/host.go's appHost,
// which does the same translation for in-process apps by calling into
// *Server directly instead of writing NDJSON.
type wireHost struct {
	write   func(typ proto.MessageType, reqID string, payload any)
	sendReq func(typ proto.MessageType, payload any, onReply func(proto.Envelope))
}

func (h *wireHost) Notify(title, body, icon string) {
	h.write(proto.TypeNotify, "", proto.NotifyPayload{Title: title, Body: body, Icon: icon})
}

func (h *wireHost) Launch(action string) error {
	h.write(proto.TypeLaunch, "", proto.LaunchPayload{Action: action})
	return nil
}

func (h *wireHost) OpenPath(path string) error {
	h.write(proto.TypeOpenPath, "", proto.OpenPathPayload{Path: path})
	return nil
}

func (h *wireHost) SetTitle(title string) {
	h.write(proto.TypeTitleChanged, "", proto.TitleChanged{Title: title})
}

func (h *wireHost) RequestClose() {
	h.write(proto.TypeCloseWindow, "", nil)
}

// WindowID is unused by wireHost itself (the window ID is baked into
// every envelope already) — kept only to satisfy uiapp.Host. Native apps
// that need their own window ID get it from Context.WindowID instead.
func (h *wireHost) WindowID() string { return "" }

func (h *wireHost) SaveCredential(key string, value []byte) error {
	done := make(chan error, 1)
	h.sendReq(proto.TypeSaveCredential, proto.SaveCredentialRequest{Key: key, Value: value}, func(env proto.Envelope) {
		p, err := proto.DecodePayload[proto.CredentialSavedResponse](env)
		if err != nil {
			done <- err
			return
		}
		if p.Err != "" {
			done <- errors.New(p.Err)
			return
		}
		done <- nil
	})
	return <-done
}

func (h *wireHost) LoadCredential(key string) ([]byte, error) {
	type result struct {
		value []byte
		err   error
	}
	done := make(chan result, 1)
	h.sendReq(proto.TypeLoadCredential, proto.LoadCredentialRequest{Key: key}, func(env proto.Envelope) {
		p, err := proto.DecodePayload[proto.CredentialLoadedResponse](env)
		if err != nil {
			done <- result{err: err}
			return
		}
		if p.Err != "" {
			done <- result{err: errors.New(p.Err)}
			return
		}
		done <- result{value: p.Value}
	})
	res := <-done
	return res.value, res.err
}

func (h *wireHost) PickFile(startDir string, extensions []string, onResult func(path string, ok bool)) {
	h.sendReq(proto.TypePickFile, proto.PickFileRequest{StartDir: startDir, Extensions: extensions}, func(env proto.Envelope) {
		p, err := proto.DecodePayload[proto.FilePickedResponse](env)
		if err != nil {
			if onResult != nil {
				onResult("", false)
			}
			return
		}
		if onResult != nil {
			onResult(p.Path, p.Ok)
		}
	})
}

func (h *wireHost) ClipboardGet() string {
	done := make(chan string, 1)
	h.sendReq(proto.TypeClipboardGet, nil, func(env proto.Envelope) {
		p, _ := proto.DecodePayload[proto.ClipboardValueResponse](env)
		done <- p.Text
	})
	return <-done
}

func (h *wireHost) ClipboardSet(text string) {
	h.write(proto.TypeClipboardSet, "", proto.ClipboardSetRequest{Text: text})
}

// PlayAudio starts a play_audio/audio_chunk/stop_audio stream. Pause/
// Resume have no wire-protocol equivalent (docs/extapp.md only defines
// play/chunk/stop) — implemented purely locally by stopping the feeder
// goroutine from draining pcm, which is exactly how internal/audio.Play's
// own Pause/Resume work in-process (a paused player stops pulling from
// its source, and that backpressure is the whole mechanism — see that
// package's doc comment). No real app uses this yet (confirmed:
// apps/matrixchat never calls PlayAudio); this exists for parity with
// the in-process Host, not a specific consumer.
func (h *wireHost) PlayAudio(pcm <-chan []int16) (uiapp.AudioPlayback, error) {
	h.write(proto.TypePlayAudio, "", nil)
	p := &wireAudioPlayback{resume: make(chan struct{}, 1), stopCh: make(chan struct{})}
	p.playing.Store(true)
	go p.feed(pcm, h.write)
	return p, nil
}

type wireAudioPlayback struct {
	playing atomic.Bool
	paused  atomic.Bool
	resume  chan struct{}
	stopCh  chan struct{}
	once    sync.Once
}

func (p *wireAudioPlayback) feed(pcm <-chan []int16, write func(proto.MessageType, string, any)) {
	for {
		if p.paused.Load() {
			select {
			case <-p.resume:
			case <-p.stopCh:
				return
			}
			continue
		}
		select {
		case samples, ok := <-pcm:
			if !ok {
				p.playing.Store(false)
				return
			}
			write(proto.TypeAudioChunk, "", proto.AudioChunkPayload{PCM: proto.EncodeAudioChunk(samples)})
		case <-p.stopCh:
			return
		}
	}
}

func (p *wireAudioPlayback) Pause() {
	p.paused.Store(true)
	p.playing.Store(false)
}

func (p *wireAudioPlayback) Resume() {
	p.paused.Store(false)
	p.playing.Store(true)
	select {
	case p.resume <- struct{}{}:
	default:
	}
}

func (p *wireAudioPlayback) Playing() bool { return p.playing.Load() }

func (p *wireAudioPlayback) Stop() {
	p.once.Do(func() {
		p.playing.Store(false)
		close(p.stopCh)
	})
}
