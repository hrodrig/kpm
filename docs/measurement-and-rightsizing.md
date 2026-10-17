# Measurement and rightsizing strategy

<a id="top"></a>

Operator-facing overview of how kpm is intended to **measure** pod resources and
**recommend** rightsizing. Normative detail lives in
[SPECIFICATIONS.md](../SPECIFICATIONS.md) (draft until implemented).

## How it works

kpm is a **history-first** rightsizing helper: capture control-plane metrics into
**local SQLite**, then analyze offline. Live tables/TUI borrow kube-capacity-like
quantities; they are not a substitute for a Prometheus stack.

```mermaid
flowchart LR
  subgraph capture ["Capture (core)"]
    CP["Kubernetes API\n+ Metrics API"]
    S["kpm sample\n(client-go)"]
    DB[(SQLite\nstore)]
    CP --> S --> DB
  end

  subgraph insights ["Insights (adjacent)"]
    R["kpm recommend"]
    C["kpm capacity / tui"]
    DB --> R
    CP --> C
  end

  subgraph optional ["Optional facades (daemon)"]
    N["notifications"]
    M["MCP read-only"]
    H["HTTP charts"]
    S -.->|"interval > 0"| N
    S -.-> M
    S -.-> H
  end
```

| Mode | `sample.interval_seconds` | What happens |
|------|---------------------------|--------------|
| One-shot | `0` | Sample → write SQLite → exit |
| Daemon | `> 0` | Loop: **always** sample → SQLite each tick; optional notify / MCP / server if enabled |

[↑ Back to top](#top)

---

## Measurement strategy

**Sources (planned):** typed **client-go** only — Pod specs (requests/limits) +
Metrics API utilization. No `kubectl` subprocess. One process ↔ one cluster
(`kube.context`).

**Grain:** per **container** each tick: CPU/memory util, request, limit,
identities (namespace, pod, container, owner), `sampled_at` (UTC).

**Failure policy:**

| Situation | Behavior |
|-----------|----------|
| Pod missing from Metrics API | **Skip** that pod (+ counter); continue |
| API / RBAC failure | **Fail closed** (do not pretend the cluster is fine) |

**Aggregation for recommend:** prefer stable keys — **workload + container**
and **pod + container** (default: both). Pod names churn; owner refs keep
history useful across restarts.

**Retention:** SQLite at `store.path` with `retention_days` (sample default
**14**). Export **csv** / **tsv** for spreadsheets; backup while the daemon runs.

Longer retention generally yields **more representative** percentiles and
rightsizing advice: weekly/month batch jobs, Monday peaks, and deploy cycles
need enough calendar days in the store. Prefer
`retention_days` ≥ `recommend.window_days` (default window **7**). Raising
retention trades disk for statistics closer to real usage; very short
retention makes p95 noisy and recommendations less trustworthy.

| Environment | Suggested `retention_days` |
|-------------|----------------------------|
| Lab / demo | **14** (sample default) |
| Staging | **30+** |
| **Production** | **> 45** (often **60–90** if disk allows) |

**Why history beats a snapshot:** oversizing shows up as util ≪ request over
days, not in a single `kubectl top`. Undersizing / risk shows as util near
request or limit across the window.

[↑ Back to top](#top)

---

## Rightsizing recommendations

`kpm recommend` is **offline** (reads SQLite only). It does **not** mutate
workloads — suggestions only (no auto-apply).

```mermaid
flowchart TD
  DB[(SQLite history)] --> W["Window\n(default 7d)"]
  W --> P["Percentile\n(default p95)"]
  P --> V{"Compare to\nrequest / limit"}
  V -->|util/request low| O["Oversized\n→ lower request"]
  V -->|util/request high| U["Undersized\n→ raise request"]
  V --> S["suggested_request\n= round_up(p / target_utilization)\ndefault 0.70 (cpu/mem overridable)"]
```

| Idea | Default (SPEC draft) | Meaning |
|------|----------------------|---------|
| Window | `window_days: 7` | How much history to trust |
| Percentile | `percentile: 95` | Peak-ish steady use, not max spike alone |
| Target util | `target_utilization: 0.70` | Shared aim, ~70% of request after rightsize |
| Per-resource target | `cpu_target_utilization`, `memory_target_utilization` | Override the shared target; **0 or unset = inherit**. CPU throttles under a burst, memory is OOMKilled, so they are often set differently (CPU lower, memory higher) |
| Suggest | `round_up(p / <resource>_target_utilization)` | Request from observed percentile |
| CPU vs memory | Separate ratios | Do not mix dimensions |
| vs request / vs limit | Both verdicts | Limit pressure ≠ request waste |

**Rule-of-thumb bands** (aligned with recommend / TUI / notify where sensible):

| util ÷ request (stable) | Reading |
|-------------------------|---------|
| ≪ ~0.35 | Likely oversize — candidates to lower request |
| ~0.50–0.80 | Reasonable headroom |
| ≳ ~0.85 | Undersize / throttling risk — raise request |
| util near **limit** | OOM / throttle risk — do not cut limit blindly |

### Reading a percentile, and reading a score run

**The percentile is a real observed value, not a smoothed estimate.** kpm takes the nearest-rank
order statistic: sort the samples, take `ceil(p/100 × n)`. With `percentile: 95` over 20 samples you
get the 19th smallest — one actual measurement. It does not interpolate and it does not extrapolate
a peak you never observed.

**A workload row pools every pod of the owner.** The workload grain keys on owner + container, so
all pods of that Deployment feed one sample set; the pod grain keys on namespace + pod + container.
A workload p95 is the p95 of that **pool**, so it does not track the hottest pod: one bursty replica
is diluted by its quieter siblings. Two consequences worth remembering:

- If a single pod is the risk (a leak, a noisy neighbour), read the **pod** grain, not the workload
  one.
- A calm-looking workload row can hide a pod that is about to be OOMKilled.

**A grain under `min_samples` disappears.** It is skipped, not flagged low-confidence, so an empty
table means either "no samples in the window" or "every grain under the threshold" — never a
per-row warning. Check the coverage note (and `kpm doctor`) before concluding nothing is wrong.

**Choosing a percentile.** p95 answers "how hot is it at a busy moment"; p99 is a stricter peak that
needs more samples to stay stable; p50 sizes to the median and will undersize anything with a burst.
There is no right answer independent of the consequence: CPU throttling is degradable, memory is not
(OOMKilled), which is why the two target utilizations are configurable separately.

**Reading `kpm recommend --score`.** Each row compares utilisation *after* a snapshot against the
request that snapshot suggested:

| Label | What it means |
|-------|---------------|
| `hit` | post-change utilisation landed inside target ± 0.15 — the suggestion was right |
| `miss-low` | utilisation stayed well below the target — the suggestion was still generous |
| `miss-high` | utilisation rose above the target — the suggestion was too tight |
| `skip` | not enough holdout samples after the snapshot; nothing to conclude |
| `no-data` | the snapshot carries no suggested request to compare against |

**The trap: mode A assumes you applied the suggestion.** The score divides by the *suggested*
request, so scoring a recommendation you never applied measures utilisation against a request the
cluster does not have. A wall of `miss-low` on unapplied advice is not a broken recommender — it is
the tool doing exactly what it was asked. Score only what you actually deployed.

### TUI delta vs last recommend

After `kpm recommend --record`, `kpm tui` (delta on) shows
`live request − last suggested request`.

**Δ is only vs that snapshot** — not vs Helm/Kustomize/Git “desired”
settings, not vs limits, and not the same thing as the bar `%`
(util ÷ live request|limit).

Anonymized example (`cpu/request` mode):

```text
> app/db-0   ████████████░░  113%   Δcpu -60m   Δmem +3Mi
```

- **113%** — util above **current** request (hot now). Infra live signal.
- **Δcpu −60m** — current CPU request is 60m below **last recorded advice**.
- **Δmem +3Mi** — mem request nearly matches that same advice snapshot.

Full table: [SPECIFICATIONS.md §5.3.1](../SPECIFICATIONS.md).

**Notifications** (daemon, optional): user `rules[]`, or if empty while
`enabled: true`, built-in **CPU + memory util vs request @ 80%** plus a
one-time startup log — see SPEC §6.0.

[↑ Back to top](#top)

---

## Daemon and optional facades

When `interval_seconds > 0`, the process is a long-running collector:

1. **Always** sample → SQLite every tick  
2. **If** `notifications.enabled` → evaluate rules (user or 80% defaults)  
3. **If** `mcp.enabled` → read-only MCP HTTP in-process (`kpm mcp` for stdio)  
4. **If** `server.enabled` → embedded charts UI  

A sample-only daemon (no optional blocks) is valid. SIGINT/SIGTERM → exit `0`.

[↑ Back to top](#top)

---

## See also

- [SPECIFICATIONS.md](../SPECIFICATIONS.md) — normative contract  
- [configs/kpm.sample.yml](../configs/kpm.sample.yml) — annotated defaults  
- [README.md](../README.md) — project entry  
