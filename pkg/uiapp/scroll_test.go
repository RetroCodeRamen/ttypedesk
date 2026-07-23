package uiapp

import "testing"

func TestScrollStateClampAndEnsure(t *testing.T) {
	s := ScrollState{Content: 100, Viewport: 10, Offset: 0}
	s.EnsureVisible(50)
	if s.Offset != 41 {
		t.Fatalf("EnsureVisible: got %d want 41", s.Offset)
	}
	s.ScrollBy(1000)
	if s.Offset != 90 {
		t.Fatalf("MaxOffset clamp: got %d want 90", s.Offset)
	}
	s.ScrollBy(-1000)
	if s.Offset != 0 {
		t.Fatalf("min clamp: got %d", s.Offset)
	}
}

func TestThumbGeom(t *testing.T) {
	s := ScrollState{Content: 100, Viewport: 10, Offset: 0}
	ty, th := s.ThumbGeom(20)
	if th < 1 || ty != 0 {
		t.Fatalf("thumb at start: y=%d h=%d", ty, th)
	}
	s.Offset = 90
	ty, th = s.ThumbGeom(20)
	if ty+th != 20 {
		t.Fatalf("thumb at end: y=%d h=%d", ty, th)
	}
}
