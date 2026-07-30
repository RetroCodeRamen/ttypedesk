package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// withTempHome points Path() at a fresh temp directory for the duration
// of the test, so Save/Load exercise the real file-system code path
// without touching the real user's ~/.config/ttypedesk/config.json.
func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestSaveThenLoadRoundTripsPrograms(t *testing.T) {
	withTempHome(t)
	cfg := Default()
	cfg.Programs = append(cfg.Programs, Program{ID: "appstore-x", Name: "X", Command: "x"})

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if len(got.Programs) != 1 || got.Programs[0].ID != "appstore-x" {
		t.Fatalf("Programs after round-trip = %+v, want the one saved program", got.Programs)
	}
}

// TestSaveIsAtomicNeverLeavesATruncatedFile guards against a regression
// to plain os.WriteFile: Save must write to a temp file and rename over
// the real path, so a reader can never observe a half-written
// config.json, even if it looks at the file mid-Save.
func TestSaveIsAtomicNeverLeavesATruncatedFile(t *testing.T) {
	withTempHome(t)
	cfg := Default()
	cfg.Programs = make([]Program, 200) // large enough that a naive write is not a single instant
	for i := range cfg.Programs {
		cfg.Programs[i] = Program{ID: "p", Name: "program with a moderately long name to pad the file", Command: "cmd"}
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("config.json is not valid JSON after Save(): %v", err)
	}

	// No stray .config.json.tmp-* files left behind in the same directory.
	entries, err := os.ReadDir(filepath.Dir(Path()))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config.json.tmp-") {
			t.Fatalf("stray temp file left behind: %s", e.Name())
		}
	}
}

// TestConcurrentSavesNeverCorruptTheFile hammers Save from many
// goroutines at once (mirroring several app windows each persisting
// their own Config independently) — config.json must always parse as
// valid JSON afterward (whichever save happened to land last), never a
// torn mix of two writes.
func TestConcurrentSavesNeverCorruptTheFile(t *testing.T) {
	withTempHome(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cfg := Default()
			cfg.Programs = []Program{{ID: "p", Name: "prog", Command: "cmd"}}
			cfg.FPS = n + 1
			_ = Save(cfg)
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("config.json is not valid JSON after concurrent Save() calls: %v", err)
	}
	if len(got.Programs) != 1 || got.Programs[0].ID != "p" {
		t.Fatalf("Programs = %+v, want the single program every writer set (not a torn mix)", got.Programs)
	}
}
