package server

import (
	"strconv"
	"strings"
)

// formatIntCommas renders n with en-US thousands separators (e.g. 100000 → "100,000").
func formatIntCommas(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/3)
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	out := b.String()
	if neg {
		return "-" + out
	}
	return out
}
