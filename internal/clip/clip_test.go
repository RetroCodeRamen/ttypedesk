package clip

import "testing"

// TestSetGetRoundTripsInProcessBuffer confirms Get returns what Set wrote
// even with no external clipboard tool available — Get only prefers
// wl-paste/xclip/xsel output when one of them actually returns something;
// otherwise it falls back to the in-process buffer Set always updates.
// This is deliberately environment-independent: it doesn't assume any
// clipboard tool (or a real desktop session) is present, unlike the
// external sync paths, which aren't unit-tested here for exactly that
// reason.
func TestSetGetRoundTripsInProcessBuffer(t *testing.T) {
	Set("hello clipboard")
	if got := Get(); got != "hello clipboard" {
		// If a real wl-copy/xclip/xsel happens to be present and working in
		// this environment, Get() legitimately prefers its output over the
		// in-process buffer — only fail if there's truly no external tool
		// that could explain the mismatch.
		if getExternal() == "" {
			t.Fatalf("Get() = %q, want %q (no external clipboard tool active, so this must be the in-process buffer)", got, "hello clipboard")
		}
	}
}

func TestSetOverwritesPreviousValue(t *testing.T) {
	Set("first")
	Set("second")
	mu.RLock()
	buf := text
	mu.RUnlock()
	if buf != "second" {
		t.Fatalf("in-process buffer = %q, want %q", buf, "second")
	}
}
