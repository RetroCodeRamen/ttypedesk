// Package clip holds an in-process clipboard plus host sync (OSC 52 + wl-copy/xclip).
package clip

import (
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var (
	mu   sync.RWMutex
	text string
)

func Set(s string) {
	mu.Lock()
	text = s
	mu.Unlock()
	// OSC 52 — many terminals accept this for system clipboard.
	if s != "" && len(s) < 100000 {
		b64 := base64.StdEncoding.EncodeToString([]byte(s))
		_, _ = os.Stdout.WriteString("\x1b]52;c;" + b64 + "\x07")
	}
	setExternal(s)
}

func Get() string {
	// Prefer host clipboard when available (wl-paste / xclip), else in-process buffer.
	if ext := getExternal(); ext != "" {
		mu.Lock()
		text = ext
		mu.Unlock()
		return ext
	}
	mu.RLock()
	defer mu.RUnlock()
	return text
}

func setExternal(s string) {
	type cand struct {
		name string
		args []string
	}
	for _, c := range []cand{
		{"wl-copy", nil},
		{"xclip", []string{"-selection", "clipboard"}},
		{"xsel", []string{"--clipboard", "--input"}},
	} {
		if _, err := exec.LookPath(c.name); err != nil {
			continue
		}
		cmd := exec.Command(c.name, c.args...)
		cmd.Stdin = strings.NewReader(s)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err == nil {
			return
		}
	}
}

func getExternal() string {
	type cand struct {
		name string
		args []string
	}
	for _, c := range []cand{
		{"wl-paste", []string{"--no-newline"}},
		{"xclip", []string{"-selection", "clipboard", "-o"}},
		{"xsel", []string{"--clipboard", "--output"}},
	} {
		if _, err := exec.LookPath(c.name); err != nil {
			continue
		}
		cmd := exec.Command(c.name, c.args...)
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
	}
	return ""
}
