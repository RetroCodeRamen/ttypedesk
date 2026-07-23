package main

import (
	"fmt"
	"strings"

	"github.com/ttypedesk/ttypedesk/internal/vterm"
)

func main() {
	t := vterm.New(5, 20, 100)
	defer t.Close()
	for i := 0; i < 12; i++ {
		t.Write([]byte(fmt.Sprintf("LINE-%02d\r\n", i)))
	}
	t.ScrollBy(3)
	cells := t.Snapshot()
	var b strings.Builder
	for i := 0; i < 20 && i < len(cells); i++ {
		b.WriteRune(cells[i].Rune)
	}
	fmt.Printf("scrollOff=%d row0=%q\n", t.ScrollOffset(), strings.TrimRight(b.String(), " "))
	if t.ScrollOffset() != 3 {
		panic("scroll offset")
	}
	fmt.Println("ok: scrollback")
}
