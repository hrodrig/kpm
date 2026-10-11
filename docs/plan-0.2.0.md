# Plan 0.2.0 — Core (sample → SQLite)

<a id="top"></a>

Band plan for **0.2.x Core** (SPEC §2 / P1). Roadmap IDs: `#1`–`#7`.
Parent: [ROADMAP.md](./ROADMAP.md). Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md)
(**draft v0.4** — Core normative).

**Goal:** Operators can load `kpm.yaml`, run `kpm sample` (one-shot or daemon
sample-only), persist container metrics to SQLite, then `backup` / `export`.

**Band status:** implementation **done in-tree**; open PR → `develop`, then
version cut when operator asks.

---

## Merge order

| Order | ID | Deliverable | Status |
|------:|----|-------------|--------|
| 1 | `#1` | `internal/config` + `internal/identity` (`KPM_CLIENT_ID`) | **Done** |
| 2 | `#2` | client-go + Metrics API; `kpm sample` one-shot | **Done** |
| 3 | `#3` | Schema version; indexes; retention prune | **Done** |
| 4 | `#4` | Daemon loop (`interval_seconds > 0`); SIGINT/TERM → exit 0 | **Done** |
| 5 | `#5` | `kpm backup` via SQLite `VACUUM INTO` | **Done** |
| 6 | `#6` | `kpm export` csv/tsv + filters | **Done** |
| 7 | `#7` | Exit code `2` for cluster/metrics/RBAC | **Done** |

Core does not depend on notify / MCP / server packages.

[↑ Back to top](#top)

---

## Package sketch

```
internal/config/   — YAML load (Viper), defaults, validation
internal/identity/ — client.id cascade
internal/cluster/  — REST config + clientset (+ metrics client)
internal/store/    — SQLite open, migrate, insert, prune, backup, export
internal/sample/   — map Pod+PodMetrics → records; RunOnce / RunLoop
internal/cli/      — sample, backup, export commands
```

---

## Success criteria (band done)

- [x] `kpm sample --config …` with `interval_seconds: 0` writes container rows
- [x] Missing pod metrics skipped (+ count); Metrics/RBAC API errors fail closed (exit `2`)
- [x] Daemon sample-only loop; no optional facades required
- [x] `backup` and `export` (csv|tsv) work offline against the store
- [x] Unit tests for config/store/sample mapping; cover gate ≥ 80% on `./internal/...`
- [x] SPEC §2 marked **Normative** (draft v0.4)
- [x] `make lint` + `make test` green; man pages mention new commands

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
