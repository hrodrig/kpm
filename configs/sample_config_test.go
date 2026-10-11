package configs

import (
	"strings"
	"testing"
)

func TestSampleYAML(t *testing.T) {
	s := SampleYAML()
	if !strings.Contains(s, "kpm sample configuration") {
		t.Fatalf("unexpected sample: %q", s)
	}
	if !strings.Contains(s, "store:") {
		t.Fatal("sample missing store section")
	}
}
