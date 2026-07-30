// Package matrixchat is a text-only Matrix client — login, room list,
// timeline, send text. Connects to an existing homeserver (matrix.org or
// any other); TTYPE Desk never runs or manages a chat backend of its own.
// E2EE is explicitly out of scope for this first pass (a real, separate
// undertaking — device verification, key backup, cross-signing — not an
// incremental add); Phase 2 in ROADMAP.md terms.
//
// This is no longer a built-in app — install via the App Store (see
// docs/appstore.md) or build cmd/matrixchat directly. It runs
// out-of-process, via pkg/extapprun, unchanged from its original
// in-process form (its uiapp.App/Context/Host usage never depended on
// AppSurface specifics) — see docs/extapp.md.
package matrixchat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

const (
	modeLogin = iota
	modeConnecting
	modeChat
)

const maxTimeline = 200 // cap per-room memory, oldest messages just fall off

type roomInfo struct {
	id       id.RoomID
	name     string
	timeline []timelineMsg
}

func (r *roomInfo) label() string {
	if r.name != "" {
		return r.name
	}
	return string(r.id)
}

type timelineMsg struct {
	sender string
	body   string
	ts     time.Time
}

// App is the Chat window.
type App struct {
	ctx *uiapp.Context

	mode int

	// Login screen.
	field                          int // 0=homeserver 1=username 2=password
	homeserver, username, password string

	// Connection.
	client  *mautrix.Client
	cancel  context.CancelFunc
	updates chan update

	rooms   []*roomInfo
	roomIdx map[id.RoomID]int
	selRoom int

	compose string
	status  string
}

func New() *App {
	return &App{roomIdx: make(map[id.RoomID]int), selRoom: -1}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	ctx.StartTimer(500 * time.Millisecond) // drains async updates even with no keypresses

	if sess, err := loadSession(ctx); err == nil {
		a.startConnect("", "", "", sess)
	}
	return nil
}

func (a *App) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

func (a *App) startConnect(homeserver, username, password string, sess *session) {
	a.mode = modeConnecting
	a.status = "Connecting…"
	cctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.updates = make(chan update, 32)
	go connect(cctx, homeserver, username, password, sess, a.updates)
}

func (a *App) Handle(e uiapp.Event) error {
	if e.Kind == uiapp.EventKey {
		return a.key(e)
	}
	return nil
}

func (a *App) key(e uiapp.Event) error {
	switch a.mode {
	case modeLogin:
		return a.loginKey(e)
	case modeChat:
		return a.chatKey(e)
	}
	return nil
}

func (a *App) loginKey(e uiapp.Event) error {
	buf := func() *string {
		switch a.field {
		case 1:
			return &a.username
		case 2:
			return &a.password
		default:
			return &a.homeserver
		}
	}()
	switch e.Key {
	case "Tab", "Down":
		a.field = (a.field + 1) % 3
	case "Up":
		a.field = (a.field + 2) % 3
	case "Enter":
		if a.field < 2 {
			a.field++
		} else {
			a.submitLogin()
		}
	case "Backspace":
		if r := []rune(*buf); len(r) > 0 {
			*buf = string(r[:len(r)-1])
		}
	default:
		if e.Rune != 0 && !e.Ctrl {
			*buf += string(e.Rune)
		}
	}
	return nil
}

func (a *App) submitLogin() {
	homeserver := strings.TrimSpace(a.homeserver)
	username := strings.TrimSpace(a.username)
	if homeserver == "" || username == "" || a.password == "" {
		a.status = "Fill in homeserver, username, and password"
		return
	}
	if !strings.Contains(homeserver, "://") {
		homeserver = "https://" + homeserver
	}
	a.startConnect(homeserver, username, a.password, nil)
	a.password = "" // never held in memory longer than needed to log in
}

func (a *App) chatKey(e uiapp.Event) error {
	switch e.Key {
	case "Up":
		if a.selRoom > 0 {
			a.selRoom--
		}
		return nil
	case "Down":
		if a.selRoom < len(a.rooms)-1 {
			a.selRoom++
		}
		return nil
	case "Enter":
		a.sendCompose()
		return nil
	case "Backspace":
		if r := []rune(a.compose); len(r) > 0 {
			a.compose = string(r[:len(r)-1])
		}
		return nil
	}
	if e.Rune != 0 && !e.Ctrl {
		a.compose += string(e.Rune)
	}
	return nil
}

func (a *App) sendCompose() {
	body := strings.TrimSpace(a.compose)
	if body == "" || a.selRoom < 0 || a.selRoom >= len(a.rooms) || a.client == nil {
		return
	}
	roomID := a.rooms[a.selRoom].id
	a.compose = ""
	go sendText(context.Background(), a.client, roomID, body, a.updates)
}

func (a *App) ensureRoom(rid id.RoomID) *roomInfo {
	if i, ok := a.roomIdx[rid]; ok {
		return a.rooms[i]
	}
	r := &roomInfo{id: rid}
	a.roomIdx[rid] = len(a.rooms)
	a.rooms = append(a.rooms, r)
	if a.selRoom < 0 {
		a.selRoom = 0
	}
	return r
}

func (a *App) room(rid id.RoomID) *roomInfo {
	if i, ok := a.roomIdx[rid]; ok {
		return a.rooms[i]
	}
	return nil
}

// drainUpdates applies any pending connect/sync/send results — called
// only from Draw, i.e. only ever on the single thread that owns every
// other App field (same contract as apps/calendar/sync.go, apps/amp,
// apps/vid).
func (a *App) drainUpdates() {
	if a.updates == nil {
		return
	}
	for {
		select {
		case u := <-a.updates:
			a.apply(u)
		default:
			return
		}
	}
}

func (a *App) apply(u update) {
	switch u.kind {
	case "connected":
		a.client = u.client
		a.mode = modeChat
		a.status = "Connected as " + u.sess.UserID
		if err := saveSession(a.ctx, u.sess); err != nil {
			a.status = "Connected, but failed to save session: " + err.Error()
		}
	case "joined":
		a.ensureRoom(u.roomID)
	case "roomname":
		if r := a.room(u.roomID); r != nil {
			r.name = u.name
		}
	case "message":
		idx, existed := a.roomIdx[u.roomID]
		r := a.ensureRoom(u.roomID)
		r.timeline = append(r.timeline, timelineMsg{sender: u.sender, body: u.body, ts: u.ts})
		if len(r.timeline) > maxTimeline {
			r.timeline = r.timeline[len(r.timeline)-maxTimeline:]
		}
		// Notify for activity in any room that isn't the one currently in
		// view — not real mention-parsing (no m.mentions handling yet),
		// but a message arriving in a backgrounded room is the common case
		// that actually needs a nudge, and this needs no new plumbing
		// beyond the notify service every other app already posts through.
		isSelf := a.client != nil && u.sender == a.client.UserID.String()
		notSelected := !(existed && idx == a.selRoom)
		if !isSelf && notSelected && a.ctx != nil {
			a.ctx.Notify(r.label(), shortSender(u.sender)+": "+u.body, "💬")
		}
	case "senderr":
		a.status = u.err.Error()
	case "syncerr":
		a.status = u.err.Error()
		a.mode = modeLogin
	case "disconnected":
		a.status = "Disconnected"
	}
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	a.drainUpdates()
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	white := cell.RGB(0xFF, 0xFF, 0xFF)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	cv.FillRect(0, 0, cols, rows, bg)
	cv.DrawText(0, 0, " Chat (Matrix) ", white, hdr, cell.AttrBold)

	switch a.mode {
	case modeLogin:
		a.drawLogin(cv, cols, rows, fg, bg, hi, white)
	case modeConnecting:
		cv.DrawText(2, 2, a.status, fg, bg, 0)
	case modeChat:
		a.drawChat(cv, cols, rows, fg, bg, hdr, white, hi)
	}

	if rows > 0 {
		help := a.status
		if help == "" {
			help = "Chat — Up/Down room, Enter send, text types into the compose box"
		}
		cv.DrawText(0, rows-1, " "+help, cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0x00, 0x00, 0x00), 0)
	}
	return nil
}

func (a *App) drawLogin(cv *uiapp.Canvas, cols, rows int, fg, bg, hi, white cell.Color) {
	labels := []string{"Homeserver:", "Username:", "Password:"}
	values := []string{a.homeserver, a.username, strings.Repeat("*", len([]rune(a.password)))}
	y := 3
	for i, label := range labels {
		f, b := fg, bg
		if i == a.field {
			f, b = white, hi
		}
		cv.DrawText(2, y, label, fg, bg, 0)
		val := values[i]
		if i == a.field {
			val += "█"
		}
		cv.FillRect(16, y, cols-18, 1, b)
		cv.DrawText(16, y, val, f, b, 0)
		y += 2
	}
	cv.DrawText(2, y+1, "Tab/arrows move, Enter next field (Enter on Password logs in)", fg, bg, 0)
}

func (a *App) drawChat(cv *uiapp.Canvas, cols, rows int, fg, bg, hdr, white, hi cell.Color) {
	listW := cols / 4
	if listW < 12 {
		listW = 12
	}
	if listW > cols-20 {
		listW = cols - 20
	}
	if listW < 1 {
		listW = 1
	}

	for i, r := range a.rooms {
		y := 1 + i
		if y >= rows-2 {
			break
		}
		f, b := fg, bg
		if i == a.selRoom {
			f, b = white, hi
		}
		cv.FillRect(0, y, listW, 1, b)
		cv.DrawText(0, y, truncateRunes(r.label(), listW), f, b, 0)
	}
	for y := 1; y < rows-2; y++ {
		cv.DrawText(listW, y, "│", fg, bg, 0)
	}

	tx := listW + 1
	tw := cols - tx
	if a.selRoom >= 0 && a.selRoom < len(a.rooms) && tw > 0 {
		r := a.rooms[a.selRoom]
		bodyRows := rows - 3
		start := 0
		if len(r.timeline) > bodyRows {
			start = len(r.timeline) - bodyRows
		}
		for i, m := range r.timeline[start:] {
			y := 1 + i
			if y >= rows-2 {
				break
			}
			line := fmt.Sprintf("%s: %s", shortSender(m.sender), m.body)
			cv.DrawText(tx, y, truncateRunes(line, tw), fg, bg, 0)
		}
	} else if tw > 0 {
		cv.DrawText(tx, 1, "(no room selected)", fg, bg, 0)
	}

	composeY := rows - 2
	if composeY > 0 {
		cv.FillRect(0, composeY, cols, 1, hdr)
		cv.DrawText(0, composeY, "> "+a.compose+"█", white, hdr, 0)
	}
}

func shortSender(mxid string) string {
	mxid = strings.TrimPrefix(mxid, "@")
	if i := strings.Index(mxid, ":"); i > 0 {
		return mxid[:i]
	}
	return mxid
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
