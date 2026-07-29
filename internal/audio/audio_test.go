package audio

import (
	"encoding/binary"
	"io"
	"sync"
	"testing"
	"time"
)

// These tests exercise chanReader directly rather than Play/sharedContext —
// oto.NewContext talks to a real audio device, which this sandbox (and
// most CI runners) doesn't have. The reader is the only part with
// interesting logic; the oto plumbing around it is a thin, trusted wrapper.

func TestChanReaderEncodesLittleEndianInt16(t *testing.T) {
	pcm := make(chan []int16, 1)
	pcm <- []int16{1, -1, 32767, -32768}
	r := &chanReader{pcm: pcm, stop: make(chan struct{})}

	buf := make([]byte, 8)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != 8 {
		t.Fatalf("Read n = %d, want 8", n)
	}
	want := []int16{1, -1, 32767, -32768}
	for i, w := range want {
		got := int16(binary.LittleEndian.Uint16(buf[i*2:]))
		if got != w {
			t.Errorf("sample %d = %d, want %d", i, got, w)
		}
	}
}

func TestChanReaderReadsAcrossSmallBuffers(t *testing.T) {
	pcm := make(chan []int16, 1)
	pcm <- []int16{100, 200, 300}
	r := &chanReader{pcm: pcm, stop: make(chan struct{})}

	var got []byte
	buf := make([]byte, 1)
	for len(got) < 6 {
		n, err := r.Read(buf)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		got = append(got, buf[:n]...)
	}
	for i, w := range []int16{100, 200, 300} {
		if s := int16(binary.LittleEndian.Uint16(got[i*2:])); s != w {
			t.Errorf("sample %d = %d, want %d", i, s, w)
		}
	}
}

func TestChanReaderEOFWhenChannelClosed(t *testing.T) {
	pcm := make(chan []int16)
	close(pcm)
	r := &chanReader{pcm: pcm, stop: make(chan struct{})}

	_, err := r.Read(make([]byte, 4))
	if err != io.EOF {
		t.Fatalf("Read err = %v, want io.EOF", err)
	}
}

// TestChanReaderReadIsSafeForConcurrentCallers is a regression test for a
// real CI failure: oto/v3's Player.Play, called again to resume after a
// Pause, doesn't reliably wait for the previous internal read goroutine to
// exit first — briefly, two goroutines called Read on the same chanReader
// concurrently, which produced a DATA RACE and then a slice-bounds panic
// in the old, unlocked implementation. This drives that exact shape
// directly: many goroutines reading concurrently while pcm is fed
// continuously, under -race, with no crash and no race report as the bar.
func TestChanReaderReadIsSafeForConcurrentCallers(t *testing.T) {
	pcm := make(chan []int16)
	stop := make(chan struct{})
	r := &chanReader{pcm: pcm, stop: stop}
	defer close(stop)

	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		for i := 0; i < 200; i++ {
			select {
			case pcm <- []int16{int16(i), int16(-i)}:
			case <-stop:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 3) // deliberately not a multiple of a sample, forces the buf/reslice path
			for i := 0; i < 100; i++ {
				if _, err := r.Read(buf); err != nil {
					return
				}
			}
		}()
	}
	wg.Wait()
	<-feedDone
}

func TestChanReaderEOFWhenStopped(t *testing.T) {
	pcm := make(chan []int16)
	r := &chanReader{pcm: pcm, stop: make(chan struct{})}
	close(r.stop)

	done := make(chan struct{})
	var err error
	go func() {
		_, err = r.Read(make([]byte, 4))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not return after stop was closed — likely blocked forever")
	}
	if err != io.EOF {
		t.Errorf("Read err = %v, want io.EOF", err)
	}
}
