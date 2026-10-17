package config

import (
	"os"
	"path/filepath"
	"testing"
)

// #25: CPU and memory are sized against different targets. The shared
// target_utilization stays the default so existing configs are unaffected.
func TestRecommendUtilTargets(t *testing.T) {
	cases := []struct {
		name             string
		rec              Recommend
		wantCPU, wantMem float64
	}{
		{"shared only", Recommend{TargetUtilization: 0.70}, 0.70, 0.70},
		{"cpu override", Recommend{TargetUtilization: 0.70, CPUTargetUtilization: 0.50}, 0.50, 0.70},
		{"memory override", Recommend{TargetUtilization: 0.70, MemoryTargetUtilization: 0.90}, 0.70, 0.90},
		{
			"both override",
			Recommend{TargetUtilization: 0.70, CPUTargetUtilization: 0.50, MemoryTargetUtilization: 0.90},
			0.50, 0.90,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rec.CPUUtilTarget(); got != tc.wantCPU {
				t.Fatalf("CPUUtilTarget() = %v, want %v", got, tc.wantCPU)
			}
			if got := tc.rec.MemUtilTarget(); got != tc.wantMem {
				t.Fatalf("MemUtilTarget() = %v, want %v", got, tc.wantMem)
			}
		})
	}
}

func writeTargetConfig(t *testing.T, name, extra string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	body := `
store:
  path: ` + filepath.Join(dir, "m.db") + `
  retention_days: 14
sample:
  interval_seconds: 0
` + extra
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAcceptsSplitTargets(t *testing.T) {
	cfg, err := Load(writeTargetConfig(t, "split.yaml", `
recommend:
  target_utilization: 0.70
  cpu_target_utilization: 0.50
  memory_target_utilization: 0.90
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Recommend.CPUUtilTarget(); got != 0.50 {
		t.Fatalf("cpu target: %v", got)
	}
	if got := cfg.Recommend.MemUtilTarget(); got != 0.90 {
		t.Fatalf("mem target: %v", got)
	}
}

// 0 means "inherit", so an explicit zero must not be rejected as out of range.
func TestLoadSplitTargetZeroInherits(t *testing.T) {
	cfg, err := Load(writeTargetConfig(t, "zero.yaml", `
recommend:
  target_utilization: 0.70
  cpu_target_utilization: 0
`))
	if err != nil {
		t.Fatalf("zero must mean inherit, not an error: %v", err)
	}
	if got := cfg.Recommend.CPUUtilTarget(); got != 0.70 {
		t.Fatalf("cpu target should inherit 0.70, got %v", got)
	}
}

func TestLoadRejectsOutOfRangeSplitTargets(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"cpu", `
recommend:
  cpu_target_utilization: 1.5
`},
		{"cpu-negative", `
recommend:
  cpu_target_utilization: -0.2
`},
		{"memory", `
recommend:
  memory_target_utilization: 1.5
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(writeTargetConfig(t, tc.name+".yaml", tc.body)); err == nil {
				t.Fatalf("expected an error for %s", tc.name)
			}
		})
	}
}
