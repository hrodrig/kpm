package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/store"
)

// fixedNow keeps the coverage arithmetic deterministic across test cases.
var fixedNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func writeConfig(t *testing.T, dir, storePath string, retention, window int) string {
	t.Helper()
	path := filepath.Join(dir, "kpm.yml")
	body := fmt.Sprintf("store:\n  path: %s\n  retention_days: %d\nrecommend:\n  window_days: %d\n",
		storePath, retention, window)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// seededStore writes a store at dir/metrics.db holding one sample per day for
// the last `days` days, and returns its path.
func seededStore(t *testing.T, dir string, days int) string {
	t.Helper()
	path := filepath.Join(dir, "metrics.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	recs := []store.Record{}
	for i := range days {
		recs = append(recs, store.Record{
			SampledAt:     fixedNow.AddDate(0, 0, -i),
			Namespace:     "app",
			Pod:           "p",
			Container:     "c",
			OwnerKind:     "Deployment",
			OwnerName:     "web",
			CPUUtilMcores: 10,
			MemUtilBytes:  1 << 20,
		})
	}
	if err := s.InsertRecords(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	return path
}

func findCheck(t *testing.T, r Report, name string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q missing from report: %v", name, r.Checks)
	return Check{}
}

// #25 + #94: the targets check must report the resolved per-resource values, not
// just echo the shared default.
func TestRunReportsPerResourceTargets(t *testing.T) {
	dir := t.TempDir()
	storePath := seededStore(t, dir, 8)
	cfgPath := filepath.Join(dir, "targets.yml")
	body := fmt.Sprintf(`store:
  path: %s
  retention_days: 14
recommend:
  window_days: 7
  target_utilization: 0.70
  cpu_target_utilization: 0.50
  memory_target_utilization: 0.90
`, storePath)
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	r := Run(context.Background(), Options{ConfigPath: cfgPath, Now: fixedNow})

	check := findCheck(t, r, CheckConfigTargets)
	if !strings.Contains(check.Detail, "cpu target 0.50") {
		t.Fatalf("cpu target not resolved: %q", check.Detail)
	}
	if !strings.Contains(check.Detail, "memory target 0.90") {
		t.Fatalf("memory target not resolved: %q", check.Detail)
	}
}

func TestRunMissingConfigFailsAndSkipsStore(t *testing.T) {
	dir := t.TempDir()
	r := Run(context.Background(), Options{ConfigPath: filepath.Join(dir, "absent.yml")})

	if got := findCheck(t, r, CheckConfigLoad).Status; got != Fail {
		t.Fatalf("config.load: got %s want fail", got)
	}
	if got := findCheck(t, r, CheckStorePath).Status; got != Skip {
		t.Fatalf("store.path: got %s want skip", got)
	}
	if !r.Broken() {
		t.Fatal("a config that does not load must be broken")
	}
	if got := r.ExitCode(); got != 1 {
		t.Fatalf("exit code: got %d want 1", got)
	}
}

// The command must stay read-only: inspecting a store that does not exist cannot
// create it.
func TestRunDoesNotCreateMissingStore(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "absent.db")
	cfgPath := writeConfig(t, dir, storePath, 14, 7)

	r := Run(context.Background(), Options{ConfigPath: cfgPath})

	if got := findCheck(t, r, CheckStorePath).Status; got != Fail {
		t.Fatalf("store.path: got %s want fail", got)
	}
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatal("doctor created the store it was asked to inspect")
	}
}

func TestRunEmptyStoreWarnsInsteadOfFailing(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, seededStore(t, dir, 0), 14, 7)

	r := Run(context.Background(), Options{ConfigPath: cfgPath, Now: fixedNow})

	if got := findCheck(t, r, CheckStoreSamples).Status; got != Warn {
		t.Fatalf("store.samples: got %s want warn", got)
	}
	if got := findCheck(t, r, CheckCoverage).Status; got != Skip {
		t.Fatalf("history.coverage: got %s want skip", got)
	}
	if r.Broken() {
		t.Fatal("an empty store is a warning, not a failure")
	}
	if got := r.ExitCode(); got != 0 {
		t.Fatalf("exit code: got %d want 0", got)
	}
}

// A fully covered rolling window touches windowDays+1 UTC days, so 8 days of
// samples fill a 7d window and must not warn.
func TestRunFullCoverageIsOK(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, seededStore(t, dir, 8), 14, 7)

	r := Run(context.Background(), Options{ConfigPath: cfgPath, Now: fixedNow})

	coverage := findCheck(t, r, CheckCoverage)
	if coverage.Status != OK {
		t.Fatalf("history.coverage: got %s (%s)", coverage.Status, coverage.Detail)
	}
	if r.Broken() {
		t.Fatalf("unexpected failures: %v", r.Checks)
	}
}

func TestRunThinCoverageWarns(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, seededStore(t, dir, 3), 14, 7)

	r := Run(context.Background(), Options{ConfigPath: cfgPath, Now: fixedNow})

	coverage := findCheck(t, r, CheckCoverage)
	if coverage.Status != Warn {
		t.Fatalf("history.coverage: got %s want warn", coverage.Status)
	}
	if !strings.Contains(coverage.Detail, "3 days with samples") {
		t.Fatalf("coverage detail: %q", coverage.Detail)
	}
	if r.Broken() {
		t.Fatal("thin coverage must warn, not fail")
	}
}

// The #24 relation is guidance, so doctor warns here too.
func TestRunRetentionThinnerThanWindowWarns(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, seededStore(t, dir, 3), 3, 7)

	r := Run(context.Background(), Options{ConfigPath: cfgPath, Now: fixedNow})

	retention := findCheck(t, r, CheckConfigRetain)
	if retention.Status != Warn {
		t.Fatalf("config.retention: got %s want warn", retention.Status)
	}
	if r.Broken() {
		t.Fatal("a thin retention warning must not fail the run")
	}
}

// Every check must appear exactly once: duplicate rows would make the output
// ambiguous to grep.
func TestRunReportsEachCheckOnce(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, seededStore(t, dir, 3), 14, 7)

	r := Run(context.Background(), Options{ConfigPath: cfgPath, Now: fixedNow})

	seen := map[string]int{}
	for _, c := range r.Checks {
		seen[c.Name]++
	}
	for _, name := range []string{
		CheckConfigLoad, CheckConfigRetain, CheckConfigTargets, CheckStorePath,
		CheckStoreSchema, CheckStoreSamples, CheckStoreBounds, CheckCoverage, CheckMetricsAPI,
	} {
		if seen[name] != 1 {
			t.Fatalf("%s appears %d times, want 1 (report: %v)", name, seen[name], r.Checks)
		}
	}
}

func TestExitCodeClusterOnlyFailureIsCluster(t *testing.T) {
	clusterOnly := Report{Checks: []Check{
		{Name: CheckConfigLoad, Status: OK},
		{Name: CheckMetricsAPI, Status: Fail},
	}}
	clusterOnly.summarise()
	if !clusterOnly.Broken() || clusterOnly.ExitCode() != 2 {
		t.Fatalf("cluster-only failure: broken=%v code=%d", clusterOnly.Broken(), clusterOnly.ExitCode())
	}

	mixed := Report{Checks: []Check{
		{Name: CheckConfigLoad, Status: Fail},
		{Name: CheckMetricsAPI, Status: Fail},
	}}
	mixed.summarise()
	if got := mixed.ExitCode(); got != 1 {
		t.Fatalf("mixed failure: got %d want 1", got)
	}

	warnOnly := Report{Checks: []Check{{Name: CheckCoverage, Status: Warn}}}
	warnOnly.summarise()
	if warnOnly.Broken() || warnOnly.ExitCode() != 0 {
		t.Fatalf("warn-only report must not fail: broken=%v code=%d", warnOnly.Broken(), warnOnly.ExitCode())
	}
}

func TestFormatTableAndJSON(t *testing.T) {
	r := Report{Checks: []Check{
		{Name: CheckConfigLoad, Status: OK, Detail: "/tmp/kpm.yml (valid)"},
		{Name: CheckCoverage, Status: Warn, Detail: "3 days with samples in a 7d window"},
	}}
	r.summarise()

	var table bytes.Buffer
	if err := FormatTable(&table, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CHECK", "config.load", "history.coverage",
		"doctor: 1 ok, 1 warn, 0 fail, 0 skipped (status warn)",
	} {
		if !strings.Contains(table.String(), want) {
			t.Fatalf("table missing %q:\n%s", want, table.String())
		}
	}

	var encoded bytes.Buffer
	if err := FormatJSON(&encoded, r); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != "warn" || len(decoded.Checks) != 2 || decoded.Counts.Warn != 1 {
		t.Fatalf("json round-trip: %+v", decoded)
	}
	if !strings.Contains(encoded.String(), `"days_with_samples"`) &&
		!strings.Contains(encoded.String(), `"history.coverage"`) {
		t.Fatalf("json shape unexpected:\n%s", encoded.String())
	}
}
