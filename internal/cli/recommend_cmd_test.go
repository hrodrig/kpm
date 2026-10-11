package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/recommend"
	"github.com/hrodrig/kpm/internal/store"
)

func TestRecommendHelp(t *testing.T) {
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"recommend", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "min_samples") && !strings.Contains(buf.String(), "Dual grain") {
		t.Fatalf("help: %q", buf.String())
	}
}

func TestRecommendMissingConfig(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"recommend", "--config", filepath.Join(t.TempDir(), "no.yaml")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteRecommendInsufficient(t *testing.T) {
	var out, errOut bytes.Buffer
	err := writeRecommend(&out, &errOut, recommend.Result{MinSamples: 24, SkippedKeys: 2}, "table")
	if err == nil || exitcode.Of(err) != exitcode.Store {
		t.Fatalf("want store exit: %v", err)
	}
}

func TestWriteRecommendTable(t *testing.T) {
	var out, errOut bytes.Buffer
	res := recommend.Result{
		Advice: []recommend.Advice{{
			Grain: recommend.GrainPod, Key: "a/b/c", Samples: 24,
			CPUPercentile: 10, CPURequest: 100, CPUSuggested: 20, CPUVerdictReq: recommend.VerdictOversized,
		}},
	}
	if err := writeRecommend(&out, &errOut, res, "table"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "a/b/c") {
		t.Fatalf("out: %q", out.String())
	}
}

func TestWriteRecommendBadFormat(t *testing.T) {
	err := writeRecommend(&bytes.Buffer{}, &bytes.Buffer{}, recommend.Result{
		Advice: []recommend.Advice{{Key: "x"}},
	}, "json")
	if err == nil {
		t.Fatal("expected format error")
	}
}

func TestRecommendEndToEndStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var recs []store.Record
	for i := 0; i < 30; i++ {
		recs = append(recs, store.Record{
			SampledAt: now.Add(-time.Duration(i) * time.Minute),
			Namespace: "app", Pod: "p", Container: "c",
			OwnerKind: "Deployment", OwnerName: "web",
			CPUUtilMcores: 40, MemUtilBytes: 16 << 20,
			CPURequestMcores: 200, MemRequestBytes: 128 << 20,
		})
	}
	if err := st.InsertRecords(t.Context(), recs); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	cfgPath := filepath.Join(dir, "kpm.yaml")
	yaml := `
cluster:
  name: t
store:
  path: "` + dbPath + `"
  retention_days: 14
sample:
  interval_seconds: 0
recommend:
  window_days: 7
  percentile: 95
  min_samples: 24
  target_utilization: 0.70
  cpu_oversize_ratio: 0.35
  cpu_undersize_ratio: 0.85
  cpu_limit_oversize_ratio: 0.35
  cpu_limit_undersize_ratio: 0.85
  memory_oversize_ratio: 0.35
  memory_undersize_ratio: 0.85
  memory_limit_oversize_ratio: 0.35
  memory_limit_undersize_ratio: 0.85
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"recommend", "--config", cfgPath, "--format", "tsv"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("recommend: %v (%s)", err, errBuf.String())
	}
	if !strings.Contains(out.String(), "workload") || !strings.Contains(out.String(), "pod") {
		t.Fatalf("tsv: %q", out.String())
	}
}

func TestRecommendRecordAndScore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var recs []store.Record
	for i := 0; i < 30; i++ {
		recs = append(recs, store.Record{
			SampledAt: now.Add(-time.Duration(i) * time.Minute),
			Namespace: "app", Pod: "p", Container: "c",
			OwnerKind: "Deployment", OwnerName: "web",
			CPUUtilMcores: 40, MemUtilBytes: 16 << 20,
			CPURequestMcores: 200, MemRequestBytes: 128 << 20,
		})
	}
	if err := st.InsertRecords(t.Context(), recs); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	cfgPath := filepath.Join(dir, "kpm.yaml")
	yaml := `
cluster:
  name: demo
client:
  id: bastion-1
store:
  path: "` + dbPath + `"
  retention_days: 14
sample:
  interval_seconds: 0
recommend:
  window_days: 7
  percentile: 95
  min_samples: 24
  target_utilization: 0.70
  cpu_oversize_ratio: 0.35
  cpu_undersize_ratio: 0.85
  cpu_limit_oversize_ratio: 0.35
  cpu_limit_undersize_ratio: 0.85
  memory_oversize_ratio: 0.35
  memory_undersize_ratio: 0.85
  memory_limit_oversize_ratio: 0.35
  memory_limit_undersize_ratio: 0.85
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"recommend", "--config", cfgPath, "--record"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("record: %v (%s)", err, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "recorded") {
		t.Fatalf("stderr: %q", errBuf.String())
	}

	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := st2.LatestRecommendations(t.Context(), "")
	_ = st2.Close()
	if err != nil || len(latest) < 1 {
		t.Fatalf("persisted: %d %v", len(latest), err)
	}

	cmd2 := NewRootCmd()
	out2 := &bytes.Buffer{}
	err2 := &bytes.Buffer{}
	cmd2.SetOut(out2)
	cmd2.SetErr(err2)
	cmd2.SetArgs([]string{"recommend", "--config", cfgPath, "--score"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("score: %v (%s)", err, err2.String())
	}
	if !strings.Contains(out2.String(), "GRAIN") {
		t.Fatalf("score out: %q", out2.String())
	}
}

func TestRecommendRecordScoreExclusive(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"recommend", "--record", "--score", "--config", filepath.Join(t.TempDir(), "x.yaml")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected mutual exclusion error")
	}
}
