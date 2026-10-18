# SPECIFICATIONS — kpm

> **Status: normative — describes the `v0.9.0` tree.** Every section below is
> implemented and must not regress without a CHANGELOG entry. The CLI surface and the
> store schema are frozen for `1.0.0` (§2.5, §2.6). Bands: Core `v0.2.0`, Insights
> `v0.3.0`, Notify `v0.4.0`, MCP `v0.5.0`, Server `v0.6.0`, Scale `v0.7.0`, Honesty
> `v0.8.0`, capacity truth + `restore` `v0.9.0`.

| Term | Meaning |
|------|---------|
| **Normative today** | Implemented; must not regress without CHANGELOG. |
| **Draft** | Agreed product direction. |
| **Core** | Must exist for kpm to be “itself”. |
| **Adjacent** | First-class CLI, not optional config layers. |
| **Optional** | Off unless configured / flagged. |

---

## 0. Product vision

### 0.1 One-line

**kpm** samples pod CPU/memory (and requests/limits) from the Kubernetes
**control plane** into **local SQLite**, then builds workflows on that history:
export/backup, rightsizing, interactive **TUI**, and optional daemon facades
(threshold **notifications**, read-only **MCP**, HTTP **server** charts).

### 0.2 Layered model

```
┌─────────────────────────────────────────────────────────────────┐
│  OPTIONAL: server — live charts + optional HTTP auth            │
├─────────────────────────────────────────────────────────────────┤
│  OPTIONAL: MCP (read-only) — daemon; stdio and/or localhost HTTP│
├─────────────────────────────────────────────────────────────────┤
│  OPTIONAL: notifications — threshold held for duration X        │
│             webhook first                                        │
├─────────────────────────────────────────────────────────────────┤
│  ADJACENT: recommend · capacity (table) · tui (heatmap list)    │
├─────────────────────────────────────────────────────────────────┤
│  CORE: sample (CP → records) → SQLite ← backup / export csv|tsv │
└─────────────────────────────────────────────────────────────────┘
```

### 0.3 Position vs kube-capacity

Live tables/TUI borrow the *quantities* (request/limit/util) and filters;
**persistence + history workflows** are the product center. No upstream source
copy.

### 0.4 Repo split

| Repo | Owns |
|------|------|
| **kpm** | Binary: core + adjacent + optional daemon facades |
| **kpm-selfhosted** (future) | cron/systemd/Helm — not `run/` here |

---

## 1. Scaffold surface (normative today)

| Item | Behavior |
|------|----------|
| Binary | `kpm` (also `kubectl-kpm`) |
| `kpm version` | version, commit, build date, branch, Go runtime |
| `kpm --print-sample-config` | embedded sample YAML → stdout, exit 0 |
| Bare `kpm` | help, exit 0 |
| `kpm doctor` | read-only diagnosis of config, store, history coverage and (opt-in) cluster — §1.1 |
| Packaged systemd unit | `contrib/systemd/kpm-sample.service` → `/lib/systemd/system/kpm-sample.service` in the `.deb` / `.rpm`. **Never enabled by the package** — the operator runs `systemctl enable --now kpm-sample`. Pins `KPM_STORE_PATH=/var/lib/kpm/metrics.db` (`StateDirectory=kpm`) because the packaged config's `~/.local/share/...` default would resolve to root's home under a system service |
| Exit codes | `0` / `1` / `2` (cluster) via `internal/exitcode` |

### 1.1 `kpm doctor` — read-only diagnosis

Reports every layer that has to work for trustworthy recommendations, and reports **all** of them
instead of stopping at the first problem:

| Check | Reports |
|-------|---------|
| `config.load` | the config file that was read, or the load error |
| `config.retention` | `retention_days` vs `recommend.window_days` (the §2.2 guidance) |
| `config.targets` | utilization target, percentile and `min_samples` in effect |
| `store.path` | resolved store path; fails when it does not exist |
| `store.schema` | schema version on disk vs the one this build expects |
| `store.samples` | row count |
| `store.bounds` | oldest → newest sample |
| `history.coverage` | days with samples inside the window — the signal `kpm recommend` prints (§3) |
| `cluster.metrics_api` | Metrics API reachability, **only** with `--live` |

**Read-only, and enforced rather than assumed.** The store is opened through `store.OpenReadOnly`: no
file creation, no migration. The normal `Open` creates a missing store and migrates an old one, so a
diagnostic must not use it — a run that created the file would report a healthy store that never
existed, and one that migrated would "fix" the version it was asked to inspect.

**Statuses** are `ok`, `warn`, `fail`, `skip`. Warnings mean "look at this" and never fail the run:
thin coverage and a retention shorter than the window are warnings, because both are legal
configurations with understood consequences.

**Exit codes:** `0` when nothing verifiably broke, `1` for a general failure, `2` when the only
failures come from the cluster probe.

**No cluster by default.** `kpm serve` and `kpm mcp` are documented as usable offline against a
copied SQLite file, so an absent cluster is reported as `skip` rather than a broken setup. `--live`
is the explicit opt-in, and a failure there is a real one.

**Output:** one greppable line per check (`CHECK  STATUS  DETAIL`) plus a summary, or `--format json`
using the same lower-snake-case field conventions as the server API.

---

## 2. CORE — capture, store, backup, export (normative today)

Implemented in-tree (`kpm sample` / `backup` / `export`). Must not regress
without CHANGELOG.

### 2.1 Capture (`kpm sample`)

- **client-go** + Metrics API only (no `kubectl` subprocess).
- Container grain: util, request, limit, identities, `sampled_at` UTC.
- Rows also persist resolved `client_id` + `cluster_name` (§9.2.1).
- Missing pod metrics → **skip** (+ count); API/RBAC failure → **fail closed**
  (exit `2`).
- One process ↔ one cluster.

### 2.2 Store (SQLite)

- `store.path` (`~` expanded), `retention_days`, schema version, time indexes.
- Persist `sampled_at` / recommendation `recorded_at` as fixed-width UTC
  (`2006-01-02T15:04:05.000000000Z`) so TEXT lexicographic order matches wall
  clock (including same-second nanos). Older RFC3339 / RFC3339Nano rows remain
  readable.
- **Guidance:** larger `retention_days` keeps more history for percentiles /
  recommend (closer to real usage patterns). Prefer
  `retention_days` ≥ `recommend.window_days`. Default sample **14**;
  **production: recommend > 45 days** (often 60–90 if disk allows). Short
  retention → noisier p95.
- The writer uses one connection. SELECTs from the HTTP API, export, recommend,
  and probes use a separate read-only pool on the same file, so a chart or
  export scan does not hold the connection the sampler inserts on. A store
  opened read-only (restore `--merge`) keeps a single read-only connection.

### 2.3 Sample loop / daemon

| `interval_seconds` | Behavior |
|--------------------|----------|
| `0` | One-shot sample → write SQLite → exit |
| `> 0` | **Daemon**: forever loop — **always** sample → write SQLite on each tick |

Daemon sample loop retries transient Metrics/API errors with exponential
backoff (1s, 2s, 4s, … capped at `interval_seconds`) for up to a **2-minute**
retry budget (`SampleRetryBudget`), then fails closed. Each failure is logged
and counted on `kpm_sample_errors_total` when ops HTTP is up. One-shot `sample`
(interval `0`) still exits on the first error.

Daemon **always** persists metrics. Optional facades attach only when
configured (each independently):

| Facade | When it runs in the daemon |
|--------|----------------------------|
| **notifications** | `notifications.enabled: true` (and rules/sink configured) |
| **MCP** | `mcp.enabled: true` (HTTP/SSE in-process; stdio remains `kpm mcp`) |
| **server** | `server.enabled: true` |

A daemon with no optional blocks configured is valid: **sample-only** long-running
collector. SIGINT/SIGTERM → clean shutdown, exit `0`.

### 2.4 Backup / export

- `kpm backup` — safe copy via SQLite **`VACUUM INTO`** (works while store open).
- `kpm restore` — install a backup at the store path, or `--merge` the rows it
  adds. **Replace** copies to a temp file and renames into place, removes stale
  `-wal`/`-shm` left by the replaced database, and refuses a non-empty store
  without `--force`. **Merge** opens the backup read-only and dedupes on each
  table's natural key (`client_id`, `cluster_name`, grain or pod, `container`,
  timestamp), so repeating it adds nothing. A backup from a newer schema is
  refused; older backups are accepted, skipping absent tables and columns.
- `kpm export` — **csv** and **tsv** only for the foreseeable future (json later
  only if demanded). Offline filters: `--since` / `--until` (RFC3339),
  `--namespace`, `--pod`, `--workload` (`owner_name`).

---

### 2.5 Store schema and compatibility (Normative)

`kpm doctor` reports the version on disk against the one the build expects. The
history below is append-only: a version number is never reused or renumbered.

| v | Release | Adds |
|---|---------|------|
| 1 | 0.2.0 | `schema_meta`, `samples` and its time/namespace indexes |
| 2 | 0.3.0 | `recommendations` + grain index (`recommend --record`) |
| 3 | 0.7.0 | `idx_samples_ns_owner_at` — index only, no table change |
| 4 | 0.7.0 | `node_snapshots` + indexes (`capacity --record-nodes`) |
| 5 | 0.8.0 | `recommendations.cpu_target_utilization`, `.memory_target_utilization` |
| 6 | 0.8.0 | `recommendations.window_days`, `.days_with_samples` |
| 7 | 0.9.0 | `node_snapshots.taints`, `.unschedulable` |

- **Forward only, additive.** Opening an older store migrates it in place:
  `CREATE TABLE IF NOT EXISTS` for tables, `addColumnIfMissing` for columns. No row
  is rewritten and no column is dropped, so a migration is additive and idempotent.
- **Irreversible.** A migrated store cannot be read by an older binary. `kpm backup`
  before upgrading across a minor is the supported way back, and `kpm restore` puts
  it back.
- **A newer store is refused, untouched.** `Open` reads `schema_meta` *before*
  running any DDL and fails when the store is newer than the build understands;
  `restore --merge` applies the same rule to its source. The fix is a newer kpm,
  never a partial read: an older binary adding the columns it thinks are missing
  would corrupt a schema it cannot interpret.
- **`1.0.0` freezes v7.** After that, new columns still need a version bump, but the
  meaning of an existing table, column or index changes only in a major release.

### 2.6 CLI surface freeze (Normative)

The frozen surface for `1.0.0` is: command names, flag names and their meanings, the
TSV/CSV column order, the JSON API field names, and the exit codes in §12.

- Adding a command, flag or field is not a breaking change.
- Removing or renaming one, reordering output columns, or reusing an exit code for a
  different meaning is breaking and requires a major release.
- A **behaviour correction** that brings the tool in line with this document is not a
  break; it ships in a patch and the CHANGELOG records it.

---

## 3. ADJACENT — recommend (normative today)

- `kpm recommend` — **offline** from SQLite (no cluster access).
- Grains: workload+container **and** pod+container (both emitted).
- Window / percentile / `min_samples` / `target_utilization` from `recommend.*`.
- `target_utilization` is the shared default; `cpu_target_utilization` and
  `memory_target_utilization` override it per resource (**0 or unset = inherit**). CPU is
  compressible and throttles under a burst, memory is incompressible and is **OOMKilled**, so
  the two are commonly sized against different targets.
- `suggested_request = round_up(p / <resource>_target_utilization)` — CPU ceil **10m**, mem
  ceil **1 MiB**. Both resolved targets are stored with each recommendation snapshot, so
  `--score` labels post-change utilization against the target the suggestion was built with.
- **Window coverage is reported, not assumed.** `window_days` is what was asked for;
  `days_with_samples` is the count of distinct UTC days inside the window holding at least one
  sample. A grain that passes `min_samples` can still rest on a fraction of the window, so every
  run prints both and flags coverage below `window_days`. A fully covered rolling window counts
  `window_days + 1` days, because the window clips the first and last day — exceeding the window
  length is the signal that coverage is complete, not a bug.
- Snapshots persist both numbers, and `GET /api/v1/recommendations` exposes them as
  `window_days` / `days_with_samples`. `days_with_samples: 0` means the row predates the column,
  i.e. coverage is **unknown** rather than zero.
- Verdicts vs request **and** limit (oversize / undersize ratios); CPU/mem **separate**.
- Keys below `min_samples` skipped; **no advice rows → exit `3`**.
- Output: `--format table` (default) or `tsv`.
- **No auto-apply** (read-only). Later: patch/Kustomize export + server `#18b`.

### 3.1 Percentile, `min_samples` and `--score`

**The percentile is a nearest-rank order statistic over observed samples.** For `n` values the rank
is `ceil(p/100 × n)` and the result is that element of the sorted samples — an actually observed
utilisation, never an interpolation. `percentile: 95` over 20 samples is the 19th smallest. Empty
input is `0`; `p` is clamped to `0..100`, and `p: 0` returns the minimum.

**Both grains pool their samples before the percentile.** The workload grain keys on
`owner_kind/owner_name/container`, so *every pod of that owner and container feeds the same sample
set*; the pod grain keys on `namespace/pod/container`. A workload p95 is therefore the p95 of the
pool, **not** the maximum of its pods' p95s — pooling can only pull a top order statistic down, and
one bursty pod is diluted by its quieter siblings. Sizing an owner from the workload grain sizes the
aggregate, not the worst pod; use the pod grain when a single pod drives the risk. A row's request,
limit and verdicts come from the newest sample in its pool.

**A grain below `min_samples` is skipped, not reported as low confidence.** It leaves the table
entirely and is only counted (`skipped N keys below min_samples`). A quiet table therefore means
either "no samples in the window" or "every grain below the threshold" — neither is a per-row state.
Exit code `3` when no row survives.

**Verdicts and the suggestion answer different questions.** The verdict compares the *percentile* to
the *current* request (and limit) through the oversize/undersize ratios, so it describes what is
allocated today and is independent of the suggestion. The suggestion is
`round_up(percentile / target_utilization)`, ceiled to 10m CPU / 1 MiB memory. A row can be
`oversized` and still carry a suggestion above the current request.

**`--score` (calibration mode A) evaluates a snapshot against the samples that followed it.** For
each latest snapshot per grain key, samples strictly *after* `recorded_at` are pooled for the same
grain key; if fewer than `recommend.min_samples` remain, both dimensions are `skip`. Otherwise the
post-change percentile is compared to the *suggested* request, with
`ratio = percentile / suggested_request`:

| Label | Condition |
|-------|-----------|
| `hit` | `ratio` inside `[target − band, target + band]` |
| `miss-low` | `ratio` below that band |
| `miss-high` | `ratio` above it |
| `skip` | fewer than `min_samples` holdout samples |
| `no-data` | suggested request is `0`, so there is nothing to compare against |

`band` defaults to **±0.15 absolute** around the target, so `target_utilization: 0.70` accepts
`0.55–0.85`. The target is the one the suggestion was built with — per resource when
`cpu_target_utilization` / `memory_target_utilization` are set, otherwise the shared
`target_utilization`.

**Mode A assumes the suggestion was applied.** The comparison divides by the *suggested* request, so
a `miss-low` on a suggestion that never landed says nothing about the recommendation: it reports
utilisation against a request the cluster does not have. Score runs are only meaningful for
suggestions that were actually applied.

---

## 4. ADJACENT — capacity (non-interactive, normative today)

- `kpm capacity` — one-shot **live** table (kube-capacity-like). Optional
  `--record-nodes` writes under-demand `node_snapshots` for offline analyze-local.
- Shared read model: `internal/live` (also for TUI/server).
- Default grain: **one row per pod** — sum util / sum request / sum limit.
- Pods without Metrics: still listed; util and ratios shown as `n/a`.
- **Requests are counted the way the scheduler counts them.** A pod's effective
  request is `max(sum(containers), max(initContainers))`: an init container asking
  for 2 CPU makes the pod occupy 2 CPU even if its app containers sum to 500m.
  Node totals use that number. Container rows keep their own per-container
  request, because rightsizing advice is per container and an init container
  consumes nothing at runtime.
- **Pods in a terminal phase are excluded** (`Succeeded`, `Failed`): they no
  longer hold their reservation and the Metrics API stops reporting them.
  `Pending` pods still count — they reserve capacity while they wait.
- **Taints never change a total.** `capacity` reports per-node taints
  (`NoSchedule` / `NoExecute` only — `PreferNoSchedule` is a hint) and cordon
  state as **pool identity**. Capacity that exists is capacity that exists;
  removing a dedicated pool from the totals would erase it from the report of the
  operator who owns it.
  `capacity --nodes` prints the per-node table and then the same numbers grouped
  by pool — a pool being the set of blocking taints, `(general)` for nodes that
  have none. Pool identity persists per snapshot (schema **v7**), so
  `GET /api/v1/nodes` carries `taints` and `unschedulable` as well.
- **`kpm capacity fit`** — workload-aware placement check. `--for kind/name`
  (`deployment`, `statefulset`, `daemonset`) or `--cpu` / `--memory`. Per node:
  fits, or why not — cordon, taint not tolerated, `nodeSelector` / required
  affinity mismatch, insufficient CPU or memory. Evaluates only what the pod and
  the nodes can decide: pod (anti-)affinity, topology spread, volume zone binding
  and fragmentation are **not** evaluated, and every run names them. Requests come
  from pod specs alone, so it does not require the Metrics API. Exit code **4** when
  no node fits (see §12) — the answer is no, which is not the same as failing to ask.
- Filters: `scope` (+ optional `tui.pod_labels` ANDed). `tui.pods` allowlist is for TUI.
- Sort: `--sort` or `tui.default_sort` (`cpu.ratio` default, hottest first).
- Exit code **2** on cluster/Metrics/RBAC failure (same as `sample`).

---

## 5. ADJACENT — TUI (interactive heatmap list, normative today)

Interactive terminal UI for watching a **preselected** set of pods.

### 5.1 Entry

- Preferred: `kpm tui` (full-screen app).
- Also accept `kpm capacity --tui` as an alias that enters the same UI.

### 5.2 Which pods (“indicados previamente”)

Selection comes from config (not ad-hoc typing every launch):

| Source | Role |
|--------|------|
| `scope` | Baseline filter (namespaces / labels / node labels) |
| `tui.pods` (optional) | Explicit allowlist: `namespace/name` entries |
| `tui.pod_labels` | Extra label selector ANDed with scope |

If `tui.pods` is non-empty → only those pods (that also pass scope).  
If empty → all pods matching `scope` (same as capacity default).

### 5.3 Presentation

- Scrollable **list of pods** (one row per pod; optional expand to containers
  later).
- Each row: identity + **horizontal bar** (heatmap style) for the selected
  metric ratio, e.g. util/request or util/limit.
- Color bands: green `[0, 0.70)`, yellow `[0.70, 0.90)`, red `[0.90, ∞)` of the
  baseline. The boundaries are **hardcoded** (`internal/tui/bar.go`), not read from
  `recommend.target_utilization` or the notify thresholds; they coincide with the
  default target by convention only, so changing a threshold in config does not move
  a bar's colour.
- Dual bars or toggle: **CPU** vs **memory**; baseline toggle: **vs request**
  vs **vs limit**.
- **vs last recommendation (P2):** when a `#9b` snapshot exists for the grain
  key, show deviation (**live request − last recorded suggested request**);
  otherwise `n/a`. Read-only join — TUI does not write the store. Populate
  snapshots with `kpm recommend --record` first.
- **Δ is not “drift vs cluster policy / Git / Helm defaults”.** It compares
  today’s live allocation to the **last `recommend --record` snapshot only**.
  The bar percent is a separate signal (live util ÷ live request|limit).
- **Enter (detail):** read-only panel comparing **owner template** resources
  (Deployment / StatefulSet / DaemonSet; ReplicaSet walked up to Deployment)
  vs **live Pod** requests/limits. No cluster writes. Esc closes. Unsupported
  owners (Job, etc.) show `n/a`. Empty/error frames still render a header so
  the alt-screen never goes blank.

#### 5.3.1 Worked example (anonymized)

Header mode: `cpu/request`, `delta=on`. One row:

```text
> app/db-0   ████████████░░  113%   Δcpu -60m   Δmem +3Mi
```

| Field | Meaning |
|-------|---------|
| `113%` | Live CPU **util ÷ live request** ≈ 1.13 — bar red (≥ 90% band). How hot the pod is **right now** vs what the cluster currently grants. **Not** Δ. |
| `Δcpu -60m` | `live_cpu_request − last_suggested_cpu` = −60 millicores. Distance from the **last recorded recommendation**, not from some other infra baseline. Advice asked for **more** CPU request than live request. |
| `Δmem +3Mi` | Live mem request ~3 MiB **above** last suggested mem — nearly aligned with that snapshot. |

**Do not confuse:** a high `%` can coexist with a negative Δ (hot vs current request, yet still below last advice) or the reverse. Infra “desired” manifests / GitOps source of truth are out of scope for Δ.

**Operator takeaway:** pressure is on CPU util vs request; vs last recommend the CPU request is still **short** (−60m). Typical follow-up is a GitOps PR to raise CPU request toward the suggestion (kpm never applies). Memory can stay as-is.

Without a prior `--record` (or no matching grain key) both Δ columns show `n/a`.

### 5.4 Sorting

Operator can sort (keys / menu) by:

| Key | Meaning |
|-----|---------|
| `name` | namespace/pod |
| `cpu.util` | absolute CPU utilization |
| `cpu.request` / `cpu.limit` | allocation |
| `cpu.ratio` | util / request (or / limit per toggle) |
| `mem.util` / `mem.request` / `mem.limit` / `mem.ratio` | same for memory |

Default sort: `cpu.ratio` descending (hottest first).

### 5.5 Refresh

- Live re-query on an interval (default 5s; `tui.refresh_seconds`).
- Does not write SQLite unless user also runs sample/daemon separately.
- Requires working Metrics API for util bars; without util, show allocation
  only and mark util as `n/a`.

### 5.6 Non-goals for TUI v1

- Editing cluster objects.
- Full node-capacity dashboard (use `capacity` table or server charts).
- Mouse-only UX (keyboard-first).

---

## 6. OPTIONAL — notifications (sustained thresholds, normative today)

Daemon only (`interval_seconds > 0`). Off unless `notifications.enabled: true`.
Does not change the sample→SQLite loop; evaluates rules against live ticks /
recent store as implemented.

### 6.0 Rules source (decided)

| `notifications.rules` | Behavior |
|-----------------------|----------|
| Non-empty | Use **user-defined** rules only (no merge with built-ins) |
| Empty / omitted while `enabled: true` | Apply **built-in default rules** (below) and log once at daemon start |

**Built-in defaults** (when rules empty):

| Rule name | Metric | `threshold_ratio` | Scope |
|-----------|--------|-------------------|-------|
| `default-cpu-util-vs-request` | `cpu.util_vs_request` | **0.80** | All pods in sample `scope` |
| `default-mem-util-vs-request` | `mem.util_vs_request` | **0.80** | All pods in sample `scope` |

Other rule fields use §6.1–6.2 defaults (`hold_for_seconds`, `repeat_seconds`,
resolve, etc.) unless later made configurable under `notifications.defaults`.

**Startup log** (English, once per process; info level):

```text
notification rules not defined in user config; using built-in defaults: CPU and memory util vs request at 80%
```

To disable alerts while keeping the daemon: set `notifications.enabled: false`
(or omit the block). Empty `rules` with `enabled: true` is **not** a silent no-op.

### 6.1 Predicate (decided)

| Field | Decision |
|-------|----------|
| Default metric | **`cpu.util_vs_request`** and/or **`mem.util_vs_request`** per rule |
| Also allowed | `*.util_vs_limit`; absolute `cpu.util_mcores` / `mem.util_bytes` |
| Fire when | metric ≥ `threshold` **continuously** for `hold_for_seconds` |

### 6.2 Resolve / repeat / dedupe (decided)

| Policy | Decision |
|--------|----------|
| Dedupe key | `rule_name + namespace + pod + container + metric` |
| Fire | Once when hold duration satisfied |
| Repeat | While still true, re-notify every `repeat_seconds` (default **3600**; `0` = no repeat) |
| Resolve | When condition false for `clear_after_seconds` (default = same as `hold_for_seconds`); send resolve webhook if `notify_on_resolve: true` (default **true**) |

### 6.3 Sinks (gghstats-school)

Config key: `notifications.sinks[]` (required non-empty when `enabled: true`).
Fan-out to **all** sinks; continue on per-sink failure; aggregate errors.
Fail-closed if any declared sink’s `*_env` is missing or empty.

| `type` | Notes |
|--------|--------|
| `slack` | Incoming Webhook; `webhook_url_env` → `{"text":...}` |
| `webhook` | JSON POST; `url_env`; optional `headers_env`; `body`: `generic` (default), `discord`, or `teams` |
| `smtp` | Email; `host_env`, `port_env`, `from_env`, `to_env`; optional `user_env` / `password_env`; `use_tls` for implicit TLS (465); `require_starttls` refuses a relay that does not advertise STARTTLS (the plaintext path is otherwise opportunistic, i.e. silently cleartext). Subject is RFC 2047 encoded; CR/LF in `from_env` / `to_env` / subject is rejected (header injection) |

**Not first-class:** native `pagerduty` / `discord` / `teams` / `loki`.
PagerDuty and similar: `type: webhook` + `body: generic` (or automation URL).
`kpm notify-test` fans out one synthetic `firing` event (no hold wait).

### 6.4 Examples (anonymized)

Illustrative only — fictional namespace/pod/client/cluster labels. Real
payloads always include resolved `client.id` + `cluster.name` (§9.2.1).

**Plain text** (Slack Incoming Webhook `text` field, SMTP body, and the
`text` string inside generic JSON):

```text
kpm ALERT: default-cpu-util-vs-request app/api-0/main
metric=cpu.util_vs_request value=0.91 threshold=0.8
event=firing level=alert client.id=bastion-a cluster=prod-east ts=2026-10-11T16:05:00Z
```

Repeat (still above threshold after `repeat_seconds`):

```text
kpm ALERT (ongoing): default-cpu-util-vs-request app/api-0/main
metric=cpu.util_vs_request value=0.93 threshold=0.8
event=repeat level=alert client.id=bastion-a cluster=prod-east ts=2026-10-11T17:05:00Z
```

Resolve (condition clear for `clear_after_seconds`; `notify_on_resolve: true`):

```text
kpm RESOLVED: default-cpu-util-vs-request app/api-0/main
metric=cpu.util_vs_request value=0.42 threshold=0.8
event=resolved level=ok client.id=bastion-a cluster=prod-east ts=2026-10-11T17:20:00Z
```

**Webhook `body: generic`** (JSON POST; fields match `internal/notify`
`GenericPayload`):

```json
{
  "event": "firing",
  "level": "alert",
  "title": "kpm ALERT: default-cpu-util-vs-request app/api-0/main",
  "text": "kpm ALERT: default-cpu-util-vs-request app/api-0/main\nmetric=cpu.util_vs_request value=0.91 threshold=0.8\nevent=firing level=alert client.id=bastion-a cluster=prod-east ts=2026-10-11T16:05:00Z",
  "rule": "default-cpu-util-vs-request",
  "metric": "cpu.util_vs_request",
  "value": 0.91,
  "threshold": 0.8,
  "namespace": "app",
  "pod": "api-0",
  "container": "main",
  "client.id": "bastion-a",
  "cluster": "prod-east",
  "cluster.name": "prod-east",
  "ts": "2026-10-11T16:05:00Z"
}
```

**Webhook `body: discord`:**

```json
{
  "content": "kpm ALERT: default-cpu-util-vs-request app/api-0/main\nmetric=cpu.util_vs_request value=0.91 threshold=0.8\nevent=firing level=alert client.id=bastion-a cluster=prod-east ts=2026-10-11T16:05:00Z"
}
```

**Webhook `body: teams`** (MessageCard-shaped; theme red on alert / green on resolve):

```json
{
  "@type": "MessageCard",
  "@context": "https://schema.org/extensions",
  "summary": "kpm ALERT: default-cpu-util-vs-request app/api-0/main",
  "themeColor": "FF0000",
  "title": "kpm ALERT: default-cpu-util-vs-request app/api-0/main",
  "text": "kpm ALERT: default-cpu-util-vs-request app/api-0/main\nmetric=cpu.util_vs_request value=0.91 threshold=0.8\nevent=firing level=alert client.id=bastion-a cluster=prod-east ts=2026-10-11T16:05:00Z"
}
```

**Config sketch** (secrets via env names only — never inline URLs/passwords):

```yaml
notifications:
  enabled: true
  rules: []   # → built-in 80% + startup log
  sinks:
    - type: slack
      webhook_url_env: KPM_SLACK_WEBHOOK
    - type: webhook
      url_env: KPM_DISCORD_WEBHOOK_URL
      body: discord
    - type: smtp
      host_env: KPM_SMTP_HOST
      port_env: KPM_SMTP_PORT
      from_env: KPM_SMTP_FROM
      to_env: KPM_SMTP_TO
```

`kpm notify-test` sends one synthetic **firing** event (rule name
`notify-test`, demo pod `demo/notify-test/main`) to all configured sinks
without waiting for hold — use to validate env/fan-out before enabling the
daemon path.

---

## 7. OPTIONAL — MCP (read-only, normative today)

Off unless explicitly started. HTTP/SSE in the daemon requires
`mcp.enabled: true`. Stdio mode is the separate `kpm mcp` command (does not
require the sample daemon, but may share the same store). Implemented in-tree
(`#16a` / `#16` / `#17`). Must not regress without CHANGELOG.

### 7.1 Transport (decided)

| Mode | When |
|------|------|
| **stdio** | `kpm mcp` (or daemon child) for editor/agent attach — default for Cursor-style |
| **HTTP/SSE** | Daemon with `mcp.enabled` + listen address (default `127.0.0.1`) |

Both supported; same tool surface.

### 7.2 Auth and HTTP access control (decided)

| Transport | Auth |
|-----------|------|
| stdio | Trust process boundary; **no** extra token |
| HTTP/SSE | **Bearer token** via `token_env` **required** if listen is non-loopback; on loopback, token **optional but recommended** |
| mTLS | **Deferred** (not in first MCP cut) |

Daemon HTTP mux access control (gghstats mold; applies to MCP mounts, **not** ops):

| Knob | Default | Role |
|------|---------|------|
| `mcp.listen` | `127.0.0.1:9740` | Bind; use `0.0.0.0` only when the operator intentionally exposes the port |
| `mcp.trusted_proxies` | empty | TCP peers allowed to supply `X-Forwarded-For` / `X-Real-IP` for rate-limit / allowlist identity |
| `mcp.allowlist_cidrs` | empty (off) | Optional client IP/CIDR allowlist; matching `Authorization: Bearer` bypasses IP check |
| `mcp.rate_limit` | enabled, 120/1m burst 20 | Per-IP token bucket; `enabled: false` disables. IPv6 buckets are grouped by `/64` (the standard subscriber allocation) so a client cannot refill by rotating its source address; IPv4 and IPv4-mapped IPv6 stay per-address (the mapped form normalises to its IPv4 bucket). The allowlist is deliberately **not** grouped this way — it matches exactly the CIDR written |

Exempt from allowlist and rate limit: `GET /healthz`, `/readyz`, `/metrics`.
When rate limit or allowlist is active and `trusted_proxies` is empty, the process
logs a one-shot warning that forwarded client-IP headers are ignored.

Loopback listen (`127.0.0.1` / `::1` / `localhost`): non-ops routes also reject
unexpected `Host` and non-loopback `Origin` (DNS-rebinding mitigation). Empty
`Origin` (curl / non-browser) is allowed. Ops probes stay exempt. Extra public
Host/Origin names may be listed in `mcp.allowed_hosts` and/or
`server.allowed_hosts` (merged) when a reverse proxy preserves a non-loopback
`Host` toward a loopback bind. These knobs apply whenever any HTTP facade is
active (`mcp.enabled` and/or `server.enabled`); allowlist / rate-limit /
trusted_proxies today live under `mcp.*` and bind the shared mux.

Tools: read-only query/summarize only; no cluster or SQLite mutations.

### 7.3 Tools (normative — store-only MVP)

Same surface on **stdio** and **HTTP/SSE**. No cluster mutations; no SQLite writes.
Live Metrics / `internal/live` tools are **deferred**.

| Tool | Reads | Notes |
|------|-------|-------|
| `list_samples` | Store samples | Namespace / workload / time window filters; hard row cap applied in SQL (`LIMIT`, newest rows) before materialization |
| `summarize_window` | Store samples | Aggregates (count; util stats when request fields present); **no** second Metrics API call |
| `list_recommendations` | Store recommendations | Optional grain filter |
| `get_status` | Config + build meta | Version, store path, sample interval, notify/mcp flags; **never** secret values |

### 7.4 Daemon HTTP ops (normative — with MCP listen)

When the sample daemon runs with `mcp.enabled: true`, the process listens on
`mcp.listen` (default loopback) and serves:

| Path | Role |
|------|------|
| `GET /healthz` | Liveness (process up) |
| `GET /readyz` | Readiness (store open; kube/Metrics reachable when sampling) |
| `GET /metrics` | **Process** Prometheus text (daemon/ops counters) — **not** a re-export of cluster pod metrics |

MCP HTTP mounts on the **same** mux:

| Path | Transport |
|------|-----------|
| `/sse` | MCP SSE (client examples in §7.5) |
| `/mcp` | MCP streamable HTTP |

Bearer auth (`token_env`) applies to MCP paths only — **not** to ops probes.
Allowlist and rate limit also skip ops probes (§7.2).
JSON REST `/api/*` and charts belong to **server** mode (§8 / P5), not MCP.

### 7.5 Client integration examples (normative)

Prefer **stdio** (`kpm mcp`) on a laptop. Use **HTTP/SSE** when the sample
daemon already runs with `mcp.enabled: true`. Paths below are anonymized;
replace with a real absolute config path and `which kpm`. HTTP mounts:
`/sse` (SSE) and `/mcp` (streamable).

**Preflight:** `kpm sample --config /home/ops/kpm.yaml` so the store has rows;
then attach a client. Smoke tools: `get_status`, `list_samples`,
`summarize_window`, `list_recommendations`.

**Bearer token** (HTTP only): opaque shared secret — not a JWT. Generate once,
export as the env named by `mcp.token_env` (sample: `KPM_MCP_TOKEN`):

```bash
export KPM_MCP_TOKEN="$(openssl rand -hex 32)"
```

Clients send `Authorization: Bearer <same value>`. Off-loopback listen requires
`token_env` set and non-empty (SPEC §7.2).

#### Cursor — `~/.cursor/mcp.json` or project `.cursor/mcp.json`

```json
{
  "mcpServers": {
    "kpm": {
      "type": "stdio",
      "command": "kpm",
      "args": ["mcp", "--config", "/home/ops/kpm.yaml"]
    }
  }
}
```

HTTP (daemon listening on loopback):

```json
{
  "mcpServers": {
    "kpm": {
      "url": "http://127.0.0.1:9740/sse",
      "headers": {
        "Authorization": "Bearer ${env:KPM_MCP_TOKEN}"
      }
    }
  }
}
```

Fully restart Cursor after edits.

#### Claude Desktop — `claude_desktop_config.json`

macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`  
Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "kpm": {
      "command": "/usr/local/bin/kpm",
      "args": ["mcp", "--config", "/home/ops/kpm.yaml"]
    }
  }
}
```

GUI apps often lack shell `PATH` — use an absolute `command`. Quit the app
completely and reopen.

#### Claude Code

```bash
claude mcp add --transport stdio kpm -- kpm mcp --config /home/ops/kpm.yaml
```

HTTP/SSE:

```bash
claude mcp add --transport sse kpm http://127.0.0.1:9740/sse \
  --header "Authorization: Bearer ${KPM_MCP_TOKEN}"
```

#### Codex — `~/.codex/config.toml` (or project `.codex/config.toml`)

```toml
[mcp_servers.kpm]
command = "kpm"
args = ["mcp", "--config", "/home/ops/kpm.yaml"]
enabled = true
```

HTTP:

```toml
[mcp_servers.kpm]
url = "http://127.0.0.1:9740/sse"
bearer_token_env_var = "KPM_MCP_TOKEN"
enabled = true
```

CLI: `codex mcp add kpm -- kpm mcp --config /home/ops/kpm.yaml`

#### Hermes Agent — `~/.hermes/config.yaml`

```yaml
mcp_servers:
  kpm:
    command: "kpm"
    args: ["mcp", "--config", "/home/ops/kpm.yaml"]
```

HTTP:

```yaml
mcp_servers:
  kpm:
    url: "http://127.0.0.1:9740/sse"
    headers:
      Authorization: "Bearer ${KPM_MCP_TOKEN}"
```

Then `hermes mcp test kpm` and `/reload-mcp` after edits.

#### Generic MCP client

| Transport | Contract |
|-----------|----------|
| **stdio** | Spawn `kpm mcp --config /home/ops/kpm.yaml`; JSON-RPC on stdin/stdout; diagnostics on **stderr** only |
| **HTTP/SSE** | Connect to `mcp.listen` (default `127.0.0.1:9740`); send `Authorization: Bearer <token>` when required (SPEC §7.2) |

### 7.6 Tool call / response examples (anonymized)

Structured arguments and results as returned by the Go MCP tool handlers
(field names match `internal/mcp`). Paths and cluster names are fake.

**`get_status`** — no arguments:

```json
{
  "version": "0.5.0",
  "store_path": "/var/lib/kpm/prod-east.db",
  "sample_interval_seconds": 60,
  "notifications_enabled": true,
  "mcp_enabled": true,
  "cluster_name": "prod-east"
}
```

**`list_samples`** — arguments:

```json
{
  "namespace": "app",
  "workload": "api",
  "since": "2026-10-11T12:00:00Z",
  "until": "2026-10-11T18:00:00Z",
  "limit": 100
}
```

Result (truncated when more rows exist than `limit`):

```json
{
  "count": 1,
  "truncated": false,
  "rows": [
    {
      "sampled_at": "2026-10-11T16:05:00Z",
      "namespace": "app",
      "pod": "api-0",
      "container": "main",
      "owner_kind": "StatefulSet",
      "owner_name": "api",
      "cpu_util_mcores": 180,
      "mem_util_bytes": 268435456,
      "cpu_request_mcores": 500,
      "mem_request_bytes": 536870912,
      "cpu_util_ratio": 0.36,
      "mem_util_ratio": 0.5
    }
  ]
}
```

**`summarize_window`** — arguments:

```json
{
  "namespace": "app",
  "since": "2026-10-11T00:00:00Z",
  "until": "2026-10-12T00:00:00Z"
}
```

Result:

```json
{
  "sample_count": 240,
  "rows_with_cpu_request": 230,
  "rows_with_mem_request": 230,
  "cpu_util_ratio_avg": 0.41,
  "cpu_util_ratio_p95": 0.72,
  "mem_util_ratio_avg": 0.55,
  "mem_util_ratio_p95": 0.81
}
```

**`list_recommendations`** — arguments:

```json
{
  "grain": "workload"
}
```

Result:

```json
{
  "count": 1,
  "rows": [
    {
      "recorded_at": "2026-10-11T15:00:00Z",
      "grain": "workload",
      "grain_key": "app/StatefulSet/api",
      "namespace": "app",
      "owner_kind": "StatefulSet",
      "owner_name": "api",
      "samples": 120,
      "cpu_sug_mcores": 350,
      "mem_sug_bytes": 402653184,
      "cpu_req_mcores": 500,
      "mem_req_bytes": 536870912,
      "target_utilization": 0.7
    }
  ]
}
```

---

## 8. OPTIONAL — server mode (charts) (Normative — 0.6.x)

Off unless `server.enabled: true` (or `kpm serve`). **Analyze-local** is a
first-class mode: open an existing SQLite store with
`sample.interval_seconds: 0` (no collection), serve charts + JSON (+ MCP HTTP
on the shared mux) without kube. Live Metrics enrichment is optional when
kubeconfig works. Band plan: [docs/plan-0.6.0.md](docs/plan-0.6.0.md).

### 8.1 Stack (Normative)

- **Boring embedded UI**: Go `net/http` + small templates + minimal JS (or HTMX).
  **No** SPA framework / npm app in-repo for v1.
- Data: SQLite history (required); optional live snapshot from Metrics API.
- Optional **bearer** auth (`token_env`); fail closed if `auth: true` and unset.
- Default listen loopback; may share `mcp.listen` when `server.listen` empty.
- HTTP facade must **not** require a running sample loop (decouple from
  `interval_seconds > 0`). Entry: **`kpm serve`** (`mcp.enabled` and/or
  `server.enabled`); `/readyz` is store-only (kube optional).
- **JSON API** (when `server.enabled`): `GET /api/v1/status`, `/workloads`,
  `/series`, `/series/batch`, `/recommendations`, `/live` (503 JSON if kube unavailable),
  `/export/patch` (GitOps YAML; never applied). Bearer via `server.token_env`
  or shared `mcp.token_env`; required off-loopback (or `server.auth: true`).
  One listen: prefer `server.listen`, else `mcp.listen`.
- **Charts UI** (when `server.enabled`): embedded `GET /` + `/static/*`
  (no SPA/npm). Workload cards fetch `/api/v1/series` and join latest
  recommendations for verdict badges; drawers show recommendations + optional
  live + copy/download patch. Summary strip counts oversized / undersized /
  ok / no-req. Window presets 1h/24h/48h/7d/15d + custom range. Browser unlock
  form stores bearer for API calls when required.

### 8.2 Relationship to capacity / TUI (Normative)

- Shared **read model** (usage snapshots) behind `capacity`, `tui`, and server API.
- `capacity` remains a **CLI** command; server is not required to use it.
- Server JSON endpoints power charts and non-MCP clients; TUI talks to cluster
  (and optionally store) directly — no hard dependency on server process.

### 8.3 Recommend export in UI (0.6.x `#18b`)

- Server **must not** apply resource patches to Deployments/StatefulSets/etc.
- `GET /api/v1/export/patch?grain_key=…&format=strategic|kustomize` returns JSON
  `{format,filename,content,grain_key}` from the latest stored recommendation.
  Strategic-merge YAML sets **requests** only (suggested CPU/memory). ReplicaSet
  owners map to Deployment (sample-time owner walk preferred; offline patches
  also strip a trailing `pod-template-hash` segment). CronJob patches target
  `spec.jobTemplate.spec.template.spec.containers`. Kustomize format wraps the
  same YAML with overlay hints (markdown). Comments state copy/download only —
  never `kubectl apply` from kpm.
- Charts drawer: **Copy strategic patch** / **Download kustomize hint** when a
  recommendation exists for the card grain. Operator pastes into Git / opens a
  PR — GitOps remains source of truth.
- Formatter lives in `internal/recommend` (`FormatResourcePatch`); CLI recommend
  export may reuse the same helpers later.

### 8.4 Analyze-local UX (Normative)

- Per-workload chart cards from store series; window presets (1h / 24h / 48h /
  7d / 15d) + custom range. Maximum window span is **15d** (including free Go
  durations and `since`/`until`). Store queries use index
  `(namespace, owner_name, sampled_at)`. `GET /series` clamps to the newest
  **10,000** matching rows (`truncated: true` when clamped). `GET /series/batch`
  accepts at most **20** workloads (HTTP 400 above that); the charts page
  requests further batches of 20. `GET /series`, `GET /series/batch`, and
  `GET /workloads` each bound the store read to **15s** and return HTTP 503
  `store query timed out` when that elapses. `GET /workloads` fails
  closed (HTTP 413) above 100k rows in the window.
- Collapsible drawer: store recommendations; if kube available, current
  request/limit (+ delta vs suggest when both exist).
- Mode chip from `status.kube_live`: **`live`** when the Metrics client was
  wired at process start; **`analyze-local`** when store-only (`/live` 503).
- Sticky footer: repo link, MIT license, advisory-only disclaimer (kpm never
  applies patches; operator owns production changes).
- UI search/filter (`#20c`): namespace and owner are sent as
  `/api/v1/workloads?namespace=&workload=` (narrows the sample scan); grain
  substring and CPU/Mem verdict filters apply client-side after joining
  recommendations. Empty grid distinguishes “no samples in window” vs “no
  matches”.

### 8.5 Recommend verdicts on cards (`#18c`) (Normative)

- `GET /api/v1/recommendations` includes `cpu_verdict` / `mem_verdict`
  (`oversized|undersized|ok|n/a`) from p-util vs current request
  (`SnapshotVerdicts` / `Classify` with config oversize/undersize ratios),
  plus `cpu_delta_mcores` / `mem_delta_bytes` (suggested − request).
- Cards show CPU/Mem badges; page summary counts worst-of-pair per workload
  (undersized beats oversized beats ok; `n/a` = no request; missing row =
  no recommend).
- Node capacity snapshots (`#20b`): `kpm capacity --record-nodes` writes
  `node_snapshots` (under-demand; not every sample tick). Analyze-local
  `GET /api/v1/nodes` reads the latest batch offline (no kube).

### 8.6 Status / recommendations examples (anonymized)

`GET /api/v1/status` (no secrets):

```json
{
  "version": "v0.6.0",
  "store_path": "/var/lib/kpm/example.db",
  "sample_interval_seconds": 0,
  "server_enabled": true,
  "cluster_name": "example-east",
  "cluster_environment": "stg",
  "kube_context": "example-east-admin",
  "kube_live": false,
  "sample_count": 4200
}
```

`GET /api/v1/recommendations?grain=workload` (one row):

```json
{
  "count": 1,
  "rows": [
    {
      "grain": "workload",
      "grain_key": "app/Deployment/api",
      "namespace": "app",
      "owner_kind": "Deployment",
      "owner_name": "api",
      "cpu_p_mcores": 40,
      "cpu_req_mcores": 200,
      "cpu_sug_mcores": 100,
      "cpu_delta_mcores": -100,
      "cpu_verdict": "oversized",
      "mem_verdict": "ok",
      "target_utilization": 0.7
    }
  ]
}
```

---

## 9. Configuration (Normative)

### 9.1 Discovery (kwd-style)

`--config` default **`kpm.yaml`**.

### 9.1.1 Precedence (decided — Viper / family mold)

Lowest → highest (later wins):

1. Built-in **defaults**
2. **YAML** file (`--config`)
3. Environment **`KPM_*`** (`AutomaticEnv` / `BindEnv`; `.` → `_`)
4. **CLI flags** — where a command implements the override explicitly:
   `--sort` wins over `tui.default_sort`, `-o` over the default backup path.

   There is **no general flag→config binding** (no `BindPFlag`): a flag is read by
   the command that consumes it, not merged into the config tree. So level 4 is a
   per-command rule, not a layer — a flag that no command reads does nothing, and
   adding a flag is not adding a configuration knob.

Every YAML knob that operators need at runtime must be overridable via **`KPM_*`**
(`AutomaticEnv` covers every key through the `.` → `_` replacer). CLI overrides are
per command and must be documented where they exist — see the note above. Secrets use
`*_env` **names** in YAML; values come only from the environment (fail closed if
missing when required).

### 9.2 Top-level keys

`cluster`, `kube`, `client`, `store`, `sample`, `scope`, `recommend`, `tui`,
`notifications`, `mcp`, `server`.

**[`configs/kpm.sample.yml`](../configs/kpm.sample.yml) is the complete key
reference** — every key the binary reads is documented there, in the block where it
belongs. Optional sections (`notifications`, `mcp`, `server`) ship commented out on
purpose: the file is packaged as `/etc/kpm/kpm.yaml` by the `.deb`/`.rpm`, and
installing it must enable nothing.

### 9.2.1 Client identity (normative today — kwd/pgwd pattern)

| Field | Role |
|-------|------|
| `cluster.name` | *What* is being sampled (cluster label in payloads) |
| `client.id` | *Who* is sampling (bastion / host running `kpm`) |

Resolved `client.id` cascade (never anonymous):

1. `KPM_CLIENT_ID` environment variable
2. `client.id` in YAML
3. hostname
4. primary non-loopback IPv4
5. primary MAC
6. literal `unknown`

Every notification payload (and sample summary lines) MUST include resolved
`client.id` and `cluster.name` so multi-bastion / multi-cluster sinks stay
distinguishable.

### 9.3 `tui` sketch

```yaml
tui:
  refresh_seconds: 5
  default_sort: cpu.ratio   # see §5.4
  baseline: request         # request | limit
  # pods: ["app/frontend-x", "app/api-y"]
  # pod_labels: "kpm.watch=true"
```

---

## 10. Security posture (Normative)

| Area | Rule |
|------|------|
| Cluster | **Read-only.** No create/update/patch/delete verb is issued against the cluster; the typed client is used for List and Get only. |
| Secrets | `*_env` **names** in YAML; the values come only from the environment, never from the file. |
| Notify / MCP HTTP / server | **Fail closed.** A sink whose env var is missing or empty refuses to build (`environment variable X is missing or empty (fail-closed)`); `mcp.token_env` is required when `mcp.listen` is non-loopback; `server.token_env` (or `mcp.token_env`) is required when server auth is on. |
| MCP / server bind | **`127.0.0.1:9740`** by default. Binding anywhere else requires a token. |
| K8s RBAC deny | **Fail closed.** A denied List surfaces as exit `2`, never as an empty result. |
| Middleware order | Browser guard (Host/Origin) → allowlist (IP/CIDR) → rate limit → mux, outermost first. |
| Ops paths | `/healthz`, `/readyz` and `/metrics` are **exempt** from allowlist and rate limit — a probe must not be rate-limited into a false unhealthy. |
| Allowlist vs bearer | A valid `Authorization: Bearer` bypasses the IP check; MCP routes stay gated by bearer auth. |
| Malformed allowlist entry | **Startup error, never a skip.** An allowlist that silently drops what it cannot parse admits everyone the moment the only entry has a typo. |
| Malformed `trusted_proxies` entry | **Startup error, never a skip.** Dropping a bad proxy CIDR makes rate-limit and allowlist see the proxy address. An allowlist that covers that address then admits every client behind it. |
| SMTP | `use_tls` for implicit TLS (465), otherwise STARTTLS; `require_starttls` refuses a relay that will not upgrade. |

An **empty** `allowlist_cidrs` still means "off", and an **empty** `trusted_proxies` still means
forwarded client-IP headers are ignored. The distinction the code makes is between nothing
configured and something configured wrongly.

The smallest role that allows all of the above is in **[docs/rbac.md](docs/rbac.md)**: `get`/`list` on
`nodes` and `pods`, `get` on `apps` workloads, `list` on `metrics.k8s.io/pods`. No `watch`, no
`secrets`, no write verbs.

---

## 11. CLI map

| Command | Layer | Status | Cluster? | Store? |
|---------|-------|--------|----------|--------|
| `sample` | Core | **Normative** | Yes | Write |
| `backup` | Core | **Normative** | No | Yes |
| `restore` | Core | **Normative** | No | Yes |
| `export` | Core | **Normative** | No | Yes |
| `recommend` | Adjacent | **Normative** | No | Yes |
| `capacity` | Adjacent | **Normative** | Yes | No |
| `tui` / `capacity --tui` | Adjacent | **Normative** | Yes | Read* |
| `mcp` | Optional | **Normative** | No** | Read |
| `serve` | Optional | **Normative** | No*** | Read |
| `notify-test` | Optional | **Normative** | No | No |
| daemon `sample` loop | Core | **Normative** (notify / MCP HTTP when enabled) | Yes | Write |

\*TUI is live Metrics; optionally **reads** existing `recommendations` for Δ (`n/a` if none). Never writes store.  
\*\*MCP store-only tools; live-cluster MCP tools deferred.  
\*\*\*`kpm serve`: ops (+ MCP and/or `/api/v1/*`) without sample loop; kube optional; `/readyz` = store only.

---

## 12. Exit codes

| Code | Meaning | Status |
|------|---------|--------|
| `0` | ok | **Normative** |
| `1` | generic failure | **Normative** |
| `2` | cluster / Metrics API / RBAC | **Normative** |
| `3` | empty / insufficient store data | **Normative** (recommend) |
| `4` | answered, and the answer is no: no node can run the workload | **Normative** (capacity fit) |

Code `4` exists so a script can tell "the cluster says no" from "the cluster could not
be asked" (`2`) and "there is no data" (`3`). A capacity tool that returns a plain
failure for both is unusable in a pipeline: `if ! kpm capacity fit ...` cannot
distinguish a genuine negative answer from a broken query.

---

## 13. Non-goals

- Mutating workloads; multi-cluster process; replacing Prometheus;
  Helm/`run/` here; MCP/HTTP write tools; TUI as cluster editor.

---

## 14. Implementation phasing

| Phase | Deliver | Status |
|-------|---------|--------|
| **P1 — Core** | sample → SQLite; backup; export csv/tsv; daemon sample-only | **Shipped** `v0.2.0` |
| **P2 — Insights CLI** | recommend; capacity table; **tui** heatmap list | **Shipped** `v0.3.0` |
| **P3 — Notify** | sustained thresholds + multi-sink (slack/webhook/smtp) | **Shipped** `v0.4.0` |
| **P4 — MCP** | stdio + HTTP/SSE + ops mux + access control | **Shipped** `v0.5.0` |
| **P5 — Server** | analyze-local + charts + JSON API | **Shipped** `v0.6.0` (`docs/plan-0.6.0.md`) |
| **P6 — Harden** | nodes snap + UI search + E2E/docs/security → 1.0 | **Shipped** across `v0.7.0`–`v0.9.0`; remainder in `docs/plan-1.0.0.md` |

**Core must not depend** on P3–P5. TUI is P2 (with capacity), not blocked on server.

---

## 15. Decision log

### 15.1 v0.1 (still in force)

Recommend dual grain; request+limit verdicts; `p/0.70` suggest; separate
memory ratios; skip missing pod metrics; daemon if interval &gt; 0;
`--config` → `kpm.yaml`; K8s RBAC fail closed; live CLI name `capacity`.

### 15.2 v0.2 (still in force)

SQLite history is **core**; optional notify/MCP/server; notify needs daemon.

### 15.3 v0.3 (decided this revision)

| Topic | Decision |
|-------|----------|
| Notify predicate | Default **util vs request**; also vs limit + absolute |
| Notify lifecycle | Dedupe by rule/pod/container/metric; hold to fire; **repeat_seconds** default 1h; resolve after **clear_after** (default = hold); resolve webhook on by default |
| MCP transport | **Both** stdio and HTTP/SSE |
| MCP auth | Bearer on HTTP; none on stdio; **no mTLS** in v1 |
| Server stack | **Embedded** templates + minimal JS; no SPA |
| capacity vs server | Shared read model; **capacity stays CLI** |
| Export formats | **csv + tsv** for a long time |
| **TUI** | `kpm tui` (+ `capacity --tui`); preselected pods via scope / `tui.pods` / labels; horizontal heatmap bars; sort by name / util / allocation / ratio |

### 15.4 Daemon facades (clarified)

| Topic | Decision |
|-------|----------|
| Daemon core duty | `interval_seconds > 0` → **always** sample and persist metrics to SQLite |
| Notifications | Optional — only if `notifications.enabled` |
| MCP | Optional — HTTP in daemon if `mcp.enabled`; stdio via `kpm mcp` |
| Server | Optional — only if `server.enabled` |
| Sample-only daemon | Valid (no notify/MCP/server blocks) |

### 15.5 Notify default rules (decided)

| Topic | Decision |
|-------|----------|
| Empty `rules` + `enabled: true` | Built-in CPU + mem **util vs request @ 80%**; log once (English) |
| Non-empty `rules` | User rules only — no merge with built-ins |
| Silent no-op on empty rules | **Rejected** |

### 15.6 Client identity (decided)

| Topic | Decision |
|-------|----------|
| `client.id` | Same cascade as kwd/pgwd (`KPM_CLIENT_ID` → YAML → hostname → IPv4 → MAC → `unknown`) |
| Payloads | Always include resolved `client.id` + `cluster.name` |
| SQLite rows | Persist `client_id` + `cluster_name` on each sample for multi-writer history |

### 15.7 Core P1 in-tree (draft v0.4)

| Topic | Decision |
|-------|----------|
| Backup mechanism | SQLite **`VACUUM INTO`** (not online backup API) |
| Export formats | csv + tsv only; filters since/until/namespace/pod/workload |
| Daemon | `interval_seconds > 0` → always sample; notify optional when enabled |
| SPEC bump (historical) | At `v0.2.0`: §1 + §2 + §9.2.1 + exit `0`/`1`/`2` → **normative** |

### 15.10 SPEC normative after Insights + Notify (`v0.3.0` / `v0.4.0`)

| Topic | Decision |
|-------|----------|
| §3–5 Insights | **Normative** when `v0.3.0` shipped |
| §6 Notifications | **Normative** when `v0.4.0` shipped (multi-sink) |
| Exit `3` | **Normative** with recommend |
| Band exit rule | Feature PR must update SPEC/man/sample/CHANGELOG; release cut must not leave status banner stale |

### 15.11 SPEC normative after MCP (`v0.5.0`)

| Topic | Decision |
|-------|----------|
| §7 MCP | **Normative** when band code is on `develop` + docs exit (stdio, HTTP/SSE, ops mux, access control) |
| §8 Server | **Normative** when 0.6.x band code + docs exit on `develop` (analyze-local, `/api/v1/*`, charts, `#18b`/`#18c`) |
| Tools | Store-only MVP (`get_status`, `list_samples`, `summarize_window`, `list_recommendations`); live-cluster tools deferred |

### 15.12 SPEC normative after Server (0.6.x / P5)

| Topic | Decision |
|-------|----------|
| §8 Server | **Normative** on band-exit docs + code (serve, API, UI, GitOps export, verdicts) |
| Cut | `chore/release-0.6.0` then develop → main + tag `v0.6.0` (operator OK) |
| Deferred | Node snapshots `#20b`, UI search `#20c` → **P6** 0.7.x |

### 15.8 Recommendation outcomes (`#9b`, **P2 required**)

| Topic | Decision |
|-------|----------|
| Scope | **In 0.3.x Insights band exit** (with `#11`); not Core / not 0.2.0 |
| Schema | SQLite schema v2 **at the time**: `recommendations` table + indexes (full history: §2.5) |
| Persist | `kpm recommend --record` writes snapshot rows (`recorded_at`, grain key, suggested CPU/mem, observed `p`, window/percentile/target, `client_id`, `cluster_name`) |
| Score | `kpm recommend --score` — holdout samples after `recorded_at`; hit / miss-high / miss-low vs `target_utilization` ± band (calibration mode A). Full label and band semantics in [§3.1](#31-percentile-min_samples-and---score) |
| TUI feed | `LatestRecommendation` / `DeltaFromLive` for `#11` delta column (`n/a` if none) — **must ship in P2** |
| Detail | [docs/plan-0.3.0.md](docs/plan-0.3.0.md) `#9b`; ROADMAP `#9b` |

### 15.9 Recommend apply vs GitOps export (decided 2026-10-11)

| Topic | Decision |
|-------|----------|
| Cluster apply | **Rejected** for kpm (read-only; CI/GitOps would overwrite) |
| CLI path | Export artifacts for PR (tsv → later patch / Kustomize overlay) |
| Server (`0.6.x`) | Show **Kustomize patch/diff** (copy/download) per workload / bulk; no apply |
| Apply-all / apply-one | Out of product binary for v1; possible future separate tool only |

---

## Related docs

- [README.md](README.md) · [CHANGELOG.md](CHANGELOG.md) · [AGENTS.md](AGENTS.md)
- [docs/ROADMAP.md](docs/ROADMAP.md) · [docs/plan-0.2.0.md](docs/plan-0.2.0.md)
  · [docs/plan-0.3.0.md](docs/plan-0.3.0.md) · [docs/plan-0.4.0.md](docs/plan-0.4.0.md)
  · [docs/plan-0.5.0.md](docs/plan-0.5.0.md) · [docs/plan-0.6.0.md](docs/plan-0.6.0.md)
- [docs/measurement-and-rightsizing.md](docs/measurement-and-rightsizing.md)
- [configs/kpm.sample.yml](configs/kpm.sample.yml)
