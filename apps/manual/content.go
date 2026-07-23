package manual

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed content/*.md
var contentFS embed.FS

// Chapter is one Manual section.
type Chapter struct {
	ID    string // filename stem, e.g. 01-welcome
	Title string
	Body  string
}

// Dir returns ~/.config/ttypedesk/System (on-disk copy of Manual chapters).
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "System"
	}
	return filepath.Join(home, ".config", "ttypedesk", "System")
}

// EnsureSystemFolder writes embedded Manual markdown into Dir().
// Existing files are overwritten so shipped docs stay current.
func EnsureSystemFolder() error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := fs.ReadDir(contentFS, "content")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := contentFS.ReadFile("content/" + e.Name())
		if err != nil {
			return err
		}
		dest := filepath.Join(dir, e.Name())
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}
	readme := filepath.Join(dir, "README.txt")
	_ = os.WriteFile(readme, []byte("TTYPE Desk System folder — Manual chapters (.md).\nOpen Start → System → Manual for the in-desk reader.\n"), 0o644)
	return nil
}

// LoadChapters returns Manual chapters sorted by filename.
func LoadChapters() ([]Chapter, error) {
	entries, err := fs.ReadDir(contentFS, "content")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]Chapter, 0, len(names))
	for _, name := range names {
		data, err := contentFS.ReadFile("content/" + name)
		if err != nil {
			return nil, err
		}
		body := string(data)
		title := titleFromMarkdown(body, name)
		id := strings.TrimSuffix(name, ".md")
		out = append(out, Chapter{ID: id, Title: title, Body: body})
	}
	return out, nil
}

func titleFromMarkdown(body, filename string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	base := strings.TrimSuffix(filename, ".md")
	if i := strings.IndexByte(base, '-'); i >= 0 && i+1 < len(base) {
		return strings.ReplaceAll(base[i+1:], "-", " ")
	}
	return base
}
