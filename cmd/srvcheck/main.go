package main

import (
	"fmt"

	"github.com/ttypedesk/ttypedesk/internal/config"
	"github.com/ttypedesk/ttypedesk/internal/server"
)

func main() {
	cfg := config.Default()
	srv := server.New(cfg)
	srv.SetHostSize(120, 40)
	defer srv.CloseAll()

	w1, err := srv.CreatePty("Terminal")
	if err != nil {
		panic(err)
	}
	w2, err := srv.CreateApp("clock", "Clock")
	if err != nil {
		panic(err)
	}
	w3, err := srv.CreateGfx("Image Viewer", "")
	if err != nil {
		panic(err)
	}
	wins := srv.Windows()
	fmt.Printf("ok: windows=%d focus=%s\n", len(wins), srv.Focused())
	fmt.Printf("  %s kind=%s\n", w1.ID, w1.Kind)
	fmt.Printf("  %s kind=%s title=%s\n", w2.ID, w2.Kind, w2.Title)
	fmt.Printf("  %s kind=%s title=%s cells=%d\n", w3.ID, w3.Kind, w3.Title, len(w3.Surface.Snapshot()))
	snap := srv.Snapshot()
	fmt.Printf("ok: snapshot windows=%d\n", len(snap.Windows))
}
