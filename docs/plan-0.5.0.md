# Plan 0.5.0 — MCP (stdio · HTTP/SSE · ops mux)

<a id="top"></a>

Band plan for **0.5.x MCP** (SPEC §7 / P4). Roadmap IDs: `#16a`, `#16`, `#17`.
Parent: [ROADMAP.md](./ROADMAP.md). Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md).

**Depends on:** Core store + sample daemon (`interval_seconds > 0`) for HTTP
path. Insights recommend rows optional (tool returns empty if none). Notify
optional (status flags only).

**Goal:** Read-only MCP tool surface over **stdio** (`kpm mcp`) and **HTTP/SSE**
(daemon when `mcp.enabled`), sharing one HTTP mux with process ops endpoints.
Core sample path stays unaware of MCP. JSON REST / charts stay in **P5 server**.

**Checklist convention:** `[x]` done · `[ ]` remaining — **do not delete** rows
(history + validation).

---

## Band checklist (P4)

### Docs / planning

- [x] `docs/plan-0.5.0.md` (this file)
- [x] ROADMAP Current focus → 0.5.x MCP + link this plan
- [x] ROADMAP `#16a` ops mux row + note JSON API → P5
- [x] SPEC §7.3 draft tools table (store-only MVP) + §7.4 ops paths
- [ ] `configs/kpm.sample.yml` mcp knobs + ops path comments (implement train)
- [ ] man `kpm.1` / `kubectl-kpm.1` — `mcp` command + daemon HTTP notes
- [ ] CHANGELOG `[Unreleased]` MCP section
- [ ] Anonymized MCP tool call / response examples in SPEC (band exit)

### `#16a` Shared HTTP mux + ops

- [ ] Package `internal/httpserver` — `net/http` mux (no SPA)
- [ ] `GET /healthz` — process up (liveness)
- [ ] `GET /readyz` — store open (+ kube/Metrics reachable when sample daemon active)
- [ ] `GET /metrics` — **process** Prometheus text (ticks, notify fan-out errors if wired, SQLite write latency); **not** cluster pod re-export
- [ ] Wire listen from `mcp.listen` when `mcp.enabled` + daemon (`interval_seconds > 0`)
- [ ] Default bind loopback; fail closed on listen error
- [ ] Unit tests (httptest) for probes + metrics content-type
- [ ] Core / `internal/sample` must **not** import `httpserver` or `mcp`

### `#16` Stdio MCP + tools

- [ ] Package `internal/mcp` — tool registry (read-only)
- [ ] Tool `list_samples` — `store.ListRecords` + ns/workload/window filters; hard row cap
- [ ] Tool `summarize_window` — aggregates over samples (count; util stats when request cols present); no second Metrics API call
- [ ] Tool `list_recommendations` — `store.LatestRecommendations`; optional grain
- [ ] Tool `get_status` — version, store path, sample interval, notify/mcp enabled flags; **no** secrets
- [ ] CLI `kpm mcp` — stdio transport; opens store read-only; no daemon required
- [ ] Same tool surface as HTTP/SSE (`#17`)
- [ ] Unit tests with temp store fixtures

### `#17` HTTP/SSE + bearer

- [ ] Mount MCP HTTP/SSE on `#16a` mux when `mcp.enabled`
- [ ] Bearer via `token_env`: **required** if bind non-loopback; optional but recommended on loopback (SPEC §7.2)
- [ ] Fail closed: enabled + non-loopback + missing/empty token env → config/start error
- [ ] Ops routes (`/healthz`, `/readyz`, `/metrics`) stay unauthenticated (kube probes)
- [ ] Integration tests: unauthorized MCP rejected; authorized tool call OK

### Gates / band exit

- [ ] `make lint` + `make cover-check` ≥ 80%
- [ ] SPEC §7 → Normative in status banner + CLI map `mcp`
- [ ] SPEC anonymized tool examples (mold §6.4 style)
- [ ] PR feat → `develop` merged
- [ ] Version-cut artifacts (`chore/release-0.5.0`) when shipping
- [ ] VHS re-record if help/version strings change
- [ ] PR develop → main + tag `v0.5.0` (operator OK)
- [ ] Sync main → develop via PR

[↑ Back to top](#top)

---

## Layering

```
internal/httpserver/  — mux, /healthz, /readyz, /metrics (process)
internal/mcp/         — tools registry + stdio + HTTP/SSE handlers
internal/cli/         — kpm mcp; daemon mounts httpserver when mcp.enabled
internal/sample/      — unchanged (no import of mcp / httpserver)
internal/store/       — read APIs only from tools
```

| Rule | Why |
|------|-----|
| **Core must not import mcp/httpserver** | Sample→SQLite stays lean |
| **One mux / one listen (P4)** | `mcp.listen`; P5 mounts charts/API on same mux (unify ports later) |
| **Store-only tools in 0.5.x** | Live cluster + notify FSM dump deferred |
| **API JSON ≠ MCP** | REST `/api/*` + charts → P5 `server` |

[↑ Back to top](#top)

---

## Tools MVP (locked for 0.5.x)

| Tool | Source | Notes |
|------|--------|-------|
| `list_samples` | `store.ListRecords` | Filters; hard row cap |
| `summarize_window` | Aggregates over samples | No Metrics API |
| `list_recommendations` | `store.LatestRecommendations` | Optional grain |
| `get_status` | Config + version meta | No secrets |

**Deferred:** live `internal/live` tools; notify FSM dump; write/apply tools (**rejected**).

[↑ Back to top](#top)

---

## Merge order

| Order | ID | Deliverable | Status |
|------:|----|-------------|--------|
| 1 | docs | This plan + ROADMAP focus + SPEC §7.3 draft | [x] |
| 2 | `#16a` | `internal/httpserver` + probes + daemon wire | [ ] |
| 3 | `#16` | tools + `kpm mcp` stdio | [ ] |
| 4 | `#17` | HTTP/SSE + bearer + tests | [ ] |
| 5 | docs ship | man / sample / CHANGELOG / SPEC normative | [ ] |
| 6 | cut | `chore/release-0.5.0` → main + tag (later) | [ ] |

**Suggested PRs:** docs-first (this), then `feat/0.5-mcp` (code slices) → develop.

[↑ Back to top](#top)

---

## Success criteria (band done)

- [ ] `kpm mcp` stdio exposes the four store-only tools
- [ ] Daemon + `mcp.enabled` serves MCP HTTP/SSE + `/healthz` `/readyz` `/metrics`
- [ ] Bearer rules per SPEC §7.2; fail-closed off-loopback
- [ ] Core sample works with mcp disabled / absent
- [ ] Cover ≥ 80%; gocyclo ≤ 14
- [ ] SPEC §7 normative; merged + release cut `v0.5.0`

## Out of scope

- [ ] REST `/api/*`, embedded charts, SPA (→ P5 `#18`–`#19`)
- [ ] Prometheus re-export of cluster pod metrics
- [ ] ServiceMonitor / probe manifests (→ `kpm-selfhosted`)
- [ ] Live cluster MCP tools (`internal/live`)
- [ ] Notify engine state dump over MCP
- [ ] mTLS
- [ ] Write tools / apply patches (**rejected**)

[↑ Back to top](#top)
