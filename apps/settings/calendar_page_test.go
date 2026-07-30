package settings

import (
	"strings"
	"testing"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
)

func TestLeadOrDefault(t *testing.T) {
	cases := []struct{ in, want int }{{0, 5}, {-1, 5}, {10, 10}, {180, 180}}
	for _, c := range cases {
		if got := leadOrDefault(c.in); got != c.want {
			t.Errorf("leadOrDefault(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCalAccountIndex(t *testing.T) {
	a := &App{cfg: config.Config{Calendar: config.CalendarCfg{Accounts: []config.CalendarAccount{
		{Provider: "google", Enabled: true},
	}}}}
	if idx := a.calAccountIndex("google"); idx != 0 {
		t.Errorf("calAccountIndex(google) = %d, want 0", idx)
	}
	if idx := a.calAccountIndex("microsoft"); idx != -1 {
		t.Errorf("calAccountIndex(microsoft) = %d, want -1 (not configured)", idx)
	}
}

func TestCalendarLinesReflectsConnectionState(t *testing.T) {
	a := &App{cfg: config.Config{Calendar: config.CalendarCfg{Accounts: []config.CalendarAccount{
		{Provider: "google", Enabled: true},
		{Provider: "microsoft", Enabled: false},
	}}}}
	lines := a.calendarLines()
	if !strings.Contains(lines[0], "connected") || strings.Contains(lines[0], "not connected") {
		t.Errorf("google line = %q, want it to say connected (not not-connected)", lines[0])
	}
	if !strings.Contains(lines[1], "not connected") {
		t.Errorf("microsoft line = %q, want not connected", lines[1])
	}
}

func TestCalendarLinesShowsConnectingURL(t *testing.T) {
	a := &App{calConnecting: "google", calAuthURL: "https://accounts.google.com/o/oauth2/v2/auth?x=1"}
	lines := a.calendarLines()
	if !strings.Contains(lines[0], "https://accounts.google.com") {
		t.Errorf("google line = %q, want it to include the pending consent URL", lines[0])
	}
}

func TestCalendarCommitEditLeadTime(t *testing.T) {
	a := &App{sel: 2, editBuf: "15"}
	a.calendarCommitEdit()
	if a.cfg.Calendar.LeadMin != 15 {
		t.Errorf("LeadMin = %d, want 15", a.cfg.Calendar.LeadMin)
	}
}

func TestCalendarCommitEditLeadTimeRejectsOutOfRange(t *testing.T) {
	a := &App{sel: 2, editBuf: "9999"}
	a.calendarCommitEdit()
	if a.cfg.Calendar.LeadMin != 0 {
		t.Errorf("LeadMin = %d, want unchanged (0) for an out-of-range value", a.cfg.Calendar.LeadMin)
	}
}

func TestCalendarCommitEditTimezone(t *testing.T) {
	a := &App{sel: 3, editBuf: "  America/New_York  "}
	a.calendarCommitEdit()
	if a.cfg.Calendar.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want trimmed America/New_York", a.cfg.Calendar.Timezone)
	}
}

func TestCalendarCommitEditRequiresClientID(t *testing.T) {
	a := &App{calEditProvider: "google", editBuf: "   "}
	a.calendarCommitEdit()
	if a.calAccountIndex("google") != -1 {
		t.Error("an account was created despite an empty Client ID")
	}
	if a.status == "" {
		t.Error("status was not set to explain why nothing happened")
	}
}

func TestApplyCalMsgURLUpdatesStatusWithoutFinishing(t *testing.T) {
	a := &App{calConnecting: "google"}
	a.applyCalMsg(calMsg{provider: "google", url: "https://example.com/consent"})
	if a.calAuthURL != "https://example.com/consent" {
		t.Errorf("calAuthURL = %q, want the consent URL", a.calAuthURL)
	}
	if a.calConnecting != "google" {
		t.Error("calConnecting was cleared by a URL-only message — flow hasn't finished yet")
	}
}

func TestApplyCalMsgSuccessEnablesAccount(t *testing.T) {
	a := &App{
		calConnecting: "google",
		cfg:           config.Config{Calendar: config.CalendarCfg{Accounts: []config.CalendarAccount{{Provider: "google"}}}},
	}
	a.applyCalMsg(calMsg{provider: "google", done: true})
	if a.calConnecting != "" {
		t.Error("calConnecting was not cleared after the flow finished")
	}
	if !a.cfg.Calendar.Accounts[0].Enabled {
		t.Error("account was not marked Enabled after a successful connect")
	}
}

func TestApplyCalMsgFailureDoesNotEnableAccount(t *testing.T) {
	a := &App{
		calConnecting: "google",
		cfg:           config.Config{Calendar: config.CalendarCfg{Accounts: []config.CalendarAccount{{Provider: "google"}}}},
	}
	a.applyCalMsg(calMsg{provider: "google", done: true, err: errCalFixture})
	if a.cfg.Calendar.Accounts[0].Enabled {
		t.Error("account was enabled despite a failed connect")
	}
	if a.status == "" {
		t.Error("status was not set to explain the failure")
	}
}

var errCalFixture = fixtureErr("network unreachable")

type fixtureErr string

func (e fixtureErr) Error() string { return string(e) }
