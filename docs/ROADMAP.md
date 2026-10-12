# Roadmap — kpm

<a id="top"></a>

Product path from scaffold to **v1.0.0**. Behavior contract:
[SPECIFICATIONS.md](../SPECIFICATIONS.md). Shipped history:
[CHANGELOG.md](../CHANGELOG.md). Band plans (when needed): `docs/plan-0.Y.Z.md`.

**v1.0 cut (decided):** tag `v1.0.0` only when CORE + adjacent
(`recommend`, `capacity`, `tui`) + optional facades (notifications
multi-sink, MCP stdio+HTTP, server embedded charts) are **stable and
normative**. `kpm-selfhosted` (cron/Helm) is out of band — does not block 1.0.

**Last reviewed:** 2026-10-11

---

## Current focus

| Band | Focus | Plan |
|------|-------|------|
| **0.2.x Core** | P1 shipped `v0.2.0` | [plan-0.2.0.md](./plan-0.2.0.md) |
| **0.3.x Insights** | P2 shipped `v0.3.0` (`#8`–`#11`, `#9b`) | [plan-0.3.0.md](./plan-0.3.0.md) |
| **0.4.x Notify** | P3 shipped `v0.4.0` (`#12`–`#15`, multi-sink) | [plan-0.4.0.md](./plan-0.4.0.md) |
| **Current** | **0.6.x Server** (analyze-local · charts · JSON) | [plan-0.6.0.md](./plan-0.6.0.md) |

---

## Semver bands

```mermaid
flowchart LR
  scaffold["0.1.0 scaffold"] --> core["0.2.x Core"]
  core --> insights["0.3.x Insights"]
  insights --> notify["0.4.x Notify"]
  notify --> mcp["0.5.x MCP"]
  mcp --> server["0.6.x Server"]
  server --> harden["0.7.x Harden P6"]
  harden --> v1["1.0.0 Stable"]
```

| Band | SPEC | Status |
|------|------|--------|
| **0.1.x** Scaffold | mold + CLI version/sample-config | **Shipped** `v0.1.0` |
| **0.2.x** Core (P1) | §2 sample, store, daemon sample-only, backup, export | **In tree** (SPEC draft v0.4 normative Core) |
| **0.3.x** Insights (P2) | §3–5 recommend, capacity, tui | **Shipped** `v0.3.0` |
| **0.4.x** Notify (P3) | §6 thresholds + multi-sink (slack/webhook/smtp) | **Shipped** `v0.4.0` |
| **0.5.x** MCP (P4) | §7 stdio + HTTP/SSE + ops mux | **Shipped** `v0.5.0` |
| **0.6.x** Server (P5) | §8 analyze-local + charts + JSON API (no SPA) | **Band exit** (cut `v0.6.0` pending) |
| **0.7.x** Harden (P6) | nodes snap + UI search/filter + E2E/docs/security | Planned |
| **1.0.0** Stable | Full SPEC surface normative | Planned |

[↑ Back to top](#top)

---

## Global work items

IDs are **stable across bands**. Cite in CHANGELOG as `(0.X.x #N)`.

### Shipped — 0.1.x

| ID | Item | Status |
|----|------|--------|
| — | Repo mold, `kpm version`, `--print-sample-config`, SPEC draft v0.3 | Done (`v0.1.0`) |

### 0.2.x — Core (P1)

| ID | Item | Status |
|----|------|--------|
| `#1` | Config load (`kpm.yaml`, Viper) + validation for `cluster` / `kube` / `client` / `store` / `sample` / `scope`; `client.id` cascade | **Done** (in tree) |
| `#2` | client-go Metrics + Pods; `kpm sample` one-shot → SQLite | **Done** (one-shot; daemon → `#4`) |
| `#3` | Schema version + retention prune + indexes | **Done** (in tree) |
| `#4` | Daemon loop (`interval_seconds > 0`) sample-only; SIGINT/TERM clean | **Done** (in tree) |
| `#5` | `kpm backup` (SQLite `VACUUM INTO`) | **Done** (in tree) |
| `#6` | `kpm export` csv/tsv + filters | **Done** (in tree) |
| `#7` | Exit codes 0/1/2 for cluster/RBAC fail-closed | **Done** (exit `2` wired) |
| `#24` | FreeBSD + OpenBSD port skeletons + `make port-*-sync` / `dist-*` | **Done** (in tree) |
| `#25` | VHS `docs/demo.tape` + `docs/demo.gif` (offline Core CLI demo) | **Done** (in tree) |
| `#26` | Document `kubectl kpm` plugin (`install-kubectl-plugin`, man, README) | **Done** (in tree) |

**Success:** fixtures or kind+metrics-server path; SPEC §2 normative; cover ≥ 80%.

### 0.3.x — Insights (P2)

| ID | Item | Status |
|----|------|--------|
| `#8` | Shared read model (live snapshot helpers) | **In tree** (`internal/live`) |
| `#9` | `kpm recommend` (dual grain, p/0.70, offline) | **In tree** (`kpm recommend`) |
| `#9b` | Persist recommend + score + **TUI delta feed** (**P2 required**) | **In tree** (`--record` / `--score`) |
| `#10` | `kpm capacity` table | **In tree** (`kpm capacity`) |
| `#11` | `kpm tui` (+ `capacity --tui`): heatmap + delta vs last recommend | **In tree** (`kpm tui`) |
| `#11b` | TUI Enter: owner template (Deploy/STS/DS) vs live Pod resources | **In tree** (Enter detail) |

**Band plan:** [docs/plan-0.3.0.md](./plan-0.3.0.md).

**Success (P2 exit):** `#8`–`#11` **and** `#9b`; SPEC §3–5 normative; VHS if help/UX change.
TUI **must** show deviation vs last recommendation when a `#9b` snapshot exists.

### 0.4.x — Notifications (P3)

Checklist detail: [plan-0.4.0.md](./plan-0.4.0.md) (`[x]`/`[ ]` — keep history).

| ID | Item | Status |
|----|------|--------|
| `#12` | Rule engine (hold / repeat / resolve / dedupe) | **Done** (`v0.4.0`) |
| `#13` | Built-in 80% defaults + startup log when rules empty | **Done** (`v0.4.0`) |
| `#14` | Multi-sink fan-out: `slack`, `webhook` (`generic`/`discord`/`teams`), `smtp`; fail-closed `*_env` | **Done** (`v0.4.0`) |
| `#15` | `kpm notify-test` | **Done** (`v0.4.0`) |

**Success:** SPEC §6 normative; daemon + notify without MCP/server.
**Note:** PagerDuty via generic `webhook` (no native type). Loki deferred.

### 0.5.x — MCP (P4)

Checklist detail: [plan-0.5.0.md](./plan-0.5.0.md) (`[x]`/`[ ]` — keep history).

| ID | Item | Status |
|----|------|--------|
| `#16a` | Shared daemon HTTP mux (`internal/httpserver`): `/healthz`, `/readyz`, process `/metrics`; listen via `mcp.listen` | **Done** (`v0.5.0`) |
| `#16` | `kpm mcp` stdio read-only store tools (`list_samples`, `summarize_window`, `list_recommendations`, `get_status`) | **Done** (`v0.5.0`) |
| `#17` | HTTP/SSE on same mux when `mcp.enabled`; bearer + gghstats-style access control (SPEC §7.2) | **Done** (`v0.5.0`) |

**Success:** SPEC §7 normative; ops probes on MCP listen; core unaware of MCP.
**Non-goal this band:** REST `/api/*` / charts (→ P5); Prometheus re-export of pod metrics; live-cluster MCP tools.

### 0.6.x — Server (P5)

Checklist detail: [plan-0.6.0.md](./plan-0.6.0.md) (`[x]`/`[ ]` — keep history).

| ID | Item | Status |
|----|------|--------|
| `#19a` | Facade listen / `kpm serve` **without** sample loop (analyze-local) | **Done** (PR) |
| `#19` | JSON `/api/v1/*` + bearer; shared read model; optional live | **Done** (PR) |
| `#18` | Embedded charts UI (history + drawers + time windows) on shared mux | **Done** (PR) |
| `#18b` | Recommend UI: **Kustomize patch/diff** copy/download (no cluster apply) | **Done** (PR) |
| `#18c` | Cards + summary: oversized / undersized / ok / no-req from recommend store | **Done** (PR) |

**Success:** SPEC §8 normative (incl. analyze-local + §8.3/§8.4); **no** SPA/npm app.
**Note:** Primary story = copy SQLite to laptop → charts/MCP/HTTP with collection off; kube live is optional enrichment.
**Non-goal:** server/CLI applying CPU/mem patches live (GitOps overwrites) — SPEC §15.9.
Visual guidance for templates: [ai-design-skills](https://github.com/elayadesign/ai-design-skills)
(Part B tokens) + [Hallmark](https://github.com/nutlope/hallmark) (anti-slop audit) —
see [plan-0.6.0 Visual guidance](./plan-0.6.0.md#visual-guidance); no SPA/npm;
no landing funnel. Marketing sites (`kpm-webs`) stay a separate repo.

### 0.7.x — Harden (P6) toward 1.0

| ID | Item | Status |
|----|------|--------|
| `#20` | Integration/E2E matrix (one-shot, daemon, recommend, tui, notify, mcp, server) | Planned |
| `#20b` | Node / capacity under-demand SQLite snapshots + analyze-local panel | Planned |
| `#20c` | Serve UI **search/filter** (ns / owner / grain / verdict) for large grids (hundreds of pods) | Planned |
| `#21` | Docs freeze: SPEC → normative v1.0; man pages; sample config complete | Planned |
| `#22` | Security pass; minimal RBAC ClusterRole example in docs (not Helm) | Planned |
| `#23` | Homebrew tap (deferred from 0.1) if still wanted | Planned |

**Success:** no open draft on shipped surface; `make release-check` + Security green; UI usable at scale (`#20c`).

### 1.0.0 — Stable

- Version cut + annotated tag on `main`.
- Semver promise: no breaking CLI/config without major.
- **Post-1.0 (not blocking):** Slack sinks, json export, mTLS, `kpm-selfhosted`, history-in-TUI.

[↑ Back to top](#top)

---

## Process

1. Work on topic branches from `develop`; PR into `develop`.
2. Large bands get `docs/plan-0.Y.Z.md` (merge order + success criteria).
3. Promote SPEC sections to **Normative** as each band ships.
4. Never tag `main` / run GoReleaser without explicit operator approval.

[↑ Back to top](#top)
