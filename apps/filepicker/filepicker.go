// Package filepicker is a small modal file browser used by Host.PickFile —
// deliberately not the full Files app (apps/files): no clipboard, trash,
// mkdir, or drag/drop, just navigate and pick, so consumers like Amp/Vid
// ("open local file") don't have to pull in a second, heavier UI.
package filepicker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
)

type entry struct {
	name  string
	isDir bool
}

// App is a modal file browser: Up/Down move, Enter opens a directory or
// picks a file, Esc cancels. onResult fires exactly once.
type App struct {
	dir        string
	extensions []string
	onResult   func(path string, ok bool)

	entries []entry
	sel     int
	scroll  int
	err     string
	closing bool
	done    bool
}

// New opens a picker rooted at startDir (falls back to the home directory
// if empty or unreadable), restricted to extensions (without the leading
// dot) if any are given.
func New(startDir string, extensions []string, onResult func(path string, ok bool)) *App {
	if startDir == "" {
		startDir, _ = os.UserHomeDir()
	}
	if info, err := os.Stat(startDir); err != nil || !info.IsDir() {
		startDir = filepath.Dir(startDir)
	}
	a := &App{dir: startDir, extensions: extensions, onResult: onResult}
	a.reload()
	return a
}

func (a *App) WantsClose() bool { return a.closing }

func (a *App) Init(ctx *uiapp.Context) error { return nil }

func (a *App) Close() error {
	if !a.done && a.onResult != nil {
		a.done = true
		a.onResult("", false)
	}
	return nil
}

func (a *App) reload() {
	a.err = ""
	items, err := os.ReadDir(a.dir)
	if err != nil {
		a.err = err.Error()
		a.entries = nil
		a.sel, a.scroll = 0, 0
		return
	}
	var dirs, files []entry
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if it.IsDir() {
			dirs = append(dirs, entry{name: name, isDir: true})
			continue
		}
		if len(a.extensions) > 0 && !hasExt(name, a.extensions) {
			continue
		}
		files = append(files, entry{name: name})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	a.entries = a.entries[:0]
	if filepath.Dir(a.dir) != a.dir {
		a.entries = append(a.entries, entry{name: "..", isDir: true})
	}
	a.entries = append(a.entries, dirs...)
	a.entries = append(a.entries, files...)
	a.sel, a.scroll = 0, 0
}

func hasExt(name string, extensions []string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	for _, e := range extensions {
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}

func (a *App) activate() {
	if a.sel < 0 || a.sel >= len(a.entries) {
		return
	}
	e := a.entries[a.sel]
	if e.isDir {
		if e.name == ".." {
			a.dir = filepath.Dir(a.dir)
		} else {
			a.dir = filepath.Join(a.dir, e.name)
		}
		a.reload()
		return
	}
	if a.onResult != nil {
		a.done = true
		a.onResult(filepath.Join(a.dir, e.name), true)
	}
	a.closing = true
}

func (a *App) cancel() {
	if a.onResult != nil {
		a.done = true
		a.onResult("", false)
	}
	a.closing = true
}

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventKey:
		switch e.Key {
		case "Up":
			if a.sel > 0 {
				a.sel--
			}
		case "Down":
			if a.sel < len(a.entries)-1 {
				a.sel++
			}
		case "Enter":
			a.activate()
		case "Escape":
			a.cancel()
		case "Backspace":
			if filepath.Dir(a.dir) != a.dir {
				a.dir = filepath.Dir(a.dir)
				a.reload()
			}
		}
	case uiapp.EventMouse:
		if e.Action == "press" && e.Y >= 1 {
			row := a.scroll + e.Y - 1
			if row >= 0 && row < len(a.entries) {
				if row == a.sel {
					a.activate()
				} else {
					a.sel = row
				}
			}
		}
	}
	return nil
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0xAA)
	sel := cell.RGB(0x00, 0x00, 0xAA)
	selFG := cell.RGB(0xFF, 0xFF, 0xFF)

	cv.FillRect(0, 0, cols, rows, bg)
	title := " " + a.dir + " "
	if len([]rune(title)) > cols {
		title = "…" + string([]rune(title)[len([]rune(title))-cols+1:])
	}
	cv.DrawText(0, 0, title, cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)

	if a.err != "" {
		cv.DrawText(1, 2, "Error: "+a.err, fg, bg, 0)
		return nil
	}

	bodyRows := rows - 2
	if bodyRows < 1 {
		return nil
	}
	if a.sel < a.scroll {
		a.scroll = a.sel
	}
	if a.sel >= a.scroll+bodyRows {
		a.scroll = a.sel - bodyRows + 1
	}
	for i := 0; i < bodyRows; i++ {
		idx := a.scroll + i
		if idx >= len(a.entries) {
			break
		}
		e := a.entries[idx]
		label := e.name
		icon := "  "
		if e.isDir {
			icon = "📁"
		}
		rowFG, rowBG := fg, bg
		if idx == a.sel {
			rowFG, rowBG = selFG, sel
		}
		cv.FillRect(0, i+1, cols, 1, rowBG)
		cv.DrawIcon(1, i+1, icon, label, rowFG, rowBG, 0)
	}
	cv.DrawText(0, rows-1, " Enter=open/pick  Backspace=up  Esc=cancel ", fg, bg, 0)
	return nil
}
