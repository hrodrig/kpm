package recommend

import "testing"

func TestYAMLQuote(t *testing.T) {
	cases := map[string]string{
		`plain`:       `"plain"`,
		`a"b`:         `"a\"b"`,
		`a\b`:         `"a\\b"`,
		"line\nbreak": `"line\nbreak"`,
		"tab\there":   `"tab\there"`,
		"ret\r":       `"ret\r"`,
	}
	for in, want := range cases {
		if got := yamlQuote(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}
