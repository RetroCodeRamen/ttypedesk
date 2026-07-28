// bridgecheck is a manual smoke test for the GUI-TUI Bridge (internal/bridge):
// launches a real X11 app (xclock by default) inside a nested Xvfb, captures
// a few frames, and checks that non-background content actually showed up.
// Requires Xvfb and the target app on PATH. Not run in CI as a `go test`
// (needs a real display server) — see .github/workflows/ci.yml for the
// scripted headless variant.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/bridge"
)

func main() {
	cmdName := "xclock"
	if len(os.Args) > 1 {
		cmdName = os.Args[1]
	}

	b, err := bridge.New("check1", cmdName, 40, 20)
	if err != nil {
		fatal(fmt.Sprintf("bridge.New: %v", err))
	}
	defer b.Close()

	fmt.Printf("-- launched %q, capturing frames --\n", cmdName)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		cells := b.Snapshot()
		// Half-block cells always use the same glyph — real content shows
		// up as color variety, not rune variety. A blank/unrendered capture
		// comes back as one solid color across every cell.
		colors := make(map[string]struct{})
		for _, c := range cells {
			colors[fmt.Sprintf("%v/%v", c.FG, c.BG)] = struct{}{}
		}
		if len(colors) > 1 {
			fmt.Printf("ok: %d distinct colors across %d cells\n", len(colors), len(cells))
			fmt.Println("all bridge smoke tests passed")
			return
		}
	}
	fatal("captured frames were a single solid color for 5s — is Xvfb/the guest app actually rendering?")
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "FAIL:", msg)
	os.Exit(1)
}
