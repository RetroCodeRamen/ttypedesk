package ffdecode

import (
	"fmt"
	"time"
)

// seekArgs returns the ffmpeg args to seek to seek before -i (an input-
// side seek — fast/keyframe-ish, not frame-accurate, which is the right
// tradeoff for interactive scrubbing rather than exact-position seeking).
// Returns nil for seek<=0, so the resulting command line is identical to
// not seeking at all.
func seekArgs(seek time.Duration) []string {
	if seek <= 0 {
		return nil
	}
	return []string{"-ss", fmt.Sprintf("%.3f", seek.Seconds())}
}
