package files

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uwidth"
)

// App is the TTYPE Desk file manager.
type App struct {
	cfg    config.Config
	onSave func(config.Config)
	ctx    *uiapp.Context

	dir    string
	ents   []entry
	sel    int
	scroll uiapp.ScrollState
	err    string
	status string

	mode       string // browse | settings | prompt | confirm
	promptKind string // mkdir | rename | path | delete
	promptBuf  string
	clipPaths  []string
	clipCut    bool

	sbDrag bool
}

type entry struct {
	name   string
	path   string
	isDir  bool
	size   int64
	mtime  time.Time
	hidden bool
}

func New(start string, cfg config.Config, onSave func(config.Config)) *App {
	if start == "" {
		start, _ = os.UserHomeDir()
	}
	a := &App{
		cfg:    cfg,
		onSave: onSave,
		dir:    start,
		mode:   "browse",
		sel:    0,
	}
	if a.cfg.Files.View == "" {
		a.cfg.Files.View = "list"
	}
	a.reload()
	return a
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	ctx.SetTitle("Files — " + a.dir)
	return nil
}

func (a *App) Close() error {
	a.persistLastDir()
	return nil
}

func (a *App) persistLastDir() {
	a.cfg.Files.LastDir = a.dir
	if a.onSave == nil {
		return
	}
	// Copy config so we don't race other Files windows / Settings on shared maps.
	nc := a.cfg
	if a.cfg.Associations != nil {
		nc.Associations = make(map[string]string, len(a.cfg.Associations))
		for k, v := range a.cfg.Associations {
			nc.Associations[k] = v
		}
	}
	a.onSave(nc)
}

func (a *App) reload() {
	a.err = ""
	a.status = ""
	ents, err := os.ReadDir(a.dir)
	if err != nil {
		a.err = err.Error()
		a.ents = nil
		return
	}
	out := make([]entry, 0, len(ents))
	for _, e := range ents {
		name := e.Name()
		hidden := strings.HasPrefix(name, ".")
		if hidden && !a.cfg.Files.ShowHidden {
			continue
		}
		info, _ := e.Info()
		ent := entry{name: name, path: filepath.Join(a.dir, name), isDir: e.IsDir(), hidden: hidden}
		if info != nil {
			ent.size = info.Size()
			ent.mtime = info.ModTime()
		}
		out = append(out, ent)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].isDir != out[j].isDir {
			return out[i].isDir
		}
		switch a.cfg.Files.Sort {
		case "size":
			return out[i].size < out[j].size
		case "mtime":
			return out[i].mtime.After(out[j].mtime)
		default:
			return strings.ToLower(out[i].name) < strings.ToLower(out[j].name)
		}
	})
	a.ents = out
	if a.sel >= len(a.ents) {
		a.sel = len(a.ents) - 1
	}
	if a.sel < 0 {
		a.sel = 0
	}
	a.scroll.EnsureVisible(a.sel)
	if a.ctx != nil {
		a.ctx.SetTitle("Files — " + a.dir)
	}
}

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventKey:
		return a.key(e)
	case uiapp.EventMouse:
		return a.mouse(e)
	case uiapp.EventResize:
		a.scroll.Clamp()
	}
	return nil
}

func (a *App) mouse(e uiapp.Event) error {
	if a.mode != "browse" {
		return nil
	}
	cols, rows := 80, 24
	if a.ctx != nil {
		cols, rows = a.ctx.Size()
	}
	if cols < 8 {
		cols = 8
	}
	if rows < 5 {
		rows = 5
	}
	listTop, listH, barX := a.layout(cols, rows)

	if e.Action == "wheel" {
		if e.Button > 0 {
			a.scroll.ScrollBy(-3)
		} else {
			a.scroll.ScrollBy(3)
		}
		return nil
	}

	style := uiapp.DefaultScrollbarStyle()
	if e.Action == "release" {
		a.sbDrag = false
		return nil
	}
	if e.Action == "drag" && a.sbDrag {
		trackH := listH
		if style.ShowArrows && listH >= 3 {
			trackH = listH - 2
		}
		rel := e.Y - listTop
		if style.ShowArrows {
			rel--
		}
		if trackH > 0 && a.scroll.MaxOffset() > 0 {
			a.scroll.Offset = (rel * a.scroll.MaxOffset()) / trackH
			a.scroll.Clamp()
		}
		return nil
	}
	if e.Action == "drag" || e.Action == "release" {
		return nil
	}

	hit := uiapp.HitScrollbar(e.X, e.Y, barX, listTop, listH, a.scroll, style.ShowArrows)
	if hit != uiapp.ScrollHitNone {
		if hit == uiapp.ScrollHitThumb {
			a.sbDrag = true
		} else {
			a.scroll.ApplyScrollHit(hit)
		}
		return nil
	}

	// Toolbar y=1
	if e.Y == 1 {
		a.toolbarClick(e.X, cols)
		return nil
	}
	if e.Y == 0 {
		a.goUp()
		return nil
	}
	if e.Y < listTop || e.Y >= listTop+listH {
		return nil
	}
	idx := a.indexAt(e.X, e.Y, cols, listTop, listH)
	if idx < 0 || idx >= len(a.ents) {
		return nil
	}
	if idx == a.sel {
		a.openSel()
	} else {
		a.sel = idx
		a.scroll.EnsureVisible(a.sel)
	}
	return nil
}

func (a *App) toolbarClick(x, cols int) {
	// Approximate button regions: Up Home Refr View Set New Wall
	labels := []struct {
		label string
		fn    func()
	}{
		{" Up ", a.goUp},
		{" Home ", a.goHome},
		{" Refr ", a.reload},
		{" View ", a.toggleView},
		{" Set ", func() { a.mode = "settings"; a.sel = 0 }},
		{" New ", func() { a.beginPrompt("mkdir", "") }},
		{" Wall ", a.setAsWallpaper},
		{" Desk ", a.sendToDesktop},
	}
	col := 0
	for _, b := range labels {
		w := uwidth.String(b.label)
		if x >= col && x < col+w {
			b.fn()
			return
		}
		col += w + 1
	}
}

func (a *App) key(e uiapp.Event) error {
	if a.mode == "settings" {
		return a.keySettings(e)
	}
	if a.mode == "prompt" || a.mode == "confirm" {
		return a.keyPrompt(e)
	}

	switch e.Key {
	case "Up":
		if a.sel > 0 {
			a.sel--
			a.scroll.EnsureVisible(a.sel)
		}
	case "Down":
		if a.sel+1 < len(a.ents) {
			a.sel++
			a.scroll.EnsureVisible(a.sel)
		}
	case "Left":
		if a.cfg.Files.View == "grid" {
			a.moveGrid(-1, 0)
		}
	case "Right":
		if a.cfg.Files.View == "grid" {
			a.moveGrid(1, 0)
		}
	case "PgUp":
		a.scroll.ScrollBy(-a.scroll.Viewport)
		a.sel = a.scroll.Offset
	case "PgDn":
		a.scroll.ScrollBy(a.scroll.Viewport)
		a.sel = a.scroll.Offset
		if a.sel >= len(a.ents) && len(a.ents) > 0 {
			a.sel = len(a.ents) - 1
		}
	case "Home":
		a.sel = 0
		a.scroll.EnsureVisible(0)
	case "End":
		if len(a.ents) > 0 {
			a.sel = len(a.ents) - 1
			a.scroll.EnsureVisible(a.sel)
		}
	case "Enter":
		a.openSel()
	case "Backspace":
		a.goUp()
	case "F5":
		a.reload()
	case "F2":
		if a.sel >= 0 && a.sel < len(a.ents) {
			a.beginPrompt("rename", a.ents[a.sel].name)
		}
	case "F7":
		a.beginPrompt("mkdir", "")
	case "Delete":
		a.beginDelete()
	default:
		if e.Ctrl {
			switch e.Rune {
			case 'l', 'L':
				a.beginPrompt("path", a.dir)
			case 'c', 'C':
				a.clipboard(false)
			case 'x', 'X':
				a.clipboard(true)
			case 'v', 'V':
				a.paste()
			}
		} else if e.Rune == 'w' || e.Rune == 'W' {
			a.setAsWallpaper()
		} else if e.Rune == 'd' || e.Rune == 'D' {
			a.sendToDesktop()
		}
	}
	return nil
}

func (a *App) keySettings(e uiapp.Event) error {
	lines := a.settingsLines()
	switch e.Key {
	case "Escape":
		a.mode = "browse"
	case "Up":
		if a.sel > 0 {
			a.sel--
		}
	case "Down":
		if a.sel+1 < len(lines) {
			a.sel++
		}
	case "Enter":
		a.activateSettings()
	}
	return nil
}

func (a *App) keyPrompt(e uiapp.Event) error {
	switch e.Key {
	case "Escape":
		a.mode = "browse"
		a.promptBuf = ""
	case "Enter":
		a.commitPrompt()
	case "Backspace":
		r := []rune(a.promptBuf)
		if len(r) > 0 {
			a.promptBuf = string(r[:len(r)-1])
		}
	default:
		if e.Rune != 0 && !e.Ctrl {
			a.promptBuf += string(e.Rune)
		}
	}
	return nil
}

func (a *App) beginPrompt(kind, initial string) {
	a.mode = "prompt"
	a.promptKind = kind
	a.promptBuf = initial
}

func (a *App) beginDelete() {
	if a.sel < 0 || a.sel >= len(a.ents) {
		return
	}
	if a.cfg.Files.ConfirmDelete {
		a.mode = "confirm"
		a.promptKind = "delete"
		a.promptBuf = a.ents[a.sel].name
		return
	}
	a.deleteSel()
}

func (a *App) commitPrompt() {
	switch a.promptKind {
	case "mkdir":
		name := strings.TrimSpace(a.promptBuf)
		if name != "" {
			if err := os.Mkdir(filepath.Join(a.dir, name), 0o755); err != nil {
				a.status = err.Error()
			} else {
				a.reload()
			}
		}
	case "rename":
		if a.sel >= 0 && a.sel < len(a.ents) {
			newName := strings.TrimSpace(a.promptBuf)
			if newName != "" {
				old := a.ents[a.sel].path
				neu := filepath.Join(a.dir, newName)
				if err := os.Rename(old, neu); err != nil {
					a.status = err.Error()
				} else {
					a.reload()
				}
			}
		}
	case "path":
		p := strings.TrimSpace(a.promptBuf)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			a.dir = p
			a.sel = 0
			a.reload()
			a.persistLastDir()
		} else if err != nil {
			a.status = err.Error()
		} else {
			a.status = "not a directory"
		}
	case "delete":
		a.deleteSel()
	}
	a.mode = "browse"
	a.promptBuf = ""
}

func (a *App) deleteSel() {
	if a.sel < 0 || a.sel >= len(a.ents) {
		return
	}
	src := a.ents[a.sel].path
	if err := moveToTrash(src); err != nil {
		a.status = err.Error()
		if a.ctx != nil {
			a.ctx.Notify("Files", err.Error(), "⚠")
		}
		return
	}
	a.status = "Moved to Trash: " + filepath.Base(src)
	a.reload()
}

func (a *App) clipboard(cut bool) {
	if a.sel < 0 || a.sel >= len(a.ents) {
		return
	}
	a.clipPaths = []string{a.ents[a.sel].path}
	a.clipCut = cut
	if cut {
		a.status = "Cut " + a.ents[a.sel].name
	} else {
		a.status = "Copied " + a.ents[a.sel].name
	}
}

func (a *App) paste() {
	if len(a.clipPaths) == 0 {
		a.status = "Clipboard empty"
		return
	}
	for _, src := range a.clipPaths {
		base := filepath.Base(src)
		dst := filepath.Join(a.dir, base)
		if a.clipCut {
			if err := os.Rename(src, dst); err != nil {
				// cross-device: copy then remove
				if err2 := copyPath(src, dst); err2 != nil {
					a.status = err2.Error()
					continue
				}
				_ = os.RemoveAll(src)
			}
		} else {
			if err := copyPath(src, dst); err != nil {
				a.status = err.Error()
				continue
			}
		}
	}
	if a.clipCut {
		a.clipPaths = nil
		a.clipCut = false
	}
	a.reload()
}

func (a *App) openSel() {
	if a.sel < 0 || a.sel >= len(a.ents) {
		return
	}
	ent := a.ents[a.sel]
	if ent.isDir {
		a.dir = ent.path
		a.sel = 0
		a.scroll.Offset = 0
		a.reload()
		a.persistLastDir()
		return
	}
	if a.ctx != nil {
		_ = a.ctx.OpenPath(ent.path)
	}
}

// setAsWallpaper uses the selected image as the desktop wallpaper (decode → half-block cache).
func (a *App) setAsWallpaper() {
	if a.sel < 0 || a.sel >= len(a.ents) {
		a.status = "Select an image first"
		return
	}
	ent := a.ents[a.sel]
	if ent.isDir {
		a.status = "Select an image file (png/jpg/gif)"
		return
	}
	ext := config.ExtOf(ent.name)
	switch ext {
	case "png", "jpg", "jpeg", "gif":
	default:
		a.status = "Wallpaper needs png/jpg/gif (got ." + ext + ")"
		if a.ctx != nil {
			a.ctx.Notify("Wallpaper", "Supported: PNG, JPEG, GIF", "🖼")
		}
		return
	}
	path := ent.path
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	a.cfg.Wallpaper.Mode = "image"
	a.cfg.Wallpaper.Path = path
	if a.cfg.Wallpaper.Fit == "" {
		a.cfg.Wallpaper.Fit = "cover"
	}
	if a.onSave != nil {
		nc := a.cfg
		if a.cfg.Associations != nil {
			nc.Associations = make(map[string]string, len(a.cfg.Associations))
			for k, v := range a.cfg.Associations {
				nc.Associations[k] = v
			}
		}
		a.onSave(nc)
	}
	a.status = "Wallpaper: " + filepath.Base(path)
	if a.ctx != nil {
		a.ctx.Notify("Wallpaper", "Set: "+filepath.Base(path), "🖼")
	}
}

// sendToDesktop adds a desktop icon shortcut for the selected file or folder.
func (a *App) sendToDesktop() {
	if a.sel < 0 || a.sel >= len(a.ents) {
		a.status = "Select a file or folder first"
		return
	}
	ent := a.ents[a.sel]
	path := ent.path
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	action := "files:" + path
	icon := "📁"
	if !ent.isDir {
		icon = "📄"
		ext := config.ExtOf(ent.name)
		switch ext {
		case "png", "jpg", "jpeg", "gif", "webp", "bmp":
			icon = "🖼"
			action = "image:" + path
		default:
			// Open via associations when launched
			action = "open:" + path
		}
	}
	// Prefer OpenPath-compatible launch: files: for dirs; for files use open: helper
	label := ent.name
	if len([]rune(label)) > 14 {
		label = string([]rune(label)[:13]) + "…"
	}
	x, y := a.nextIconPos()
	ic := config.DesktopIcon{Label: label, Icon: icon, X: x, Y: y, Action: action}
	a.cfg.DesktopIcons = append(a.cfg.DesktopIcons, ic)
	if a.onSave != nil {
		nc := a.cfg
		if a.cfg.Associations != nil {
			nc.Associations = make(map[string]string, len(a.cfg.Associations))
			for k, v := range a.cfg.Associations {
				nc.Associations[k] = v
			}
		}
		if a.cfg.Hotkeys != nil {
			nc.Hotkeys = make(map[string]string, len(a.cfg.Hotkeys))
			for k, v := range a.cfg.Hotkeys {
				nc.Hotkeys[k] = v
			}
		}
		a.onSave(nc)
	}
	a.status = "Desktop shortcut: " + label
	if a.ctx != nil {
		a.ctx.Notify("Desktop", "Added "+label, icon)
	}
}

func (a *App) nextIconPos() (x, y int) {
	x, y = 30, 2
	used := map[string]bool{}
	for _, ic := range a.cfg.DesktopIcons {
		used[fmt.Sprintf("%d,%d", ic.X, ic.Y)] = true
	}
	for row := 0; row < 12; row++ {
		for col := 0; col < 6; col++ {
			nx, ny := 2+col*14, 2+row*4
			if !used[fmt.Sprintf("%d,%d", nx, ny)] {
				return nx, ny
			}
		}
	}
	return x, y
}

func (a *App) goUp() {
	a.dir = filepath.Dir(a.dir)
	a.sel = 0
	a.scroll.Offset = 0
	a.reload()
	a.persistLastDir()
}

func (a *App) goHome() {
	home, _ := os.UserHomeDir()
	if home != "" {
		a.dir = home
		a.sel = 0
		a.scroll.Offset = 0
		a.reload()
		a.persistLastDir()
	}
}

func (a *App) toggleView() {
	if a.cfg.Files.View == "grid" {
		a.cfg.Files.View = "list"
	} else {
		a.cfg.Files.View = "grid"
	}
	a.persistLastDir()
	a.status = "View: " + a.cfg.Files.View
}

func (a *App) moveGrid(dx, dy int) {
	cols := 4
	if a.ctx != nil {
		c, _ := a.ctx.Size()
		cols, _ = a.gridCols(c)
	}
	if cols < 1 {
		cols = 1
	}
	if len(a.ents) == 0 {
		return
	}
	n := a.sel + dy*cols + dx
	if n < 0 {
		n = 0
	}
	if n >= len(a.ents) {
		n = len(a.ents) - 1
	}
	a.sel = n
	a.scroll.EnsureVisible(a.sel / cols)
}

func (a *App) layout(cols, rows int) (listTop, listH, barX int) {
	listTop = 2
	listH = rows - 3 // header, toolbar, status
	if listH < 1 {
		listH = 1
	}
	barX = cols - 1
	return
}

func (a *App) gridCols(cols int) (int, int) {
	cellW := 14
	avail := cols - 1
	n := avail / cellW
	if n < 1 {
		n = 1
	}
	return n, cellW
}

func (a *App) indexAt(x, y, cols, listTop, listH int) int {
	if a.cfg.Files.View == "grid" {
		gc, cellW := a.gridCols(cols)
		row := y - listTop + a.scroll.Offset
		col := x / cellW
		if col >= gc {
			col = gc - 1
		}
		idx := row*gc + col
		return idx
	}
	return y - listTop + a.scroll.Offset
}

func (a *App) settingsLines() []string {
	on := map[bool]string{true: "ON", false: "OFF"}
	return []string{
		fmt.Sprintf("View: %s", a.cfg.Files.View),
		fmt.Sprintf("Show hidden: %s", on[a.cfg.Files.ShowHidden]),
		fmt.Sprintf("Sort: %s", a.cfg.Files.Sort),
		fmt.Sprintf("Start dir: %s", a.cfg.Files.StartDir),
		fmt.Sprintf("Confirm delete: %s", on[a.cfg.Files.ConfirmDelete]),
		"Back to Files",
	}
}

func (a *App) activateSettings() {
	switch a.sel {
	case 0:
		a.toggleView()
	case 1:
		a.cfg.Files.ShowHidden = !a.cfg.Files.ShowHidden
		a.reload()
	case 2:
		switch a.cfg.Files.Sort {
		case "name":
			a.cfg.Files.Sort = "size"
		case "size":
			a.cfg.Files.Sort = "mtime"
		default:
			a.cfg.Files.Sort = "name"
		}
		a.reload()
	case 3:
		switch a.cfg.Files.StartDir {
		case "home":
			a.cfg.Files.StartDir = "last"
		case "last":
			a.cfg.Files.StartDir = "home"
		default:
			a.cfg.Files.StartDir = "home"
		}
	case 4:
		a.cfg.Files.ConfirmDelete = !a.cfg.Files.ConfirmDelete
	case 5:
		a.mode = "browse"
		a.persistLastDir()
		return
	}
	a.persistLastDir()
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	cv.FillRect(0, 0, cols, rows, bg)

	if a.mode == "settings" {
		return a.drawSettings(cv, cols, rows, fg, bg, hdr, hi)
	}

	title := " " + uwidth.ASCIIIcon("📁", a.cfg.Palette.ASCIIIcons) + " " + a.dir
	cv.DrawText(0, 0, uwidth.Truncate(title, cols), cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)
	tb := " Up  Home  Refr  View  Set  New  Wall  Desk "
	cv.DrawText(0, 1, uwidth.Truncate(tb, cols), fg, cell.RGB(0xA0, 0xA0, 0xA0), 0)

	listTop, listH, barX := a.layout(cols, rows)
	if a.err != "" {
		cv.DrawText(1, listTop, a.err, cell.RGB(0xAA, 0x00, 0x00), bg, 0)
	} else if a.cfg.Files.View == "grid" {
		a.drawGrid(cv, cols, listTop, listH, fg, bg, hi)
	} else {
		a.drawList(cv, cols, listTop, listH, fg, bg, hi)
	}

	style := uiapp.DefaultScrollbarStyle()
	cv.DrawScrollbar(barX, listTop, listH, a.scroll, style)

	status := a.status
	if status == "" {
		status = fmt.Sprintf(" %d items  Enter open  W wallpaper  D desktop  Del trash ", len(a.ents))
	}
	if a.mode == "prompt" || a.mode == "confirm" {
		label := a.promptKind + ": "
		if a.promptKind == "delete" {
			label = "Trash " + a.promptBuf + "? Enter/Esc "
			cv.DrawText(0, rows-1, uwidth.Truncate(label, cols), cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
		} else {
			cv.DrawText(0, rows-1, uwidth.Truncate(label+a.promptBuf+"█", cols), cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
		}
	} else {
		cv.DrawText(0, rows-1, uwidth.Truncate(status, cols), cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
	}
	return nil
}

func (a *App) drawList(cv *uiapp.Canvas, cols, listTop, listH int, fg, bg, hi cell.Color) {
	a.scroll.Content = len(a.ents)
	a.scroll.Viewport = listH
	a.scroll.Clamp()
	a.scroll.EnsureVisible(a.sel)
	for row := 0; row < listH; row++ {
		i := a.scroll.Offset + row
		y := listTop + row
		if i >= len(a.ents) {
			break
		}
		ent := a.ents[i]
		icon := "📄"
		if ent.isDir {
			icon = "📁"
		}
		icon = uwidth.ASCIIIcon(icon, a.cfg.Palette.ASCIIIcons)
		label := icon + " " + ent.name
		f, b := fg, bg
		if i == a.sel {
			f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
		}
		cv.FillRect(0, y, cols-1, 1, b)
		cv.DrawText(1, y, uwidth.Truncate(label, cols-3), f, b, 0)
	}
}

func (a *App) drawGrid(cv *uiapp.Canvas, cols, listTop, listH int, fg, bg, hi cell.Color) {
	gc, cellW := a.gridCols(cols)
	rowsNeeded := (len(a.ents) + gc - 1) / gc
	a.scroll.Content = rowsNeeded
	a.scroll.Viewport = listH
	a.scroll.Clamp()
	if gc > 0 {
		a.scroll.EnsureVisible(a.sel / gc)
	}
	for row := 0; row < listH; row++ {
		gr := a.scroll.Offset + row
		for col := 0; col < gc; col++ {
			i := gr*gc + col
			if i >= len(a.ents) {
				break
			}
			ent := a.ents[i]
			x := col * cellW
			y := listTop + row
			icon := "📄"
			if ent.isDir {
				icon = "📁"
			} else {
				switch config.ExtOf(ent.name) {
				case "png", "jpg", "jpeg", "gif", "webp", "bmp":
					icon = "🖼"
				}
			}
			icon = uwidth.ASCIIIcon(icon, a.cfg.Palette.ASCIIIcons)
			f, b := fg, bg
			if i == a.sel {
				f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
			}
			cv.FillRect(x, y, cellW-1, 1, b)
			cv.DrawText(x, y, uwidth.Truncate(icon+" "+ent.name, cellW-1), f, b, 0)
		}
	}
}

func (a *App) drawSettings(cv *uiapp.Canvas, cols, rows int, fg, bg, hdr, hi cell.Color) error {
	cv.DrawText(0, 0, uwidth.Truncate(" Files Settings ", cols), cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)
	for i, line := range a.settingsLines() {
		y := 2 + i
		if y >= rows-1 {
			break
		}
		f, b := fg, bg
		if i == a.sel {
			f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
		}
		cv.DrawText(1, y, line, f, b, 0)
	}
	cv.DrawText(0, rows-1, " Enter toggle  Esc back ", cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
	return nil
}

func moveToTrash(src string) error {
	trash := trashDir()
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return err
	}
	base := filepath.Base(src)
	dst := filepath.Join(trash, base)
	if _, err := os.Stat(dst); err == nil {
		dst = filepath.Join(trash, fmt.Sprintf("%s.%d", base, time.Now().Unix()))
	}
	if err := os.Rename(src, dst); err != nil {
		if err2 := copyPath(src, dst); err2 != nil {
			return err2
		}
		return os.RemoveAll(src)
	}
	return nil
}

func trashDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "Trash")
	}
	xdg := filepath.Join(home, ".local", "share", "Trash", "files")
	if err := os.MkdirAll(xdg, 0o755); err == nil {
		return xdg
	}
	return filepath.Join(home, ".config", "ttypedesk", "Trash")
}

func copyPath(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range ents {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if err := copyPath(s, d); err != nil {
			return err
		}
	}
	return nil
}
