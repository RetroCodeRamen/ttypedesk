package palette

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// HistoryPath is ~/.config/ttypedesk/palette_history.json
func HistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "palette_history.json"
	}
	return filepath.Join(home, ".config", "ttypedesk", "palette_history.json")
}

// LoadHistory reads recent palette queries (newest first).
func LoadHistory(max int) []string {
	if max <= 0 {
		max = 20
	}
	data, err := os.ReadFile(HistoryPath())
	if err != nil {
		return nil
	}
	var items []string
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	if len(items) > max {
		items = items[:max]
	}
	return items
}

// SaveHistory writes recent queries.
func SaveHistory(items []string, max int) {
	if max <= 0 {
		max = 20
	}
	if len(items) > max {
		items = items[:max]
	}
	path := HistoryPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}
