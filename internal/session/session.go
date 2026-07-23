package session

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Entry is one window to restore on next launch.
type Entry struct {
	Action      string `json:"action"` // terminal | notes | prog:id | pty:cmd | …
	Title       string `json:"title,omitempty"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	W           int    `json:"w"`
	H           int    `json:"h"`
	Maximized   bool   `json:"maximized,omitempty"`
	Minimized   bool   `json:"minimized,omitempty"`
	TitlePinned bool   `json:"title_pinned,omitempty"`
	Focused     bool   `json:"focused,omitempty"`
}

// State is the last desktop session.
type State struct {
	Windows []Entry `json:"windows"`
}

// Path returns ~/.config/ttypedesk/session.json
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "session.json"
	}
	return filepath.Join(home, ".config", "ttypedesk", "session.json")
}

func Load() (State, error) {
	var st State
	data, err := os.ReadFile(Path())
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

func Save(st State) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Clear removes the saved session (optional).
func Clear() error {
	err := os.Remove(Path())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
