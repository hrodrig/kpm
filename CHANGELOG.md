# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Man pages now cover **every flag the binary accepts** (22/22, and none documented that no longer
  exist). `capacity fit` is its own entry instead of a paragraph glued onto `capacity`, and
  `--nodes` / `--record-nodes` are named rather than described as "the nodes flag". Two stale claims
  removed along the way: `kubectl ... once the sampler is implemented` (it has been since `v0.2.0`)
  and `node_snapshots (schema v4)` (it is v7).
- SPEC is now **normative end to end**: the `draft v0.4` banner is gone, §9
  (Configuration) and §10 (Security posture) are promoted, and the remaining `draft`
  markers are gone or historical decision records. Each promotion was audited against
  the code first; where a claim did not survive, the claim changed, not the heading.
- SPEC §10 records the controls that exist and were previously unlisted: middleware
  order (browser guard → allowlist → rate limit → mux), ops paths exempt from
  allowlist and rate limiting, bearer bypassing the IP check, and the SMTP
  TLS/STARTTLS rules. It also records a **known gap**: an unparseable `allowlist_cidrs`
  entry is skipped and a list with no valid entries disables the allowlist, so a typo
  fails open. Tracked as `#22`.
- `configs/kpm.sample.yml` now documents every key the binary reads. Two were missing:
  `threshold` (absolute threshold, the alternative to `threshold_ratio`) and `headers_env`
  (extra webhook headers). SPEC §9 names the sample as the complete key reference, so this
  file is now the contract, not an example.
- [docs/migrating.md](docs/migrating.md): operator notes for upgrading — back up first, let the store
  migrate in place, what a refused newer store means, and which releases moved `capacity` numbers.
- SPEC §2.5 records the full store schema history (v1–v7) and the compatibility rules; §2.6 freezes the
  CLI surface for `1.0.0` (command/flag names, column order, JSON fields, exit codes) and states that a
  behaviour correction is not a break (roadmap `#29`).
- `kpm capacity --nodes`: per-node table with each node's pool, then the same numbers grouped by
  pool. A pool is the set of blocking taints — what decides who may use the capacity — with
  `(general)` for nodes that have none. No total is filtered by taint. Pool identity is persisted
  per snapshot (schema **v7**: `node_snapshots.taints`, `.unschedulable`) and exposed on
  `GET /api/v1/nodes` ([#111](https://github.com/hrodrig/kpm/issues/111)).
- `kpm capacity fit` exits **4** when no node can run the workload: the question was answered and the
  answer is no, which a script must be able to tell from a failed query (`2`) or missing data (`3`)
  (SPEC §12).
- `kpm capacity fit`: a placement check that answers "can this workload be scheduled here" rather
  than "how much is free". Evaluates cordon, taints and tolerations, `nodeSelector`, required node
  affinity, and remaining CPU/memory, reading the constraints from a real workload (`--for
  deployment/api`) or a hypothetical pod (`--cpu`/`--memory`). Prints what it did not evaluate —
  pod (anti-)affinity, topology spread, volume zone binding, fragmentation — on every run
  ([#111](https://github.com/hrodrig/kpm/issues/111)).
- `kpm restore`: the missing counterpart to `kpm backup`. By default it installs a backup at the
  store path (temp file, rename, stale `-wal`/`-shm` removed, `--force` required to replace a
  non-empty store). With `--merge` it opens the backup read-only and adds only the rows the store
  does not already hold, matched on each table's natural key — so a second merge of the same file
  adds nothing. Older backups are accepted (absent tables skipped, newer columns defaulted); a
  backup from a newer schema is refused
  ([#110](https://github.com/hrodrig/kpm/issues/110)).

### Fixed

- SPEC §9 documented a configuration precedence the binary does not implement: "CLI flags
  override config when explicitly set" implies a Viper flag binding, and there is no
  `BindPFlag` anywhere in the tree. Flags win only where a command reads them (`--sort` over
  `tui.default_sort`, `-o` over the default backup path), so level 4 of the chain is now
  described as a per-command rule. The sibling rule "every knob overridable via ENV and CLI"
  was corrected the same way: `KPM_*` covers every key, CLI does not. §9 is promoted to
  Normative with the correction in place.
- Opening a store written by a **newer** kpm is now refused **before any migration runs**. The version
  was read after the DDL, so a downgrade could have added the columns it thought were missing to a
  schema it cannot interpret. `restore --merge` already refused a newer source; `Open` now applies the
  same rule, and a test asserts the refused store is left untouched.
- `kpm capacity` no longer counts `Succeeded`/`Failed` pods in node requests or in the per-node pod
  count. The scheduler releases a pod's reservation when it reaches a terminal phase and the Metrics
  API stops reporting it, so completed Jobs inflated both numbers and read as permanently idle
  workloads that still held their requests.
- `kpm capacity` now counts init container requests. A pod's effective request is
  `max(sum(containers), max(initContainers))` — an init container asking for 2 CPU reserves 2 CPU
  even when the app containers sum to 500m. Summing only regular containers understated how full a
  node was and overstated free headroom.

## [0.8.0] - 2026-10-16

### Added

- `kpm doctor`: read-only diagnosis of config, store, history coverage and, with `--live`, the
  cluster Metrics API. Opens the store without creating or migrating it, reports every check instead
  of stopping at the first problem, and exits non-zero only when something it can verify is broken
  ([#94](https://github.com/hrodrig/kpm/issues/94)).
- Window coverage in `kpm recommend`: `days_with_samples` and the oldest sample seen are reported
  alongside `window_days`, so a percentile resting on a fraction of the window is visible instead
  of implied. Persisted per snapshot (schema v6) and exposed as `window_days` /
  `days_with_samples` on `GET /api/v1/recommendations`
  ([#92](https://github.com/hrodrig/kpm/issues/92)).
- `.deb` / `.rpm` install a `kpm-sample.service` systemd unit. The package never enables it — the
  operator runs `systemctl enable --now kpm-sample`. The unit pins
  `KPM_STORE_PATH=/var/lib/kpm/metrics.db` with `StateDirectory=kpm`, because the packaged config's
  `~/.local/share/...` default would resolve to root's home under a system service.

### Changed

- `recommend.cpu_target_utilization` and `recommend.memory_target_utilization` override the shared
  `target_utilization` per resource, so CPU and memory can be sized against different targets
  ([#91](https://github.com/hrodrig/kpm/issues/91)).
- `kpm recommend` warns when `retention_days` is thinner than `recommend.window_days` instead of
  silently computing percentiles from pruned history
  ([#87](https://github.com/hrodrig/kpm/issues/87)).

### Fixed

- Release workflow fails when the pushed tag is not an ancestor of `main`, and
  drops the redundant workflow-level write permissions
  ([#90](https://github.com/hrodrig/kpm/issues/90)).

### Security

- SMTP subjects are RFC 2047 encoded and CR/LF in headers is rejected, closing a header-injection
  path; `require_starttls` refuses a relay that does not advertise STARTTLS
  ([#89](https://github.com/hrodrig/kpm/issues/89)).
- Rate-limit buckets group IPv6 clients by `/64`, so rotating the source address no longer refills
  the budget ([#88](https://github.com/hrodrig/kpm/issues/88)).

## [0.7.0] - 2026-10-14

### Added

- Node capacity snapshots: `kpm capacity --record-nodes` → `node_snapshots`
  (schema v4); analyze-local `GET /api/v1/nodes` + Nodes panel offline
  ([#70](https://github.com/hrodrig/kpm/issues/70) / `#20b`).
- Analyze-local search/filter: namespace, owner, grain, CPU/Mem verdict
  ([#69](https://github.com/hrodrig/kpm/issues/69) / `#20c`).
- Roadmap split **0.7 Scale / 0.8 Honesty / 0.9 Freeze → 1.0**; band plan
  [docs/plan-0.7.0.md](docs/plan-0.7.0.md).
- `GET /api/v1/series/batch` so analyze-local can load many card series in one
  request ([#72](https://github.com/hrodrig/kpm/issues/72)).
- `mcp.allowed_hosts` / `server.allowed_hosts` for BrowserGuard behind a reverse
  proxy that preserves a public `Host` to a loopback bind
  ([#76](https://github.com/hrodrig/kpm/issues/76)).
- Ops counter `kpm_sample_errors_total` for transient daemon sample failures
  ([#75](https://github.com/hrodrig/kpm/issues/75)).
- README product captures (`docs/kpm-serve-analyze-local-dark.png`,
  `docs/kpm-tui-cpu-request-13pods.png`, `docs/kpm-flow-diagram.png`) plus link to the
  [DEV.to walkthrough](https://dev.to/hrodrig/kpm-kubernetes-rightsizing-without-prometheus-history-on-your-laptop-4272).

### Fixed

- Store scale: index `(namespace, owner_name, sampled_at)`, SQL `LIMIT` on
  `ListRecords`, MCP `list_samples` caps before full materialization, analyze-local
  window max 15d and series/workloads scan caps
  ([#71](https://github.com/hrodrig/kpm/issues/71)).
- Sample daemon retries with exponential backoff and a 2-minute budget (was five
  short linear attempts); failures are logged
  ([#75](https://github.com/hrodrig/kpm/issues/75)).
- SQLite store creates the DB at `0600` before open, chmods `-wal`/`-shm`, and
  encodes paths with `?`/`#` safely in the DSN
  ([#77](https://github.com/hrodrig/kpm/issues/77)).
- GitOps patch for CronJob uses `spec.jobTemplate.spec.template…`; ReplicaSet
  names strip `pod-template-hash` when mapping to Deployment
  ([#78](https://github.com/hrodrig/kpm/issues/78)).
- Sample path resolves ReplicaSet → Deployment for stable recommend grains
  ([#74](https://github.com/hrodrig/kpm/issues/74)).
- SMTP sends use a per-call timeout and TLS connection deadline
  ([#73](https://github.com/hrodrig/kpm/issues/73)).
- Rate-limit skips `/static/*` and returns JSON 429 bodies; UI tolerates non-JSON
  errors ([#72](https://github.com/hrodrig/kpm/issues/72)).
- `make govulncheck` no longer treats tool/network failures as a clean PASS
  (`|| true` fail-open); exit `3` still means vulnerabilities to filter via
  `.govulncheck-ignore.yaml` ([#54](https://github.com/hrodrig/kpm/issues/54)).
- Notify Slack/webhook use a 15s HTTP client timeout; SMTP respects context and
  TLS dial timeout so a hung sink cannot stall the sample loop
  ([#55](https://github.com/hrodrig/kpm/issues/55)).
- Allowlist / rate-limit / trusted_proxies apply whenever any HTTP facade is
  active (including `server.enabled` without MCP)
  ([#56](https://github.com/hrodrig/kpm/issues/56)).
- `ClientIP` walks `X-Forwarded-For` right-to-left, skipping trusted hops
  ([#57](https://github.com/hrodrig/kpm/issues/57)).
- Bearer and allowlist bearer bypass use constant-time compare
  ([#61](https://github.com/hrodrig/kpm/issues/61)).
- Rate-limiter `lastSeen` updates are mutex-protected against the idle cleanup
  ([#62](https://github.com/hrodrig/kpm/issues/62)).
- SQLite opens with WAL + `busy_timeout`; store dir `0700` and DB file `0600`
  ([#59](https://github.com/hrodrig/kpm/issues/59)).
- Loopback HTTP facades reject unexpected `Host` / non-loopback `Origin`
  (DNS-rebinding mitigation) ([#58](https://github.com/hrodrig/kpm/issues/58)).
- Store timestamps use fixed-width UTC so lexicographic order matches nanos
  ([#60](https://github.com/hrodrig/kpm/issues/60)).
- GitOps patch export validates DNS-1123 names and allowlists owner kinds
  ([#63](https://github.com/hrodrig/kpm/issues/63)).
- Sample daemon retries transient API errors (up to 5) before fail-closed
  ([#64](https://github.com/hrodrig/kpm/issues/64)).
- Notify engine prunes idle non-firing keys so pod churn cannot grow state
  unboundedly ([#65](https://github.com/hrodrig/kpm/issues/65)).

### Changed

- README release blurb and `AGENTS.md` overview match shipped 0.6.x surface
  ([#66](https://github.com/hrodrig/kpm/issues/66)).
- CI unit-test job also runs `./configs/`
  ([#67](https://github.com/hrodrig/kpm/issues/67)).

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

[Unreleased]: https://github.com/hrodrig/kpm/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/hrodrig/kpm/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/hrodrig/kpm/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/hrodrig/kpm/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/hrodrig/kpm/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/hrodrig/kpm/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/hrodrig/kpm/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/hrodrig/kpm/releases/tag/v0.1.0
