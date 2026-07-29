package ffdecode

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os/exec"
	"sync"
	"time"
)

// VideoFrame is one decoded frame: raw RGB24 (3 bytes/pixel, no alpha),
// W*H*3 bytes, row-major.
type VideoFrame struct {
	Pix  []byte
	W, H int
}

// Image returns f as an image.Image — wraps Pix directly, no copy, for
// feeding straight into internal/gfx.EncodeHalfBlockFit.
func (f VideoFrame) Image() image.Image {
	return &rgb24Image{pix: f.Pix, w: f.W, h: f.H}
}

// rgb24Image is the minimal image.Image over a raw RGB24 buffer — the
// stdlib has no plain-RGB (alpha-less) image type, and converting to a
// full image.RGBA per frame would be a needless copy on top of decode
// that's already happening every frame.
type rgb24Image struct {
	pix  []byte
	w, h int
}

func (im *rgb24Image) ColorModel() color.Model { return color.RGBAModel }
func (im *rgb24Image) Bounds() image.Rectangle { return image.Rect(0, 0, im.w, im.h) }
func (im *rgb24Image) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= im.w || y >= im.h {
		return color.RGBA{}
	}
	i := (y*im.w + x) * 3
	return color.RGBA{R: im.pix[i], G: im.pix[i+1], B: im.pix[i+2], A: 0xff}
}

// VideoEvent is how a VideoStream's feeder goroutine reports back — same
// drain-from-your-single-threaded-path contract as AudioEvent.
type VideoEvent struct {
	Ended bool
	Err   error
}

// VideoStream decodes one file's video track to raw RGB24 frames via an
// ffmpeg subprocess, pre-scaled to w x h pixels at fps — scaling in
// ffmpeg rather than after decode keeps the pipe's byte volume (and the
// per-frame conversion cost) down to roughly what the terminal can
// actually show, instead of piping full source resolution just to
// downsample it a moment later.
type VideoStream struct {
	cmd    *exec.Cmd
	Frames chan VideoFrame
	w, h   int

	stopCh   chan struct{}
	stopOnce sync.Once
}

// DecodeVideo launches ffmpeg on path and begins feeding frames into the
// returned stream's Frames channel. events receives exactly one
// VideoEvent when the video ends (Ended=true) or decode fails (Err set).
func DecodeVideo(path string, w, h, fps int, events chan<- VideoEvent) (*VideoStream, error) {
	return DecodeVideoAt(path, 0, w, h, fps, events)
}

// DecodeVideoAt is DecodeVideo starting from seek into the file (0 for
// the beginning) — see DecodeAudioAt's doc comment; Vid uses this for
// scrubbing.
func DecodeVideoAt(path string, seek time.Duration, w, h, fps int, events chan<- VideoEvent) (*VideoStream, error) {
	if !Available() {
		return nil, fmt.Errorf("ffmpeg not found on PATH — install it to play video (e.g. apt install ffmpeg)")
	}
	if w < 1 || h < 1 {
		return nil, fmt.Errorf("ffdecode: invalid frame size %dx%d", w, h)
	}
	if fps < 1 {
		fps = 15
	}
	args := seekArgs(seek)
	args = append(args,
		"-i", path,
		"-an",
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-vf", fmt.Sprintf("scale=%d:%d", w, h),
		"-r", fmt.Sprint(fps),
		"-loglevel", "error",
		"-nostdin",
		"pipe:1",
	)
	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffdecode: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffdecode: start ffmpeg: %w", err)
	}

	s := &VideoStream{cmd: cmd, Frames: make(chan VideoFrame, 2), w: w, h: h, stopCh: make(chan struct{})}
	go s.feed(stdout, events)
	return s, nil
}

// feed reads exactly one w*h*3-byte frame at a time — a partial trailing
// frame at EOF isn't usable (can't display half a frame), unlike audio's
// trailing partial chunk, so any read error just ends the stream rather
// than trying to salvage a partial buffer. Same backpressure/stopCh
// contract as AudioStream.feed.
func (s *VideoStream) feed(r io.Reader, events chan<- VideoEvent) {
	defer close(s.Frames)
	frameSize := s.w * s.h * 3
	for {
		buf := make([]byte, frameSize)
		if _, err := io.ReadFull(r, buf); err != nil {
			ev := VideoEvent{Ended: true}
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				ev = VideoEvent{Err: fmt.Errorf("ffdecode: %w", err)}
			}
			select {
			case events <- ev:
			case <-s.stopCh:
			}
			return
		}
		select {
		case s.Frames <- VideoFrame{Pix: buf, W: s.w, H: s.h}:
		case <-s.stopCh:
			return
		}
	}
}

// Stop kills the ffmpeg subprocess and unblocks feed if it's stuck
// mid-send with nothing left to drain it. Safe to call more than once,
// and safe to call after decode has already finished on its own.
func (s *VideoStream) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
}
