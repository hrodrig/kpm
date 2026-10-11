package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecommendationsSchemaVersion(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var ver int
	if err := st.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	if ver != SchemaVersion {
		t.Fatalf("schema: got %d want %d", ver, SchemaVersion)
	}
}

func TestRecommendationsInsertLatest(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.InsertRecommendations(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	rows := []Recommendation{
		sampleRec(now.Add(-time.Hour), "pod", "app/p/c", 60),
		sampleRec(now, "pod", "app/p/c", 55),
		{
			RecordedAt: now, ClientID: "c", ClusterName: "demo",
			Grain: "workload", GrainKey: "Deployment/web/c",
			OwnerKind: "Deployment", OwnerName: "web", Container: "c",
			Samples: 40, Percentile: 95, TargetUtilization: 0.7, WindowDays: 7,
			CPUSugMcores: 50, MemSugBytes: 28 << 20,
		},
	}
	if err := st.InsertRecommendations(t.Context(), rows); err != nil {
		t.Fatal(err)
	}

	latest, err := st.LatestRecommendation(t.Context(), "pod", "app/p/c")
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.CPUSugMcores != 55 {
		t.Fatalf("latest pod: %+v", latest)
	}
	none, err := st.LatestRecommendation(t.Context(), "pod", "missing")
	if err != nil || none != nil {
		t.Fatalf("missing: %v %+v", err, none)
	}

	all, err := st.LatestRecommendations(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("latest all: %d", len(all))
	}
	pods, err := st.LatestRecommendations(t.Context(), "pod")
	if err != nil || len(pods) != 1 {
		t.Fatalf("latest pod grain: %d %v", len(pods), err)
	}
}

func sampleRec(at time.Time, grain, key string, cpuSug float64) Recommendation {
	return Recommendation{
		RecordedAt: at, ClientID: "c", ClusterName: "demo",
		Grain: grain, GrainKey: key, Namespace: "app", Pod: "p", Container: "c",
		Samples: 30, Percentile: 95, TargetUtilization: 0.7, WindowDays: 7,
		CPUPMcores: 40, MemPBytes: 16 << 20, CPUSugMcores: cpuSug, MemSugBytes: 32 << 20,
		CPUReqMcores: 100, MemReqBytes: 64 << 20,
	}
}
