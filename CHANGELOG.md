# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
  measurement + rights strategy (Mermaid); OSS hero `docs/kpm-hero-oss.jpg`.

### Changed

- Daemon contract: `interval_seconds > 0` **always** persists metrics;
  notifications, MCP, and HTTP server are each optional when enabled.
- Notifications: empty `rules` + `enabled` → built-in CPU+mem util vs request
  @ **80%** plus one-time English startup log.
- `retention_days` guidance: longer history improves percentiles; **production
  > 45 days** recommended.
- README family header (badges, nav, disclaimer, Related tools) + overview Mermaid.
- `.gitignore`: drop duplicate `!.goreleaser.yaml` whitelist entry.
