// Package credstore is a minimal secret store for native apps: OAuth2
// tokens (Calendar), a Matrix session token (Chat), and anything else that
// needs to survive a restart without living in config.json or git. One
// file per key under ~/.config/ttypedesk/credentials/, 0600, no external
// keyring dependency — matches the project's "no new required deps"
// posture for anything that isn't opt-in.
package credstore

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// keyPattern keeps keys usable directly as filenames — no path separators,
// no "..", nothing that needs escaping.
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("credstore: %w", err)
	}
	return filepath.Join(home, ".config", "ttypedesk", "credentials"), nil
}

func checkKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("credstore: invalid key %q", key)
	}
	return nil
}

// Save writes value under key, replacing any existing value. The file is
// created (or rewritten) with 0600 permissions.
func Save(key string, value []byte) error {
	if err := checkKey(key); err != nil {
		return err
	}
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return fmt.Errorf("credstore: %w", err)
	}
	if err := os.WriteFile(filepath.Join(d, key), value, 0o600); err != nil {
		return fmt.Errorf("credstore: %w", err)
	}
	return nil
}

// Load reads back the value stored under key. Returns an error wrapping
// os.ErrNotExist (check with errors.Is) if nothing has been saved yet.
func Load(key string) ([]byte, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	d, err := dir()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(d, key))
	if err != nil {
		return nil, fmt.Errorf("credstore: %w", err)
	}
	return b, nil
}

// Delete removes any value stored under key. Not an error if none exists.
func Delete(key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(d, key)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("credstore: %w", err)
	}
	return nil
}
