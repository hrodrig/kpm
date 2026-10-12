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
- [x] `configs/kpm.sample.yml` mcp knobs + ops path comments (shipped `v0.5.0`)
- [x] man `kpm.1` / `kubectl-kpm.1` — `mcp` command + daemon HTTP notes (`v0.5.0`)
- [x] CHANGELOG MCP section (moved into `[0.5.0]`)
- [x] Client integration examples in SPEC §7.5 (Cursor / Claude / Codex / Hermes / generic)
- [x] Anonymized MCP tool call / response examples in SPEC (band exit)

### `#16a` Shared HTTP mux + ops

- [x] Package `internal/httpserver` — `net/http` mux (no SPA)
- [x] `GET /healthz` — process up (liveness)
- [x] `GET /readyz` — store open (+ kube discovery when clientset present)
- [x] `GET /metrics` — **process** Prometheus text (ticks, notify fan-out errors, SQLite write avg); **not** cluster pod re-export
- [x] Wire listen from `mcp.listen` when `mcp.enabled` + daemon (`interval_seconds > 0`)
- [x] Default bind loopback; fail closed on listen error; `token_env` required off-loopback
- [x] Unit tests (httptest) for probes + metrics content-type
- [x] Core / `internal/sample` must **not** import `httpserver` or `mcp`

### `#16` Stdio MCP + tools

- [x] Package `internal/mcp` — tool registry (read-only)
- [x] Tool `list_samples` — `store.ListRecords` + ns/workload/window filters; hard row cap
- [x] Tool `summarize_window` — aggregates over samples (count; util stats when request cols present); no second Metrics API call
- [x] Tool `list_recommendations` — `store.LatestRecommendations`; optional grain
- [x] Tool `get_status` — version, store path, sample interval, notify/mcp enabled flags; **no** secrets
- [x] CLI `kpm mcp` — stdio transport; opens store; no daemon required
- [x] Same tool surface as HTTP/SSE (`#17`)
- [x] Unit tests with temp store fixtures

### `#17` HTTP/SSE + bearer + access control

- [x] Mount MCP HTTP/SSE on `#16a` mux when `mcp.enabled` (`/sse` + streamable `/mcp`)
- [x] gghstats-style access: default loopback listen; `trusted_proxies`;
  `allowlist_cidrs`; per-IP `rate_limit` (ops exempt)
- [x] Bearer via `token_env`: **required** if bind non-loopback; optional but recommended on loopback (SPEC §7.2)
- [x] Fail closed: enabled + non-loopback + missing/empty token env → config/start error
- [x] Ops routes (`/healthz`, `/readyz`, `/metrics`) stay unauthenticated (kube probes)
- [x] Integration tests: unauthorized MCP rejected; authorized request not 401

### Gates / band exit

- [x] `make lint` + `make cover-check` ≥ 80% (CI on PR `#31`)
- [x] SPEC §7 → Normative in status banner + CLI map `mcp`
- [x] SPEC anonymized tool examples (mold §6.4 style) — §7.6
- [x] PR feat → `develop` merged (`#31`)
- [x] Version-cut artifacts (`chore/release-0.5.0`) when shipping
- [x] VHS re-record if help/version strings change
- [x] PR develop → main + tag `v0.5.0` (operator OK) — `#34` / tag
- [x] Sync main → develop via PR — `#35`

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

## Client integration (see SPEC §7.5)

Prefer **stdio** for IDE/agent attach. HTTP only when daemon + `mcp.enabled`.
Full anonymized snippets live in SPEC §7.5; summary:

| Client | Config | Stdio sketch |
|--------|--------|--------------|
| Cursor | `~/.cursor/mcp.json` | `command: kpm`, `args: [mcp, --config, …]` |
| Claude Desktop | `claude_desktop_config.json` | absolute path to `kpm` + same args |
| Claude Code | `claude mcp add` | `claude mcp add --transport stdio kpm -- kpm mcp --config …` |
| Codex | `~/.codex/config.toml` | `[mcp_servers.kpm]` command/args |
| Hermes | `~/.hermes/config.yaml` | `mcp_servers.kpm.command/args` |
| Generic | spawn / URL | stdio JSON-RPC or HTTP/SSE + bearer |

HTTP URL: `http://127.0.0.1:9740/sse` (and streamable `/mcp`).

[↑ Back to top](#top)

---

## Merge order

| Order | ID | Deliverable | Status |
|------:|----|-------------|--------|
| 1 | docs | This plan + ROADMAP + SPEC §7.3–7.5 draft | [x] |
| 2 | `#16a` | `internal/httpserver` + probes + daemon wire | [x] |
| 3 | `#16` | tools + `kpm mcp` stdio | [x] |
| 4 | `#17` | HTTP/SSE + bearer + tests | [x] |
| 5 | docs ship | man / sample / CHANGELOG / SPEC normative | [x] |
| 6 | cut | `chore/release-0.5.0` → main + tag `v0.5.0` | [x] `#33` / `#34` / tag; sync `#35` |

**Suggested PRs:** docs-first (this), then `feat/0.5-mcp` (code slices) → develop.
**Band closed:** `v0.5.0` shipped; next focus [plan-0.6.0.md](./plan-0.6.0.md).

[↑ Back to top](#top)

---

## Success criteria (band done)

- [x] `kpm mcp` stdio exposes the four store-only tools
- [x] Daemon + `mcp.enabled` serves MCP HTTP/SSE + `/healthz` `/readyz` `/metrics`
- [x] Bearer rules per SPEC §7.2; fail-closed off-loopback
- [x] Core sample works with mcp disabled / absent
- [x] Cover ≥ 80%; gocyclo ≤ 14 (on merge)
- [x] SPEC §7 normative (docs); release cut artifacts + tag `v0.5.0`

## Out of scope

- [ ] REST `/api/*`, embedded charts, SPA (→ P5 `#18`–`#19`)
- [ ] Prometheus re-export of cluster pod metrics
- [ ] ServiceMonitor / probe manifests (→ `kpm-selfhosted`)
- [ ] Live cluster MCP tools (`internal/live`)
- [ ] Notify engine state dump over MCP
- [ ] mTLS
- [ ] Write tools / apply patches (**rejected**)

[↑ Back to top](#top)
