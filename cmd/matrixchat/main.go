// Command matrixchat is the Matrix messenger client (apps/matrixchat),
// run as a standalone out-of-process TTYPE Desk app via pkg/extapprun —
// see docs/appstore.md and docs/extapp.md. Install via the App Store, or
// build/run it directly and launch with an "extapp:/path/to/matrixchat"
// action string.
package main

import (
	"fmt"
	"os"

	"github.com/RetroCodeRamen/ttypedesk/apps/matrixchat"
	"github.com/RetroCodeRamen/ttypedesk/pkg/extapprun"
)

func main() {
	if err := extapprun.Run(matrixchat.New()); err != nil {
		fmt.Fprintln(os.Stderr, "matrixchat:", err)
		os.Exit(1)
	}
}
