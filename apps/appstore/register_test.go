package appstore

import (
	"testing"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
)

func TestRegisterEntryNewProgramNoCollision(t *testing.T) {
	cfg := &config.Config{}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-taskmgr", Name: "Task Manager", Command: "pty:btop", Menu: config.MenuSystem},
	}}

	if !registerEntry(cfg, e) {
		t.Fatal("registerEntry() = false, want true (cfg mutated)")
	}
	if len(cfg.Programs) != 1 {
		t.Fatalf("Programs len = %d, want 1", len(cfg.Programs))
	}
	p := cfg.Programs[0]
	if p.Name != "Task Manager" || p.Menu != config.MenuSystem || p.Command != "pty:btop" {
		t.Fatalf("Programs[0] = %+v, want Name=Task Manager Menu=system Command=pty:btop", p)
	}
}

func TestRegisterEntrySuffixesOnNameCollision(t *testing.T) {
	cfg := &config.Config{Programs: []config.Program{
		{ID: "manual-taskmgr", Name: "Task Manager", Command: "htop"},
	}}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-taskmgr", Name: "Task Manager", Command: "pty:btop", Menu: config.MenuSystem},
	}}

	registerEntry(cfg, e)

	if len(cfg.Programs) != 2 {
		t.Fatalf("Programs len = %d, want 2 (original + suffixed)", len(cfg.Programs))
	}
	if cfg.Programs[0].Name != "Task Manager" || cfg.Programs[0].Command != "htop" {
		t.Fatalf("existing program was mutated: %+v", cfg.Programs[0])
	}
	if cfg.Programs[1].Name != "Task Manager-store" {
		t.Fatalf("new program Name = %q, want %q", cfg.Programs[1].Name, "Task Manager-store")
	}
	if cfg.Programs[1].Command != "pty:btop" {
		t.Fatalf("new program Command = %q, want pty:btop", cfg.Programs[1].Command)
	}
}

func TestRegisterEntrySuffixIncrementsOnRepeatCollision(t *testing.T) {
	cfg := &config.Config{Programs: []config.Program{
		{ID: "manual-taskmgr", Name: "Task Manager", Command: "htop"},
		{ID: "other-store-taskmgr", Name: "Task Manager-store", Command: "gotop"},
	}}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-taskmgr", Name: "Task Manager", Command: "pty:btop"},
	}}

	registerEntry(cfg, e)

	if len(cfg.Programs) != 3 {
		t.Fatalf("Programs len = %d, want 3", len(cfg.Programs))
	}
	got := cfg.Programs[2].Name
	if got != "Task Manager-store-2" {
		t.Fatalf("new program Name = %q, want %q", got, "Task Manager-store-2")
	}
}

func TestRegisterEntryByIDUpdatesInPlaceWithoutResuffixing(t *testing.T) {
	cfg := &config.Config{Programs: []config.Program{
		{ID: "manual-taskmgr", Name: "Task Manager", Command: "htop"},
		{ID: "appstore-taskmgr", Name: "Task Manager-store", Command: "pty:btop"},
	}}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-taskmgr", Name: "Task Manager", Command: "pty:btop --utf-force"},
	}}

	changed := registerEntry(cfg, e)

	if !changed {
		t.Fatal("registerEntry() = false, want true (command changed)")
	}
	if len(cfg.Programs) != 2 {
		t.Fatalf("Programs len = %d, want 2 (no new duplicate)", len(cfg.Programs))
	}
	p := cfg.Programs[1]
	if p.Name != "Task Manager-store" {
		t.Fatalf("Name changed on an ID-matched update: got %q", p.Name)
	}
	if p.Command != "pty:btop --utf-force" {
		t.Fatalf("Command = %q, want updated command", p.Command)
	}
}

func TestRegisterEntryByIDNoopWhenUnchanged(t *testing.T) {
	cfg := &config.Config{Programs: []config.Program{
		{ID: "appstore-taskmgr", Name: "Task Manager", Command: "pty:btop"},
	}}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-taskmgr", Name: "Task Manager", Command: "pty:btop"},
	}}

	if registerEntry(cfg, e) {
		t.Fatal("registerEntry() = true, want false (nothing changed)")
	}
}

func TestRegisterEntryNewProgramCarriesBridgeFlag(t *testing.T) {
	cfg := &config.Config{}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-gimp", Name: "GIMP", Command: "gimp", Bridge: true},
	}}

	registerEntry(cfg, e)

	if len(cfg.Programs) != 1 {
		t.Fatalf("Programs len = %d, want 1", len(cfg.Programs))
	}
	if !cfg.Programs[0].Bridge {
		t.Fatal("Programs[0].Bridge = false, want true")
	}
}

func TestRegisterEntryByIDUpdatesBridgeFlagInPlace(t *testing.T) {
	cfg := &config.Config{Programs: []config.Program{
		{ID: "appstore-gimp", Name: "GIMP", Command: "gimp", Bridge: false},
	}}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-gimp", Name: "GIMP", Command: "gimp", Bridge: true},
	}}

	changed := registerEntry(cfg, e)

	if !changed {
		t.Fatal("registerEntry() = false, want true (bridge flag changed)")
	}
	if len(cfg.Programs) != 1 {
		t.Fatalf("Programs len = %d, want 1 (no duplicate)", len(cfg.Programs))
	}
	if !cfg.Programs[0].Bridge {
		t.Fatal("Programs[0].Bridge = false, want true after update")
	}
}

func TestRegisterEntryNewProgramCarriesExtAppFlag(t *testing.T) {
	cfg := &config.Config{}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-matrixchat", Name: "Matrix Chat", Command: "matrixchat", ExtApp: true},
	}}

	registerEntry(cfg, e)

	if len(cfg.Programs) != 1 {
		t.Fatalf("Programs len = %d, want 1", len(cfg.Programs))
	}
	if !cfg.Programs[0].ExtApp {
		t.Fatal("Programs[0].ExtApp = false, want true")
	}
}

func TestRegisterEntryByIDUpdatesExtAppFlagInPlace(t *testing.T) {
	cfg := &config.Config{Programs: []config.Program{
		{ID: "appstore-matrixchat", Name: "Matrix Chat", Command: "matrixchat", ExtApp: false},
	}}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-matrixchat", Name: "Matrix Chat", Command: "matrixchat", ExtApp: true},
	}}

	changed := registerEntry(cfg, e)

	if !changed {
		t.Fatal("registerEntry() = false, want true (ext_app flag changed)")
	}
	if len(cfg.Programs) != 1 {
		t.Fatalf("Programs len = %d, want 1 (no duplicate)", len(cfg.Programs))
	}
	if !cfg.Programs[0].ExtApp {
		t.Fatal("Programs[0].ExtApp = false, want true after update")
	}
}

func TestRegisterEntrySetsRoleOnNewProgram(t *testing.T) {
	cfg := &config.Config{}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-superfile", Name: "SuperFile", Command: "pty:spf", SetRole: "filemgr"},
	}}

	registerEntry(cfg, e)

	if cfg.Roles.FileMgr != "prog:appstore-superfile" {
		t.Fatalf("Roles.FileMgr = %q, want %q", cfg.Roles.FileMgr, "prog:appstore-superfile")
	}
}

func TestRegisterEntrySetsRoleOnIDMatchedUpdate(t *testing.T) {
	cfg := &config.Config{Programs: []config.Program{
		{ID: "appstore-superfile", Name: "SuperFile", Command: "pty:spf"},
	}}
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-superfile", Name: "SuperFile", Command: "pty:spf", SetRole: "filemgr"},
	}}

	changed := registerEntry(cfg, e)

	if !changed {
		t.Fatal("registerEntry() = false, want true (role changed)")
	}
	if cfg.Roles.FileMgr != "prog:appstore-superfile" {
		t.Fatalf("Roles.FileMgr = %q, want %q", cfg.Roles.FileMgr, "prog:appstore-superfile")
	}
}

func TestRegisterEntryWithoutSetRoleLeavesRolesAlone(t *testing.T) {
	cfg := &config.Config{}
	cfg.Roles.FileMgr = "files"
	e := RemoteEntry{Register: []RegisterEntry{
		{ID: "appstore-gotube", Name: "GoTube", Command: "pty:gophertube"},
	}}

	registerEntry(cfg, e)

	if cfg.Roles.FileMgr != "files" {
		t.Fatalf("Roles.FileMgr = %q, want unchanged %q", cfg.Roles.FileMgr, "files")
	}
}

func TestApplySetRoleUnknownRoleIsNoop(t *testing.T) {
	cfg := &config.Config{}
	if applySetRole(cfg, "bogus-role", "x") {
		t.Fatal("applySetRole() = true for an unknown role, want false")
	}
}

func TestUniqueProgramName(t *testing.T) {
	cfg := &config.Config{Programs: []config.Program{
		{Name: "Task Manager"},
		{Name: "Task Manager-store"},
	}}
	if got := uniqueProgramName(cfg, "Notes"); got != "Notes" {
		t.Errorf("uniqueProgramName(Notes) = %q, want unchanged", got)
	}
	if got := uniqueProgramName(cfg, "Task Manager"); got != "Task Manager-store-2" {
		t.Errorf("uniqueProgramName(Task Manager) = %q, want %q", got, "Task Manager-store-2")
	}
}
