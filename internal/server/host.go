package server

import "github.com/ttypedesk/ttypedesk/pkg/uiapp"

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

var _ uiapp.Host = (*appHost)(nil)
