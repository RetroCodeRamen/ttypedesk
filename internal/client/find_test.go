package client

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

func makeRow(cols int, text string) []cell.Cell {
	out := make([]cell.Cell, cols)
	for i := range out {
		out[i] = cell.Cell{Rune: ' '}
	}
	for i, r := range []rune(text) {
		if i >= cols {
			break
		}
		out[i].Rune = r
	}
	return out
}

func TestFindMatchAt(t *testing.T) {
	cols := 20
	cells := makeRow(cols, "hello world")

	cases := []struct {
		name  string
		row   int
		col   int
		query string
		want  bool
	}{
		{"inside match, first char", 0, 0, "hello", true},
		{"inside match, last char", 0, 4, "hello", true},
		{"just past match", 0, 5, "hello", false},
		{"case-insensitive", 0, 0, "HELLO", true},
		{"second word", 0, 6, "world", true},
		{"no match anywhere", 0, 0, "xyz", false},
		{"empty query", 0, 0, "", false},
		{"negative col", 0, -1, "hello", false},
		{"col past width", 0, cols, "hello", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := findMatchAt(cells, cols, c.row, c.col, c.query); got != c.want {
				t.Errorf("findMatchAt(row=%d,col=%d,q=%q) = %v, want %v", c.row, c.col, c.query, got, c.want)
			}
		})
	}
}

func TestFindMatchAtZeroWidth(t *testing.T) {
	if findMatchAt(nil, 0, 0, 0, "x") {
		t.Error("findMatchAt with cols=0 should always be false")
	}
}

func TestHandleFindKeyEscapeCloses(t *testing.T) {
	c := &Client{findOpen: true, findWin: "w1", findQuery: "abc"}
	if !c.handleFindKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)) {
		t.Fatal("handleFindKey(Escape) should report the event as consumed")
	}
	if c.findOpen {
		t.Fatal("Escape should close the find bar")
	}
	if c.findWin != "" {
		t.Fatal("Escape should clear findWin")
	}
}

func TestHandleFindKeyWhenNotOpenIsNoop(t *testing.T) {
	c := &Client{findOpen: false}
	if c.handleFindKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)) {
		t.Fatal("handleFindKey should report false (unconsumed) when the find bar isn't open")
	}
}

func TestHandleFindKeyTypingAndBackspace(t *testing.T) {
	c := &Client{findOpen: true}
	c.handleFindKey(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	c.handleFindKey(tcell.NewEventKey(tcell.KeyRune, 'b', tcell.ModNone))
	c.handleFindKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))
	if c.findQuery != "abc" {
		t.Fatalf("findQuery after typing = %q, want %q", c.findQuery, "abc")
	}
	c.handleFindKey(tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone))
	if c.findQuery != "ab" {
		t.Fatalf("findQuery after Backspace = %q, want %q", c.findQuery, "ab")
	}
}

func TestHandleFindKeyCtrlUClears(t *testing.T) {
	c := &Client{findOpen: true, findQuery: "something"}
	c.handleFindKey(tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModCtrl))
	if c.findQuery != "" {
		t.Fatalf("findQuery after Ctrl+U = %q, want empty", c.findQuery)
	}
}
