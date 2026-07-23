package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Notice is a system notification.
type Notice struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	Icon    string    `json:"icon"`
	Created time.Time `json:"created"`
	Source  string    `json:"source"`
	Read    bool      `json:"read"`
}

// Service is the desktop-wide notification bus.
type Service struct {
	mu        sync.Mutex
	items     []Notice
	maxHist   int
	persist   bool
	path      string
	listeners []func(Notice)
	seq       atomic.Uint64
}

// DefaultPath is ~/.config/ttypedesk/notifications.json
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "notifications.json"
	}
	return filepath.Join(home, ".config", "ttypedesk", "notifications.json")
}

func New() *Service {
	return &Service{maxHist: 50, path: DefaultPath()}
}

// Configure enables/disables disk history and reloads when turning persist on.
func (s *Service) Configure(persist bool, maxHist int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxHist <= 0 {
		maxHist = 50
	}
	if maxHist > 500 {
		maxHist = 500
	}
	was := s.persist
	s.persist = persist
	s.maxHist = maxHist
	if persist && !was && len(s.items) == 0 {
		s.loadLocked()
	}
	s.trimLocked()
	if persist {
		s.saveLocked()
	}
}

func (s *Service) Load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.persist {
		s.loadLocked()
	}
}

func (s *Service) loadLocked() {
	data, err := os.ReadFile(s.path)
	if err != nil || len(data) == 0 {
		return
	}
	var items []Notice
	if err := json.Unmarshal(data, &items); err != nil {
		return
	}
	s.items = items
	s.trimLocked()
	var maxSeq uint64
	for _, it := range s.items {
		var n uint64
		if _, err := fmt.Sscanf(it.ID, "n%d", &n); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	if maxSeq > s.seq.Load() {
		s.seq.Store(maxSeq)
	}
}

func (s *Service) saveLocked() {
	if !s.persist || s.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0o644)
}

func (s *Service) trimLocked() {
	if s.maxHist < 1 {
		s.maxHist = 50
	}
	if len(s.items) > s.maxHist {
		s.items = s.items[:s.maxHist]
	}
}

// Post posts a notice and invokes subscribers.
func (s *Service) Post(title, body, icon, source string) Notice {
	s.mu.Lock()
	n := Notice{
		ID:      fmt.Sprintf("n%d", s.seq.Add(1)),
		Title:   title,
		Body:    body,
		Icon:    icon,
		Created: time.Now(),
		Source:  source,
	}
	s.items = append([]Notice{n}, s.items...)
	s.trimLocked()
	s.saveLocked()
	listeners := append([]func(Notice){}, s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(n)
	}
	return n
}

func (s *Service) Subscribe(fn func(Notice)) {
	s.mu.Lock()
	s.listeners = append(s.listeners, fn)
	s.mu.Unlock()
}

func (s *Service) List() []Notice {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Notice, len(s.items))
	copy(out, s.items)
	return out
}

func (s *Service) Unread() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, it := range s.items {
		if !it.Read {
			n++
		}
	}
	return n
}

func (s *Service) Dismiss(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dst := s.items[:0]
	for _, it := range s.items {
		if it.ID != id {
			dst = append(dst, it)
		}
	}
	s.items = dst
	s.saveLocked()
}

func (s *Service) DismissAll() {
	s.mu.Lock()
	s.items = nil
	s.saveLocked()
	s.mu.Unlock()
}

func (s *Service) MarkAllRead() {
	s.mu.Lock()
	for i := range s.items {
		s.items[i].Read = true
	}
	s.saveLocked()
	s.mu.Unlock()
}
