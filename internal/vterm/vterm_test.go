package vterm

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestWriteAndSnapshot(t *testing.T) {
	term := New(5, 20, 0)
	defer term.Close()

	term.Write([]byte("hello"))
	cells := term.Snapshot()
	if len(cells) != 20*5 {
		t.Fatalf("Snapshot() len = %d, want %d", len(cells), 20*5)
	}
	var got strings.Builder
	for i := 0; i < 5; i++ {
		got.WriteRune(cells[i].Rune)
	}
	if got.String() != "hello" {
		t.Fatalf("row0 = %q, want %q", got.String(), "hello")
	}
}

func TestWriteTrueColor(t *testing.T) {
	term := New(8, 40, 0)
	defer term.Close()

	term.Write([]byte("\x1b[38;2;255;80;0mTRUECOLOR\x1b[0m"))
	diff := term.ProduceDiff()
	if diff.Rect.W == 0 {
		t.Fatal("ProduceDiff() returned an empty diff after writing content")
	}
	found := false
	for _, c := range diff.Cells {
		if c.FG.R == 255 && c.FG.G == 80 && c.FG.B == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("truecolor FG (255,80,0) not found in diff cells")
	}
}

func TestResize(t *testing.T) {
	term := New(10, 40, 0)
	defer term.Close()

	term.Resize(20, 60)
	cols, rows := term.Size()
	if cols != 60 || rows != 20 {
		t.Fatalf("Size() = (%d,%d), want (60,20)", cols, rows)
	}
	cells := term.Snapshot()
	if len(cells) != 60*20 {
		t.Fatalf("Snapshot() len after resize = %d, want %d", len(cells), 60*20)
	}
}

func TestResizeIgnoresNonPositive(t *testing.T) {
	term := New(10, 40, 0)
	defer term.Close()

	term.Resize(0, 60)
	term.Resize(20, -1)
	cols, rows := term.Size()
	if cols != 40 || rows != 10 {
		t.Fatalf("Size() after invalid resizes = (%d,%d), want unchanged (40,10)", cols, rows)
	}
}

func TestTitle(t *testing.T) {
	term := New(5, 20, 0)
	defer term.Close()

	term.Write([]byte("\x1b]2;My Title\x07"))
	if got := term.Title(); got != "My Title" {
		t.Fatalf("Title() = %q, want %q", got, "My Title")
	}
}

func TestBell(t *testing.T) {
	term := New(5, 20, 0)
	defer term.Close()

	if term.TakeBell() {
		t.Fatal("TakeBell() = true before any bell was written")
	}
	term.Write([]byte("\x07"))
	if !term.TakeBell() {
		t.Fatal("TakeBell() = false after writing a bell byte")
	}
	if term.TakeBell() {
		t.Fatal("TakeBell() should be false again after being consumed")
	}
}

func TestScrollback(t *testing.T) {
	term := New(5, 20, 100)
	defer term.Close()

	for i := 0; i < 12; i++ {
		term.Write([]byte(fmt.Sprintf("LINE-%02d\r\n", i)))
	}
	if !term.HasScrollback() {
		t.Fatal("HasScrollback() = false after writing more lines than the screen height")
	}
	term.ScrollBy(3)
	if term.ScrollOffset() != 3 {
		t.Fatalf("ScrollOffset() = %d, want 3", term.ScrollOffset())
	}
	cells := term.Snapshot()
	var row0 strings.Builder
	for i := 0; i < 20 && i < len(cells); i++ {
		row0.WriteRune(cells[i].Rune)
	}
	if got := strings.TrimRight(row0.String(), " "); got == "" {
		t.Fatal("scrolled-back row0 is empty, expected scrollback content")
	}
}

func TestScrollByClampsToBounds(t *testing.T) {
	term := New(5, 20, 100)
	defer term.Close()
	for i := 0; i < 12; i++ {
		term.Write([]byte(fmt.Sprintf("LINE-%02d\r\n", i)))
	}
	term.ScrollBy(-100)
	if term.ScrollOffset() != 0 {
		t.Fatalf("ScrollOffset() after large negative scroll = %d, want 0", term.ScrollOffset())
	}
	term.ScrollBy(100000)
	maxOff := term.ScrollOffset()
	if maxOff <= 0 {
		t.Fatalf("ScrollOffset() after large positive scroll = %d, want > 0 (clamped to history length)", maxOff)
	}
}

func TestSearchScrollback(t *testing.T) {
	term := New(5, 20, 100)
	defer term.Close()
	for i := 0; i < 12; i++ {
		term.Write([]byte(fmt.Sprintf("LINE-%02d\r\n", i)))
	}
	found, matches := term.SearchScrollback("line-03", true)
	if !found || matches == 0 {
		t.Fatalf("SearchScrollback(line-03) = (%v,%d), want a match", found, matches)
	}
	found, _ = term.SearchScrollback("not-present-anywhere", true)
	if found {
		t.Fatal("SearchScrollback found a match for a query that shouldn't exist")
	}
}

// TestConcurrentAccessDuringClose is the regression test for b4bfd63: Write,
// Resize, KeyboardKey, and MouseButton all raced Close's vterm_free by
// capturing t.vt/t.screen, releasing the lock, then calling into libvterm —
// a concurrent Close could free the pointer in that window. vtMu now covers
// the whole capture-through-call span, so this should be race- and
// crash-free under `go test -race`.
func TestConcurrentAccessDuringClose(t *testing.T) {
	for iter := 0; iter < 25; iter++ {
		term := New(24, 80, 50)
		var wg sync.WaitGroup
		wg.Add(5)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				term.Write([]byte("x"))
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				term.Resize(20+i%5, 78+i%5)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				term.KeyboardKey(KeyEnter, ModNone)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				term.MouseButton(1, i%2 == 0, 0, 0, ModNone)
			}
		}()
		go func() {
			defer wg.Done()
			term.Close()
		}()
		wg.Wait()
	}
}

// TestConcurrentNewClose guards the cgo.Handle registry (registryMu/registry)
// under real concurrent New/Write/Close churn across goroutines — the other
// half of the b4bfd63 fix (threading the handle as a uintptr through C
// rather than an invalid unsafe.Pointer conversion).
func TestConcurrentNewClose(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(10)
	for g := 0; g < 10; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				term := New(10, 40, 0)
				term.Write([]byte("hi"))
				term.Close()
			}
		}()
	}
	wg.Wait()
}

func TestCloseIsSafeToCallTwice(t *testing.T) {
	term := New(5, 20, 0)
	term.Close()
	term.Close() // must not panic
}

func TestMethodsAfterCloseAreSafeNoops(t *testing.T) {
	term := New(5, 20, 0)
	term.Close()

	term.Write([]byte("still writing?"))
	term.Resize(10, 10)
	term.KeyboardKey(KeyEnter, ModNone)
	term.MouseButton(1, true, 0, 0, ModNone)
	_ = term.Snapshot()
	_ = term.ProduceDiff()
}
