# Plan 1.3.0 — Post-1.2 audit harden + serve UI pressure

<a id="top"></a>

Band plan for **1.3.0**. Parent: [ROADMAP.md](./ROADMAP.md). Contract:
[SPECIFICATIONS.md](../SPECIFICATIONS.md). Previous shipped minor: `v1.2.0`.

**Depends on:** `v1.2.0` on `main` and synced to `develop`.

**Goal:** Close the **v1.2.0 static audit** train (`#171`–`#178`) so operators can
trust daemon / serve / store / recommend / capacity again, and ship the **analyze-local
UI** polish (`#168`, `#169`) that stops silent Mem-pressure misses. No new CLI
commands. Schema / grain-key semantics may tighten where honesty requires it
(document in SPEC + CHANGELOG; migration notes if recommend keys gain namespace).

**Checklist convention:** `[x]` done · `[ ]` remaining — **do not delete** rows.

**Closed elsewhere:** `#170` closed as duplicate of `#169` (2026-11-21).

**Overlap note:** leftovers from [plan-1.0.0.md](./plan-1.0.0.md) `#37` (UI security
headers, IdleTimeout, BrowserGuard host checks, systemd harden) fold into `#171` /
`#177` here — do not track twice.

---

## Work units

| # | Work unit | GitHub | Status |
|---|-----------|--------|--------|
| 1 | HTTP / MCP / notify security harden | [#171](https://github.com/hrodrig/kpm/issues/171) | `[x]` |
| 2 | Store merge / restore / WAL / scan ceilings | [#172](https://github.com/hrodrig/kpm/issues/172) | `[x]` |
| 3 | Recommend grain namespace + sample fidelity | [#173](https://github.com/hrodrig/kpm/issues/173) | `[x]` |
| 4 | Daemon HTTP lifecycle, SMTP, kube.context | [#174](https://github.com/hrodrig/kpm/issues/174) | `[ ]` |
| 5 | Capacity / fit / TUI correctness | [#175](https://github.com/hrodrig/kpm/issues/175) | `[ ]` |
| 6 | Config validation + export/doctor edges | [#176](https://github.com/hrodrig/kpm/issues/176) | `[ ]` |
| 7 | CI / packaging / systemd / GoReleaser | [#177](https://github.com/hrodrig/kpm/issues/177) | `[ ]` |
| 8 | SPEC / docs / man / AGENTS sync | [#178](https://github.com/hrodrig/kpm/issues/178) | `[ ]` |
| 9 | Serve UI sticky chrome + Metric CPU\|Mem | [#168](https://github.com/hrodrig/kpm/issues/168) | `[ ]` |
| 10 | Serve UI trend pressure badges | [#169](https://github.com/hrodrig/kpm/issues/169) | `[ ]` |
| 11 | Solo version cut `chore/release-1.3.0` | — | `[ ]` |

Issue bodies remain the detailed checklist; this plan owns **merge order**,
**UI design lock**, and **band exit**.

---

## Band checklist

### Docs / planning

- [x] `docs/plan-1.3.0.md` (this file)
- [x] ROADMAP Current focus → **1.3.0**; mark **1.2.0** shipped
- [x] `docs/README.md` lists this plan
- [ ] CHANGELOG `[Unreleased]` tracks landings; cut to `[1.3.0]` only on version PR
- [ ] SPEC / man / sample updates land with the trains that change behavior (`#178` + feature PRs)

### `#171` — Security (HTTP, MCP, notify)

Track high items first (details on the issue):

- [x] Multi-header `X-Forwarded-For` (all header lines, right-to-left)
- [x] MCP `summarize_window` row cap + timeout + window validation
- [x] MCP Streamable HTTP session bounds / loopback reverse-proxy
- [x] Redact webhook/Slack secrets from errors and daemon logs
- [x] RateLimiter memory bound, Shutdown race, middleware order
- [x] HTTP `IdleTimeout` / handler timeouts
- [x] Notify: no redirect follow; validate URL schemes
- [x] `PathSkip` clean path before `/static/` match
- [x] BrowserGuard: reject `Host: 0.0.0.0`, empty Host, bad Origin schemes
- [x] UI security headers + move inline theme script to `/static/` for CSP

### `#172` — Store

- [x] `MergeFrom` preserve `taints` / `unschedulable` on `node_snapshots`
- [x] `RestoreReplace` checkpoint / include WAL safely
- [x] `Store.Open` chmod only newly created parent dirs
- [x] Surface errors from `LatestNodeSnapshots`
- [x] Deduplicate `LatestRecommendations` on `recorded_at` ties
- [x] Retention prune includes `recommendations` / `node_snapshots` (or document exclusion)
- [x] Enforce `MaxScanRows` when `Limit == 0`; Export respects ceiling
- [x] Pin `*sql.Conn` for TEMP TABLE merge; backup umask; RFC3339 normalize on merge

### `#173` — Recommend / sample

- [x] Namespace-scope workload grain keys + score/TUI match (SPEC + migration note)
- [x] No synthetic `0` util for containers missing from PodMetrics
- [x] Prefer `Controller: true` owner ref
- [x] Fix `onSampleFailFn` data race; typed-nil adapter checks
- [x] Deduplicate `scope.namespaces`; List pagination where needed
- [x] ScoreCalibration perf / percentile / CLI filter consistency
- [x] Ceil / NaN guards; patch / MemQuantity edge formatting

### `#174` — Daemon / notify / cluster

- [ ] Ops HTTP: bind before “listening”; track ListenAndServe; shutdown before store close
- [ ] SMTP timeout path: no goroutine / socket leak
- [ ] Explicit `kube.context` failure must not silent-fall back to in-cluster
- [ ] Shared facade token / allowlist / listen wiring
- [ ] Notify engine: mutate state only after successful delivery
- [ ] Reject duplicate `notifications.rules[].name`; drain sink bodies

### `#175` — Capacity / fit / TUI

- [ ] Node capacity / `--record-nodes` cluster-wide pods when scope/labels set
- [ ] TUI Δ match Deployment grain (RS→Deploy alignment with sample)
- [ ] Native sidecar + `spec.Overhead` in `EffectivePodRequest`
- [ ] Fit affinity / evaluate fidelity; fit flag + exit-code fixes
- [ ] TUI store mode, exit codes, async detail race, WindowSize clamp, detail findRes

### `#176` — Config / CLI

- [ ] Dead `scope.namespace_labels` / `scope.node_labels`: implement **or** reject/warn
- [ ] Close `validateRecommend` / `validateMCP` (rate limit when server-only) gaps
- [ ] Export: validate format before truncate; check Close
- [ ] Doctor: honest Count / schema messaging
- [ ] JSON API query validation edges; `exitcode.New` nil guard

### `#177` — CI / packaging

- [ ] govulncheck ignore parser: comments, fail-closed, no double-count
- [ ] systemd: non-root + sandbox directives (align `#37`)
- [ ] Release workflow: tag matches `VERSION`
- [ ] Scheduled security/CodeQL; scan JS UI
- [ ] Grype PATH / GOBIN; GoReleaser hygiene; Makefile staging / CGO; BSD stubs
- [ ] Distroless digest refresh; pin/readme script tighten; gitignore dead entry

### `#178` — Docs sync

- [ ] SPEC §7.4 `/readyz` vs code; §9.1.1 `KPM_*` bound set; fix sample link
- [ ] `docs/README.md` PATH typo; AGENTS packages / version banner
- [ ] Stale ROADMAP / README / measurement / man / sample / workflow comments for 1.2→1.3

### `#168` — Serve UI sticky + Metric (design locked)

Client-only (`internal/server/ui/`). Approach A.

- [ ] Sticky `#chrome`: brand + toolbar + filters (`position: sticky; top: 0; z-index`; solid background)
- [ ] Replace metric `<select>` with **CPU | Mem** segment + **util | vs request** segment
- [ ] Map to existing API metric IDs; default CPU + util
- [ ] Persist in `localStorage` (`kpm_metric` or resource+scale keys)
- [ ] Filter first options: `CPU recommend` / `Mem recommend` (not “any”)
- [ ] Smoke: ≥50 cards, scroll mid-list, change metric, charts refresh

### `#169` — Serve UI pressure badges (design locked)

Client-only on util series (not ratio charts). Independent of rightsizing verdict.

| Trigger | Badge | Severity |
|---------|--------|----------|
| `max(util) > req` **or** ≥25% points `> req` | `above request` | watch |
| `max(util) ≥ 0.8×lim` **or** ≥25% points `≥ 0.8×lim` | `near limit` | warn (wins if both) |
| slope only / headroom | none | — |

- [ ] Card badge + optional Details note; no toast spam
- [ ] Title/hint: pressure ≠ rightsizing when `no recommend`
- [ ] Works for CPU and Mem util metrics
- [ ] No badge when req/lim unset

### Gates / ship

- [ ] High-priority items in `#171`–`#175` closed or explicitly deferred with issue comment
- [ ] `make release-check` green on the commit that will be tagged
- [ ] Solo `chore/release-1.3.0` → develop → main → annotated `v1.3.0` — **only with operator OK**
- [ ] Sync PR `main` → `develop` after tag

---

## What this band deliberately does not do

- Per-card dual CPU+Mem sparklines; external notify for pressure (Slack/PD)
- New SPA / npm; Prometheus second source; multi-cluster
- Capacity-table wishlist `#38`–`#44` from plan-1.0.0 (still later 1.x unless pulled)
- json export, mTLS, history-in-TUI as product features

---

## Merge order

Prefer **topic PR per issue** (or tight pairs) into `develop`. Suggested order:

1. `#171` security (unblocks trust for everything listening)
2. `#172` store integrity (data correctness)
3. `#173` recommend/sample fidelity (grain namespace — coordinate SPEC early)
4. `#174` daemon / notify / kube.context
5. `#175` capacity / fit / TUI
6. `#176` config / CLI edges
7. `#177` CI / packaging (can parallel earlier if no code deps)
8. `#168` + `#169` UI (same PR OK — shared `app.js`/`app.css`)
9. `#178` docs sweep (or incremental with each train; final pass before cut)
10. Solo `chore/release-1.3.0`

**Success:** audit highs closed; UI acceptance for `#168`/`#169` met; `make release-check` + required PR checks green; tag `v1.3.0` on `main`.

[↑ Back to top](#top)
