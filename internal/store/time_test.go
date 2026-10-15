package store

import (
	"testing"
	"time"
)

func TestFormatParseUTCRoundTrip(t *testing.T) {
	in := time.Date(2026, 10, 14, 12, 0, 0, 123456789, time.UTC)
	s := FormatUTC(in)
	if len(s) != len("2006-01-02T15:04:05.000000000Z") {
		t.Fatalf("fixed width: %q", s)
	}
	got, err := ParseUTC(s)
	if err != nil || !got.Equal(in) {
		t.Fatalf("round-trip: %v %v", got, err)
	}
}

func TestParseUTCLegacyLayouts(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{"2026-10-14T12:00:00.123456789Z", time.Date(2026, 10, 14, 12, 0, 0, 123456789, time.UTC)},
		{"2026-10-14T12:00:00Z", time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)},
		{"2026-10-14T12:00:00.1Z", time.Date(2026, 10, 14, 12, 0, 0, 100000000, time.UTC)},
	}
	for _, tc := range cases {
		got, err := ParseUTC(tc.in)
		if err != nil || !got.Equal(tc.want) {
			t.Fatalf("%q: got %v err=%v want %v", tc.in, got, err, tc.want)
		}
	}
	if _, err := ParseUTC("not-a-time"); err == nil {
		t.Fatal("want parse error")
	}
}
