package appstore

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// installEntry downloads an entry's install script and runs it in a new
// terminal window. The script runs interactively and unsandboxed — it may
// use sudo, in which case the user is prompted for their password right in
// that window, same as running any other command in a Terminal.
func installEntry(ctx *uiapp.Context, src config.AppSource, e RemoteEntry) error {
	branch := branchOrDefault(src.Branch)
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", src.Repo, branch, e.Install.Script)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("%s: %w", e.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", e.Name, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", e.Name, err)
	}

	safeID := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return '-'
		}
	}, e.ID)
	// Written to a space-free temp path and launched as "pty:bash <path>"
	// (exactly two whitespace-safe fields) because the "pty:" action is
	// parsed with plain strings.Fields — no shell/quote handling — so an
	// inline multi-word script would get corrupted.
	path := filepath.Join(os.TempDir(), fmt.Sprintf("ttypedesk-appstore-%s-%d.sh", safeID, os.Getuid()))
	if err := os.WriteFile(path, body, 0o700); err != nil {
		return err
	}
	return ctx.Launch("pty:bash " + path)
}
