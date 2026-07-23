package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/ttypedesk/ttypedesk/internal/attach"
	"github.com/ttypedesk/ttypedesk/internal/client"
	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/server"
	"github.com/ttypedesk/ttypedesk/internal/slog"
)

func main() {
	listen := flag.String("listen", "", "Unix socket for remote attach (e.g. /tmp/ttypedesk.sock)")
	attachPath := flag.String("attach", "", "attach to an existing TTYPE Desk socket (read-only)")
	execCmd := flag.String("e", "", "command to run in initial terminal (instead of $SHELL)")
	imagePath := flag.String("image", "", "open Image Viewer on this file at startup")
	clock := flag.Bool("clock", false, "open Clock app at startup")
	flag.Parse()

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
