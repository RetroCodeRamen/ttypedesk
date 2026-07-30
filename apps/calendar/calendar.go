package calendar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
)

// Event is a local calendar event.
type Event struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	AllDay bool      `json:"all_day"`
	Notes  string    `json:"notes,omitempty"`
	Source string    `json:"source"`
}

// App is a month-view calendar with local events.
type App struct {
	view    time.Time // first of displayed month
	sel     time.Time // selected day
	events  []Event
	mode    int // 0 month, 1 agenda/edit title
	editBuf string
	status  string
	cursor  int // agenda selection
	dirty   bool

	cfg config.Config
	ctx *uiapp.Context

	syncing      bool
	pendingSyncs int
	syncResults  chan syncResult
}

// New builds a Calendar bound to cfg — specifically cfg.Calendar.Accounts,
// to know what (if anything) to sync. Account setup itself (connect flow,
// enable/disable, lead time, timezone) lives in Settings → Calendar, not
// here; Calendar only ever reads config, it never writes it.
func New(cfg config.Config) *App {
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return &App{
		view: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()),
		sel:  day,
		cfg:  cfg,
	}
}

func storePath() string { return StorePath() }

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	a.load()
	a.syncAll()
	return nil
}

func (a *App) Close() error {
	if a.dirty {
		_ = a.save()
	}
	return nil
}

func (a *App) load() {
	data, err := os.ReadFile(storePath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &a.events)
}

func (a *App) save() error {
	path := storePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a.events, "", "  ")
	if err != nil {
		return err
	}
	a.dirty = false
	return os.WriteFile(path, data, 0o644)
}

func (a *App) Handle(e uiapp.Event) error {
	switch e.Kind {
	case uiapp.EventKey:
		return a.key(e)
	case uiapp.EventMouse:
		return a.mouse(e)
	}
	return nil
}

func (a *App) mouse(e uiapp.Event) error {
	if e.Action == "drag" || e.Action == "release" {
		return nil
	}
	if a.mode == 1 {
		return nil
	}
	// Month nav on title: click left half = prev, right = next
	if e.Y == 0 {
		cols := 40
		if e.X < cols/3 {
			a.view = a.view.AddDate(0, -1, 0)
			a.sel = clampDay(a.view, a.sel.Day())
		} else if e.X > 2*cols/3 {
			a.view = a.view.AddDate(0, 1, 0)
			a.sel = clampDay(a.view, a.sel.Day())
		}
		return nil
	}
	cellW := 4
	gridX := 2
	gridY := 2
	if e.Y <= gridY {
		return nil
	}
	row := e.Y - gridY - 1
	col := (e.X - gridX) / cellW
	if row < 0 || col < 0 || col > 6 {
		return nil
	}
	first := a.view
	startPad := int(first.Weekday())
	daysInMonth := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, first.Location()).Day()
	d := row*7 + col - startPad + 1
	if d < 1 || d > daysInMonth {
		return nil
	}
	day := time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, first.Location())
	if sameDay(day, a.sel) {
		// second click on selected day → add event
		a.mode = 1
		a.editBuf = ""
		a.status = "New event title — Enter to save"
	} else {
		a.sel = day
	}
	return nil
}

func (a *App) key(e uiapp.Event) error {
	if a.mode == 1 {
		switch e.Key {
		case "Enter":
			title := strings.TrimSpace(a.editBuf)
			if title != "" {
				start := a.sel
				a.events = append(a.events, Event{
					ID:     fmt.Sprintf("e%d", time.Now().UnixNano()),
					Title:  title,
					Start:  start,
					End:    start.Add(time.Hour),
					AllDay: true,
					Source: "local",
				})
				a.dirty = true
				_ = a.save()
				a.status = "Event added"
			}
			a.mode = 0
			a.editBuf = ""
		case "Escape":
			a.mode = 0
			a.editBuf = ""
		case "Backspace":
			r := []rune(a.editBuf)
			if len(r) > 0 {
				a.editBuf = string(r[:len(r)-1])
			}
		default:
			if e.Rune != 0 && !e.Ctrl {
				a.editBuf += string(e.Rune)
			}
		}
		return nil
	}

	switch e.Key {
	case "Left":
		a.sel = a.sel.AddDate(0, 0, -1)
		a.syncView()
	case "Right":
		a.sel = a.sel.AddDate(0, 0, 1)
		a.syncView()
	case "Up":
		a.sel = a.sel.AddDate(0, 0, -7)
		a.syncView()
	case "Down":
		a.sel = a.sel.AddDate(0, 0, 7)
		a.syncView()
	case "PgUp":
		a.view = a.view.AddDate(0, -1, 0)
		a.sel = clampDay(a.view, a.sel.Day())
	case "PgDn":
		a.view = a.view.AddDate(0, 1, 0)
		a.sel = clampDay(a.view, a.sel.Day())
	case "Enter":
		a.mode = 1
		a.editBuf = ""
		a.status = "New event title — Enter to save"
	case "Delete", "Backspace":
		a.deleteSelected()
	}
	if e.Rune == 'n' || e.Rune == 'N' {
		a.mode = 1
		a.editBuf = ""
		a.status = "New event title — Enter to save"
	}
	if e.Rune == 's' || e.Rune == 'S' {
		a.syncAll()
	}
	if e.Rune == '[' {
		a.view = a.view.AddDate(0, -1, 0)
		a.sel = clampDay(a.view, a.sel.Day())
	}
	if e.Rune == ']' {
		a.view = a.view.AddDate(0, 1, 0)
		a.sel = clampDay(a.view, a.sel.Day())
	}
	return nil
}

func (a *App) syncView() {
	a.view = time.Date(a.sel.Year(), a.sel.Month(), 1, 0, 0, 0, 0, a.sel.Location())
}

func clampDay(month time.Time, day int) time.Time {
	last := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, month.Location()).Day()
	if day > last {
		day = last
	}
	if day < 1 {
		day = 1
	}
	return time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, month.Location())
}

func (a *App) deleteSelected() {
	dayEvents := a.eventsOn(a.sel)
	if len(dayEvents) == 0 {
		return
	}
	id := dayEvents[0].ID
	dst := a.events[:0]
	for _, ev := range a.events {
		if ev.ID != id {
			dst = append(dst, ev)
		}
	}
	a.events = dst
	a.dirty = true
	_ = a.save()
	a.status = "Deleted event"
}

func (a *App) eventsOn(day time.Time) []Event {
	var out []Event
	for _, ev := range a.events {
		if sameDay(ev.Start, day) {
			out = append(out, ev)
		}
	}
	return out
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	a.drainSyncResults()
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	mark := cell.RGB(0x00, 0x80, 0x00)
	cv.FillRect(0, 0, cols, rows, bg)

	title := fmt.Sprintf(" %s ", a.view.Format("January 2006"))
	cv.DrawText(0, 0, title, cell.RGB(0xFF, 0xFF, 0xFF), hdr, cell.AttrBold)
	hint := "[ ] months  arrows day  Enter=add  Del=remove"
	if a.hasEnabledAccount() {
		hint += "  S=sync"
	}
	cv.DrawText(len(title)+1, 0, hint, fg, bg, 0)

	// weekday headers
	days := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	cellW := 4
	gridX := 2
	gridY := 2
	for i, d := range days {
		cv.DrawText(gridX+i*cellW, gridY, d, hdr, bg, cell.AttrBold)
	}

	first := a.view
	startPad := int(first.Weekday()) // Sunday=0
	daysInMonth := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, first.Location()).Day()
	today := time.Now()

	for d := 1; d <= daysInMonth; d++ {
		idx := startPad + d - 1
		row := idx / 7
		col := idx % 7
		x := gridX + col*cellW
		y := gridY + 1 + row
		day := time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, first.Location())
		label := fmt.Sprintf("%2d", d)
		f, b := fg, bg
		if sameDay(day, a.sel) {
			f, b = cell.RGB(0xFF, 0xFF, 0xFF), hi
		} else if sameDay(day, today) {
			f = hdr
		}
		if len(a.eventsOn(day)) > 0 && !sameDay(day, a.sel) {
			f = mark
		}
		cv.DrawText(x, y, label, f, b, 0)
	}

	agendaY := gridY + 8
	if agendaY < rows-3 {
		cv.DrawText(2, agendaY, a.sel.Format("Mon Jan 2, 2006"), hdr, bg, cell.AttrBold)
		evs := a.eventsOn(a.sel)
		if len(evs) == 0 {
			cv.DrawText(2, agendaY+1, "(no events)", fg, bg, 0)
		} else {
			for i, ev := range evs {
				if agendaY+1+i >= rows-2 {
					break
				}
				cv.DrawText(2, agendaY+1+i, "• "+ev.Title, fg, bg, 0)
			}
		}
	}

	if a.mode == 1 {
		cv.DrawText(2, rows-2, "Title: "+a.editBuf+"█", cell.RGB(0xFF, 0xFF, 0xFF), hi, 0)
	}

	help := "Local calendar — events in ~/.config/ttypedesk/calendar/"
	if a.syncing {
		help = "Syncing…"
	}
	if a.status != "" {
		help = a.status
	}
	if rows > 0 {
		cv.DrawText(0, rows-1, " "+help, cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0x00, 0x00, 0x00), 0)
	}
	return nil
}
