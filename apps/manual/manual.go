package manual

import (
	"strings"
	"unicode"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
	"github.com/ttypedesk/ttypedesk/pkg/uwidth"
)

// App is the in-desk Manual reader (TOC + chapter body).
type App struct {
	ctx      *uiapp.Context
	chapters []Chapter
	idx      int
	scroll   uiapp.ScrollState
	lines    []string // wrapped body for current chapter + width
	wrapCols int
	sbDrag   bool
	focusTOC bool // when true, arrows move TOC; else scroll body
}

// New loads embedded chapters. Call EnsureSystemFolder separately for the on-disk copy.
func New() *App {
	ch, err := LoadChapters()
	if err != nil || len(ch) == 0 {
		ch = []Chapter{{
			ID:    "error",
			Title: "Manual unavailable",
			Body:  "Could not load Manual chapters from the TTYPE Desk binary.",
		}}
	}
	return &App{chapters: ch, idx: 0}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	if ctx != nil {
		ctx.SetTitle("Manual")
	}
	return nil
}

func (a *App) Close() error { return nil }

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventResize:
		a.wrapCols = 0
		a.rebuildLines()
	case uiapp.EventKey:
		return a.key(e)
	case uiapp.EventMouse:
		return a.mouse(e)
	}
	return nil
}

func (a *App) key(e uiapp.Event) error {
	switch e.Key {
	case "Tab":
		a.focusTOC = !a.focusTOC
	case "Left":
		a.prevChapter()
	case "Right":
		a.nextChapter()
	case "Up":
		if a.focusTOC {
			a.prevChapter()
		} else {
			a.scroll.ScrollBy(-1)
		}
	case "Down":
		if a.focusTOC {
			a.nextChapter()
		} else {
			a.scroll.ScrollBy(1)
		}
	case "PgUp":
		a.scroll.ScrollBy(-a.scroll.Viewport)
	case "PgDn":
		a.scroll.ScrollBy(a.scroll.Viewport)
	case "Home":
		if a.focusTOC {
			a.setChapter(0)
		} else {
			a.scroll.Offset = 0
		}
	case "End":
		if a.focusTOC {
			a.setChapter(len(a.chapters) - 1)
		} else {
			a.scroll.Offset = a.scroll.MaxOffset()
		}
	default:
		if e.Rune >= '1' && e.Rune <= '9' {
			n := int(e.Rune - '1')
			if n < len(a.chapters) {
				a.setChapter(n)
			}
		}
	}
	return nil
}

func (a *App) mouse(e uiapp.Event) error {
	cols, rows := a.size()
	tocW, bodyX, bodyW, listTop, listH, barX := a.layout(cols, rows)

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
	if e.Action != "press" {
		return nil
	}

	// TOC click
	if e.X < tocW && e.Y >= listTop && e.Y < listTop+listH {
		i := e.Y - listTop
		if i >= 0 && i < len(a.chapters) {
			a.setChapter(i)
			a.focusTOC = true
		}
		return nil
	}

	hit := uiapp.HitScrollbar(e.X, e.Y, barX, listTop, listH, a.scroll, style.ShowArrows)
	if hit != uiapp.ScrollHitNone {
		if hit == uiapp.ScrollHitThumb {
			a.sbDrag = true
			return nil
		}
		a.scroll.ApplyScrollHit(hit)
		return nil
	}

	if e.X >= bodyX && e.X < bodyX+bodyW {
		a.focusTOC = false
	}
	_ = bodyW
	return nil
}

func (a *App) prevChapter() {
	if a.idx > 0 {
		a.setChapter(a.idx - 1)
	}
}

func (a *App) nextChapter() {
	if a.idx+1 < len(a.chapters) {
		a.setChapter(a.idx + 1)
	}
}

func (a *App) setChapter(i int) {
	if i < 0 || i >= len(a.chapters) {
		return
	}
	a.idx = i
	a.wrapCols = 0
	a.scroll.Offset = 0
	a.rebuildLines()
	if a.ctx != nil {
		a.ctx.SetTitle("Manual — " + a.chapters[i].Title)
	}
}

func (a *App) size() (cols, rows int) {
	if a.ctx != nil {
		return a.ctx.Size()
	}
	return 80, 24
}

func (a *App) layout(cols, rows int) (tocW, bodyX, bodyW, listTop, listH, barX int) {
	listTop = 2
	listH = rows - 3
	if listH < 1 {
		listH = 1
	}
	tocW = 22
	if cols < 48 {
		tocW = 0
	} else if tocW > cols/3 {
		tocW = cols / 3
	}
	if tocW > 0 {
		bodyX = tocW + 1
	} else {
		bodyX = 0
	}
	barX = cols - 1
	bodyW = barX - bodyX
	if bodyW < 8 {
		bodyW = 8
	}
	return
}

func (a *App) rebuildLines() {
	cols, rows := a.size()
	_, bodyX, bodyW, _, listH, _ := a.layout(cols, rows)
	_ = bodyX
	wrap := bodyW
	if wrap < 8 {
		wrap = 8
	}
	if wrap == a.wrapCols && a.lines != nil {
		a.scroll.Content = len(a.lines)
		a.scroll.Viewport = listH
		a.scroll.Clamp()
		return
	}
	a.wrapCols = wrap
	ch := a.chapters[a.idx]
	a.lines = wrapManual(ch.Body, wrap)
	a.scroll.Content = len(a.lines)
	a.scroll.Viewport = listH
	a.scroll.Clamp()
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	a.rebuildLines()

	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0xAA)
	tocBG := cell.RGB(0xA8, 0xA8, 0xA8)
	selBG := cell.RGB(0x00, 0x00, 0xAA)
	selFG := cell.RGB(0xFF, 0xFF, 0xFF)
	dim := cell.RGB(0x40, 0x40, 0x40)

	cv.FillRect(0, 0, cols, rows, bg)
	title := " Manual "
	if a.idx >= 0 && a.idx < len(a.chapters) {
		title = " Manual — " + a.chapters[a.idx].Title + " "
	}
	cv.DrawText(0, 0, uwidth.Truncate(title, cols), selFG, hdr, cell.AttrBold)

	tocW, bodyX, bodyW, listTop, listH, barX := a.layout(cols, rows)

	if tocW > 0 {
		cv.FillRect(0, listTop, tocW, listH, tocBG)
		for i, ch := range a.chapters {
			if i >= listH {
				break
			}
			label := " " + ch.Title
			tfg, tbg := fg, tocBG
			if i == a.idx {
				tfg, tbg = selFG, selBG
			}
			cv.DrawText(0, listTop+i, uwidth.Truncate(label, tocW), tfg, tbg, 0)
		}
		for y := listTop; y < listTop+listH; y++ {
			cv.DrawText(tocW, y, "│", dim, bg, 0)
		}
	}

	for row := 0; row < listH; row++ {
		li := a.scroll.Offset + row
		line := ""
		if li >= 0 && li < len(a.lines) {
			line = a.lines[li]
		}
		cv.DrawText(bodyX, listTop+row, uwidth.Truncate(line, bodyW), fg, bg, 0)
	}

	cv.DrawScrollbar(barX, listTop, listH, a.scroll, uiapp.DefaultScrollbarStyle())

	help := " ←→ chapter  ↑↓/Pg scroll  Tab TOC  1-9 jump  wheel ok "
	if tocW == 0 {
		help = " ←→ chapter  ↑↓/Pg scroll  wheel  (widen for TOC) "
	}
	cv.DrawText(0, rows-1, uwidth.Truncate(help, cols), cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0, 0, 0), 0)
	return nil
}

func wrapManual(body string, width int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		raw := strings.TrimRightFunc(para, unicode.IsSpace)
		if raw == "" {
			out = append(out, "")
			continue
		}
		// Simple markdown cues → plain emphasis for the TUI
		display := strings.TrimPrefix(raw, "# ")
		display = strings.TrimPrefix(display, "## ")
		display = strings.TrimPrefix(display, "### ")
		if strings.HasPrefix(raw, "# ") {
			out = append(out, "")
			out = append(out, wrapWords(strings.ToUpper(display), width)...)
			out = append(out, strings.Repeat("─", min(width, 40)))
			continue
		}
		if strings.HasPrefix(raw, "## ") || strings.HasPrefix(raw, "### ") {
			out = append(out, "")
			out = append(out, wrapWords(display, width)...)
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(raw), "|") {
			// Keep table-ish lines mostly intact (truncate)
			out = append(out, uwidth.Truncate(raw, width))
			continue
		}
		if strings.HasPrefix(raw, "```") {
			continue
		}
		out = append(out, wrapWords(display, width)...)
	}
	return out
}

func wrapWords(s string, width int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{""}
	}
	words := strings.Fields(s)
	var lines []string
	var cur string
	for _, w := range words {
		cand := w
		if cur != "" {
			cand = cur + " " + w
		}
		if uwidth.String(cand) <= width {
			cur = cand
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		if uwidth.String(w) <= width {
			cur = w
		} else {
			lines = append(lines, uwidth.Truncate(w, width))
			cur = ""
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
