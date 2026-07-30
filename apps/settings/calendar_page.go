package settings

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/RetroCodeRamen/ttypedesk/internal/calsync"
	"github.com/RetroCodeRamen/ttypedesk/internal/config"
)

// calMsg is how a background OAuth2 connect flow (calsync.Connect) reports
// back to the single-threaded Handle/Draw path — never by touching App
// fields directly from its own goroutine. A flow sends one message as
// soon as the consent URL is known (url set — critical to surface, since
// many users of this project have no local browser at all, over SSH),
// then a final one when it completes (done set, err nil on success).
type calMsg struct {
	provider string
	url      string
	done     bool
	err      error
}

func leadOrDefault(n int) int {
	if n <= 0 {
		return 5
	}
	return n
}

func (a *App) calAccountIndex(provider string) int {
	for i, x := range a.cfg.Calendar.Accounts {
		if x.Provider == provider {
			return i
		}
	}
	return -1
}

func (a *App) calendarLines() []string {
	status := func(provider, label string) string {
		if a.calConnecting == provider {
			if a.calAuthURL != "" {
				return fmt.Sprintf("%s: open this URL to finish connecting — %s", label, a.calAuthURL)
			}
			return fmt.Sprintf("%s: connecting…", label)
		}
		if idx := a.calAccountIndex(provider); idx >= 0 && a.cfg.Calendar.Accounts[idx].Enabled {
			return label + ": connected — Enter to disconnect"
		}
		return label + ": not connected — Enter to connect"
	}
	tz := a.cfg.Calendar.Timezone
	if tz == "" {
		tz = "(system default)"
	}
	return []string{
		status("google", "Google Calendar"),
		status("microsoft", "Microsoft Calendar"),
		fmt.Sprintf("Reminder lead time: %d min", leadOrDefault(a.cfg.Calendar.LeadMin)),
		"Timezone: " + tz,
	}
}

func (a *App) calendarActivate() {
	switch a.sel {
	case 0, 1:
		provider, label := "google", "Google"
		if a.sel == 1 {
			provider, label = "microsoft", "Microsoft"
		}
		if a.calConnecting == provider {
			return // a connect flow is already running for this provider
		}
		idx := a.calAccountIndex(provider)
		if idx >= 0 && a.cfg.Calendar.Accounts[idx].Enabled {
			a.cfg.Calendar.Accounts[idx].Enabled = false
			a.persist()
			a.status = label + " Calendar disconnected"
			return
		}
		a.editing = true
		a.calEditProvider = provider
		if idx >= 0 {
			a.editBuf = a.cfg.Calendar.Accounts[idx].ClientID
		} else {
			a.editBuf = ""
		}
		a.status = label + " OAuth Client ID (from your own Google Cloud / Azure Portal app) — Enter to connect"
	case 2:
		a.editing = true
		a.editBuf = strconv.Itoa(leadOrDefault(a.cfg.Calendar.LeadMin))
		a.status = "Reminder lead time in minutes"
	case 3:
		a.editing = true
		a.editBuf = a.cfg.Calendar.Timezone
		a.status = "IANA timezone (e.g. America/New_York) — blank means system default"
	}
}

func (a *App) calendarCommitEdit() {
	if a.calEditProvider != "" {
		provider := a.calEditProvider
		a.calEditProvider = ""
		clientID := strings.TrimSpace(a.editBuf)
		if clientID == "" {
			a.status = "Client ID required — not connecting"
			return
		}
		idx := a.calAccountIndex(provider)
		if idx < 0 {
			a.cfg.Calendar.Accounts = append(a.cfg.Calendar.Accounts, config.CalendarAccount{Provider: provider})
			idx = len(a.cfg.Calendar.Accounts) - 1
		}
		a.cfg.Calendar.Accounts[idx].ClientID = clientID
		a.startCalConnect(provider, clientID)
		return
	}
	switch a.sel {
	case 2:
		if n, err := strconv.Atoi(a.editBuf); err == nil && n > 0 && n <= 180 {
			a.cfg.Calendar.LeadMin = n
			a.status = fmt.Sprintf("Reminder lead time: %d min", n)
		}
	case 3:
		a.cfg.Calendar.Timezone = strings.TrimSpace(a.editBuf)
		if a.cfg.Calendar.Timezone == "" {
			a.status = "Timezone: system default"
		} else {
			a.status = "Timezone: " + a.cfg.Calendar.Timezone
		}
	}
}

// startCalConnect runs the interactive OAuth2 flow (internal/calsync) on
// its own goroutine — see calMsg's doc comment for why results only ever
// come back through a.calResults, drained from Draw.
func (a *App) startCalConnect(provider, clientID string) {
	if a.calResults == nil {
		a.calResults = make(chan calMsg, 8)
	}
	a.calConnecting = provider
	a.calAuthURL = ""
	a.status = "Connecting " + provider + " — opening your browser…"

	ctx := a.ctx
	results := a.calResults
	go func() {
		onURL := func(url string) {
			select {
			case results <- calMsg{provider: provider, url: url}:
			default:
			}
			if ctx != nil {
				ctx.MarkDirty()
			}
		}
		save := func(tok []byte) error {
			return ctx.SaveCredential("calendar."+provider+".token", tok)
		}
		err := calsync.Connect(context.Background(), calsync.Provider(provider), clientID, onURL, save)
		select {
		case results <- calMsg{provider: provider, done: true, err: err}:
		default:
		}
		if ctx != nil {
			ctx.MarkDirty()
		}
	}()
}

// drainCalResults applies any calMsg values posted by a background
// connect flow since the last Draw — called only from Draw, i.e. only
// ever on the single thread that owns every other App field.
func (a *App) drainCalResults() {
	if a.calResults == nil {
		return
	}
	for {
		select {
		case msg := <-a.calResults:
			a.applyCalMsg(msg)
		default:
			return
		}
	}
}

func (a *App) applyCalMsg(msg calMsg) {
	if msg.url != "" {
		a.calAuthURL = msg.url
		a.status = "Open this URL to connect " + msg.provider + ": " + msg.url
		return
	}
	if !msg.done {
		return
	}
	a.calConnecting = ""
	a.calAuthURL = ""
	if msg.err != nil {
		a.status = msg.provider + " connect failed: " + msg.err.Error()
		return
	}
	if idx := a.calAccountIndex(msg.provider); idx >= 0 {
		a.cfg.Calendar.Accounts[idx].Enabled = true
		a.persist()
	}
	a.status = msg.provider + " connected"
}
