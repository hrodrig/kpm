# Plan 0.2.0 — Core (sample → SQLite)

<a id="top"></a>

Band plan for **0.2.x Core** (SPEC §2 / P1). Roadmap IDs: `#1`–`#7`.
Parent: [ROADMAP.md](./ROADMAP.md). Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md).

**Goal:** Operators can load `kpm.yaml`, run `kpm sample` (one-shot or daemon
sample-only), persist container metrics to SQLite, then `backup` / `export`.

---

## Merge order

| Order | ID | Deliverable | Depends |
|------:|----|-------------|---------|
| 1 | `#1` | `internal/config`: Load + validate `cluster` / `kube` / `client` / `store` / `sample` / `scope`; `~` expand on `store.path`; `--config` (default `kpm.yaml`); `internal/identity` cascade (`KPM_CLIENT_ID`) | — |
| 2 | `#2` | client-go + Metrics API; `kpm sample` one-shot → insert rows | `#1` |
| 3 | `#3` | Schema version table; time indexes; retention prune on write/open | `#2` |
| 4 | `#4` | Daemon loop when `interval_seconds > 0`; SIGINT/TERM → clean exit 0 | `#2`, `#3` |
| 5 | `#5` | `kpm backup` via SQLite backup API | `#2` |
| 6 | `#6` | `kpm export` csv/tsv + offline filters | `#2` |
| 7 | `#7` | Exit code `2` for cluster/metrics/RBAC fail-closed | `#2` |

Core must not depend on notify / MCP / server packages.

[↑ Back to top](#top)

---

## Package sketch

```
internal/config/   — YAML load (Viper), defaults, validation
internal/cluster/  — REST config + clientset (+ metrics client)
internal/store/    — SQLite open, migrate, insert, prune, backup helpers
internal/sample/   — pure map Pod+PodMetrics → records; runner
internal/cli/      — sample, backup, export commands
```

---

## Success criteria (band done)

- [ ] `kpm sample --config …` with `interval_seconds: 0` writes container rows
- [ ] Missing pod metrics skipped (+ count); Metrics/RBAC API errors fail closed (exit `2`)
- [ ] Daemon sample-only loop; no optional facades required
- [ ] `backup` and `export` (csv|tsv) work offline against the store
- [ ] Unit tests for config/store/sample mapping; cover gate ≥ 80% on `./internal/...`
- [ ] SPEC §2 marked **Normative** (or equivalent status bump) when band tags
- [ ] `make lint` + `make test` green; man pages mention new commands before release cut

---

## Release checklist (when cutting 0.2.x)

1. Move CHANGELOG `[Unreleased]` → `[0.2.x]`; cite IDs `(0.2.x #N)`
2. Bump `VERSION`, README badge, both man `.TH` lines
3. Update Status table in README
4. PR → `develop`; then develop → `main` + tag only with operator OK

[↑ Back to top](#top)

---

## Out of scope (later bands)

- recommend / capacity / tui (`0.3.x`)
- notifications / MCP / server (`0.4`–`0.6`)
- kind e2e workflow (nice-to-have; fixtures acceptable for 0.2)
- Homebrew (`#23` in 0.7)

[↑ Back to top](#top)
