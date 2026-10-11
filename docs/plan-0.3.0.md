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
| 3 | `#9` | `internal/recommend` + `kpm recommend` — window/percentile/`min_samples`; dual grain (workload+container **and** pod+container); CPU/mem separate; vs request **and** limit; `suggested_request = round_up(p / target_utilization)`; table/tsv out; exit `3` if empty | store schema |
| 4 | `#11` | `kpm tui` (+ `capacity --tui`) — keyboard-first list, heatmap bars, sort keys, refresh; [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss) | `#8` |

**Suggested PR splits:** `#8`+`#10` together (live surface); `#9` solo (math-heavy); `#11` solo (UI deps + cover care).

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
| Keys | sort cycle / cpu↔mem / request↔limit / quit |
| Refresh | `tui.refresh_seconds` (default 5); no store write |
| Missing metrics | show allocation; util `n/a` |

Non-goals: cluster edit, mouse-only, full node dashboard (SPEC §5.6).

**Coverage:** extract pure sort/filter/bar math for unit tests; UI smoke optional.

[↑ Back to top](#top)

---

## Decisions (locked 2026-10-11)

1. **Capacity default grain:** **pod-aggregated** (sum util / sum request); container rows later (`--containers` or expand).
2. **Recommend empty / no keys ≥ `min_samples`:** exit **`3`**.
3. **round_up:** CPU ceil to **10 millicores**; memory ceil to **1 MiB** (document in SPEC when `#9` lands).
4. **No cluster apply** of recommendations (read-only). Emit exports for GitOps PR.
   Server later (`0.6.x` `#18b`, SPEC §8.3 / §15.9): show **Kustomize patch/diff**
   (copy/download) per workload / bulk — not `kubectl patch` from kpm.

## Follow-on idea — recommendation outcomes (not blocking P2 MVP)

**Problem:** after `kpm recommend` emits suggested CPU/mem, operators want to know later whether those suggestions were *good* given future samples.

**Proposal (sketch for SPEC / `#9b` after `#9` ships):**

| Piece | Behavior |
|-------|----------|
| Persist | On each `recommend` run (or `--record`), write rows to e.g. `recommendations` table: `recorded_at`, grain key, suggested request/limit CPU+mem, observed `p`, window/percentile/target, `client_id`, `cluster_name`, optional run id |
| Evaluate | Later command or `recommend --score`: for each recorded suggestion, take samples **after** `recorded_at` in a holdout window; compare subsequent util percentile (or util vs *suggested* request) to `target_utilization` |
| Score | e.g. “hit” if later util/suggested ∈ [target±band]; “miss-high” / “miss-low”; % hits over N recommendations |
| Caveat | We do **not** know if the operator *applied* the patch. Two modes: (A) **calibration** — was the suggestion consistent with future util assuming they apply it; (B) **applied** — detect request fields in later samples ≈ suggested (±tol) then score util vs target |

Keep Core sample path unaware. Optional table + offline scorer only. Slot after `#9` MVP, before or with `#11` — not in 0.2.0 ship.

[↑ Back to top](#top)

---

## Success criteria (band done)

- [ ] `kpm recommend --config …` offline table from store; exit `3` when insufficient
- [x] `kpm capacity` live table; exit `2` on API/RBAC fail
- [ ] `kpm tui` / `capacity --tui` keyboard heatmap; no store writes
- [x] Shared `internal/live` used by capacity (+ tui later)
- [ ] SPEC §3–5 → Normative (draft bump); man + sample YAML; VHS re-record if help changes
- [ ] `make lint` + `make cover-check` ≥ 80%

## Out of scope

- Auto-apply patches / `kubectl set resources` / apply-all (deferred forever for Core;
  GitOps export + server Kustomize view instead — SPEC §15.9)
- Notify / MCP / server (`0.4`–`0.6`) — server recommend export is `#18b` in that band
- Node-capacity dashboard

[↑ Back to top](#top)
