package appstore

import (
	"fmt"
	"strings"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
)

// registerEntry upserts an entry's launchers into cfg.Programs. Matching is
// by ID only — an entry we've registered before (repeat install, or a
// re-detect on a later App Store open) gets its Command refreshed in place.
// A brand new entry never takes over some other existing program just
// because the display Name matches; instead uniqueProgramName suffixes it
// so an App Store install can coexist with an unrelated program of the same
// name already on the system. Reports whether cfg was mutated.
func registerEntry(cfg *config.Config, e RemoteEntry) bool {
	changed := false
	for _, w := range e.Register {
		if p := cfg.FindProgram(w.ID); p != nil {
			if p.Command != w.Command {
				p.Command = w.Command
				changed = true
			}
			if p.Bridge != w.Bridge {
				p.Bridge = w.Bridge
				changed = true
			}
			if p.ExtApp != w.ExtApp {
				p.ExtApp = w.ExtApp
				changed = true
			}
			if w.SetRole != "" && applySetRole(cfg, w.SetRole, w.ID) {
				changed = true
			}
			continue
		}
		menu := config.MenuPrograms
		if w.Menu == config.MenuSystem {
			menu = config.MenuSystem
		}
		cfg.Programs = append(cfg.Programs, config.Program{
			ID:      w.ID,
			Name:    uniqueProgramName(cfg, w.Name),
			Command: w.Command,
			Icon:    w.Icon,
			Menu:    menu,
			Desktop: false,
			Bridge:  w.Bridge,
			ExtApp:  w.ExtApp,
		})
		if w.SetRole != "" {
			applySetRole(cfg, w.SetRole, w.ID)
		}
		changed = true
	}
	if changed {
		cfg.SyncProgramDesktop()
	}
	return changed
}

// applySetRole points cfg.Roles.<role> at progID's prog: launch action.
// Reports whether it actually changed anything (unknown roles are no-ops).
func applySetRole(cfg *config.Config, role, progID string) bool {
	action := config.ProgramAction(progID)
	var target *string
	switch strings.ToLower(role) {
	case "filemgr", "files":
		target = &cfg.Roles.FileMgr
	case "editor":
		target = &cfg.Roles.Editor
	case "browser":
		target = &cfg.Roles.Browser
	case "terminal":
		target = &cfg.Roles.Terminal
	case "image":
		target = &cfg.Roles.Image
	default:
		return false
	}
	if *target == action {
		return false
	}
	*target = action
	return true
}

// uniqueProgramName returns name unchanged if no existing program already
// uses it, otherwise "<name>-store", then "<name>-store-2", "<name>-store-3",
// … until it finds one that's free.
func uniqueProgramName(cfg *config.Config, name string) string {
	taken := func(n string) bool {
		for i := range cfg.Programs {
			if strings.TrimSpace(cfg.Programs[i].Name) == n {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	candidate := name + "-store"
	for n := 2; taken(candidate); n++ {
		candidate = fmt.Sprintf("%s-store-%d", name, n)
	}
	return candidate
}
