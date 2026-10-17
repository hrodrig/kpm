// Package doctor runs read-only diagnostics over a kpm setup: configuration,
// store, history coverage and — only when asked — the cluster.
//
// Every check reports what it observed instead of stopping at the first problem,
// so one run answers "what is missing?" rather than "the first thing that broke".
// Nothing here writes: the store is opened read-only (see store.OpenReadOnly) and
// no sampler or cluster connection is started unless Options.Live is set.
package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hrodrig/kpm/internal/cluster"
	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/exitcode"
	"github.com/hrodrig/kpm/internal/store"
)

// Status is one check outcome.
type Status string

const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
	Skip Status = "skip"
)

// Check names. Stable and greppable: they are part of the output contract.
const (
	CheckConfigLoad    = "config.load"
	CheckConfigRetain  = "config.retention"
	CheckConfigTargets = "config.targets"
	CheckStorePath     = "store.path"
	CheckStoreSchema   = "store.schema"
	CheckStoreSamples  = "store.samples"
	CheckStoreBounds   = "store.bounds"
	CheckCoverage      = "history.coverage"
	CheckMetricsAPI    = "cluster.metrics_api"
)

// Check is one diagnostic result.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

// Counts summarises a report by status.
type Counts struct {
	OK   int `json:"ok"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
	Skip int `json:"skip"`
}

// Report is one doctor run.
type Report struct {
	Status string  `json:"status"` // ok | warn | fail
	Checks []Check `json:"checks"`
	Counts Counts  `json:"counts"`
}

// Options configures a run.
type Options struct {
	ConfigPath string
	Kubeconfig string
	Live       bool      // also probe the cluster / Metrics API
	Now        time.Time // injectable clock; zero means time.Now()
}

// Broken reports whether any check that can actually be verified failed.
func (r Report) Broken() bool { return r.Counts.Fail > 0 }

// ExitCode maps the report to a process exit code. A failure confined to the
// cluster probe is a cluster failure (SPEC §12 code 2); anything else is a
// general failure. Warnings are never a failure — they are "look at this", not
// "this is broken".
func (r Report) ExitCode() int {
	if r.Counts.Fail == 0 {
		return exitcode.Success
	}
	for _, c := range r.Checks {
		if c.Status == Fail && !strings.HasPrefix(c.Name, "cluster.") {
			return exitcode.Failure
		}
	}
	return exitcode.Cluster
}

// Run performs every check it can and returns the report.
func Run(ctx context.Context, opt Options) Report {
	now := opt.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	cfg, cfgErr := config.Load(opt.ConfigPath)

	r := Report{Checks: []Check{}}
	r.add(checkConfigLoad(opt.ConfigPath, cfgErr))
	r.add(checkRetention(cfg, cfgErr))
	r.add(checkTargets(cfg, cfgErr))
	r.add(storeChecks(ctx, cfg, cfgErr, now)...)
	r.add(checkMetricsAPI(ctx, opt, cfg))
	r.summarise()
	return r
}

func (r *Report) add(checks ...Check) {
	r.Checks = append(r.Checks, checks...)
}

func (r *Report) summarise() {
	r.Counts = Counts{}
	for _, c := range r.Checks {
		switch c.Status {
		case OK:
			r.Counts.OK++
		case Warn:
			r.Counts.Warn++
		case Fail:
			r.Counts.Fail++
		case Skip:
			r.Counts.Skip++
		}
	}
	switch {
	case r.Counts.Fail > 0:
		r.Status = "fail"
	case r.Counts.Warn > 0:
		r.Status = "warn"
	default:
		r.Status = "ok"
	}
}

func checkConfigLoad(path string, cfgErr error) Check {
	if cfgErr != nil {
		return Check{Name: CheckConfigLoad, Status: Fail, Detail: cfgErr.Error()}
	}
	label := strings.TrimSpace(path)
	if label == "" {
		label = config.DefaultConfigPath
	}
	return Check{Name: CheckConfigLoad, Status: OK, Detail: label + " (valid)"}
}

// checkRetention mirrors the warn #24 emits while loading: a warning here, not a
// failure, because the spec states the relation as guidance.
func checkRetention(cfg *config.Config, cfgErr error) Check {
	if cfgErr != nil {
		return Check{Name: CheckConfigRetain, Status: Skip, Detail: "config did not load"}
	}
	retention, window := cfg.Store.RetentionDays, cfg.Recommend.WindowDays
	detail := fmt.Sprintf("retention %dd, recommend window %dd", retention, window)
	if retention >= window {
		return Check{Name: CheckConfigRetain, Status: OK, Detail: detail}
	}
	return Check{Name: CheckConfigRetain, Status: Warn,
		Detail: detail + " — retention prunes the history the window measures"}
}

// checkTargets reports the utilization targets the suggestion formula will use,
// resolved per resource (#25) rather than just echoing the shared default.
func checkTargets(cfg *config.Config, cfgErr error) Check {
	if cfgErr != nil {
		return Check{Name: CheckConfigTargets, Status: Skip, Detail: "config did not load"}
	}
	detail := fmt.Sprintf("cpu target %.2f, memory target %.2f, p%d, min_samples %d",
		cfg.Recommend.CPUUtilTarget(),
		cfg.Recommend.MemUtilTarget(),
		cfg.Recommend.Percentile,
		cfg.Recommend.MinSamples,
	)
	return Check{Name: CheckConfigTargets, Status: OK, Detail: detail}
}

func storeChecks(ctx context.Context, cfg *config.Config, cfgErr error, now time.Time) []Check {
	if cfgErr != nil {
		return skippedStoreChecks("config did not load")
	}
	path, err := cfg.ExpandedStorePath()
	if err != nil {
		return failedStoreChecks("resolve store path: " + err.Error())
	}
	if _, err := os.Stat(path); err != nil {
		return failedStoreChecks(err.Error())
	}
	st, err := store.OpenReadOnly(path)
	if err != nil {
		return failedStoreChecks(err.Error())
	}
	defer func() { _ = st.Close() }()

	out := []Check{{Name: CheckStorePath, Status: OK, Detail: st.Path()}}
	out = append(out, checkSchema(ctx, st))

	rows, countCheck := checkSampleCount(ctx, st)
	out = append(out, countCheck)
	if rows == 0 {
		return append(out,
			Check{Name: CheckStoreBounds, Status: Skip, Detail: "no samples to bound"},
			Check{Name: CheckCoverage, Status: Skip, Detail: "no samples to count"})
	}
	bounds, boundsOK := checkBounds(ctx, st)
	out = append(out, bounds)
	if !boundsOK {
		return append(out, Check{Name: CheckCoverage, Status: Skip, Detail: "sample bounds unavailable"})
	}
	out = append(out, checkCoverage(ctx, st, cfg.Recommend.WindowDays, now))
	return out
}

func skippedStoreChecks(reason string) []Check {
	return []Check{
		{Name: CheckStorePath, Status: Skip, Detail: reason},
		{Name: CheckStoreSchema, Status: Skip, Detail: reason},
		{Name: CheckStoreSamples, Status: Skip, Detail: reason},
		{Name: CheckStoreBounds, Status: Skip, Detail: reason},
		{Name: CheckCoverage, Status: Skip, Detail: reason},
	}
}

func failedStoreChecks(reason string) []Check {
	return []Check{
		{Name: CheckStorePath, Status: Fail, Detail: reason},
		{Name: CheckStoreSchema, Status: Skip, Detail: "store unavailable"},
		{Name: CheckStoreSamples, Status: Skip, Detail: "store unavailable"},
		{Name: CheckStoreBounds, Status: Skip, Detail: "store unavailable"},
		{Name: CheckCoverage, Status: Skip, Detail: "store unavailable"},
	}
}

func checkSchema(ctx context.Context, st *store.Store) Check {
	ver, err := st.SchemaVersionOf(ctx)
	if err != nil {
		return Check{Name: CheckStoreSchema, Status: Fail, Detail: err.Error()}
	}
	if ver == store.SchemaVersion {
		return Check{Name: CheckStoreSchema, Status: OK, Detail: fmt.Sprintf("schema v%d", ver)}
	}
	return Check{Name: CheckStoreSchema, Status: Warn,
		Detail: fmt.Sprintf("schema v%d, this build expects v%d — open the store with any kpm command to migrate", ver, store.SchemaVersion)}
}

func checkSampleCount(ctx context.Context, st *store.Store) (int64, Check) {
	rows, err := st.Count(ctx)
	if err != nil {
		return 0, Check{Name: CheckStoreSamples, Status: Fail, Detail: err.Error()}
	}
	if rows == 0 {
		return 0, Check{Name: CheckStoreSamples, Status: Warn, Detail: "0 rows — nothing has been sampled yet"}
	}
	return rows, Check{Name: CheckStoreSamples, Status: OK, Detail: fmt.Sprintf("%d rows", rows)}
}

// checkBounds reports the span of stored samples. The boolean is false when the
// bounds could not be read, so the caller can skip the coverage check instead of
// claiming it passed.
func checkBounds(ctx context.Context, st *store.Store) (Check, bool) {
	oldest, newest, err := st.SampleBounds(ctx)
	if err != nil {
		return Check{Name: CheckStoreBounds, Status: Fail, Detail: err.Error()}, false
	}
	return Check{Name: CheckStoreBounds, Status: OK,
		Detail: fmt.Sprintf("%s → %s", oldest.Format("2006-01-02"), newest.Format("2006-01-02"))}, true
}

// checkCoverage reports the same days-with-samples signal kpm recommend prints
// (#26), so "I sampled for a week" and "I sampled for a week but only 3 days have
// data" do not look the same.
func checkCoverage(ctx context.Context, st *store.Store, windowDays int, now time.Time) Check {
	if windowDays < 1 {
		windowDays = 7
	}
	since := now.UTC().AddDate(0, 0, -windowDays)
	days, err := st.DaysWithSamplesSince(ctx, since)
	if err != nil {
		return Check{Name: CheckCoverage, Status: Fail, Detail: err.Error()}
	}
	detail := fmt.Sprintf("%d days with samples in a %dd window", days, windowDays)
	if days < windowDays {
		return Check{Name: CheckCoverage, Status: Warn,
			Detail: detail + " — percentiles rest on thinner history than the window implies"}
	}
	return Check{Name: CheckCoverage, Status: OK, Detail: detail}
}

// checkMetricsAPI is opt-in: kpm serve and kpm mcp are documented as usable
// offline against a copied SQLite file, so a cluster that is not there is not a
// broken setup unless the caller asked for a live check.
func checkMetricsAPI(ctx context.Context, opt Options, cfg *config.Config) Check {
	if !opt.Live {
		return Check{Name: CheckMetricsAPI, Status: Skip, Detail: "offline — pass --live to probe the cluster"}
	}
	kubeContext := ""
	if cfg != nil {
		kubeContext = cfg.Kube.Context
	}
	restCfg, err := cluster.LoadRESTConfig(opt.Kubeconfig, kubeContext)
	if err != nil {
		return Check{Name: CheckMetricsAPI, Status: Fail, Detail: "rest config: " + err.Error()}
	}
	mcs, err := cluster.NewMetricsClientset(restCfg)
	if err != nil {
		return Check{Name: CheckMetricsAPI, Status: Fail, Detail: "metrics client: " + err.Error()}
	}
	// One item is enough to prove the Metrics API is served and RBAC allows it.
	list, err := mcs.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return Check{Name: CheckMetricsAPI, Status: Fail, Detail: "list pod metrics: " + err.Error()}
	}
	return Check{Name: CheckMetricsAPI, Status: OK,
		Detail: fmt.Sprintf("metrics API reachable (%d pod(s) in the first page)", len(list.Items))}
}
