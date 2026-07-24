//go:build linux

package client

// deadlineTty is a tcell.Tty backed by /dev/tty, identical to tcell's own
// default implementation except that every write is bounded by a deadline.
//
// tcell's internal resize handler (tScreen.mainLoop, triggered directly by
// SIGWINCH) does a raw, unbounded write to the tty while holding tcell's
// screen lock. Over a stalled or slow SSH link that write can block forever
// once the pty's output buffer fills, which wedges the entire desktop with
// no panic, no log line, and no way to recover short of killing the session.
// Bounding the write means a stalled connection produces at worst one
// incomplete frame instead of a permanent freeze.
import (
	"errors"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// writeDeadline bounds a single write to the terminal. It's generous enough
// to not trip on ordinary latency, but short enough that a stalled
// connection recovers instead of hanging indefinitely.
const writeDeadline = 2 * time.Second

type deadlineTty struct {
	fd    int
	f     *os.File
	saved *term.State
	sig   chan os.Signal
	cb    func()
	stopQ chan struct{}
	dev   string
	wg    sync.WaitGroup
	l     sync.Mutex

	// timedOut is set when a write is torn by the deadline: the terminal
	// received a partial escape sequence, and tcell's internal idea of what's
	// on screen no longer matches reality. It self-corrects invisibly, since
	// nothing else notices a short write (bytes.Buffer.WriteTo drops the
	// unsent tail and tcell's next draw diffs against its own stale model).
	// The client polls ConsumeTimedOut to force a full repaint once writes
	// are flowing again, instead of leaving garbled bytes on screen forever.
	timedOut atomic.Bool
}

// newDeadlineTty opens /dev/tty, mirroring tcell.NewDevTty.
func newDeadlineTty() (*deadlineTty, error) {
	dev := "/dev/tty"
	f, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		_ = f.Close()
		return nil, errors.New("device is not a terminal")
	}
	saved, err := term.GetState(fd)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &deadlineTty{dev: dev, fd: fd, f: f, saved: saved, sig: make(chan os.Signal, 1)}, nil
}

func (t *deadlineTty) Read(b []byte) (int, error) {
	return t.f.Read(b)
}

func (t *deadlineTty) Write(b []byte) (int, error) {
	_ = t.f.SetWriteDeadline(time.Now().Add(writeDeadline))
	n, err := t.f.Write(b)
	_ = t.f.SetWriteDeadline(time.Time{})
	if n < len(b) {
		t.timedOut.Store(true)
	}
	return n, err
}

// ConsumeTimedOut reports whether a write has been torn by the deadline
// since the last call, clearing the flag.
func (t *deadlineTty) ConsumeTimedOut() bool {
	return t.timedOut.Swap(false)
}

func (t *deadlineTty) Close() error {
	return t.f.Close()
}

func (t *deadlineTty) Start() error {
	t.l.Lock()
	defer t.l.Unlock()

	var err error
	if t.f, err = os.OpenFile(t.dev, os.O_RDWR, 0); err != nil {
		return err
	}
	t.fd = int(t.f.Fd())
	if !term.IsTerminal(t.fd) {
		return errors.New("device is not a terminal")
	}

	_ = t.f.SetReadDeadline(time.Time{})
	saved, err := term.MakeRaw(t.fd) // also sets vMin and vTime
	if err != nil {
		return err
	}
	t.saved = saved

	t.stopQ = make(chan struct{})
	t.wg.Add(1)
	go func(stopQ chan struct{}) {
		defer t.wg.Done()
		for {
			select {
			case <-t.sig:
				t.l.Lock()
				cb := t.cb
				t.l.Unlock()
				if cb != nil {
					cb()
				}
			case <-stopQ:
				return
			}
		}
	}(t.stopQ)

	signal.Notify(t.sig, syscall.SIGWINCH)
	return nil
}

func (t *deadlineTty) Drain() error {
	_ = t.f.SetReadDeadline(time.Now())
	_ = unix.SetNonblock(t.fd, true)
	tio, err := unix.IoctlGetTermios(t.fd, unix.TCGETS)
	if err != nil {
		return err
	}
	tio.Cc[unix.VMIN] = 0
	tio.Cc[unix.VTIME] = 0
	return unix.IoctlSetTermios(t.fd, unix.TCSETSW, tio)
}

func (t *deadlineTty) Stop() error {
	t.l.Lock()
	if err := term.Restore(t.fd, t.saved); err != nil {
		t.l.Unlock()
		return err
	}
	_ = t.f.SetReadDeadline(time.Now())

	signal.Stop(t.sig)
	close(t.stopQ)
	t.l.Unlock()

	t.wg.Wait()
	_ = t.f.Close()
	return nil
}

func (t *deadlineTty) WindowSize() (tcell.WindowSize, error) {
	size := tcell.WindowSize{}
	ws, err := unix.IoctlGetWinsize(t.fd, unix.TIOCGWINSZ)
	if err != nil {
		return size, err
	}
	w := int(ws.Col)
	h := int(ws.Row)
	if w == 0 {
		w, _ = strconv.Atoi(os.Getenv("COLUMNS"))
	}
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h, _ = strconv.Atoi(os.Getenv("LINES"))
	}
	if h == 0 {
		h = 25
	}
	size.Width = w
	size.Height = h
	size.PixelWidth = int(ws.Xpixel)
	size.PixelHeight = int(ws.Ypixel)
	return size, nil
}

func (t *deadlineTty) NotifyResize(cb func()) {
	t.l.Lock()
	t.cb = cb
	t.l.Unlock()
}
