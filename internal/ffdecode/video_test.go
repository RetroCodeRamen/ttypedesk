package ffdecode

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// synthVideo generates a short real video file via ffmpeg's lavfi
// testsrc (a moving test pattern, so frames have real non-uniform pixel
// content, not just a solid color).
func synthVideo(t *testing.T, seconds int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=duration="+strconv.Itoa(seconds)+":size=64x64:rate=10",
		"-y", "-loglevel", "error", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg synth failed: %v\n%s", err, out)
	}
	return path
}

func TestDecodeVideoProducesRealFrames(t *testing.T) {
	requireFFmpeg(t)
	path := synthVideo(t, 1)

	events := make(chan VideoEvent, 2)
	s, err := DecodeVideo(path, 32, 24, 10, events)
	if err != nil {
		t.Fatalf("DecodeVideo: %v", err)
	}
	defer s.Stop()

	var frameCount int
	var gotVariedPixels bool
	deadline := time.After(10 * time.Second)
loop:
	for {
		select {
		case f, ok := <-s.Frames:
			if !ok {
				break loop
			}
			if len(f.Pix) != 32*24*3 {
				t.Fatalf("frame %d Pix len = %d, want %d", frameCount, len(f.Pix), 32*24*3)
			}
			if f.W != 32 || f.H != 24 {
				t.Fatalf("frame %d dims = %dx%d, want 32x24", frameCount, f.W, f.H)
			}
			if !allSameByte(f.Pix) {
				gotVariedPixels = true
			}
			frameCount++
		case ev := <-events:
			if ev.Err != nil {
				t.Fatalf("decode error: %v", ev.Err)
			}
			if ev.Ended {
				break loop
			}
		case <-deadline:
			t.Fatal("timed out waiting for decoded frames — ffmpeg pipeline likely stuck")
		}
	}
	if frameCount == 0 {
		t.Fatal("no frames decoded")
	}
	if !gotVariedPixels {
		t.Error("every frame was a single flat color — expected testsrc's moving pattern to produce real variation")
	}
}

func allSameByte(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	first := b[0]
	for _, v := range b {
		if v != first {
			return false
		}
	}
	return true
}

func TestDecodeVideoStopUnblocksFeedGoroutine(t *testing.T) {
	requireFFmpeg(t)
	path := synthVideo(t, 3)

	events := make(chan VideoEvent, 2)
	s, err := DecodeVideo(path, 32, 24, 10, events)
	if err != nil {
		t.Fatalf("DecodeVideo: %v", err)
	}
	// Deliberately never drain s.Frames.
	s.Stop()

	done := make(chan struct{})
	go func() {
		for range s.Frames {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("feed goroutine did not exit after Stop() — Frames channel never closed")
	}
}

func countFrames(t *testing.T, s *VideoStream, events chan VideoEvent) int {
	t.Helper()
	var n int
	deadline := time.After(10 * time.Second)
	for {
		select {
		case _, ok := <-s.Frames:
			if !ok {
				return n
			}
			n++
		case ev := <-events:
			if ev.Err != nil {
				t.Fatalf("decode error: %v", ev.Err)
			}
			if ev.Ended {
				return n
			}
		case <-deadline:
			t.Fatal("timed out counting frames — ffmpeg pipeline likely stuck")
		}
	}
}

func TestDecodeVideoAtSeekSkipsContent(t *testing.T) {
	requireFFmpeg(t)
	path := synthVideo(t, 4)

	fullEvents := make(chan VideoEvent, 2)
	full, err := DecodeVideoAt(path, 0, 32, 24, 10, fullEvents)
	if err != nil {
		t.Fatalf("DecodeVideoAt(seek=0): %v", err)
	}
	fullCount := countFrames(t, full, fullEvents)
	full.Stop()

	seekEvents := make(chan VideoEvent, 2)
	seeked, err := DecodeVideoAt(path, 2*time.Second, 32, 24, 10, seekEvents)
	if err != nil {
		t.Fatalf("DecodeVideoAt(seek=2s): %v", err)
	}
	seekedCount := countFrames(t, seeked, seekEvents)
	seeked.Stop()

	if seekedCount >= fullCount {
		t.Errorf("seeked decode produced %d frames, want fewer than the unseeked %d (seeking 2s into a 4s clip should skip roughly half of it)", seekedCount, fullCount)
	}
}

func TestVideoFrameImageSamplesCorrectly(t *testing.T) {
	// 2x2 RGB24: (0,0)=red, (1,0)=green, (0,1)=blue, (1,1)=white.
	f := VideoFrame{
		W: 2, H: 2,
		Pix: []byte{
			255, 0, 0, 0, 255, 0,
			0, 0, 255, 255, 255, 255,
		},
	}
	img := f.Image()
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Fatalf("Bounds() = %v, want 2x2", b)
	}
	cases := []struct {
		x, y       int
		r, g, b, a uint8
	}{
		{0, 0, 255, 0, 0, 255},
		{1, 0, 0, 255, 0, 255},
		{0, 1, 0, 0, 255, 255},
		{1, 1, 255, 255, 255, 255},
	}
	for _, c := range cases {
		r, g, b, a := img.At(c.x, c.y).RGBA()
		got := [4]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
		want := [4]uint8{c.r, c.g, c.b, c.a}
		if got != want {
			t.Errorf("At(%d,%d) = %v, want %v", c.x, c.y, got, want)
		}
	}
}

func TestDecodeVideoRejectsInvalidSize(t *testing.T) {
	requireFFmpeg(t)
	if _, err := DecodeVideo("irrelevant.mp4", 0, 24, 10, nil); err == nil {
		t.Error("DecodeVideo with w=0: want error, got nil")
	}
}
