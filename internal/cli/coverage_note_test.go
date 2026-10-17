package cli

import (
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/recommend"
)

func TestCoverageNote(t *testing.T) {
	oldest := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		res  recommend.Result
		want string
	}{
		{
			name: "nothing in the window",
			res:  recommend.Result{WindowDays: 7},
			want: "window 7d, no samples in window",
		},
		{
			name: "fewer covered days than the window asks for",
			res:  recommend.Result{WindowDays: 7, DaysWithSamples: 3, OldestSample: oldest},
			want: "window 7d, only 3 days with samples (oldest 2026-10-09) — thinner history than the window implies",
		},
		{
			// A fully covered rolling window touches WindowDays+1 UTC dates: the
			// window clips the first and last day. That is full, not a gap.
			name: "full coverage has one more day than the window length",
			res:  recommend.Result{WindowDays: 7, DaysWithSamples: 8},
			want: "window 7d, 8 days with samples",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := coverageNote(tt.res); got != tt.want {
				t.Errorf("coverageNote:\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}
