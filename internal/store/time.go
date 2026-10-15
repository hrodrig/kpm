package store

import (
	"fmt"
	"time"
)

// UTCLayout is a fixed-width UTC timestamp for SQLite TEXT columns.
// Lexicographic order matches chronological order (including same-second nanos).
const UTCLayout = "2006-01-02T15:04:05.000000000Z"

// FormatUTC formats t as UTCLayout.
func FormatUTC(t time.Time) string {
	return t.UTC().Format(UTCLayout)
}

// ParseUTC parses a store timestamp. New rows use UTCLayout; legacy RFC3339 /
// RFC3339Nano rows from older kpm versions are still accepted on read.
func ParseUTC(s string) (time.Time, error) {
	if t, err := time.Parse(UTCLayout, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("parse store time %q", s)
}
