package uiapp

import (
	"testing"
	"time"
)

func TestMediaClockStartsPausedAtZero(t *testing.T) {
	m := NewMediaClock()
	if m.Playing() {
		t.Error("Playing() = true, want false at start")
	}
	if m.Position() != 0 {
		t.Errorf("Position() = %v, want 0", m.Position())
	}
}

func TestMediaClockAdvancesWhilePlaying(t *testing.T) {
	m := NewMediaClock()
	m.Play()
	time.Sleep(30 * time.Millisecond)
	pos := m.Position()
	if pos < 20*time.Millisecond {
		t.Errorf("Position() = %v, want at least ~20ms after playing for 30ms", pos)
	}
}

func TestMediaClockFreezesOnPause(t *testing.T) {
	m := NewMediaClock()
	m.Play()
	time.Sleep(20 * time.Millisecond)
	m.Pause()
	frozen := m.Position()
	time.Sleep(20 * time.Millisecond)
	if m.Position() != frozen {
		t.Errorf("Position() after pause changed: %v -> %v", frozen, m.Position())
	}
	if m.Playing() {
		t.Error("Playing() = true after Pause()")
	}
}

func TestMediaClockToggle(t *testing.T) {
	m := NewMediaClock()
	m.Toggle()
	if !m.Playing() {
		t.Error("Toggle() from paused: want playing")
	}
	m.Toggle()
	if m.Playing() {
		t.Error("Toggle() from playing: want paused")
	}
}

func TestMediaClockSetPosition(t *testing.T) {
	m := NewMediaClock()
	m.SetPosition(5 * time.Second)
	if m.Position() != 5*time.Second {
		t.Errorf("Position() = %v, want 5s", m.Position())
	}
	m.Play()
	time.Sleep(10 * time.Millisecond)
	if m.Position() < 5*time.Second {
		t.Errorf("Position() = %v, want >= 5s after playing from a seek", m.Position())
	}
}

func TestMediaClockPlayIsIdempotent(t *testing.T) {
	m := NewMediaClock()
	m.Play()
	time.Sleep(10 * time.Millisecond)
	m.Play() // should not reset lastMark
	pos := m.Position()
	if pos < 8*time.Millisecond {
		t.Errorf("Position() = %v, want ~10ms+ (second Play() should be a no-op)", pos)
	}
}
