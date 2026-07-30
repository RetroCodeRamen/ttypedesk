package appstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/config"
)

func branchOrDefault(branch string) string {
	if branch == "" {
		return "main"
	}
	return branch
}

// fetchCatalog downloads and parses a source repo's index.json.
func fetchCatalog(src config.AppSource) ([]RemoteEntry, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/index.json", src.Repo, branchOrDefault(src.Branch))
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src.Repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", src.Repo, resp.Status)
	}
	var idx catalogIndex
	if err := json.NewDecoder(resp.Body).Decode(&idx); err != nil {
		return nil, fmt.Errorf("%s: %w", src.Repo, err)
	}
	return idx.Apps, nil
}

// detectEntry reports whether an entry's app already appears installed.
func detectEntry(e RemoteEntry) bool {
	if e.Detect.Which != "" {
		if _, err := exec.LookPath(e.Detect.Which); err == nil {
			return true
		}
	}
	home, _ := os.UserHomeDir()
	for _, p := range e.Detect.Paths {
		if home != "" && strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}
