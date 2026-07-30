package main

import (
	"fmt"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/surface"
)

func main() {
	s, err := surface.NewPtySurface("t1", "/bin/bash", 40, 12, 100)
	if err != nil {
		panic(err)
	}
	defer s.Close()
	time.Sleep(200 * time.Millisecond)
	_ = s.HandleInput(surface.InputEvent{Bytes: []byte("echo HELLO_TTYPE\n")})
	time.Sleep(400 * time.Millisecond)
	cells := s.Snapshot()
	nonspace := 0
	for _, c := range cells {
		if c.Rune != ' ' && c.Rune != 0 {
			nonspace++
		}
	}
	row := make([]rune, 0, 40)
	for i := 0; i < 40 && i < len(cells); i++ {
		row = append(row, cells[i].Rune)
	}
	fmt.Printf("ok: pty cells=%d nonspace=%d row0=%q\n", len(cells), nonspace, string(row))
}
