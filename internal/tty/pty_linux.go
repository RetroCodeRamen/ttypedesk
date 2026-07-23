package tty

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

func ptyStart(c *exec.Cmd, cols, rows int) (*os.File, error) {
	return pty.StartWithSize(c, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func ptySetsize(f *os.File, cols, rows int) error {
	if f == nil {
		return os.ErrClosed
	}
	return pty.Setsize(f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
