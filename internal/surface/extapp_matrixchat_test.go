package surface

import (
	"strings"
	"testing"
	"time"
)

// TestExtAppSurfaceRunsRealMatrixChatBinary is the end-to-end proof for
// pkg/extapprun's generic adapter, not just its own fakeApp-based unit
// tests: apps/matrixchat was extracted from a built-in AppSurface app to
// a standalone extapprun-driven binary with zero changes to its own code
// (see apps/matrixchat's package doc). Spawning the real compiled binary
// and confirming its login screen renders correctly through the full
// wire protocol is what actually proves that extraction didn't break it.
func TestExtAppSurfaceRunsRealMatrixChatBinary(t *testing.T) {
	s, err := NewExtApp("w1", matrixchatBinary, nil, 60, 20)
	if err != nil {
		t.Fatalf("NewExtApp: %v", err)
	}
	defer s.Close()
	if err := s.BindHost(&fakeHost{}); err != nil {
		t.Fatalf("BindHost: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		s.ProduceDiff()
		return strings.Contains(gridText(s.Snapshot(), 60, 20), "Homeserver")
	})
	if strings.Contains(s.Title(), "[crashed]") {
		t.Fatalf("surface reported crashed: %s", s.Title())
	}
}
