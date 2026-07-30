package appstore

import (
	"testing"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
)

// TestOnTimerRegistersAgainstLiveConfigNotStaleSnapshot is a regression
// test for the "apps installed from the App Store occasionally disappear
// until you reopen the App Store" bug: this window's own a.cfg is a
// snapshot taken once at construction, but the window can stay open a
// long time (its own detect loop polls every ~2s) while some *other*
// window saves an unrelated config change in the meantime. Without
// refreshCfg, this app's next registerEntry-triggered persist() would
// save its stale snapshot — silently reverting that other change. With
// getCfg wired up, the persisted result must reflect the live config's
// other fields, not just the ones this snapshot happened to have at
// construction time.
func TestOnTimerRegistersAgainstLiveConfigNotStaleSnapshot(t *testing.T) {
	staleCfg := config.Config{FPS: 30} // what this window was constructed with
	liveCfg := config.Config{FPS: 30, Scrollback: 9999, Programs: []config.Program{
		{ID: "manual-other", Name: "Other Program", Command: "other"},
	}} // what another window saved *after* this one opened

	var saved config.Config
	saveCount := 0
	a := New(staleCfg,
		func(nc config.Config) { saved = nc; saveCount++ },
		func() config.Config { return liveCfg },
	)

	entry := RemoteEntry{
		ID:     "appstore-sh",
		Name:   "Shell",
		Detect: DetectSpec{Which: "sh"}, // "sh" is always on PATH — detectEntry() = true, no mocking needed
		Register: []RegisterEntry{
			{ID: "appstore-sh", Name: "Shell", Command: "sh"},
		},
	}
	a.loadCh <- loadResult{rows: []row{{entry: entry}}}
	a.onTimer()

	if saveCount != 1 {
		t.Fatalf("save count = %d, want exactly 1 (one new registration)", saveCount)
	}
	// The other window's changes must have survived.
	if saved.Scrollback != 9999 {
		t.Fatalf("Scrollback = %d, want 9999 (the live config's value, not the stale snapshot's zero)", saved.Scrollback)
	}
	if len(saved.Programs) != 2 {
		t.Fatalf("Programs = %+v, want 2 (the other window's program plus this app's own new registration)", saved.Programs)
	}
	foundOther, foundNew := false, false
	for _, p := range saved.Programs {
		if p.ID == "manual-other" {
			foundOther = true
		}
		if p.ID == "appstore-sh" {
			foundNew = true
		}
	}
	if !foundOther {
		t.Fatal("the other window's program was dropped — the stale-snapshot bug regressed")
	}
	if !foundNew {
		t.Fatal("this app's own new registration is missing")
	}
}

// TestOnTimerWithoutGetCfgStillWorks confirms getCfg is optional (e.g.
// for any other test that constructs App without it) — refreshCfg must
// be a no-op, not a nil-pointer panic, when it's unset.
func TestOnTimerWithoutGetCfgStillWorks(t *testing.T) {
	var saved config.Config
	a := New(config.Config{}, func(nc config.Config) { saved = nc }, nil)

	entry := RemoteEntry{
		ID:       "appstore-sh2",
		Detect:   DetectSpec{Which: "sh"},
		Register: []RegisterEntry{{ID: "appstore-sh2", Name: "Shell2", Command: "sh"}},
	}
	a.loadCh <- loadResult{rows: []row{{entry: entry}}}
	a.onTimer()

	if len(saved.Programs) != 1 || saved.Programs[0].ID != "appstore-sh2" {
		t.Fatalf("Programs = %+v, want the one registered program", saved.Programs)
	}
}
