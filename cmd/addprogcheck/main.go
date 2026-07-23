package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/server"
	"github.com/ttypedesk/ttypedesk/internal/slog"
)

func main() {
	_ = slog.Init()
	defer slog.Close()
	done := make(chan struct{})
	go func() {
		srv := server.New(config.Default())
		srv.SetHostSize(120, 40)
		if err := srv.LaunchAction("addprog"); err != nil {
			fmt.Fprintf(os.Stderr, "launch: %v\n", err)
			os.Exit(1)
		}
		wins := srv.Windows()
		fmt.Println("windows", len(wins))
		for _, w := range wins {
			fmt.Println(" ", w.ID, w.Title, w.Kind)
			_ = w.Surface.Snapshot()
		}
		close(done)
	}()
	select {
	case <-done:
		fmt.Println("ok")
	case <-time.After(3 * time.Second):
		fmt.Fprintln(os.Stderr, "TIMEOUT — likely deadlock (same bug as Add Program hang)")
		os.Exit(1)
	}
}
