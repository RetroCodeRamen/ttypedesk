package settings

import (
	"strings"
	"testing"

	"github.com/RetroCodeRamen/ttypedesk/internal/lanchat"
)

// newTestLanchat starts a real lanchat.Service in an isolated temp
// directory — consistent with this project's preference for real
// objects over mocks in tests.
func newTestLanchat(t *testing.T) *lanchat.Service {
	t.Helper()
	svc, err := lanchat.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("lanchat.New() = %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc
}

func TestLANChatLinesShowsDisplayName(t *testing.T) {
	svc := newTestLanchat(t)
	if err := svc.SetDisplayName("Alice"); err != nil {
		t.Fatal(err)
	}
	a := &App{lanchat: svc, page: 12}
	lines := a.lines()
	if len(lines) == 0 || !strings.Contains(lines[0], "Alice") {
		t.Fatalf("lines()[0] = %q, want it to mention the display name", lines[0])
	}
}

func TestLANChatLinesReportsUnavailableWhenServiceNil(t *testing.T) {
	a := &App{lanchat: nil, page: 12}
	lines := a.lines()
	if len(lines) != 1 || !strings.Contains(lines[0], "unavailable") {
		t.Fatalf("lines() = %v, want a single unavailable message", lines)
	}
}

func TestLANChatActivateEditNameStartsEditingWithCurrentName(t *testing.T) {
	svc := newTestLanchat(t)
	if err := svc.SetDisplayName("Bob"); err != nil {
		t.Fatal(err)
	}
	a := &App{lanchat: svc, page: 12, sel: 0}
	a.activate()
	if !a.editing || a.editBuf != "Bob" {
		t.Fatalf("editing=%v editBuf=%q, want editing=true editBuf=%q", a.editing, a.editBuf, "Bob")
	}
}

func TestLANChatCommitEditSetsDisplayName(t *testing.T) {
	svc := newTestLanchat(t)
	a := &App{lanchat: svc, page: 12, sel: 0, editBuf: "Carol"}
	a.commitEdit()
	if _, name := svc.Self(); name != "Carol" {
		t.Fatalf("Self() name = %q, want %q", name, "Carol")
	}
	if !strings.Contains(a.status, "Carol") {
		t.Fatalf("status = %q, want it to mention the new name", a.status)
	}
}

func TestLANChatActivateRegenerateIdentityChangesID(t *testing.T) {
	svc := newTestLanchat(t)
	id1, _ := svc.Self()
	a := &App{lanchat: svc, page: 12, sel: 1}
	a.activate()
	id2, _ := svc.Self()
	if id1 == id2 {
		t.Fatal("PeerID unchanged after regenerate-identity activation")
	}
}
