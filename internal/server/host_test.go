package server

import (
	"testing"

	"github.com/RetroCodeRamen/ttypedesk/internal/clip"
)

// TestAppHostClipboardDelegatesToClipPackage confirms appHost.ClipboardGet/
// ClipboardSet aren't a separate clipboard from the one the desktop's own
// copy/paste keybindings use (internal/client) — they're the same
// internal/clip package, so an app and the user's own Ctrl+Shift+C/paste
// see each other's writes.
func TestAppHostClipboardDelegatesToClipPackage(t *testing.T) {
	h := &appHost{srv: newTestServer(t)}
	h.ClipboardSet("from app")
	if got := clip.Get(); got != "from app" {
		t.Fatalf("clip.Get() = %q, want %q after appHost.ClipboardSet", got, "from app")
	}
	clip.Set("from desktop")
	if got := h.ClipboardGet(); got != "from desktop" {
		t.Fatalf("appHost.ClipboardGet() = %q, want %q after clip.Set", got, "from desktop")
	}
}
