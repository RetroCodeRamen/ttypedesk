package tty

import (
	"sync"
	"testing"
	"time"
)

// TestCloseUnblocksRead is the direct regression test for the File/Cmd race
// this package used to have: Close swapping h.File to nil concurrently with
// Read/Write dereferencing it, unguarded. Spawns a real PTY process, blocks
// a goroutine in Read, then Closes — Read must return promptly rather than
// hang or crash.
func TestCloseUnblocksRead(t *testing.T) {
	h, err := Spawn("cat", nil, 24, 80)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 64)
		_, _ = h.Read(buf)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond) // let the goroutine get into Read
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not unblock within 2s of Close")
	}
}

// TestConcurrentReadWriteCloseSetSize hammers every method from multiple
// goroutines against a single Handle while Close runs concurrently, under
// -race, mirroring the internal/vterm regression test for the same bug
// class (unsynchronized field access racing a concurrent Close).
func TestConcurrentReadWriteCloseSetSize(t *testing.T) {
	for iter := 0; iter < 10; iter++ {
		h, err := Spawn("cat", nil, 24, 80)
		if err != nil {
			t.Fatalf("Spawn: %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(4)
		go func() {
			defer wg.Done()
			buf := make([]byte, 32)
			for i := 0; i < 50; i++ {
				_, _ = h.Read(buf)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_, _ = h.Write([]byte("x"))
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = h.SetSize(24, 80+i%5)
			}
		}()
		go func() {
			defer wg.Done()
			_ = h.Close()
		}()
		wg.Wait()
	}
}
