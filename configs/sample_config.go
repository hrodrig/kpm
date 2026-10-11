// Package configs embeds the sample configuration referenced by
// `kpm --print-sample-config` (and the configs/kpm.sample.yml artifact).
package configs

import _ "embed"

//go:embed kpm.sample.yml
var sampleYAML string

// SampleYAML returns the annotated sample config as a string.
func SampleYAML() string {
	return sampleYAML
}
