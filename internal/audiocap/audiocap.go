// Package audiocap captures the host's current audio output (whatever's
// already playing — desktop sounds, Amp, Vid, a bridged app) as raw PCM,
// for internal/attach to stream to an attached remote client. It shells
// out to parec against the default sink's monitor source — never a
// microphone — mirroring internal/ffdecode's ffmpeg-subprocess pattern: no
// linked capture library, a soft runtime dependency only needed if audio
// streaming is actually turned on. parec covers both PulseAudio proper and
// PipeWire systems running pipewire-pulse, which is the default on nearly
// every modern desktop distro; pw-record has no equivalently portable
// "default sink monitor" target name, so it isn't wired up here.
package audiocap

import (
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// SampleRate/Channels match internal/audio's fixed output format, so a
// captured chunk can go straight to an attached client's Playback with no
// resampling.
const (
	SampleRate = 48000
	Channels   = 2
)

// Available reports whether parec is on PATH — checked before Capture for
// a clear error instead of a raw exec failure.
func Available() bool {
	_, err := exec.LookPath("parec")
	return err == nil
}

// Event is how a Stream's feeder goroutine reports back — never by
// touching caller state directly, exactly like internal/ffdecode.AudioEvent.
type Event struct {
	Err error
}

// defaultMonitorSource resolves parec's device argument to the current
// default sink's monitor. parec's own "@DEFAULT_SINK@.monitor" magic
// token exists for exactly this, but confirmed unreliable in at least one
// real environment (a minimal container running PulseAudio 16.1: it
// fails outright with "Stream error: Invalid argument", while the same
// monitor addressed by its literal resolved name works fine) — resolving
// it ourselves via `pactl get-default-sink` sidesteps whatever's wrong
// with parec's own resolution. Falls back to the magic token if pactl
// itself is unavailable or fails, which is no worse than what shipped
// before this existed.
func defaultMonitorSource() string {
	out, err := exec.Command("pactl", "get-default-sink").Output()
	sink := strings.TrimSpace(string(out))
	if err != nil || sink == "" {
		return "@DEFAULT_SINK@.monitor"
	}
	return sink + ".monitor"
}

// Stream captures raw PCM from the default sink's monitor via a parec
// subprocess. PCM delivers interleaved int16 samples at SampleRate/Channels.
type Stream struct {
	cmd      *exec.Cmd
	PCM      chan []int16
	stopCh   chan struct{}
	stopOnce sync.Once
}

// Capture starts parec and begins feeding PCM into the returned stream's
// PCM channel. events receives exactly one Event if capture ends
// unexpectedly (Err set); a clean Stop sends nothing.
func Capture(events chan<- Event) (*Stream, error) {
	if !Available() {
		return nil, fmt.Errorf("audiocap: parec not found on PATH — install pulseaudio-utils (or pipewire-pulse) to stream audio")
	}
	cmd := exec.Command("parec",
		"--device="+defaultMonitorSource(),
		"--rate", fmt.Sprint(SampleRate),
		"--channels", fmt.Sprint(Channels),
		"--format=s16le",
		"--raw",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("audiocap: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("audiocap: start parec: %w", err)
	}
	s := &Stream{cmd: cmd, PCM: make(chan []int16, 8), stopCh: make(chan struct{})}
	go s.feed(stdout, events)
	return s, nil
}

// feed reads raw s16le stereo PCM off r in fixed-size chunks and sends
// each into s.PCM — blocking there is deliberate backpressure exactly like
// internal/ffdecode.AudioStream.feed: a slow/stalled remote connection
// naturally stops draining, which stops this send, which stops this Read,
// which stops parec's own capture buffer from being drained, which is
// harmless (parec just blocks, nothing is corrupted).
func (s *Stream) feed(r io.Reader, events chan<- Event) {
	defer close(s.PCM)
	const chunkBytes = 4096 // 1024 stereo frames
	buf := make([]byte, chunkBytes)
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			usable := n - (n % 4)
			if usable > 0 {
				select {
				case s.PCM <- bytesToInt16(buf[:usable]):
				case <-s.stopCh:
					return
				}
			}
		}
		if err != nil {
			select {
			case events <- Event{Err: fmt.Errorf("audiocap: %w", err)}:
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

// Stop kills the parec subprocess and unblocks feed if it's stuck mid-send
// with nothing left to ever drain it. Safe to call more than once.
func (s *Stream) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
}
