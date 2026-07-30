// Package chat is the built-in Chat app — a UI over internal/lanchat's
// decentralized LAN messenger. No login, no server: on first run it asks
// for a display name, then shows the rooms you're in, the peers
// currently visible on the LAN, and a timeline + compose box for
// whichever room is selected. The actual discovery/gossip/persistence
// logic all lives in internal/lanchat.Service, which is instantiated
// once for the whole process (see internal/server) and keeps running
// even when no Chat window is open — this App is just a view onto it.
//
// The Matrix client that used to live in this slot is now
// apps/matrixchat, installable via the App Store.
package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/lanchat"
	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
	"github.com/RetroCodeRamen/ttypedesk/pkg/uiapp"
)

const (
	modeName = iota
	modeChat
	modeNewRoom
)

const (
	panelRooms = iota
	panelPeers
	panelCompose
)

// App is the Chat window.
type App struct {
	ctx *uiapp.Context
	svc *lanchat.Service

	mode int

	nameInput     string
	roomNameInput string

	panel        int
	selRoomIdx   int // index into the rendered room list, 0 = "+ New Room"
	selPeerIdx   int
	selectedRoom lanchat.RoomID

	compose string
	status  string
}

// New returns a Chat app backed by svc — svc is shared process-wide
// (constructed once in internal/server.New, per the same pattern as
// notify.Service), so its rooms/peers keep updating even across this
// window being closed and reopened.
func New(svc *lanchat.Service) *App {
	return &App{svc: svc}
}

func (a *App) Init(ctx *uiapp.Context) error {
	a.ctx = ctx
	ctx.StartTimer(1 * time.Second) // fallback redraw; MarkDirty below covers the common case promptly

	if _, name := a.svc.Self(); name == "" {
		a.mode = modeName
	} else {
		a.mode = modeChat
		a.selectFirstRoom()
	}

	a.svc.Subscribe(func(lanchat.Event) {
		ctx.MarkDirty()
	})
	return nil
}

func (a *App) Close() error {
	return nil
}

func (a *App) selectFirstRoom() {
	rooms := a.svc.Rooms()
	if len(rooms) > 0 {
		a.selectedRoom = rooms[0].ID
		a.selRoomIdx = 1 // 0 is the "+ New Room" entry
	}
}

func (a *App) Handle(e uiapp.Event) error {
	if e.Kind == uiapp.EventKey {
		return a.key(e)
	}
	return nil
}

func (a *App) key(e uiapp.Event) error {
	switch a.mode {
	case modeName:
		return a.nameKey(e)
	case modeNewRoom:
		return a.newRoomKey(e)
	case modeChat:
		return a.chatKey(e)
	}
	return nil
}

func (a *App) nameKey(e uiapp.Event) error {
	switch e.Key {
	case "Enter":
		name := strings.TrimSpace(a.nameInput)
		if name == "" {
			a.status = "Enter a display name"
			return nil
		}
		if err := a.svc.SetDisplayName(name); err != nil {
			a.status = "Couldn't save name: " + err.Error()
			return nil
		}
		a.mode = modeChat
		a.status = ""
		a.selectFirstRoom()
	case "Backspace":
		a.nameInput = trimLastRune(a.nameInput)
	default:
		if e.Rune != 0 && !e.Ctrl {
			a.nameInput += string(e.Rune)
		}
	}
	return nil
}

func (a *App) newRoomKey(e uiapp.Event) error {
	switch e.Key {
	case "Enter":
		name := strings.TrimSpace(a.roomNameInput)
		if name == "" {
			a.mode = modeChat
			return nil
		}
		id := a.svc.CreateRoom(name)
		a.roomNameInput = ""
		a.mode = modeChat
		a.selectedRoom = id
		a.panel = panelCompose
	case "Escape":
		a.roomNameInput = ""
		a.mode = modeChat
	case "Backspace":
		a.roomNameInput = trimLastRune(a.roomNameInput)
	default:
		if e.Rune != 0 && !e.Ctrl {
			a.roomNameInput += string(e.Rune)
		}
	}
	return nil
}

func (a *App) chatKey(e uiapp.Event) error {
	if e.Key == "Tab" {
		a.panel = (a.panel + 1) % 3
		return nil
	}
	switch a.panel {
	case panelRooms:
		return a.roomsKey(e)
	case panelPeers:
		return a.peersKey(e)
	case panelCompose:
		return a.composeKey(e)
	}
	return nil
}

func (a *App) roomsKey(e uiapp.Event) error {
	rooms := a.svc.Rooms()
	switch e.Key {
	case "Up":
		if a.selRoomIdx > 0 {
			a.selRoomIdx--
		}
	case "Down":
		if a.selRoomIdx < len(rooms) {
			a.selRoomIdx++
		}
	case "Enter":
		if a.selRoomIdx == 0 {
			a.mode = modeNewRoom
			return nil
		}
		i := a.selRoomIdx - 1
		if i >= 0 && i < len(rooms) {
			r := rooms[i]
			a.selectedRoom = r.ID
			if !r.Joined {
				a.svc.JoinRoom(r.ID)
			}
			a.panel = panelCompose
		}
	}
	return nil
}

func (a *App) peersKey(e uiapp.Event) error {
	peers := a.svc.Peers()
	switch e.Key {
	case "Up":
		if a.selPeerIdx > 0 {
			a.selPeerIdx--
		}
	case "Down":
		if a.selPeerIdx < len(peers)-1 {
			a.selPeerIdx++
		}
	case "Enter":
		if a.selPeerIdx >= 0 && a.selPeerIdx < len(peers) {
			a.selectedRoom = a.svc.DMRoom(peers[a.selPeerIdx].ID)
			a.panel = panelCompose
		}
	}
	return nil
}

func (a *App) composeKey(e uiapp.Event) error {
	switch e.Key {
	case "Enter":
		a.sendCompose()
	case "Backspace":
		a.compose = trimLastRune(a.compose)
	default:
		if e.Rune != 0 && !e.Ctrl {
			a.compose += string(e.Rune)
		}
	}
	return nil
}

func (a *App) sendCompose() {
	body := strings.TrimSpace(a.compose)
	if body == "" || a.selectedRoom == "" {
		return
	}
	if err := a.svc.SendMessage(a.selectedRoom, body); err != nil {
		a.status = err.Error()
		return
	}
	a.compose = ""
	a.status = ""
}

func trimLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

func (a *App) Draw(cv *uiapp.Canvas) error {
	cols, rows := cv.Bounds()
	bg := cell.RGB(0xC0, 0xC0, 0xC0)
	fg := cell.RGB(0x00, 0x00, 0x00)
	hdr := cell.RGB(0x00, 0x00, 0x80)
	white := cell.RGB(0xFF, 0xFF, 0xFF)
	hi := cell.RGB(0x00, 0x00, 0xAA)
	cv.FillRect(0, 0, cols, rows, bg)
	cv.DrawText(0, 0, " Chat ", white, hdr, cell.AttrBold)

	switch a.mode {
	case modeName:
		a.drawNamePrompt(cv, cols, rows, fg, bg, hi, white)
	case modeNewRoom:
		a.drawNewRoomPrompt(cv, cols, rows, fg, bg, hi, white)
	case modeChat:
		a.drawChat(cv, cols, rows, fg, bg, hdr, white, hi)
	}

	if rows > 0 {
		help := a.status
		if help == "" {
			help = "Chat — Tab switches panel, Up/Down navigate, Enter selects/sends"
		}
		cv.DrawText(0, rows-1, " "+help, cell.RGB(0xFF, 0xFF, 0x00), cell.RGB(0x00, 0x00, 0x00), 0)
	}
	return nil
}

func (a *App) drawNamePrompt(cv *uiapp.Canvas, cols, rows int, fg, bg, hi, white cell.Color) {
	cv.DrawText(2, 2, "Pick a display name for this LAN chat identity:", fg, bg, 0)
	cv.FillRect(2, 4, cols-4, 1, hi)
	cv.DrawText(2, 4, a.nameInput+"█", white, hi, 0)
	cv.DrawText(2, 6, "This name (and a new keypair) is saved locally and shown to", fg, bg, 0)
	cv.DrawText(2, 7, "other people on the LAN. Enter to continue.", fg, bg, 0)
}

func (a *App) drawNewRoomPrompt(cv *uiapp.Canvas, cols, rows int, fg, bg, hi, white cell.Color) {
	cv.DrawText(2, 2, "New room name:", fg, bg, 0)
	cv.FillRect(2, 4, cols-4, 1, hi)
	cv.DrawText(2, 4, a.roomNameInput+"█", white, hi, 0)
	cv.DrawText(2, 6, "Enter to create, Escape to cancel.", fg, bg, 0)
}

func (a *App) drawChat(cv *uiapp.Canvas, cols, rows int, fg, bg, hdr, white, hi cell.Color) {
	listW := cols / 4
	if listW < 14 {
		listW = 14
	}
	if listW > cols-24 {
		listW = cols - 24
	}
	if listW < 1 {
		listW = 1
	}

	rooms := a.svc.Rooms()
	peers := a.svc.Peers()
	peerNames := make(map[lanchat.PeerID]string, len(peers))
	for _, p := range peers {
		peerNames[p.ID] = p.Name
	}

	// Split the left column: rooms on top, peers on the bottom half.
	roomsH := (rows - 2) / 2
	if roomsH < 3 {
		roomsH = 3
	}

	cv.DrawText(0, 1, truncateRunes("Rooms", listW), fg, bg, cell.AttrBold)
	newRoomSel := a.panel == panelRooms && a.selRoomIdx == 0
	drawListRow(cv, 0, 2, listW, "+ New room", fg, bg, white, hi, newRoomSel)
	for i, r := range rooms {
		y := 3 + i
		if y >= roomsH {
			break
		}
		sel := a.panel == panelRooms && a.selRoomIdx == i+1
		label := roomLabel(r, peerNames)
		if !r.Joined {
			label = "(" + label + ")"
		}
		drawListRow(cv, 0, y, listW, label, fg, bg, white, hi, sel)
	}

	peersY := roomsH + 1
	if peersY < rows-2 {
		online := 0
		for _, p := range peers {
			if p.Online {
				online++
			}
		}
		cv.DrawText(0, peersY, truncateRunes(fmt.Sprintf("Peers (%d online)", online), listW), fg, bg, cell.AttrBold)
		for i, p := range peers {
			y := peersY + 1 + i
			if y >= rows-2 {
				break
			}
			sel := a.panel == panelPeers && a.selPeerIdx == i
			status := "○"
			if p.Online {
				status = "●"
			}
			drawListRow(cv, 0, y, listW, status+" "+p.Name, fg, bg, white, hi, sel)
		}
	}

	for y := 1; y < rows-2; y++ {
		cv.DrawText(listW, y, "│", fg, bg, 0)
	}

	tx := listW + 1
	tw := cols - tx
	if a.selectedRoom != "" && tw > 0 {
		msgs := a.svc.Messages(a.selectedRoom)
		bodyRows := rows - 3
		start := 0
		if len(msgs) > bodyRows {
			start = len(msgs) - bodyRows
		}
		for i, m := range msgs[start:] {
			y := 1 + i
			if y >= rows-2 {
				break
			}
			line := fmt.Sprintf("%s: %s", m.SenderName, m.Body)
			cv.DrawText(tx, y, truncateRunes(line, tw), fg, bg, 0)
		}
	} else if tw > 0 {
		cv.DrawText(tx, 1, "(select or create a room)", fg, bg, 0)
	}

	composeY := rows - 2
	if composeY > 0 {
		cbg := hdr
		ctext := white
		if a.panel != panelCompose {
			cbg, ctext = bg, fg
		}
		cv.FillRect(0, composeY, cols, 1, cbg)
		cursor := ""
		if a.panel == panelCompose {
			cursor = "█"
		}
		cv.DrawText(0, composeY, "> "+a.compose+cursor, ctext, cbg, 0)
	}
}

// roomLabel returns what to show in the room list: a DM room has no
// Name of its own (see lanchat.RoomSummary), so it's labeled with the
// other participant's current display name instead — falling back to a
// short fingerprint if we don't have a name for them yet (e.g. they're
// offline and we never received their hello).
func roomLabel(r lanchat.RoomSummary, peerNames map[lanchat.PeerID]string) string {
	if !r.IsDM {
		return r.Name
	}
	if name, ok := peerNames[r.DMPeer]; ok && name != "" {
		return "DM: " + name
	}
	id := string(r.DMPeer)
	if len(id) > 8 {
		id = id[:8]
	}
	return "DM: " + id
}

func drawListRow(cv *uiapp.Canvas, x, y, w int, label string, fg, bg, white, hi cell.Color, sel bool) {
	f, b := fg, bg
	if sel {
		f, b = white, hi
	}
	cv.FillRect(x, y, w, 1, b)
	cv.DrawText(x, y, truncateRunes(label, w), f, b, 0)
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
