//go:build e2e

// Package e2e drives the built kpm binary as a real process, against a real SQLite
// file, asserting real exit codes.
//
// No cluster is involved, and that is the point: everything that can be wrong without
// Kubernetes -- a store written by a newer build, restore idempotence, the backup round
// trip, the exit codes a script branches on -- is checked here on every run.
//
// The cluster path is NOT covered here. It needs a kind cluster plus metrics-server
// (which needs kubelet-insecure-tls patching), and a gate that heavy and that flaky is
// better run deliberately than on every push. What this file deliberately does check is
// that the cluster-facing commands fail cleanly -- with an isolated KUBECONFIG -- rather
// than reporting success against nothing.
package e2e

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/store"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "kpm-e2e")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "kpm")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/kpm")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build kpm: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// result is what a run of the binary produced.
type result struct {
	stdout string
	stderr string
	code   int
}

// run executes the binary with args and never fails on a non-zero exit: the exit code
// is the thing under test.
func run(t *testing.T, args ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = t.TempDir()
	// Hermetic on purpose: cmd.Dir does not isolate kubeconfig resolution, so without
	// this the cluster-facing commands would use whatever cluster the machine running
	// the test happens to have, and "fails without a cluster" would pass or fail
	// depending on who ran it. (Learned the hard way: this test once reached a real
	// cluster named my-k8s.)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+filepath.Join(cmd.Dir, "no-such-kubeconfig"))
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("running %v: %v", args, err)
		}
		code = exit.ExitCode()
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// config writes a minimal config pointing the store at dbPath and returns its path.
func config(t *testing.T, dir, dbPath string) string {
	t.Helper()
	body := fmt.Sprintf("cluster:\n  name: e2e\nclient:\n  id: e2e-client\nstore:\n  path: %q\n", dbPath)
	path := filepath.Join(dir, "kpm.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// seed writes samples for one container across two days: enough for recommend to have
// something to say (min_samples), and stable, so assertions can name the pod.
func seed(t *testing.T, dbPath string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}()
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Minute)
	records := make([]store.Record, 0, 30)
	for i := range 30 {
		records = append(records, store.Record{
			SampledAt:        base.Add(time.Duration(i) * time.Hour),
			ClientID:         "e2e-client",
			ClusterName:      "e2e",
			Namespace:        "default",
			Pod:              "api-0",
			Container:        "api",
			OwnerKind:        "Deployment",
			OwnerName:        "api",
			Node:             "node-a",
			CPUUtilMcores:    120,
			MemUtilBytes:     128 << 20,
			CPURequestMcores: 500,
			MemRequestBytes:  512 << 20,
			CPULimitMcores:   1000,
			MemLimitBytes:    1 << 30,
		})
	}
	if err := st.InsertRecords(ctx, records); err != nil {
		t.Fatalf("insert records: %v", err)
	}
}

func TestRecommendOnMissingStoreExits3(t *testing.T) {
	dir := t.TempDir()
	cfg := config(t, dir, filepath.Join(dir, "absent.db"))

	got := run(t, "recommend", "--config", cfg)
	if got.code != 3 {
		t.Fatalf("empty store must exit 3 (store), got %d\nstdout: %s\nstderr: %s", got.code, got.stdout, got.stderr)
	}
}

func TestRecommendAfterSeed(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.db")
	cfg := config(t, dir, dbPath)
	seed(t, dbPath)

	got := run(t, "recommend", "--config", cfg)
	if got.code != 0 {
		t.Fatalf("recommend must exit 0 with data, got %d\nstdout: %s\nstderr: %s", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stdout, "api-0") {
		t.Fatalf("recommend output does not mention the seeded pod:\n%s", got.stdout)
	}
}

func TestExportCSV(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.db")
	cfg := config(t, dir, dbPath)
	seed(t, dbPath)

	out := filepath.Join(dir, "dump.csv")
	got := run(t, "export", "--config", cfg, "--format", "csv", "-o", out)
	if got.code != 0 {
		t.Fatalf("export must exit 0, got %d\nstderr: %s", got.code, got.stderr)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !strings.Contains(string(body), "api-0") {
		t.Fatalf("export without the seeded pod:\n%s", body)
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	srcDB := filepath.Join(dir, "metrics.db")
	srcCfg := config(t, dir, srcDB)
	seed(t, srcDB)

	backup := filepath.Join(dir, "backup.db")
	if got := run(t, "backup", "--config", srcCfg, "-o", backup); got.code != 0 {
		t.Fatalf("backup must exit 0, got %d\nstderr: %s", got.code, got.stderr)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	// Restore into a store that does not exist yet, then read it back with a fresh
	// process: the point is that the data survives the crossing.
	dstDir := t.TempDir()
	dstDB := filepath.Join(dstDir, "restored.db")
	dstCfg := config(t, dstDir, dstDB)
	if got := run(t, "restore", backup, "--config", dstCfg); got.code != 0 {
		t.Fatalf("restore must exit 0, got %d\nstderr: %s", got.code, got.stderr)
	}

	got := run(t, "recommend", "--config", dstCfg)
	if got.code != 0 {
		t.Fatalf("recommend on the restored store must exit 0, got %d\nstdout: %s\nstderr: %s", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stdout, "api-0") {
		t.Fatalf("restored store lost the pod:\n%s", got.stdout)
	}
}

func TestRestoreMergeIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.db")
	cfg := config(t, dir, dbPath)
	seed(t, dbPath)

	backup := filepath.Join(dir, "backup.db")
	if got := run(t, "backup", "--config", cfg, "-o", backup); got.code != 0 {
		t.Fatalf("backup must exit 0, got %d\nstderr: %s", got.code, got.stderr)
	}

	first := run(t, "restore", backup, "--merge", "--config", cfg)
	if first.code != 0 {
		t.Fatalf("first merge must exit 0, got %d\nstderr: %s", first.code, first.stderr)
	}
	second := run(t, "restore", backup, "--merge", "--config", cfg)
	if second.code != 0 {
		t.Fatalf("second merge must exit 0, got %d\nstderr: %s", second.code, second.stderr)
	}
	// Merging the same rows twice must add nothing the second time: that is the whole
	// contract behind deduping on the natural key.
	if !strings.Contains(second.stdout, "samples         +0") {
		t.Fatalf("second merge was not idempotent, output:\n%s", second.stdout)
	}
}

func TestNewerSchemaIsRefusedWithoutBeingTouched(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.db")
	cfg := config(t, dir, dbPath)
	seed(t, dbPath)

	// Pretend a future kpm wrote this store.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open for bump: %v", err)
	}
	if _, err := db.Exec(`UPDATE schema_meta SET version = 99`); err != nil {
		_ = db.Close()
		t.Fatalf("bump schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	got := run(t, "recommend", "--config", cfg)
	if got.code == 0 {
		t.Fatalf("a store from a newer build must not be readable, output:\n%s", got.stdout)
	}
	combined := got.stdout + got.stderr
	if !strings.Contains(combined, "newer") && !strings.Contains(combined, "upgrade") {
		t.Fatalf("refusal must say the schema is newer, got:\n%s", combined)
	}

	// The file must be left exactly as it was found.
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db.Close() }()
	var version int
	if err := db.QueryRow(`SELECT version FROM schema_meta`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 99 {
		t.Fatalf("refusal mutated the store: version is now %d", version)
	}
}

func TestCapacityFitWithoutClusterFails(t *testing.T) {
	dir := t.TempDir()
	cfg := config(t, dir, filepath.Join(dir, "metrics.db"))

	got := run(t, "capacity", "fit", "--config", cfg, "--cpu", "1")
	if got.code == 0 {
		t.Fatalf("capacity fit without a cluster must not report success:\n%s%s", got.stdout, got.stderr)
	}
}
