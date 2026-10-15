# Plan 0.7.0 — Scale (nodes · search · store · harden leftovers)

<a id="top"></a>

Band plan for **0.7.x Scale** (was half of “P6 Harden”). Roadmap IDs:
`#20b`, `#20c`, plus store/UI scale and post-0.6 harden leftovers.
Parent: [ROADMAP.md](./ROADMAP.md). Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md).

**Depends on:** 0.6.x server (analyze-local + JSON + charts) on `develop`
(including audit train `#54`–`#67`, not yet tagged).

**Goal:** Analyze-local usable at **hundreds of workloads**; offline **node**
context; close remaining serve/sample/store/patch hardenings from the 0.6.x
security pass so operators can trust daemon + `kpm serve` at scale.

**Checklist convention:** `[x]` done · `[ ]` remaining — **do not delete** rows.

**Out of band for 0.7:** Honesty (`#24`–`#28` → 0.8), E2E/docs freeze/CI pins
(`#20`/`#21`/`#22` → 0.9), Homebrew (`#23`), async notify queue.

---

## Band checklist

### Docs / planning

- [ ] `docs/plan-0.7.0.md` (this file)
- [ ] ROADMAP Current focus → 0.7.x Scale; mermaid 0.7 / 0.8 / 0.9 → 1.0
- [ ] SPEC notes for search/filter, node snapshots, `allowed_hosts`, store caps
- [ ] `configs/kpm.sample.yml` knobs for new surface
- [ ] man pages when CLI/help strings change
- [ ] CHANGELOG `[Unreleased]` → `[0.7.0]` on version cut only

### `#20c` Serve UI search/filter

- [ ] Filter controls: namespace, owner, grain, verdict (CPU/mem)
- [ ] Client-side filter of loaded cards **or** server query params on
  `/api/v1/workloads` (prefer API params when list is large)
- [ ] Empty / no-match state; no layout thrash on type
- [ ] Unit / UI-facing tests where practical

### `#20b` Node / capacity snapshots

- [ ] Under-demand capture of node allocatable / requests / util → SQLite
- [ ] Analyze-local panel (offline) reading store (no live kube required)
- [ ] SPEC + sample config notes
- [ ] Tests with fixture store

### Store scale

- [ ] Index `(namespace, owner_name, sampled_at)` (and related as needed)
- [ ] SQL `LIMIT` / push aggregation closer to SQL for list/series paths
- [ ] Hard window / row caps documented (fail closed or clamp + log)
- [ ] MCP `list_samples` cap applied before full RAM load when possible

### UI vs rate-limit (serve)

- [ ] Exempt `GET /static/*` from rate-limit skip set
- [ ] Batch or coalesce `/api/v1/series` (kill N+1 card fan-out)
- [ ] UI handles 429 without broken `r.json()` (JSON error body or guard)

### Harden leftovers (post `#54`–`#67`)

- [ ] SMTP: per-send `context.WithTimeout` + connection `SetDeadline`
- [ ] Sample daemon: exponential backoff + time budget; log failures; ops counter
- [ ] `allowed_hosts` (or equivalent) for BrowserGuard behind reverse proxy
- [ ] Store: `OpenFile` 0600 before open; chmod `-wal`/`-shm`; safe DSN encoding
- [ ] GitOps patch: CronJob `jobTemplate` path; clean apiVersion switch
- [ ] Sample path: resolve ReplicaSet → Deployment for stable grain + patch name

### Gates / band exit

- [ ] `make lint` + `make cover-check` ≥ 80%
- [ ] SPEC Scale sections updated (status / CLI map as needed)
- [ ] Feature PRs → `develop` green
- [ ] Solo `chore/release-0.7.0` (VERSION, CHANGELOG, badge, both man `.TH`)
- [ ] `make release-check` green
- [ ] PR `develop` → `main` + annotated tag `v0.7.0` (operator OK)
- [ ] Sync `main` → `develop`; optional selfhosted / webs image pins

[↑ Back to top](#top)

---

## Merge order (suggested)

1. Docs ROADMAP + this plan  
2. Harden leftovers (SMTP, retry, hosts, WAL modes, CronJob, RS→Deploy) — unblocks serve  
3. Rate-limit / static / series batch — unblocks large UI  
4. Store index + LIMIT/caps  
5. `#20c` search/filter  
6. `#20b` node snapshots + panel  
7. Version cut `0.7.0`

[↑ Back to top](#top)
