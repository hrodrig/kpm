# Plan 0.3.0 — Insights (recommend · capacity · tui)

<a id="top"></a>

Band plan for **0.3.x Insights** (SPEC §3–5 / P2). Roadmap IDs: `#8`–`#11`.
Parent: [ROADMAP.md](./ROADMAP.md). Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md)
(draft v0.4 — Core normative; adjacent still draft until this band ships).

**Depends on:** Core P1 in-tree (`sample` → SQLite, `scope`, `recommend`/`tui`
config knobs already in YAML).

**Goal:** Offline rightsizing from history + live capacity table + interactive
heatmap TUI — without mutating the cluster.

---

## Layering (critical)

```
internal/live/     — shared Pod+Metrics snapshot → rows (capacity + tui + future server)
internal/recommend/ — pure SQLite → percentile → verdicts (no cluster)
internal/cli/      — recommend, capacity, tui commands
```

| Rule | Why |
|------|-----|
| **Core must not import** recommend/tui UI | Keep sample path lean |
| **recommend** = store only | Offline; exit `3` if insufficient samples |
| **capacity / tui** = live API | No SQLite required; reuse `#8` read model |
| **tui** does not write store | Collector stays `sample`/daemon |

[↑ Back to top](#top)

---

## Merge order

| Order | ID | Deliverable | Depends |
|------:|----|-------------|---------|
| 1 | `#8` | `internal/live`: list pods+metrics under `scope` (+ optional `tui.pods` / labels); row type with util/request/limit ratios; unit tests with fake clientsets | Core `#2` helpers |
| 2 | `#10` | `kpm capacity` — one-shot text/table to stdout (kube-capacity-like columns); `--config`; exit `2` on API fail | `#8` |
| 3 | `#9` | `internal/recommend` + `kpm recommend` — window/percentile/`min_samples`; dual grain; table/tsv; exit `3` if empty | store schema |
| 4 | `#9b` | Persist last recommend snapshots (`recommendations` table / `--record`); offline score helper; **required for P2** | `#9` |
| 5 | `#11` | `kpm tui` (+ `capacity --tui`) — heatmap + **delta vs last recommendation** (from `#9b`); bubbletea + lipgloss | `#8`, `#9b` |

**Suggested PR splits:** `#8`+`#10` together; `#9` solo; `#9b` solo (or with `#11` if small); `#11` solo (UI deps).

**P2 band exit (locked):** `#8`–`#11` **and** `#9b` — TUI ships with deviation column (or `n/a` when no prior recommend).

[↑ Back to top](#top)

---

## `#8` Shared live read model

Reuse patterns from `internal/sample` Capture (listers) but **do not** insert SQLite.

**Output row (sketch):**

| Field | Source |
|-------|--------|
| ns/pod/container | Pod + Metrics join |
| owner_kind/name, node | Pod |
| cpu/mem util, request, limit | same units as store (mcores / bytes) |
| `cpu_ratio_request`, `cpu_ratio_limit`, mem analogues | util/request (0 if request=0 → mark n/a) |

**Filters:** `scope.namespaces` / `pod_labels` / …; if `tui.pods` non-empty → allowlist
`namespace/name` ∩ scope (SPEC §5.2). Capacity uses same filter with empty
`tui.pods` by default.

[↑ Back to top](#top)

---

## `#9` Recommend (offline)

Config already present (`recommend.*` in sample YAML).

| Knob | Default | Role |
|------|---------|------|
| `window_days` | 7 | History window |
| `percentile` | 95 | Peak-ish steady use |
| `min_samples` | 24 | Fail closed → exit `3` if below |
| `target_utilization` | 0.70 | Suggest formula |
| `*_oversize_ratio` / `*_undersize_ratio` | 0.35 / 0.85 | Verdict bands (CPU/mem × request/limit) |

**Algorithm (per grain key):**

1. Load samples in `[now - window, now]` for key
2. If count < `min_samples` → skip key or fail command (prefer: skip key + summary count; empty result → exit `3`)
3. `p = percentile(util)` per dimension
4. `suggested_request = round_up(p / target_utilization)` (CPU millicores / mem bytes — document rounding)
5. Verdict vs current request/limit using oversize/undersize ratios

**Output:** table (human) + optional `--format tsv` for scripts. No cluster write.

**SQL note:** aggregate by `(owner_kind, owner_name, container)` and
`(namespace, pod, container)`. Index already on `sampled_at` / ns+pod — may add
composite later if slow.

[↑ Back to top](#top)

---

## `#10` Capacity (CLI table)

One-shot, live. Columns (kube-capacity-like, container or pod-rolled — **decide
pod-aggregate for default row, expand later**):

Draft default: **one row per pod** (sum/max containers? Prefer **hottest
container** or **sum util / sum request** — pick **sum util / sum request** for
pod row to match “oversizing” glance).

Flags: `--config`, `--kubeconfig`, maybe `--sort` mirroring tui keys.

[↑ Back to top](#top)

---

## `#11` TUI

| Topic | Decision (proposed) |
|-------|---------------------|
| Library | [`charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea) + [`charmbracelet/lipgloss`](https://github.com/charmbracelet/lipgloss) |
| Entry | `kpm tui`; alias `kpm capacity --tui` |
| Row | One per pod; bars for selected metric ratio |
| Colors | green &lt;70%, yellow 70–90%, red ≥90% of baseline |
| Keys | sort cycle / cpu↔mem / request↔limit / quit; toggle **delta vs last recommend** |
| Refresh | `tui.refresh_seconds` (default 5); no store write |
| Missing metrics | show allocation; util `n/a` |
| **vs last recommend (P2)** | Join live grain key → latest `#9b` snapshot; show Δ request (live − suggested) and/or util vs suggested; **`n/a`** if no snapshot |

Non-goals: cluster edit, mouse-only, full node dashboard (SPEC §5.6).

**Coverage:** extract pure sort/filter/bar + delta math for unit tests; UI smoke optional.

[↑ Back to top](#top)

---

## Decisions (locked 2026-10-11)

1. **Capacity default grain:** **pod-aggregated** (sum util / sum request); container rows later (`--containers` or expand).
2. **Recommend empty / no keys ≥ `min_samples`:** exit **`3`**.
3. **round_up:** CPU ceil to **10 millicores**; memory ceil to **1 MiB** (document in SPEC when `#9` lands).
4. **No cluster apply** of recommendations (read-only). Emit exports for GitOps PR.
   Server later (`0.6.x` `#18b`, SPEC §8.3 / §15.9): show **Kustomize patch/diff**
   (copy/download) per workload / bulk — not `kubectl patch` from kpm.
5. **`#9b` + TUI delta are in P2** (band exit) — not deferred past 0.3.x Insights.

## `#9b` Recommendation outcomes + TUI delta (**P2 required**)

**Problem:** after `kpm recommend` emits suggested CPU/mem, operators want live
deviation vs that advice (TUI) and later offline scoring.

| Piece | Behavior |
|-------|----------|
| Persist | On each `recommend` run (or `--record`), write `recommendations` rows: `recorded_at`, grain key, suggested CPU/mem, observed `p`, window/percentile/target, `client_id`, `cluster_name` |
| TUI delta | `#11` joins live row → latest snapshot; show Δ vs suggested (`n/a` if none) — **must ship in P2** |
| Score | `recommend --score` (or similar): holdout samples after `recorded_at`; hit/miss vs `target_utilization` |
| Caveat | Modes: (A) calibration assuming apply; (B) applied if live request ≈ suggested (±tol) |

Core sample path stays unaware. `#9b` after `#9`, before or with `#11`.

[↑ Back to top](#top)

---

## Success criteria (band done)

- [x] `kpm recommend --config …` offline table from store; exit `3` when insufficient
- [x] `kpm capacity` live table; exit `2` on API/RBAC fail
- [ ] `#9b` persist last recommend + score path; feed TUI delta
- [ ] `kpm tui` / `capacity --tui` heatmap **with** delta vs last recommend (`n/a` ok); no store writes
- [x] Shared `internal/live` used by capacity (+ tui)
- [ ] SPEC §3–5 → Normative (draft bump); man + sample YAML; VHS re-record if help changes
- [ ] `make lint` + `make cover-check` ≥ 80%

## Out of scope

- Auto-apply patches / `kubectl set resources` / apply-all (deferred forever for Core;
  GitOps export + server Kustomize view instead — SPEC §15.9)
- Notify / MCP / server (`0.4`–`0.6`) — server recommend export is `#18b` in that band
- Node-capacity dashboard
- Shipping P2 **without** `#9b` / TUI delta (**rejected** — both required for band exit)

[↑ Back to top](#top)
