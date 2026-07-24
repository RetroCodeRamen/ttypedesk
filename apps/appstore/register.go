package appstore

import (
	"strings"

	"github.com/ttypedesk/ttypedesk/internal/config"
)

// registerEntry upserts an entry's launchers into cfg.Programs. Matching is
// by ID first (an entry we've registered before), then by Name (absorbs
// pre-existing manually-added programs so repeat installs — or a first
// App Store open on a machine that already has the app — don't create
// duplicates). Reports whether cfg was mutated.
func registerEntry(cfg *config.Config, e RemoteEntry) bool {
	changed := false
	for _, w := range e.Register {
		if p := cfg.FindProgram(w.ID); p != nil {
			if p.Command != w.Command {
				p.Command = w.Command
				changed = true
			}
			continue
		}
		matched := false
		for i := range cfg.Programs {
			if strings.TrimSpace(cfg.Programs[i].Name) == w.Name {
				if cfg.Programs[i].Command != w.Command {
					cfg.Programs[i].Command = w.Command
					changed = true
				}
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		menu := config.MenuPrograms
		if w.Menu == config.MenuSystem {
			menu = config.MenuSystem
		}
		cfg.Programs = append(cfg.Programs, config.Program{
			ID:      w.ID,
			Name:    w.Name,
			Command: w.Command,
			Icon:    w.Icon,
			Menu:    menu,
			Desktop: false,
		})
		changed = true
	}
	if changed {
		cfg.SyncProgramDesktop()
	}
	return changed
}
