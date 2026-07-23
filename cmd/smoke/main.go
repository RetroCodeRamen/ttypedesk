package main

import (
	"fmt"
	"os"

	"github.com/ttypedesk/ttypedesk/internal/gfx"
	"github.com/ttypedesk/ttypedesk/internal/proto"
	"github.com/ttypedesk/ttypedesk/internal/vterm"
	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

func main() {
	// libvterm truecolor
	t := vterm.New(8, 40, 100)
	defer t.Close()
	seq := "\x1b[38;2;255;80;0mTRUECOLOR\x1b[0m\n\x1b[48;2;0;100;200m  BG  \x1b[0m\r\n"
	t.Write([]byte(seq))
	diff := t.ProduceDiff()
	if diff.Rect.W == 0 {
		fatal("empty diff")
	}
	found := false
	for _, c := range diff.Cells {
		if c.FG.R == 255 && c.FG.G == 80 && c.FG.B == 0 {
			found = true
			break
		}
	}
	if !found {
		// dump first row for debug
		for i := 0; i < diff.Rect.W && i < len(diff.Cells); i++ {
			c := diff.Cells[i]
			fmt.Printf("%c fg=%v bg=%v\n", c.Rune, c.FG, c.BG)
		}
		fatal("truecolor FG not found")
	}
	fmt.Println("ok: libvterm truecolor")

	// half-block gfx
	img := gfx.SolidImage(32, 32)
	cells := gfx.EncodeHalfBlock(img, 16, 8, 0, 0)
	if len(cells) != 16*8 {
		fatal("gfx size")
	}
	if cells[0].Rune != '▄' {
		fatal("halfblock rune")
	}
	fmt.Println("ok: half-block gfx")

	// proto roundtrip
	b, err := proto.Encode(proto.TypeScreenDiff, "w1", proto.ScreenDiffPayload{
		Diff: cell.FullGridDiff(2, 1, []cell.Cell{{Rune: 'A', FG: cell.RGB(1, 2, 3), BG: cell.RGB(4, 5, 6)}}),
	})
	if err != nil {
		fatal(err.Error())
	}
	env, err := proto.Decode(b)
	if err != nil || env.Type != proto.TypeScreenDiff {
		fatal("proto decode")
	}
	fmt.Println("ok: proto")

	fmt.Println("all smoke tests passed")
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "FAIL:", msg)
	os.Exit(1)
}
