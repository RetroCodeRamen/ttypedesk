// Package slog writes TTYPE Desk diagnostics to a rotating log file.
// Use this for troubleshooting crashes and UI issues without spoiling the TUI.
package slog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

var (
	mu     sync.Mutex
	file   *os.File
	level  Level = LevelInfo
	inited bool
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Path returns the default log file location.
func Path() string {
	if p := os.Getenv("TTYPEDESK_LOG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "ttypedesk.log"
	}
	return filepath.Join(home, ".config", "ttypedesk", "ttypedesk.log")
}

// Init opens the log file. Safe to call multiple times.
func Init() error {
	mu.Lock()
	defer mu.Unlock()
	if inited && file != nil {
		return nil
	}
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Truncate if larger than ~2MB so the file stays readable.
	if st, err := os.Stat(path); err == nil && st.Size() > 2<<20 {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	file = f
	inited = true
	if v := os.Getenv("TTYPEDESK_LOG_LEVEL"); v != "" {
		switch v {
		case "debug", "DEBUG":
			level = LevelDebug
		case "warn", "WARN":
			level = LevelWarn
		case "error", "ERROR":
			level = LevelError
		default:
			level = LevelInfo
		}
	}
	_, _ = fmt.Fprintf(file, "\n──── %s session start pid=%d ────\n", time.Now().Format(time.RFC3339), os.Getpid())
	return nil
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_, _ = fmt.Fprintf(file, "──── %s session end ────\n", time.Now().Format(time.RFC3339))
		_ = file.Close()
		file = nil
		inited = false
	}
}

func logf(lv Level, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if lv < level {
		return
	}
	if file == nil {
		_ = initUnlocked()
	}
	if file == nil {
		return
	}
	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(file, "%s %-5s %s\n", ts, lv.String(), msg)
	_ = file.Sync()
}

func initUnlocked() error {
	if inited && file != nil {
		return nil
	}
	path := Path()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	file = f
	inited = true
	return nil
}

func Debug(format string, args ...any) { logf(LevelDebug, format, args...) }
func Info(format string, args ...any)  { logf(LevelInfo, format, args...) }
func Warn(format string, args ...any)  { logf(LevelWarn, format, args...) }
func Error(format string, args ...any) { logf(LevelError, format, args...) }

// Panic logs a panic value and stack, then re-panics.
func Panic(recovered any) {
	mu.Lock()
	if file == nil {
		_ = initUnlocked()
	}
	if file != nil {
		_, _ = fmt.Fprintf(file, "%s ERROR panic: %v\n%s\n", time.Now().Format("15:04:05.000"), recovered, debug.Stack())
		_ = file.Sync()
	}
	mu.Unlock()
	panic(recovered)
}

// Guard runs fn and recovers panics, logging them. Returns true if fn completed without panic.
func Guard(where string, fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			Error("panic in %s: %v\n%s", where, r, debug.Stack())
			ok = false
		}
	}()
	fn()
	return true
}

// RecoverAndLog recovers from panic in a defer, logs it, and returns the panic value (nil if none).
func RecoverAndLog(where string) (panicked any) {
	if r := recover(); r != nil {
		Error("panic in %s: %v\n%s", where, r, debug.Stack())
		return r
	}
	return nil
}

// Writer returns an io.Writer that appends as INFO lines (for optional redirection).
func Writer() io.Writer {
	return writer{}
}

type writer struct{}

func (writer) Write(p []byte) (int, error) {
	Info("%s", string(p))
	return len(p), nil
}
