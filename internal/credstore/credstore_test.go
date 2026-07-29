package credstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Save("calendar.google.token", []byte("secret-token")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("calendar.google.token")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != "secret-token" {
		t.Errorf("Load = %q, want %q", got, "secret-token")
	}
}

func TestSaveOverwrites(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_ = Save("k", []byte("first"))
	_ = Save("k", []byte("second"))
	got, err := Load("k")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("Load after overwrite = %q, want %q", got, "second")
	}
}

func TestFilePermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := Save("k", []byte("v")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(home, ".config", "ttypedesk", "credentials", "k"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perms = %o, want 0600", perm)
	}
}

func TestLoadMissingKeyReturnsNotExist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Load("never-saved")
	if err == nil {
		t.Fatal("Load of missing key: want error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load error = %v, want wrapping os.ErrNotExist", err)
	}
}

func TestInvalidKeyRejected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := []string{"", "../escape", "a/b", "a b", "a\x00b"}
	for _, k := range cases {
		if err := Save(k, []byte("x")); err == nil {
			t.Errorf("Save(%q): want error for invalid key, got nil", k)
		}
		if _, err := Load(k); err == nil {
			t.Errorf("Load(%q): want error for invalid key, got nil", k)
		}
	}
}

func TestDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_ = Save("k", []byte("v"))
	if err := Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Load("k"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load after Delete: want ErrNotExist, got %v", err)
	}
	// Deleting an already-absent key is not an error.
	if err := Delete("k"); err != nil {
		t.Errorf("Delete of already-absent key: want nil, got %v", err)
	}
}
