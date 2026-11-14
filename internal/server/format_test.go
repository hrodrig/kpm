package server

import "testing"

func TestFormatIntCommas(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{12, "12"},
		{999, "999"},
		{1000, "1,000"},
		{100000, "100,000"},
		{376059, "376,059"},
		{-2600, "-2,600"},
	}
	for _, tc := range cases {
		if got := formatIntCommas(tc.n); got != tc.want {
			t.Fatalf("formatIntCommas(%d)=%q want %q", tc.n, got, tc.want)
		}
	}
}
