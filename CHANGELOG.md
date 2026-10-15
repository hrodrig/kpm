# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- README product captures (`docs/kpm-serve-analyze-local-dark.png`,
  `docs/kpm-tui-cpu-request-13pods.png`, `docs/kpm-flow-diagram.png`) plus link to the
  [DEV.to walkthrough](https://dev.to/hrodrig/kpm-kubernetes-rightsizing-without-prometheus-history-on-your-laptop-4272).

### Fixed

- `make govulncheck` no longer treats tool/network failures as a clean PASS
  (`|| true` fail-open); exit `3` still means vulnerabilities to filter via
  `.govulncheck-ignore.yaml` ([#54](https://github.com/hrodrig/kpm/issues/54)).

## [0.6.0] - 2026-10-12

### Added

- [docs/plan-0.6.0.md](docs/plan-0.6.0.md) — Server band plan (analyze-local,
  charts, JSON API; `#19a` / `#19` / `#18` / `#18b` / `#18c`; Elaya tokens +
  Hallmark anti-slop for `#18` UI).
- `kpm serve` — ops + MCP HTTP against an existing store without a sample loop
  (`mcp.enabled`; `/readyz` store-only; kube optional) `(0.6.x #19a)`.
- JSON read API `GET /api/v1/{status,workloads,series,recommendations,live}`
  when `server.enabled` on the shared mux; bearer via `server.token_env`
  (or shared `mcp.token_env`); live 503 without kube `(0.6.x #19)`.
- Embedded analyze-local charts UI at `GET /` (+ `/static/*`) when
  `server.enabled`: workload cards, canvas series, window presets, detail
  drawer (recommendations + live) `(0.6.x #18)`.
- GitOps patch export: `GET /api/v1/export/patch` + drawer copy/download
  (strategic-merge YAML / kustomize hint; never applied) `(0.6.x #18b)`.
- Recommend verdicts on cards + summary strip (oversized / undersized / ok /
  no-req) from stored p-util vs request; API fields `cpu_verdict` /
  `mem_verdict` + deltas `(0.6.x #18c)`.
- Status `kube_live` + UI mode chip (`live` / `analyze-local`); sticky footer
  with repo, MIT license, and advisory-only disclaimer.
- SPEC §8 → **Normative** (P5 band exit); ROADMAP **P6** 0.7.x adds `#20c`
  search/filter (large grids) beside `#20b` nodes.

### Changed

- Daemon ops `/readyz`: store open required; kube Discovery no longer gates ready
  `(0.6.x #19a)`.
- `kpm serve` accepts `mcp.enabled` and/or `server.enabled`; shared listen prefers
  `server.listen` when set `(0.6.x #19)`.
- Charts UI polish: chart req/limit overlays, local TZ patch stamp, light/dark
  theme, cluster/context badge, version display fix.

## [0.5.0] - 2026-10-11

### Added

- [docs/plan-0.5.0.md](docs/plan-0.5.0.md) — MCP band plan (`#16a` / `#16` / `#17`);
  checklist `[x]`/`[ ]` kept for history.
- Daemon HTTP ops mux when `mcp.enabled` (`internal/httpserver`):
  `GET /healthz`, `/readyz`, process `/metrics` on `mcp.listen`
  `(0.5.x #16a)`.
- `kpm mcp` stdio read-only tools (`get_status`, `list_samples`,
  `summarize_window`, `list_recommendations`) via official Go MCP SDK
  `(0.5.x #16)`.
- Daemon MCP HTTP: SSE `/sse` + streamable `/mcp` on `mcp.listen`, bearer
  via `token_env` (ops `/healthz` `/readyz` `/metrics` stay open)
  `(0.5.x #17)`.
- Daemon HTTP access control (gghstats mold): `mcp.trusted_proxies`,
  `mcp.allowlist_cidrs`, per-IP `mcp.rate_limit` (ops paths exempt)
  `(0.5.x #17)`.
- SPEC §7.5 + sample: how to generate `KPM_MCP_TOKEN` (`openssl rand -hex 32`).

### Changed

- SPECIFICATIONS.md: promote §7 MCP to **normative**; CLI map `mcp`;
  anonymized tool examples §7.6 (P4 band exit).
- VHS `docs/demo.gif` re-recorded for `v0.5.0` (includes `kpm mcp --help`).

## [0.4.0] - 2026-10-11

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

- SPECIFICATIONS.md: promote §3–6 (Insights + notifications) and exit `3` to
  **normative**; status banner + CLI map + phasing tables; AGENTS band-exit /
  version-bump docs must stay in lockstep with features.
- SPEC §6.4 — anonymized notification examples (plain text, generic/discord/teams
  JSON, config sketch) so sink payloads stay documented with the feature.
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

[Unreleased]: https://github.com/hrodrig/kpm/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/hrodrig/kpm/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/hrodrig/kpm/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/hrodrig/kpm/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/hrodrig/kpm/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/hrodrig/kpm/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/hrodrig/kpm/releases/tag/v0.1.0
