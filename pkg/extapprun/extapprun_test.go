package extapprun

import (
	"bufio"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/proto"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// fakeApp is a uiapp.App test double whose Handle method dispatches on
// the incoming event's Rune to exercise every Host method the runner
// wires up — results land in mu-guarded fields tests can poll.
type fakeApp struct {
	mu sync.Mutex

	ctx       *uiapp.Context
	initErr   error
	events    []uiapp.Event
	closed    bool
	drawCount int
	drawText  string

	saveErr    error
	saveCalled bool
	loadValue  []byte
	loadErr    error
	loadCalled bool
	pickPath   string
	pickOK     bool
	pickCalled bool
	clipValue  string
	clipCalled bool
}

func (a *fakeApp) Init(ctx *uiapp.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = ctx
	return a.initErr
}

func (a *fakeApp) Handle(e uiapp.Event) error {
	a.mu.Lock()
	ctx := a.ctx
	a.events = append(a.events, e)
	a.mu.Unlock()

	switch e.Rune {
	case 's':
		err := ctx.SaveCredential("k", []byte("v"))
		a.mu.Lock()
		a.saveErr, a.saveCalled = err, true
		a.mu.Unlock()
	case 'l':
		v, err := ctx.LoadCredential("k")
		a.mu.Lock()
		a.loadValue, a.loadErr, a.loadCalled = v, err, true
		a.mu.Unlock()
	case 'p':
		ctx.PickFile("/tmp", nil, func(path string, ok bool) {
			a.mu.Lock()
			a.pickPath, a.pickOK, a.pickCalled = path, ok, true
			a.mu.Unlock()
		})
	case 'c':
		ctx.ClipboardSet("hello")
		v := ctx.ClipboardGet()
		a.mu.Lock()
		a.clipValue, a.clipCalled = v, true
		a.mu.Unlock()
	case 'n':
		ctx.Notify("t", "b", "i")
	case 'g':
		_ = ctx.Launch("terminal")
	case 'o':
		_ = ctx.OpenPath("/tmp/x")
	case 't':
		ctx.SetTitle("New Title")
	case 'x':
		ctx.RequestClose()
	}
	return nil
}

func (a *fakeApp) Draw(cv *uiapp.Canvas) error {
	a.mu.Lock()
	a.drawCount++
	text := a.drawText
	a.mu.Unlock()
	cv.DrawText(0, 0, text, cell.RGB(255, 255, 255), cell.RGB(0, 0, 0), 0)
	return nil
}

func (a *fakeApp) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	return nil
}

func (a *fakeApp) snapshot() fakeApp {
	a.mu.Lock()
	defer a.mu.Unlock()
	return fakeApp{
		events: append([]uiapp.Event(nil), a.events...), closed: a.closed, drawCount: a.drawCount,
		saveErr: a.saveErr, saveCalled: a.saveCalled,
		loadValue: a.loadValue, loadErr: a.loadErr, loadCalled: a.loadCalled,
		pickPath: a.pickPath, pickOK: a.pickOK, pickCalled: a.pickCalled,
		clipValue: a.clipValue, clipCalled: a.clipCalled,
	}
}

// harness wires a fakeApp up to run() over in-memory pipes, playing the
// role of the host on one end (writing envelopes to the app's stdin,
// reading its stdout) so these tests exercise the exact same encode/
// decode path a real ExtAppSurface would, just without a real subprocess
// (internal/surface's own tests already cover the host side against a
// real spawned binary; this covers the child side).
type harness struct {
	app     *fakeApp
	toApp   *io.PipeWriter
	fromApp *bufio.Scanner
	runErr  chan error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	app := &fakeApp{}
	appR, hostW := io.Pipe()
	hostR, appW := io.Pipe()
	h := &harness{app: app, toApp: hostW, fromApp: bufio.NewScanner(hostR), runErr: make(chan error, 1)}
	h.fromApp.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	go func() {
		h.runErr <- run(app, appR, appW)
	}()
	t.Cleanup(func() { _ = hostW.Close() })
	return h
}

func (h *harness) send(t *testing.T, typ proto.MessageType, payload any) {
	t.Helper()
	h.sendReq(t, typ, "", payload)
}

func (h *harness) sendReq(t *testing.T, typ proto.MessageType, reqID string, payload any) {
	t.Helper()
	data, err := proto.EncodeReq(typ, "w1", reqID, payload)
	if err != nil {
		t.Fatalf("EncodeReq: %v", err)
	}
	if _, err := h.toApp.Write(append(data, '\n')); err != nil {
		t.Fatalf("write to app: %v", err)
	}
}

func (h *harness) recv(t *testing.T) proto.Envelope {
	t.Helper()
	if !h.fromApp.Scan() {
		t.Fatalf("scan from app: %v", h.fromApp.Err())
	}
	env, err := proto.Decode(h.fromApp.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env
}

// recvUntil scans until it finds an envelope of typ, skipping others
// (e.g. extra screen_diffs from redraws that aren't the thing under test).
func (h *harness) recvUntil(t *testing.T, typ proto.MessageType, timeout time.Duration) proto.Envelope {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		env := h.recv(t)
		if env.Type == typ {
			return env
		}
	}
	t.Fatalf("did not see a %q envelope within %s", typ, timeout)
	return proto.Envelope{}
}

func (h *harness) init(t *testing.T, cols, rows int) {
	t.Helper()
	h.send(t, proto.TypeInit, proto.InitPayload{WindowID: "w1", Cols: cols, Rows: rows})
	if env := h.recv(t); env.Type != proto.TypeReady {
		t.Fatalf("first envelope after init = %q, want ready", env.Type)
	}
	if env := h.recv(t); env.Type != proto.TypeScreenDiff {
		t.Fatalf("second envelope after init = %q, want screen_diff", env.Type)
	}
}

func TestRunInitReadyScreenDiffHandshake(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)
}

func TestRunInitErrorSendsReadyWithErrAndReturnsIt(t *testing.T) {
	app := &fakeApp{initErr: errBoom}
	appR, hostW := io.Pipe()
	hostR, appW := io.Pipe()
	sc := bufio.NewScanner(hostR)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	runErr := make(chan error, 1)
	go func() { runErr <- run(app, appR, appW) }()

	data, _ := proto.EncodeReq(proto.TypeInit, "w1", "", proto.InitPayload{WindowID: "w1", Cols: 40, Rows: 12})
	if _, err := hostW.Write(append(data, '\n')); err != nil {
		t.Fatalf("write init: %v", err)
	}
	if !sc.Scan() {
		t.Fatalf("scan ready: %v", sc.Err())
	}
	env, err := proto.Decode(sc.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != proto.TypeReady {
		t.Fatalf("type = %q, want ready", env.Type)
	}
	p, _ := proto.DecodePayload[proto.ReadyPayload](env)
	if p.Err != errBoom.Error() {
		t.Fatalf("ready.Err = %q, want %q", p.Err, errBoom.Error())
	}
	select {
	case err := <-runErr:
		if err == nil || !strings.Contains(err.Error(), errBoom.Error()) {
			t.Fatalf("run() returned %v, want an error containing %q", err, errBoom.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not return after Init failed")
	}
	_ = hostW.Close()
}

var errBoom = fakeErr("boom")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func TestRunKeyEventReachesAppAndTriggersRedraw(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 'z'})
	h.recvUntil(t, proto.TypeScreenDiff, 2*time.Second)

	snap := h.app.snapshot()
	if len(snap.events) != 1 || snap.events[0].Kind != uiapp.EventKey || snap.events[0].Rune != 'z' {
		t.Fatalf("events = %+v, want one EventKey Rune=z", snap.events)
	}
}

func TestRunMouseEventReachesApp(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeMouse, proto.MouseEvent{X: 3, Y: 4, Button: 1, Action: "press"})
	h.recvUntil(t, proto.TypeScreenDiff, 2*time.Second)

	snap := h.app.snapshot()
	if len(snap.events) != 1 || snap.events[0].Kind != uiapp.EventMouse || snap.events[0].X != 3 || snap.events[0].Y != 4 {
		t.Fatalf("events = %+v, want one EventMouse at (3,4)", snap.events)
	}
}

func TestRunFocusAndBlurReachApp(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeFocus, proto.FocusEvent{Focused: true})
	h.recvUntil(t, proto.TypeScreenDiff, 2*time.Second)
	h.send(t, proto.TypeFocus, proto.FocusEvent{Focused: false})
	h.recvUntil(t, proto.TypeScreenDiff, 2*time.Second)

	snap := h.app.snapshot()
	if len(snap.events) != 2 || snap.events[0].Kind != uiapp.EventFocus || snap.events[1].Kind != uiapp.EventBlur {
		t.Fatalf("events = %+v, want [EventFocus, EventBlur]", snap.events)
	}
}

func TestRunResizeUpdatesCanvasAndContext(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeResize, proto.ResizeEvent{Cols: 60, Rows: 20})
	env := h.recvUntil(t, proto.TypeScreenDiff, 2*time.Second)
	p, err := proto.DecodePayload[proto.ScreenDiffPayload](env)
	if err != nil {
		t.Fatalf("decode screen_diff: %v", err)
	}
	if len(p.Diff.Cells) != 60*20 {
		t.Fatalf("cell count = %d, want %d (60x20)", len(p.Diff.Cells), 60*20)
	}

	h.app.mu.Lock()
	cols, rows := h.app.ctx.Size()
	h.app.mu.Unlock()
	if cols != 60 || rows != 20 {
		t.Fatalf("ctx.Size() = %d,%d want 60,20", cols, rows)
	}
}

func TestRunSelfDrivenRedrawViaMarkDirty(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.app.mu.Lock()
	ctx := h.app.ctx
	h.app.mu.Unlock()
	ctx.MarkDirty() // no incoming host message — must still cause a redraw within tickInterval

	h.recvUntil(t, proto.TypeScreenDiff, 2*time.Second)
}

func TestRunTimerFiresIntoHandle(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.app.mu.Lock()
	ctx := h.app.ctx
	h.app.mu.Unlock()
	ctx.StartTimer(20 * time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := h.app.snapshot()
		for _, e := range snap.events {
			if e.Kind == uiapp.EventTimer {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no EventTimer observed within 2s of StartTimer(20ms)")
}

func TestRunSaveCredentialRoundTrip(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 's'})
	env := h.recvUntil(t, proto.TypeSaveCredential, 2*time.Second)
	p, err := proto.DecodePayload[proto.SaveCredentialRequest](env)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Key != "k" || string(p.Value) != "v" {
		t.Fatalf("SaveCredentialRequest = %+v, want Key=k Value=v", p)
	}
	if env.ReqID == "" {
		t.Fatal("save_credential sent with no req_id")
	}
	h.sendReq(t, proto.TypeCredentialSaved, env.ReqID, proto.CredentialSavedResponse{})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snap := h.app.snapshot(); snap.saveCalled {
			if snap.saveErr != nil {
				t.Fatalf("SaveCredential returned %v, want nil", snap.saveErr)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("SaveCredential never returned")
}

func TestRunLoadCredentialErrorPropagates(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 'l'})
	env := h.recvUntil(t, proto.TypeLoadCredential, 2*time.Second)
	h.sendReq(t, proto.TypeCredentialLoaded, env.ReqID, proto.CredentialLoadedResponse{Err: "not found"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snap := h.app.snapshot(); snap.loadCalled {
			if snap.loadErr == nil || snap.loadErr.Error() != "not found" {
				t.Fatalf("LoadCredential err = %v, want %q", snap.loadErr, "not found")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("LoadCredential never returned")
}

func TestRunPickFileAsyncCallback(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 'p'})
	env := h.recvUntil(t, proto.TypePickFile, 2*time.Second)
	p, _ := proto.DecodePayload[proto.PickFileRequest](env)
	if p.StartDir != "/tmp" {
		t.Fatalf("PickFileRequest.StartDir = %q, want /tmp", p.StartDir)
	}

	// Simulate a real delay before the user actually picks something.
	time.Sleep(50 * time.Millisecond)
	h.sendReq(t, proto.TypeFilePicked, env.ReqID, proto.FilePickedResponse{Path: "/tmp/chosen", Ok: true})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snap := h.app.snapshot(); snap.pickCalled {
			if snap.pickPath != "/tmp/chosen" || !snap.pickOK {
				t.Fatalf("pick result = %q,%v want /tmp/chosen,true", snap.pickPath, snap.pickOK)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("PickFile callback never fired")
}

func TestRunClipboardSetThenGetRoundTrip(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 'c'})

	setEnv := h.recvUntil(t, proto.TypeClipboardSet, 2*time.Second)
	sp, _ := proto.DecodePayload[proto.ClipboardSetRequest](setEnv)
	if sp.Text != "hello" {
		t.Fatalf("clipboard_set text = %q, want hello", sp.Text)
	}

	getEnv := h.recvUntil(t, proto.TypeClipboardGet, 2*time.Second)
	h.sendReq(t, proto.TypeClipboardValue, getEnv.ReqID, proto.ClipboardValueResponse{Text: "hello"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snap := h.app.snapshot(); snap.clipCalled {
			if snap.clipValue != "hello" {
				t.Fatalf("ClipboardGet = %q, want hello", snap.clipValue)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("ClipboardGet never returned")
}

func TestRunNotifyLaunchOpenPathSetTitleRequestClose(t *testing.T) {
	h := newHarness(t)
	h.init(t, 40, 12)

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 'n'})
	env := h.recvUntil(t, proto.TypeNotify, 2*time.Second)
	np, _ := proto.DecodePayload[proto.NotifyPayload](env)
	if np.Title != "t" || np.Body != "b" || np.Icon != "i" {
		t.Fatalf("NotifyPayload = %+v", np)
	}

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 'g'})
	env = h.recvUntil(t, proto.TypeLaunch, 2*time.Second)
	lp, _ := proto.DecodePayload[proto.LaunchPayload](env)
	if lp.Action != "terminal" {
		t.Fatalf("LaunchPayload.Action = %q, want terminal", lp.Action)
	}

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 'o'})
	env = h.recvUntil(t, proto.TypeOpenPath, 2*time.Second)
	op, _ := proto.DecodePayload[proto.OpenPathPayload](env)
	if op.Path != "/tmp/x" {
		t.Fatalf("OpenPathPayload.Path = %q, want /tmp/x", op.Path)
	}

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 't'})
	env = h.recvUntil(t, proto.TypeTitleChanged, 2*time.Second)
	tp, _ := proto.DecodePayload[proto.TitleChanged](env)
	if tp.Title != "New Title" {
		t.Fatalf("TitleChanged.Title = %q, want %q", tp.Title, "New Title")
	}

	h.send(t, proto.TypeKey, proto.KeyEvent{Rune: 'x'})
	h.recvUntil(t, proto.TypeCloseWindow, 2*time.Second)
}

func TestRunStdinCloseCallsAppCloseAndReturnsNil(t *testing.T) {
	app := &fakeApp{}
	appR, hostW := io.Pipe()
	hostR, appW := io.Pipe()
	sc := bufio.NewScanner(hostR)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	runErr := make(chan error, 1)
	go func() { runErr <- run(app, appR, appW) }()

	data, _ := proto.EncodeReq(proto.TypeInit, "w1", "", proto.InitPayload{WindowID: "w1", Cols: 40, Rows: 12})
	if _, err := hostW.Write(append(data, '\n')); err != nil {
		t.Fatalf("write init: %v", err)
	}
	sc.Scan() // ready
	sc.Scan() // screen_diff

	_ = hostW.Close() // host closes the app's stdin, as it does when tearing down a window

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run() = %v, want nil on clean stdin close", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not return after stdin closed")
	}
	snap := app.snapshot()
	if !snap.closed {
		t.Fatal("app.Close() was not called")
	}
}
