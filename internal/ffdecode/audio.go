// Package ffdecode wraps ffmpeg subprocesses for raw audio/video decode —
// shared between apps/amp (audio only) and apps/vid (video, plus a
// separate ffmpeg process for the video's own audio track). Never a
// linked decoder library, on purpose: keeps the desktop binary itself
// free of format-specific code, matching the GUI-TUI Bridge's Xvfb — a
// soft runtime dependency, only needed if you actually use the feature.
package ffdecode

import (
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// SampleRate/Channels match internal/audio's fixed output format — decode
// straight to what the shared oto.Context expects, no separate resample
// step.
const (
	SampleRate = 48000
	Channels   = 2
)

// Available reports whether ffmpeg is on PATH — checked once by callers
// before decoding, for a clear error instead of a raw exec failure.
func Available() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// AudioEvent is how an AudioStream's feeder goroutine reports back —
// never by touching caller state directly (see AudioStream's doc comment
// for why); drain this from whatever single-threaded path owns your app
// state, exactly like every other background-goroutine result channel in
// this codebase (apps/calendar/sync.go, apps/settings/calendar_page.go).
type AudioEvent struct {
	Ended bool
	Err   error
}

// AudioStream decodes one file's audio track to raw PCM via an ffmpeg
// subprocess. PCM delivers interleaved int16 samples at SampleRate/
// Channels.
type AudioStream struct {
	cmd     *exec.Cmd
	PCM     chan []int16
	onChunk func([]int16)

	stopCh   chan struct{}
	stopOnce sync.Once
}

// DecodeAudio launches ffmpeg on path and begins feeding PCM into the
// returned stream's PCM channel. events receives exactly one AudioEvent
// when the track ends (Ended=true) or decode fails (Err set). onChunk,
// if non-nil, sees each chunk synchronously in the feeder goroutine,
// before it's sent to PCM — callers needing per-chunk work (Amp's
// visualizer) should keep that work cheap and do their own
// synchronization, since it runs on a different goroutine than whatever's
// reading PCM. Passed in here rather than set on the returned struct: it
// must be fixed before the feeder goroutine starts, not mutated after —
// setting it post-construction would race that goroutine's first read of
// it (confirmed by go test -race while writing this).
func DecodeAudio(path string, onChunk func([]int16), events chan<- AudioEvent) (*AudioStream, error) {
	if !Available() {
		return nil, fmt.Errorf("ffmpeg not found on PATH — install it to play audio (e.g. apt install ffmpeg)")
	}
	cmd := exec.Command("ffmpeg",
		"-i", path,
		"-vn",
		"-f", "s16le",
		"-ar", fmt.Sprint(SampleRate),
		"-ac", fmt.Sprint(Channels),
		"-loglevel", "error",
		"-nostdin",
		"pipe:1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffdecode: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffdecode: start ffmpeg: %w", err)
	}

	s := &AudioStream{cmd: cmd, PCM: make(chan []int16, 8), onChunk: onChunk, stopCh: make(chan struct{})}
	go s.feed(stdout, events)
	return s, nil
}

// feed reads raw s16le stereo PCM off r in fixed-size chunks and sends
// each chunk into s.PCM — blocking there is the whole pipeline's
// backpressure mechanism (see internal/audio.Playback's doc comment): a
// paused player stops draining s.PCM, which stops this send, which stops
// this Read, which stops ffmpeg's stdout write. stopCh exists for the
// other direction: if Stop runs while this goroutine is blocked mid-send
// with nothing left to ever drain s.PCM again, it needs its own way out.
func (s *AudioStream) feed(r io.Reader, events chan<- AudioEvent) {
	defer close(s.PCM)
	const chunkBytes = 4096 // 1024 stereo frames
	buf := make([]byte, chunkBytes)
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			// io.ReadFull can return a short read alongside
			// io.ErrUnexpectedEOF on the final partial chunk — still real
			// samples, just trim to a whole number of frames.
			usable := n - (n % 4)
			if usable > 0 {
				samples := bytesToInt16(buf[:usable])
				if s.onChunk != nil {
					s.onChunk(samples)
				}
				select {
				case s.PCM <- samples:
				case <-s.stopCh:
					return
				}
			}
		}
		if err != nil {
			ev := AudioEvent{Ended: true}
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				ev = AudioEvent{Err: fmt.Errorf("ffdecode: %w", err)}
			}
			select {
			case events <- ev:
			case <-s.stopCh:
			}
			return
		}
	}
}

func bytesToInt16(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return out
}

// Stop kills the ffmpeg subprocess and unblocks feed if it's stuck
// mid-send with nothing left to drain it. Safe to call more than once,
// and safe to call after decode has already finished on its own.
func (s *AudioStream) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
}
