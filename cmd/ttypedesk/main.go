package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/RetroCodeRamen/ttypedesk/internal/attach"
	"github.com/RetroCodeRamen/ttypedesk/internal/client"
	"github.com/RetroCodeRamen/ttypedesk/internal/config"
	"github.com/RetroCodeRamen/ttypedesk/internal/server"
	"github.com/RetroCodeRamen/ttypedesk/internal/slog"
)

// version is baked in at build time via build.sh's -ldflags -X, computed by
// scripts/version.sh as MAJOR.MINOR.YYMMNN (NN = commits this month).
var version = "dev"

// installerURL is the same one-line installer documented in the README —
// re-running it updates an existing checkout (git fetch + reset --hard)
// and rebuilds/reinstalls, so -update just shells out to it rather than
// duplicating that logic here.
const installerURL = "https://raw.githubusercontent.com/RetroCodeRamen/ttypedesk/master/install.sh"

// runUpdate fetches and runs the installer script, inheriting the
// terminal's stdio so build output and any sudo password prompt (for
// missing build deps) show up live. Returns the process exit code.
func runUpdate() int {
	fmt.Fprintln(os.Stderr, "TTYPE Desk: fetching and running the latest installer...")
	cmd := exec.Command("bash", "-c", "curl -fsSL "+installerURL+" | bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
	return 1
}

func main() {
	// A stalled/dropped SSH link can make sshd tear down the session and send
	// SIGHUP to the foreground process group. The OS default disposition is
	// to terminate immediately, skipping every defer (session save, terminal
	// reset) and leaving the remote pty in raw/alt-screen mode. Ignore it and
	// let the tty read/write paths (bounded by deadlineTty, surfaced as
	// *tcell.EventError) drive a clean shutdown instead.
	signal.Ignore(syscall.SIGHUP)
	// clip.Set writes OSC 52 straight to os.Stdout (fd 1). Go specially
	// crashes the whole process on SIGPIPE for writes to fd 1/2 unless told
	// otherwise, so a clipboard sync over a dead link would kill ttypedesk
	// outright with no panic, no log line — indistinguishable from the
	// SIGHUP case above. Ignoring SIGPIPE makes those writes fail with a
	// normal (discarded) EPIPE error instead.
	signal.Ignore(syscall.SIGPIPE)

	listen := flag.String("listen", "", "Unix socket for remote attach (e.g. /tmp/ttypedesk.sock)")
	attachPath := flag.String("attach", "", "attach to an existing TTYPE Desk socket (read-only)")
	execCmd := flag.String("e", "", "command to run in initial terminal (instead of $SHELL)")
	imagePath := flag.String("image", "", "open Image Viewer on this file at startup")
	clock := flag.Bool("clock", false, "open Clock app at startup")
	update := flag.Bool("update", false, "rebuild and reinstall the latest master, then exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("TTYPE Desk " + version)
		return
	}

	if *update {
		os.Exit(runUpdate())
	}

	if err := slog.Init(); err != nil {
		log.Printf("log init: %v", err)
	}
	defer slog.Close()
	slog.Info("ttypedesk starting")

	defer func() {
		if r := recover(); r != nil {
			slog.Error("fatal panic: %v\n%s", r, debug.Stack())
			_, _ = os.Stdout.WriteString("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?25h\x1b[?1049l\x1b[0m\r\n")
			fmt.Fprintf(os.Stderr, "TTYPE Desk crashed — see log: %s\n", slog.Path())
			os.Exit(1)
		}
	}()

	if *attachPath != "" {
		if err := attach.RunViewer(*attachPath); err != nil {
			log.Fatal(err)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Warn("config load: %v (using defaults)", err)
		log.Printf("config: %v (using defaults)", err)
		cfg = config.Default()
	}

	if os.Getenv("COLORTERM") == "" {
		_ = os.Setenv("COLORTERM", "truecolor")
	}

	srv := server.New(cfg)

	if *listen != "" {
		go func() {
			if err := attach.Serve(srv, *listen); err != nil {
				slog.Error("attach server: %v", err)
				log.Printf("attach server: %v", err)
			}
		}()
		fmt.Fprintf(os.Stderr, "TTYPE Desk attach socket: %s\n", *listen)
	}

	boot := client.BootOptions{
		ImagePath: *imagePath,
		OpenClock: *clock,
	}
	if *execCmd != "" {
		parts := splitCommand(*execCmd)
		boot.ExecCommand = parts[0]
		if len(parts) > 1 {
			boot.ExecArgs = parts[1:]
		}
	}
	if flag.NArg() > 0 && boot.ExecCommand == "" {
		boot.ExecCommand = flag.Arg(0)
		boot.ExecArgs = flag.Args()[1:]
	}

	cli, err := client.New(srv, cfg, boot)
	if err != nil {
		slog.Error("client.New: %v", err)
		log.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic during run/teardown: %v\n%s", r, debug.Stack())
			_, _ = os.Stdout.WriteString("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?25h\x1b[?1049l\x1b[0m\r\n")
			cli.Close()
			fmt.Fprintf(os.Stderr, "TTYPE Desk crashed — see log: %s\n", slog.Path())
			os.Exit(1)
		}
		cli.Close()
	}()

	fmt.Fprintf(os.Stderr, "TTYPE Desk — Ctrl+Q quit | F10 / Ctrl+Esc menu | Ctrl+Space palette | Alt+Arrows move\n")
	fmt.Fprintf(os.Stderr, "  Alt+Shift+Arrows resize | Alt+Ctrl+Arrows snap | Ctrl+W close | Ctrl+M minimize | log: %s\n", slog.Path())
	if err := cli.Run(); err != nil {
		slog.Error("client.Run: %v", err)
		log.Fatal(err)
	}
	slog.Info("ttypedesk exit ok")
}

func splitCommand(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}
