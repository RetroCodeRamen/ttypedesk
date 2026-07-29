package amp

import (
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// visBars is how many amplitude bars the visualizer shows — not a real
// spectrum (no FFT), just windowed peak amplitude split across this many
// buckets of the decoded stream, refreshed every chunk.
const visBars = 16

// ffmpegAvailable is checked once — ffmpeg is a soft runtime dependency
// (only needed if you actually use Amp), same posture as Xvfb for the
// GUI-TUI Bridge.
func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// trackEvent is how the decode goroutine reports back — never by
// touching App fields directly (Handle/Draw run under AppSurface's own
// lock, but this goroutine doesn't). Drained only from Draw.
type trackEvent struct {
	ended bool
	err   error
}

// decoder owns one ffmpeg subprocess decoding a single track to raw PCM,
// plus the visualizer amplitude data it computes along the way. vis is
// the only field touched from both the feeder goroutine and Draw — guarded
// by its own mutex since it updates far more often than a trackEvent is
// worth round-tripping through a channel for.
type decoder struct {
	cmd      *exec.Cmd
	pcm      chan []int16
	stopCh   chan struct{}
	stopOnce sync.Once

	visMu sync.Mutex
	vis   [visBars]float64
}

// startDecode launches ffmpeg on path and begins feeding decoded PCM into
// the returned decoder's pcm channel. events receives exactly one
// trackEvent when the track ends (ended=true) or decode fails (err set).
func startDecode(path string, events chan<- trackEvent) (*decoder, error) {
	if !ffmpegAvailable() {
		return nil, fmt.Errorf("ffmpeg not found on PATH — install it to play audio (e.g. apt install ffmpeg)")
	}
	cmd := exec.Command("ffmpeg",
		"-i", path,
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
		"-loglevel", "error",
		"-nostdin",
		"pipe:1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("amp: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("amp: start ffmpeg: %w", err)
	}

	d := &decoder{cmd: cmd, pcm: make(chan []int16, 8), stopCh: make(chan struct{})}
	go d.feed(stdout, events)
	return d, nil
}

// feed reads raw s16le stereo PCM off r in fixed-size chunks, converts to
// []int16, updates the visualizer amplitude bins, and sends each chunk
// into d.pcm — blocking there is the whole pipeline's backpressure
// mechanism (see internal/audio.Playback's doc comment): a paused player
// stops draining d.pcm, which stops this send, which stops this Read,
// which stops ffmpeg's stdout write. The stopCh case exists for the other
// direction: if stop() runs (user hit Stop/Next) while this goroutine is
// blocked mid-send with nothing left to ever drain d.pcm again (the
// player was Closed, not just paused), it needs its own way out or it
// leaks forever.
func (d *decoder) feed(r io.Reader, events chan<- trackEvent) {
	defer close(d.pcm)
	const chunkBytes = 4096 // 1024 stereo frames
	buf := make([]byte, chunkBytes)
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			// io.ReadFull can return a short read alongside io.ErrUnexpectedEOF
			// on the final partial chunk — still real samples, still worth
			// playing/visualizing, just trim to a whole number of frames.
			usable := n - (n % 4)
			if usable > 0 {
				samples := bytesToInt16(buf[:usable])
				d.updateVis(samples)
				select {
				case d.pcm <- samples:
				case <-d.stopCh:
					return
				}
			}
		}
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				select {
				case events <- trackEvent{ended: true}:
				case <-d.stopCh:
				}
			} else {
				select {
				case events <- trackEvent{err: fmt.Errorf("amp: decode: %w", err)}:
				case <-d.stopCh:
				}
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

// updateVis computes visBars peak-amplitude buckets from samples and
// stores them — safe to call concurrently with Draw reading via Vis().
func (d *decoder) updateVis(samples []int16) {
	if len(samples) == 0 {
		return
	}
	perBar := len(samples) / visBars
	if perBar == 0 {
		perBar = 1
	}
	var bars [visBars]float64
	for i := 0; i < visBars; i++ {
		start := i * perBar
		if start >= len(samples) {
			break
		}
		end := start + perBar
		if end > len(samples) {
			end = len(samples)
		}
		var peak int32
		for _, s := range samples[start:end] {
			v := int32(s)
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
		bars[i] = float64(peak) / 32768.0
	}
	d.visMu.Lock()
	d.vis = bars
	d.visMu.Unlock()
}

// Vis returns the current visualizer amplitude bars (0..1 each).
func (d *decoder) Vis() [visBars]float64 {
	d.visMu.Lock()
	defer d.visMu.Unlock()
	return d.vis
}

// stop kills the ffmpeg subprocess and unblocks feed if it's stuck mid-
// send with nothing left to drain it. Safe to call once decode has
// already finished on its own (Wait on an already-exited process just
// errors, which is discarded — this is teardown, not a status check);
// also safe to call twice (closing stopCh twice would panic, guarded by
// sync.Once).
func (d *decoder) stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
	if d.cmd == nil || d.cmd.Process == nil {
		return
	}
	_ = d.cmd.Process.Kill()
	_ = d.cmd.Wait()
}
