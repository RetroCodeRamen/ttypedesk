package client

import (
	"os"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/slog"
)

// pollConfigReload notices external edits to config.json (hand-edited, synced
// from another machine, written by a second instance, ...) and hot-applies
// them the same way a Settings-app Save already does. Saves made from within
// this process update c.cfgModTime themselves (see ApplyConfig), so this
// only ever fires on a change this process didn't make itself.
func (c *Client) pollConfigReload() {
	now := time.Now()
	if !c.lastCfgPoll.IsZero() && now.Sub(c.lastCfgPoll) < 2*time.Second {
		return
	}
	c.lastCfgPoll = now

	info, err := os.Stat(config.Path())
	if err != nil || !info.ModTime().After(c.cfgModTime) {
		return
	}

	nc, err := config.Load()
	if err != nil {
		slog.Warn("config hot-reload: %v", err)
		c.cfgModTime = info.ModTime() // don't retry a broken file every 2s
		return
	}
	c.srv.SetConfig(nc)
	c.ApplyConfig(nc)
	slog.Info("config hot-reloaded from disk")
}
