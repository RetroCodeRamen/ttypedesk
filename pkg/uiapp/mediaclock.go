package uiapp

import (
	"sync"
	"time"
)

// MediaClock is a play/pause/position ticker for transport UI and frame
// pacing — used by Amp (audio) and Vid (video), not backed by any Host
// resource, so it's a plain constructable type rather than routed through
// Host: there's no shared desktop-level state here, just local timing math
// each window's own player state needs.
type MediaClock struct {
	mu       sync.Mutex
	playing  bool
	pos      time.Duration // position as of the last state change
	lastMark time.Time     // wall-clock time pos was last accurate
}

// NewMediaClock returns a clock starting paused at position zero.
func NewMediaClock() *MediaClock {
	return &MediaClock{lastMark: time.Now()}
}

// Play resumes from the current position.
func (m *MediaClock) Play() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.playing {
		return
	}
	m.playing = true
	m.lastMark = time.Now()
}

// Pause freezes the current position.
func (m *MediaClock) Pause() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.playing {
		return
	}
	m.pos += time.Since(m.lastMark)
	m.playing = false
}

// Toggle switches between Play and Pause.
func (m *MediaClock) Toggle() {
	m.mu.Lock()
	playing := m.playing
	m.mu.Unlock()
	if playing {
		m.Pause()
	} else {
		m.Play()
	}
}

// Playing reports whether the clock is currently running.
func (m *MediaClock) Playing() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.playing
}

// Position returns the current elapsed position.
func (m *MediaClock) Position() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.playing {
		return m.pos
	}
	return m.pos + time.Since(m.lastMark)
}

// SetPosition jumps to d (e.g. after a scrub/seek), preserving play state.
func (m *MediaClock) SetPosition(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pos = d
	m.lastMark = time.Now()
}
