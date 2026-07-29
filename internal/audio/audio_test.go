package audio

import (
	"encoding/binary"
	"io"
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
