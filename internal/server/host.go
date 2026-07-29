package server

import (
	"github.com/ttypedesk/ttypedesk/internal/audio"
	"github.com/ttypedesk/ttypedesk/internal/credstore"
	"github.com/ttypedesk/ttypedesk/internal/slog"
	"github.com/ttypedesk/ttypedesk/pkg/uiapp"
)

// appHost exposes desktop services to a native uiapp.
type appHost struct {
	srv *Server
	id  string
}

func (h *appHost) Notify(title, body, icon string) {
	if h.srv.notify != nil {
		h.srv.notify.Post(title, body, icon, "app:"+h.id)
	}
}

func (h *appHost) Launch(action string) error {
	return h.srv.LaunchAction(action)
}

func (h *appHost) OpenPath(path string) error {
	return h.srv.OpenPath(path)
}

func (h *appHost) SetTitle(title string) {
	h.srv.SetWindowTitle(h.id, title)
}

func (h *appHost) RequestClose() {
	h.srv.CloseWindow(h.id)
}

func (h *appHost) WindowID() string { return h.id }

func (h *appHost) SaveCredential(key string, value []byte) error {
	return credstore.Save(key, value)
}

func (h *appHost) LoadCredential(key string) ([]byte, error) {
	return credstore.Load(key)
}

func (h *appHost) PickFile(startDir string, extensions []string, onResult func(path string, ok bool)) {
	if _, err := h.srv.CreateFilePicker(startDir, extensions, onResult); err != nil {
		slog.Error("PickFile failed: %v", err)
		if onResult != nil {
			onResult("", false)
		}
	}
}

func (h *appHost) PlayAudio(pcm <-chan []int16) (uiapp.AudioPlayback, error) {
	return audio.Play(pcm)
}

var _ uiapp.Host = (*appHost)(nil)
