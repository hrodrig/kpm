# SPECIFICATIONS — kpm

> **Status: draft v0.4 — Normative in-tree:** scaffold (§1), Core (§2),
> adjacent Insights (§3–5), notifications (§6), `client.id` (§9.2.1), and
> exit codes `0`/`1`/`2`/`3`. Optional facades MCP (§7) and server (§8)
> remain **draft** until those bands ship.

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
| Exit codes | `0` / `1` / `2` (cluster) via `internal/exitcode` |

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
- **Guidance:** larger `retention_days` keeps more history for percentiles /
  recommend (closer to real usage patterns). Prefer
  `retention_days` ≥ `recommend.window_days`. Default sample **14**;
  **production: recommend > 45 days** (often 60–90 if disk allows). Short
  retention → noisier p95.

### 2.3 Sample loop / daemon

| `interval_seconds` | Behavior |
|--------------------|----------|
| `0` | One-shot sample → write SQLite → exit |
| `> 0` | **Daemon**: forever loop — **always** sample → write SQLite on each tick |

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
- `kpm export` — **csv** and **tsv** only for the foreseeable future (json later
  only if demanded). Offline filters: `--since` / `--until` (RFC3339),
  `--namespace`, `--pod`, `--workload` (`owner_name`).

---

## 3. ADJACENT — recommend (normative today)

- `kpm recommend` — **offline** from SQLite (no cluster access).
- Grains: workload+container **and** pod+container (both emitted).
- Window / percentile / `min_samples` / `target_utilization` from `recommend.*`.
- `suggested_request = round_up(p / target_utilization)` — CPU ceil **10m**, mem ceil **1 MiB**.
- Verdicts vs request **and** limit (oversize / undersize ratios); CPU/mem **separate**.
- Keys below `min_samples` skipped; **no advice rows → exit `3`**.
- Output: `--format table` (default) or `tsv`.
- **No auto-apply** (read-only). Later: patch/Kustomize export + server `#18b`.

---

## 4. ADJACENT — capacity (non-interactive, normative today)

- `kpm capacity` — one-shot **live** table (kube-capacity-like); **no SQLite**.
- Shared read model: `internal/live` (also for TUI/server).
- Default grain: **one row per pod** — sum util / sum request / sum limit.
- Pods without Metrics: still listed; util and ratios shown as `n/a`.
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
- Color bands (draft): green &lt; 70%, yellow 70–90%, red ≥ 90% of baseline
  (aligned with `target_utilization` / notify thresholds where sensible).
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
| `smtp` | Email; `host_env`, `port_env`, `from_env`, `to_env`; optional `user_env` / `password_env`; `use_tls` for implicit TLS (465), else STARTTLS |

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

## 7. OPTIONAL — MCP (read-only)

Off unless explicitly started. HTTP/SSE in the daemon requires
`mcp.enabled: true`. Stdio mode is the separate `kpm mcp` command (does not
require the sample daemon, but may share the same store).

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
| `mcp.rate_limit` | enabled, 120/1m burst 20 | Per-IP token bucket; `enabled: false` disables |

Exempt from allowlist and rate limit: `GET /healthz`, `/readyz`, `/metrics`.
When rate limit or allowlist is active and `trusted_proxies` is empty, the process
logs a one-shot warning that forwarded client-IP headers are ignored.

Tools: read-only query/summarize only; no cluster or SQLite mutations.

### 7.3 Tools (draft — 0.5.x store-only MVP)

Same surface on **stdio** and **HTTP/SSE**. No cluster mutations; no SQLite writes.
Live Metrics / `internal/live` tools are **deferred** (not in first MCP cut).

| Tool | Reads | Notes |
|------|-------|-------|
| `list_samples` | Store samples | Namespace / workload / time window filters; hard row cap |
| `summarize_window` | Store samples | Aggregates (count; util stats when request fields present); **no** second Metrics API call |
| `list_recommendations` | Store recommendations | Optional grain filter |
| `get_status` | Config + build meta | Version, store path, sample interval, notify/mcp flags; **never** secret values |

### 7.4 Daemon HTTP ops (draft — with MCP listen)

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

### 7.5 Client integration examples (draft — target after `#16`/`#17`)

Prefer **stdio** (`kpm mcp`) on a laptop. Use **HTTP/SSE** when the sample
daemon already runs with `mcp.enabled: true`. Paths below are anonymized;
replace with a real absolute config path and `which kpm`. The HTTP MCP path
`/sse` is a draft until `#17` locks the mount.

**Preflight:** `kpm sample --config /home/ops/kpm.yaml` so the store has rows;
then attach a client. Smoke tools: `get_status`, `list_samples`,
`summarize_window`, `list_recommendations`.

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

---

## 8. OPTIONAL — server mode (charts)

Off unless `server.enabled: true`. Intended to run inside the sample daemon
(`interval_seconds > 0`) so charts see a live scrape path; does not replace
core sampling.

### 8.1 Stack (decided)

- **Boring embedded UI**: Go `net/http` + small templates + minimal JS (or HTMX).
  **No** SPA framework / npm app in-repo for v1.
- Data: last scrape cache and/or SQLite recent window.
- Optional **bearer** auth (`token_env`); fail closed if `auth: true` and unset.
- Default listen `127.0.0.1`.

### 8.2 Relationship to capacity / TUI (decided)

- Shared **read model** (usage snapshots) behind `capacity`, `tui`, and server API.
- `capacity` remains a **CLI** command; server is not required to use it.
- Server JSON endpoints may power charts; TUI talks to cluster (and optionally
  store) directly — no hard dependency on server process.

### 8.3 Recommend export in UI (planned)

- Server **must not** apply resource patches to Deployments/StatefulSets/etc.
- When recommend data is shown, UI exposes **copy/download** of suggested changes
  as **Kustomize patch/diff** (and/or strategic-merge / JSON patch) per workload
  (and optionally bulk). Operator pastes into Git / opens a PR — GitOps remains
  source of truth.
- Reuses the same formatters as CLI recommend export when those land.

---

## 9. Configuration (draft)

### 9.1 Discovery (kwd-style)

`--config` default **`kpm.yaml`**.

### 9.2 Top-level keys

`cluster`, `kube`, `client`, `store`, `sample`, `scope`, `recommend`, `tui`,
`notifications`, `mcp`, `server`.

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

## 10. Security posture (draft)

| Area | Rule |
|------|------|
| Cluster | Read-only |
| Secrets | `*_env` / env only |
| Notify / MCP HTTP / server | Fail closed if enabled and secret missing |
| MCP / server bind | Localhost default |
| K8s RBAC deny | Fail closed |

---

## 11. CLI map

| Command | Layer | Status | Cluster? | Store? |
|---------|-------|--------|----------|--------|
| `sample` | Core | **Normative** | Yes | Write |
| `backup` | Core | **Normative** | No | Yes |
| `export` | Core | **Normative** | No | Yes |
| `recommend` | Adjacent | **Normative** | No | Yes |
| `capacity` | Adjacent | **Normative** | Yes | No |
| `tui` / `capacity --tui` | Adjacent | **Normative** | Yes | Read* |
| `mcp` | Optional | Draft | No** | Read |
| `notify-test` | Optional | **Normative** | No | No |
| daemon `sample` loop | Core | **Normative** (notify when `notifications.enabled`) | Yes | Write |

\*TUI is live Metrics; optionally **reads** existing `recommendations` for Δ (`n/a` if none). Never writes store.  
\*\*MCP read tools use store; cluster tools optional and read-only.

---

## 12. Exit codes

| Code | Meaning | Status |
|------|---------|--------|
| `0` | ok | **Normative** |
| `1` | generic failure | **Normative** |
| `2` | cluster / Metrics API / RBAC | **Normative** |
| `3` | empty / insufficient store data | **Normative** (recommend) |

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
| **P4 — MCP** | stdio + localhost HTTP/SSE, bearer on HTTP | Planned |
| **P5 — Server** | embedded charts UI + optional auth | Planned |

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
| §7–8 | Stay **draft** until MCP / server bands |
| Band exit rule | Feature PR must update SPEC/man/sample/CHANGELOG; release cut must not leave status banner stale |

### 15.8 Recommendation outcomes (`#9b`, **P2 required**)

| Topic | Decision |
|-------|----------|
| Scope | **In 0.3.x Insights band exit** (with `#11`); not Core / not 0.2.0 |
| Schema | SQLite schema v2: `recommendations` table + indexes |
| Persist | `kpm recommend --record` writes snapshot rows (`recorded_at`, grain key, suggested CPU/mem, observed `p`, window/percentile/target, `client_id`, `cluster_name`) |
| Score | `kpm recommend --score` — holdout samples after `recorded_at`; hit / miss-high / miss-low vs `target_utilization` ± band (calibration mode A) |
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
  · [docs/plan-0.5.0.md](docs/plan-0.5.0.md)
- [docs/measurement-and-rightsizing.md](docs/measurement-and-rightsizing.md)
- [configs/kpm.sample.yml](configs/kpm.sample.yml)
