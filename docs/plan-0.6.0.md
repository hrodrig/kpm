# Plan 0.6.0 — Server (analyze-local · charts · JSON API)

<a id="top"></a>

Band plan for **0.6.x Server** (SPEC §8 / P5). Roadmap IDs: `#18`, `#18b`,
`#19` (+ facade listen decoupling). Parent: [ROADMAP.md](./ROADMAP.md).
Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md).

**Depends on:** Core SQLite store (samples + optional recommendations). MCP ops
mux (`internal/httpserver`) from 0.5.x. Live Metrics / kube **optional**.

**Goal:** Read-only **embedded charts + JSON API** so an operator can analyze
pod metrics **locally** from a copied SQLite DB (no client-network access), or
on a bastion with optional live kube enrichment. Collection (sample loop) must
be **independently disableable** while MCP HTTP / server / ops still listen.

**Checklist convention:** `[x]` done · `[ ]` remaining — **do not delete** rows
(history + validation).

---

## Product story (locked for this band)

### Analyze-local (primary)

1. On the client bastion: `kpm sample` daemon fills SQLite (or periodic one-shot).
2. Operator copies the DB / `kpm backup` artifact to a laptop.
3. On the laptop: open **store only** — no scrape, no kube required.
4. Laptop serves charts + JSON + MCP HTTP (stdio already works) against that DB.
5. Time windows: presets (1h / 24h / 48h / 7d / 15d) + custom `since`/`until`.

### Bastion / with kube (optional enrichment)

Same UI/API. When kubeconfig works:

- Charts may overlay **current** util (live Metrics) on historical series.
- Per-workload drawer shows **live** requests/limits (owner template / pod) in
  addition to store recommendations.

Without kube: history + recommendations only; live fields = `n/a`.

### UI sketch (per workload / service)

- Chart card: historical CPU/mem util (and ratio vs request when present).
- Collapsible drawer under each chart:
  - Latest **recommendations** from store (grain workload/pod).
  - If kube: current **request/limit** (+ delta vs suggest when both exist).
  - `#18b`: copy/download **Kustomize patch/diff** (no cluster apply).

Grain default for cards: **workload** (namespace + owner); drill to pod later
if needed.

---

## Band checklist (P5)

### Docs / planning

- [x] `docs/plan-0.6.0.md` (this file)
- [x] ROADMAP Current focus → 0.6.x Server + link this plan
- [x] SPEC §8 expand: analyze-local, listen decoupling, UI contract (API paths in implement)
- [x] SPEC §9.1.1 config precedence: defaults → YAML → ENV → CLI
- [ ] `configs/kpm.sample.yml` `server:` + analyze-local comments (implement train)
- [ ] man pages: `serve` / server flags + listen without sample
- [x] CHANGELOG `[Unreleased]` Server plan section
- [ ] Anonymized API/UI examples in SPEC (band exit; mold §6.4 / §7.6)

### Config / CLI contract (band-wide)

Precedence (**lowest → highest**):

```
defaults → YAML (--config) → ENV (KPM_*) → CLI flags (Changed only)
```

Rules:

- Every new YAML knob in this band gets `BindEnv` + Cobra flag (or documented
  exception).
- Secrets stay `*_env` names in YAML; values only from environment (fail closed).
- CLI must be able to override: `store.path`, `sample.interval_seconds`,
  `mcp.enabled` / `mcp.listen` / `mcp.token_env`, `server.enabled` / listen /
  auth / token_env, kube context (empty = no live), window presets if exposed.

### `#19a` Facade listen without sample (analyze-local enablement)

Decouple HTTP mux from daemon sample loop (today: MCP listen requires
`interval_seconds > 0`).

- [x] Start ops + MCP HTTP (+ later server) when `mcp.enabled` and/or
      `server.enabled`, **even if** `sample.interval_seconds == 0`
      (`kpm serve`; `server.enabled` still pending `#19`/`#18`)
- [x] No sample ticks / no Metrics scrape in that mode
- [x] `/readyz`: store open required; kube **optional** (skip or soft-fail when
      no clientset / unreachable)
- [x] Prefer dedicated CLI entry: `kpm serve` (open store → mux → block on
      signal) **or** document `kpm sample` path that listens without looping —
      pick one in implement PR; default recommendation: **`kpm serve`**
- [x] Unit/integration: serve with temp DB, `interval=0`, hit `/healthz` + MCP
      mount; assert no sample writes

### `#19` JSON read API + auth

- [x] Mount under same mux (default loopback; share `mcp.listen` **or**
      `server.listen` — decide: **one listen** when both enabled, prefer
      `server.listen` if set else `mcp.listen`)
- [x] Prefix `/api/v1/` (read-only)
- [x] Endpoints (MVP):
  - `GET /api/v1/status` — version, store path, sample interval, flags; no secrets
  - `GET /api/v1/workloads` — list grains with sample activity in window
  - `GET /api/v1/series` — query: namespace, workload, metric, since, until
    (downsample server-side for large windows)
  - `GET /api/v1/recommendations` — latest rows; optional grain filter
  - `GET /api/v1/live` — optional; 503/empty JSON when kube unavailable
- [x] Bearer auth via `server.token_env` (or shared with MCP when both on) —
      required off-loopback; optional on loopback
- [x] Reuse gghstats-style access already on mux (trusted_proxies / allowlist /
      rate_limit); ops paths stay exempt
- [x] Fail closed: auth required + empty env → start error
- [x] Tests: httptest unauthorized; series from fixture store; live without kube

### `#18` Embedded charts UI

- [ ] Boring UI: Go `html/template` + minimal JS (HTMX **or** small `fetch` —
      no npm/SPA)
- [ ] Pages: overview (workload cards) + optional detail
- [ ] Each card: chart from `/api/v1/series` + collapsible drawer
      (recommendations; live req/limit when `/api/v1/live` ok)
- [ ] Window controls: presets **1h / 24h / 48h / 7d / 15d** + custom range
- [ ] Static assets embedded (`embed`); no external CDN requirement
- [ ] Chart lib: pick one embeddable approach in implement PR (canvas/SVG or
      single vendored file) — keep binary lean
- [ ] Apply **Visual guidance** below (tokens / states / motion); not landing
      page structure
- [ ] Core / `internal/sample` must **not** import server UI packages

### `#18b` Recommend GitOps export in UI

- [ ] Drawer actions: copy/download **Kustomize patch/diff** (and/or
      strategic-merge / JSON patch) per workload; optional bulk later
- [ ] **Never** apply to cluster
- [ ] Reuse CLI recommend formatters when present; otherwise minimal formatter
      in `internal/recommend` or `internal/server`

### Gates / band exit

- [ ] `make lint` + `make cover-check` ≥ 80%
- [ ] SPEC §8 → Normative in status banner + CLI map (`serve` / server)
- [ ] SPEC anonymized API + UI examples
- [ ] PR feat → `develop` merged
- [ ] Version-cut artifacts (`chore/release-0.6.0`) when shipping
- [ ] VHS re-record if help/version strings change
- [ ] PR develop → main + tag `v0.6.0` (operator OK)
- [ ] Sync main → develop via PR

[↑ Back to top](#top)

---

## Visual guidance

Two agent skills — **tokens** + **anti-slop**. Neither ships as a runtime dependency
(no `npx` in the binary). Use when implementing / reviewing `#18` HTML+CSS.

### A. Tokens — [ai-design-skills](https://github.com/elayadesign/ai-design-skills)

Source: [landing-page-design](https://github.com/elayadesign/ai-design-skills/tree/main/skills/landing-page-design)
**Part B** only.

**Adopt (ops UI):**

| Topic | Take from skill |
|-------|-----------------|
| Type | Single family (Geist / Manrope / Poppins); Geist Mono OK for numbers/data; no italics; weight ≤ bold |
| Type scale | Snap sizes to a fixed scale (12–24px body/UI; larger only for page title) |
| Spacing | Discrete steps only (2 / 4 / 8 / 12 / 16 / 24 / 32 / …) — no magic `13px` gaps |
| Radius | Consistent radius scale; nested radius ≈ outer − gap when gap &lt; 32px |
| Borders | Full border or none — no one-sided card rules |
| Backgrounds | Flat (no background gradients); prefer light ops theme by default |
| Icons | Phosphor / Solar / Iconamoon (not Material) |
| Motion | Custom easing (e.g. `cubic-bezier(0.32, 0.72, 0, 1)`); drawer open/close; no scroll spam listeners |
| States | Hover / active / focus / loading (skeleton) / empty / error for controls |
| Content | Real labels; no Lorem; sentence case; no AI cliché copy |

**Do not adopt (landing-only):**

- Full-bleed hero, brand-first marketing viewport, single primary CTA funnel
- Mandatory tagline reveal / staggered word scroll theater
- Glass floating nav island / hamburger morph
- “One offer → one audience” page structure (Part A strategy)
- Ban on interactive cards — workload **chart cards + drawers** are in scope

### B. Anti-slop — [Hallmark](https://github.com/nutlope/hallmark)

Source: [nutlope/hallmark](https://github.com/nutlope/hallmark) (skill + slop-test gates).

**Adopt:**

- Refuse on-distribution LLM defaults (generic purple/glow, Inter-only stacks,
  empty decorative cards, vague “seamless” copy).
- One deliberate fingerprint for **kpm ops / analyze-local** (sober, data-first) —
  not a new marketing theme per page.
- Before shipping `#18` UI: run Hallmark-style **audit** mindset (or skill
  `audit` if installed) against embedded templates/CSS; fix punch-list items
  that apply to dense ops UI.
- Prefer craft that still works as self-contained HTML+CSS (matches `embed`).

**Do not adopt:**

- Catalog marketing themes (Carnival, Riso, atmospheric travel heroes, …) as
  the daemon skin.
- Full landing rebuild verbs (`redesign` / `study` → pixel DNA) as the default
  workflow for every chart tweak — reserve for rare visual reset, not day-to-day.

### Constraints for kpm binary

- No npm build; CSS (+ optional tiny JS) embedded
- No CDN fonts required at runtime (system stack OK if we skip webfont fetch;
  if embedding a face, keep one file / woff2 under `embed`)
- Accessibility: focus rings, semantic landmarks, skip-link if multi-region page

When implementing `#18`: resolve spacing/type/radius from **Elaya Part B**;
pass **Hallmark** anti-slop gates before merge.

[↑ Back to top](#top)

---

## Layering

```
internal/httpserver/  — shared mux, ops, access control (0.5.x)
internal/mcp/         — MCP tools (unchanged contract)
internal/server/      — JSON API + templates/assets (new)
internal/live/        — optional live snapshot for /api/v1/live + drawer
internal/store/       — series queries (may add helpers for downsample)
internal/cli/         — kpm serve; wire flags/env overrides
internal/sample/      — untouched; no import of server
```

| Rule | Why |
|------|-----|
| **Core must not import server** | Sample→SQLite stays lean |
| **Store-first UI** | Analyze-local works offline |
| **Kube optional** | Live is enrichment, not required |
| **One process ↔ one store/cluster** | Same as MCP |
| **No apply** | GitOps overwrites; `#18b` export only |
| **YAML knobs ↔ ENV ↔ CLI** | Operator overrides without editing files |

[↑ Back to top](#top)

---

## Config sketch (target)

```yaml
store:
  path: ~/Downloads/prod-east.db   # copied / backup from client

sample:
  interval_seconds: 0             # no collection on laptop

kube:
  context: ""                     # empty → no live enrichment

mcp:
  enabled: true
  listen: "127.0.0.1:9740"
  token_env: KPM_MCP_TOKEN        # optional on loopback

server:
  enabled: true
  # listen: ""                    # empty → share mcp.listen
  auth: false                     # true → token_env required
  # token_env: KPM_SERVER_TOKEN
```

CLI examples (target):

```bash
kpm serve --config laptop.yaml
kpm serve --config laptop.yaml --store-path ./prod-east.db --listen 127.0.0.1:9740
kpm serve --config laptop.yaml --interval 0 --mcp=true --server=true
```

Exact flag names locked in implement PR; must cover sample/mcp/server/store/kube.

[↑ Back to top](#top)

---

## Merge order

| Order | ID | Deliverable | Status |
|------:|----|-------------|--------|
| 1 | docs | This plan + ROADMAP + SPEC §8 draft expand | [x] |
| 2 | `#19a` | Listen / `kpm serve` without sample loop | [x] (this PR) |
| 3 | `#19` | JSON `/api/v1/*` + auth | [x] (this branch) |
| 4 | `#18` | Charts UI + drawers + windows | [ ] |
| 5 | `#18b` | Kustomize/patch export in drawer | [ ] |
| 6 | cut | `chore/release-0.6.0` → main + tag | [ ] |

**Suggested PRs:** docs-first (done `#36`), `feat/0.6-serve` (`#19a`), then
`#19` JSON, then `feat/0.6-charts` (`#18`), then `#18b` (can ride charts PR).

[↑ Back to top](#top)

---

## Success criteria (band done)

- [ ] Laptop + copied SQLite: `kpm serve` (or equivalent) shows charts + API
      with `interval_seconds: 0` and no kube
- [ ] Bastion + kube: live overlay / drawer req-limit when available
- [ ] MCP HTTP still works on same mux in analyze-local mode
- [ ] Bearer / access-control rules consistent with 0.5.x
- [ ] No cluster mutations; `#18b` is copy/download only
- [ ] Cover ≥ 80%; gocyclo ≤ 14; SPEC §8 normative; cut `v0.6.0`

## Out of scope

- SPA / npm app; Prometheus re-export of pod metrics
- Multi-cluster process; in-cluster Helm/`run/` (→ kpm-selfhosted)
- Native PagerDuty; Loki
- Auto-apply of recommendations
- Replacing `capacity` / `tui` CLIs

[↑ Back to top](#top)
