package amp

import (
	"sync"

	"github.com/ttypedesk/ttypedesk/internal/ffdecode"
)

// visBars is how many amplitude bars the visualizer shows — not a real
// spectrum (no FFT), just windowed peak amplitude split across this many
// buckets of the decoded stream, refreshed every chunk.
const visBars = 16

// ffmpegAvailable is checked once — ffmpeg is a soft runtime dependency
// (only needed if you actually use Amp), same posture as Xvfb for the
// GUI-TUI Bridge.
func ffmpegAvailable() bool { return ffdecode.Available() }

// trackEvent is how the decode goroutine reports back — never by
// touching App fields directly (Handle/Draw run under AppSurface's own
// lock, but this goroutine doesn't). Drained only from Draw.
type trackEvent = ffdecode.AudioEvent

// decoder pairs internal/ffdecode's shared ffmpeg-audio-decode plumbing
// with Amp's own visualizer, computed per-chunk via ffdecode's onChunk
// hook — kept out of the shared package since no other consumer
// (apps/vid) needs it.
type decoder struct {
	stream *ffdecode.AudioStream
	pcm    chan []int16 // alias of stream.PCM, for amp.go's existing field access

	visMu sync.Mutex
	vis   [visBars]float64
}

// startDecode launches ffmpeg on path and begins feeding decoded PCM;
// events receives exactly one trackEvent when the track ends or decode
// fails.
func startDecode(path string, events chan<- trackEvent) (*decoder, error) {
	d := &decoder{}
	stream, err := ffdecode.DecodeAudio(path, d.updateVis, events)
	if err != nil {
		return nil, err
	}
	d.stream = stream
	d.pcm = stream.PCM
	return d, nil
}

// updateVis computes visBars peak-amplitude buckets from samples and
// stores them — called from ffdecode's feeder goroutine (via onChunk),
// safe to call concurrently with Draw reading via Vis().
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

// stop kills the ffmpeg subprocess. Safe to call once decode has already
// finished on its own, and safe to call twice.
func (d *decoder) stop() {
	d.stream.Stop()
}
