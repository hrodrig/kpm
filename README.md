# kpm — Kubernetes Pods Metrics

[![Version](https://img.shields.io/badge/version-0.1.0-blue)](https://github.com/hrodrig/kpm/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
[![Go 1.27.2](https://img.shields.io/badge/Go-1.27.2-00ADD8.svg)](https://go.dev/dl/)

**Repo:** [github.com/hrodrig/kpm](https://github.com/hrodrig/kpm) · **Spec:** [SPECIFICATIONS.md](SPECIFICATIONS.md) · **Changelog:** [CHANGELOG.md](CHANGELOG.md)

> **Scaffold (v0.1.0).** [SPECIFICATIONS.md](SPECIFICATIONS.md) is **draft v0.3**
> (core store, TUI, optional notify/MCP/server). The binary still ships only
> `version` and `--print-sample-config`. **Use at your own risk** (MIT **AS IS**).

**The problem:** Snapshots (`kubectl top`, kube-capacity) lack history; full
Prometheus stacks are heavy for bastion workflows.

**How kpm will solve it:** **core** — sample control-plane metrics into
**local SQLite**, then **backup** / **export** (csv/tsv). On top: **recommend**,
live **capacity** / interactive **TUI** (heatmap pod list), optional
**notifications** (threshold held for X), daemon **read-only MCP**, and
optional **server** charts. Bastion CLI first; in-cluster packaging later via
**[kpm-selfhosted](https://github.com/hrodrig/kpm-selfhosted)**.

**Related tools (same maintainer):**
- **[kwd](https://github.com/hrodrig/kwd)** — Kubernetes workload readiness watchdog
- **[groot](https://github.com/hrodrig/groot)** — Kubernetes diagnostics archive
- **[kzero](https://github.com/hrodrig/kzero)** — bastion-first declarative workload reset
- **[pgwd](https://github.com/hrodrig/pgwd)** — PostgreSQL connection watchdog
- **[gghstats](https://github.com/hrodrig/gghstats)** — GitHub repo traffic beyond 14 days

## Status

| Area | State |
|------|--------|
| Repo mold (Make, CI, GoReleaser, man, whitelist `.gitignore`) | **Done** (scaffold) |
| `kpm version` / `--print-sample-config` | **Done** |
| Core: sample → SQLite, backup, export | Pending (SPEC draft v0.3) |
| Recommend / capacity / TUI | Pending |
| Notifications / MCP / server charts | Pending (optional layers) |
| `kpm-selfhosted` | Deferred |

## Quick start (scaffold)

```bash
make build
./bin/kpm version
./bin/kpm --print-sample-config
```

## Development

```bash
make lint
make test
make cover-check
```

Before a release tag: **`make release-check`**.

## License

[MIT](./LICENSE)
