// Package audio wraps github.com/ebitengine/oto/v3 for local PCM playback
// — the "audio decode hooks" slice of the roadmap's Host/App API line,
// consumed via uiapp.Host.PlayAudio. Decoding (ffmpeg subprocess, for Amp
// and Vid) stays entirely outside this package; it only plays already-
// decoded interleaved int16 samples.
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	oto "github.com/ebitengine/oto/v3"
)

// SampleRate/Channels are the fixed output format every caller must decode
// to. oto's Context — and the real audio device behind it — is created
// once per process at a single sample rate/channel count; there is no
// per-call resampling (oto's own docs: "Creating multiple contexts is NOT
// supported"). ffmpeg-based decoders should always target these via
// -ar/-ac flags rather than passing a rate through here.
const (
	SampleRate = 48000
	Channels   = 2
)

var (
	once   sync.Once
	ctx    *oto.Context
	ctxErr error
)

func sharedContext() (*oto.Context, error) {
	once.Do(func() {
		c, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:   SampleRate,
			ChannelCount: Channels,
			Format:       oto.FormatSignedInt16LE,
		})
		if err != nil {
			ctxErr = fmt.Errorf("audio: %w", err)
			return
		}
		<-ready
		ctx = c
	})
	return ctx, ctxErr
}

// Playback controls audio started via Play. Pause/Resume map straight onto
// oto's own Player.Pause/Play — pausing doesn't restart or re-seek
// anything, and it doubles as backpressure on the whole pipeline for free:
// once the player stops pulling from the reader, the feeding goroutine's
// channel send blocks, which blocks whatever's decoding upstream (e.g. an
// ffmpeg subprocess whose stdout pipe just fills up) — no separate pause
// signal needs to reach the decoder.
type Playback struct {
	player *oto.Player
	stopFn func()
}

func (p *Playback) Pause()        { p.player.Pause() }
func (p *Playback) Resume()       { p.player.Play() }
func (p *Playback) Playing() bool { return p.player.IsPlaying() }
func (p *Playback) Stop()         { p.stopFn() }

// Play streams pcm (interleaved int16 samples at SampleRate/Channels) to
// the shared audio output until pcm is closed or Stop is called. Stop is
// safe to call more than once (and safe to never call, if pcm closes on
// its own — e.g. end of track).
func Play(pcm <-chan []int16) (*Playback, error) {
	c, err := sharedContext()
	if err != nil {
		return nil, err
	}
	r := &chanReader{pcm: pcm, stop: make(chan struct{})}
	player := c.NewPlayer(r)
	player.Play()
	var stopOnce sync.Once
	return &Playback{
		player: player,
		stopFn: func() {
			stopOnce.Do(func() {
				close(r.stop)
				_ = player.Close()
			})
		},
	}, nil
}

// chanReader adapts a channel of int16 PCM samples into the io.Reader
// oto's Player pulls from (raw little-endian bytes, matching
// FormatSignedInt16LE) — decoupled from any real oto.Context so it can be
// tested without real audio hardware.
//
// mu guards buf: confirmed via a real CI failure (a DATA RACE, then a
// slice-bounds panic) that oto/v3's Player.Play, called again to resume
// after a Pause, doesn't reliably wait for the previous internal read
// goroutine to fully exit before starting a new one — briefly, two
// goroutines can call Read concurrently on the same chanReader. The
// channel receive from r.pcm is already safe for that on its own (Go
// channels guarantee it), but the buf/copy/reslice sequence below isn't:
// two goroutines racing copy(p, r.buf) and r.buf = r.buf[n:] against each
// other is exactly what produced the out-of-range slice panic.
type chanReader struct {
	pcm  <-chan []int16
	stop chan struct{}
	mu   sync.Mutex
	buf  []byte
}

func (r *chanReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.buf) == 0 {
		select {
		case samples, ok := <-r.pcm:
			if !ok {
				return 0, io.EOF
			}
			r.buf = make([]byte, len(samples)*2)
			for i, s := range samples {
				binary.LittleEndian.PutUint16(r.buf[i*2:], uint16(s))
			}
		case <-r.stop:
			return 0, io.EOF
		}
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}
