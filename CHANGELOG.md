# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- [docs/plan-0.4.0.md](docs/plan-0.4.0.md) — Notifications band plan (`#12`–`#15`);
  checklist `[x]`/`[ ]` kept for history.
- Daemon notifications (SPEC §6): hold/repeat/resolve/dedupe rule engine;
  built-in 80% util-vs-request defaults + startup log; multi-sink fan-out
  `slack` / `webhook` (`generic`|`discord`|`teams`) / `smtp` (fail-closed
  `*_env`); `kpm notify-test` `(0.4.x #12`–`#15)`.
- `kpm tui` **Enter**: read-only detail — Deploy/STS/DS pod template resources
  vs live Pod requests/limits (ReplicaSet → Deployment); Esc closes; never blank
  alt-screen on empty/error.

### Changed

- SPEC §6.3 — multi-sink gghstats-school (replaces solo webhook sketch);
  PagerDuty via generic webhook; Loki deferred.

## [0.3.0] - 2026-10-11

### Added

- [docs/plan-0.3.0.md](docs/plan-0.3.0.md) — Insights band plan (`#8`–`#11`, `#9b`).
- ROADMAP `#9b` (recommendation outcomes) and `#18b` (server Kustomize patch/diff UI).
- Shared live read model `internal/live` (Pod+Metrics → container/pod rows) `(0.3.x #8)`.
- `kpm capacity` — one-shot live pod capacity table (no SQLite); `--sort`; exit `2`
  on API fail `(0.3.x #10)`.
- `kpm recommend` — offline rightsizing from SQLite (dual grain, table/tsv);
  exit `3` when insufficient samples `(0.3.x #9)`.
- `kpm recommend --record` / `--score` — persist `recommendations` snapshots
  (schema v2) and offline calibration vs later samples; `DeltaFromLive` helper
  for TUI `(0.3.x #9b)`.
- `kpm tui` / `capacity --tui` — interactive heatmap (bubbletea + lipgloss);
  Δ vs last recommend when store snapshots exist (`n/a` otherwise); no store
  writes `(0.3.x #11)`.

### Changed

- SPEC §5.3.1 — anonymized TUI row example; Δ = vs last `recommend --record`
  only (not vs Git/Helm infra desired).
- SPEC: recommend stays read-only (no cluster apply); §8.3 + §15.8/`#9b` + §15.9
  GitOps export / server Kustomize copy-download (no live patch).
- SPEC §4 capacity: pod-sum grain, `n/a` util, shared `internal/live`.
- P2 band exit: `#9b` + TUI delta vs last recommendation **required** (SPEC §5.3 /
  §15.8; plan-0.3.0).

## [0.2.0] - 2026-10-11

### Added

- [docs/ROADMAP.md](docs/ROADMAP.md) — path to v1.0.0 (full SPEC stack); bands
  `0.2.x`–`1.0.0` and global work IDs `#1`–`#23`.
- [docs/plan-0.2.0.md](docs/plan-0.2.0.md) — Core band merge order and success
  criteria (`#1`–`#7`).
- Config load + validation (`internal/config`, `--config`, Viper) `(0.2.x #1)`.
- `client.id` identity cascade (`KPM_CLIENT_ID` → YAML → hostname → IPv4 → MAC →
  `unknown`); persisted on sample rows with `cluster.name` (kwd/pgwd pattern).
- `kpm sample` one-shot: client-go + Metrics API → SQLite
  (`internal/cluster`, `internal/store`, `internal/sample`) `(0.2.x #2)`.
- Schema + retention prune + indexes `(0.2.x #3)`.
- Daemon sample loop when `interval_seconds > 0` (SIGINT/SIGTERM clean)
  `(0.2.x #4)`.
- `kpm backup` via SQLite `VACUUM INTO` `(0.2.x #5)`.
- `kpm export` csv/tsv with since/until/namespace/pod/workload filters
  `(0.2.x #6)`.
- Exit code `2` for cluster/Metrics/RBAC failures `(0.2.x #7)`.
- FreeBSD / OpenBSD port skeletons under `contrib/freebsd/` and
  `contrib/openbsd/`; `make port-freebsd-sync`, `port-openbsd-sync`,
  `dist-freebsd`, `dist-openbsd` `(0.2.x #24)`.
- VHS terminal demo [`docs/demo.tape`](docs/demo.tape) →
  [`docs/demo.gif`](docs/demo.gif) `(0.2.x #25)`.
- README + man docs for `kubectl kpm` (`make install-kubectl-plugin`)
  `(0.2.x #26)`.

### Changed

- [SPECIFICATIONS.md](SPECIFICATIONS.md) **draft v0.4**: §1, §2 (Core), §9.2.1
  (`client.id`), and exit codes `0`/`1`/`2` marked **normative**; adjacent and
  optional layers remain draft. Backup documented as `VACUUM INTO`.
- `contrib/man/man1/kubectl-kpm.1`: document `sample` / `backup` / `export`.

## [0.1.0] - 2026-10-11

### Added

- Repository scaffold aligned with the [kwd](https://github.com/hrodrig/kwd) mold:
  whitelist `.gitignore`, `GNUmakefile` / BSD `Makefile`, CI / Security / CodeQL /
  Release workflows, GoReleaser (Homebrew deferred), Docker (distroless), man pages,
  `AGENTS.md`, sample config embed.
- CLI scaffold: `kpm version`, `kpm --print-sample-config`, bare `kpm` → help.
- [SPECIFICATIONS.md](SPECIFICATIONS.md) **draft v0.3** — product contract for
  sample→SQLite, recommend, capacity/TUI, optional notify/MCP/server (not yet
  implemented in the binary).
- [docs/measurement-and-rightsizing.md](docs/measurement-and-rightsizing.md) —
  measurement + dimension strategy (Mermaid); OSS hero `docs/kpm-hero-oss.jpg`.

### Changed

- Daemon contract: `interval_seconds > 0` **always** persists metrics;
  notifications, MCP, and HTTP server are each optional when enabled.
- Notifications: empty `rules` + `enabled` → built-in CPU+mem util vs request
  @ **80%** plus one-time English startup log.
- `retention_days` guidance: longer history improves percentiles; **production
  > 45 days** recommended.
- README family header (badges, nav, disclaimer, Related tools) + overview Mermaid.
- `.gitignore`: drop duplicate `!.goreleaser.yaml` whitelist entry.

[Unreleased]: https://github.com/hrodrig/kpm/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/hrodrig/kpm/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/hrodrig/kpm/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/hrodrig/kpm/releases/tag/v0.1.0
