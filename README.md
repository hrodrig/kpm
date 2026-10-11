# kpm — Kubernetes Pods Metrics

<a id="top"></a>

[![Version](https://img.shields.io/badge/version-0.1.0-blue)](https://github.com/hrodrig/kpm/releases)
[![GitHub release](https://img.shields.io/github/v/release/hrodrig/kpm)](https://github.com/hrodrig/kpm/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
[![Go 1.27.2](https://img.shields.io/badge/Go-1.27.2-00ADD8.svg)](https://go.dev/dl/)
[![CI](https://github.com/hrodrig/kpm/actions/workflows/ci.yml/badge.svg)](https://github.com/hrodrig/kpm/actions/workflows/ci.yml)
[![Security](https://github.com/hrodrig/kpm/actions/workflows/security.yml/badge.svg)](https://github.com/hrodrig/kpm/actions/workflows/security.yml)
[![CodeQL](https://github.com/hrodrig/kpm/actions/workflows/codeql.yml/badge.svg)](https://github.com/hrodrig/kpm/actions/workflows/codeql.yml)
[![gghstats clones](https://gghstats.hermesrodriguez.com/api/v1/badge/hrodrig/kpm?metric=clones)](https://gghstats.hermesrodriguez.com/hrodrig/kpm)
[![pkg.go.dev](https://pkg.go.dev/badge/github.com/hrodrig/kpm)](https://pkg.go.dev/github.com/hrodrig/kpm)
[![deps.dev](https://img.shields.io/badge/deps.dev-go%20module-blue)](https://deps.dev/go/github.com%2Fhrodrig%2Fkpm)

**Repo:** [github.com/hrodrig/kpm](https://github.com/hrodrig/kpm) · **Releases:** [GitHub Releases](https://github.com/hrodrig/kpm/releases) · **Spec:** [SPECIFICATIONS.md](SPECIFICATIONS.md) · **Operator:** [kpm-selfhosted](https://github.com/hrodrig/kpm-selfhosted) · **Changelog:** [CHANGELOG.md](CHANGELOG.md) · **Contributing:** [CONTRIBUTING.md](CONTRIBUTING.md)

<a id="early-development-disclaimer"></a>

> **Early development (pre-1.0).** kpm is under active development.
> [SPECIFICATIONS.md](SPECIFICATIONS.md) is **draft v0.3**; config schema, CLI
> surface, SQLite layout, and recommend semantics may change without a
> stability guarantee until **v1.0.0**. The binary today ships only `version`
> and `--print-sample-config`. **Do not** treat this as a production
> rightsizing gate before then. **Use at your own risk.** The software is
> provided under the [MIT License](./LICENSE) **“AS IS”**, without warranty of
> any kind (see [LICENSE](./LICENSE)).

**The problem:** Cluster CPU/memory **requests** often dwarf real utilization
(oversizing). Snapshots (`kubectl top`, kube-capacity) lack history. Full
Prometheus + VPA/Goldilocks stacks are heavy for a laptop/bastion workflow.

**How kpm will solve it:** a single-binary CLI (**client-go**, no `kubectl`
subprocess) that samples pod CPU/memory into **local SQLite** and surfaces
**rightsizing recommendations** (rules over historical p95 vs requests/limits),
plus planned capacity/TUI and optional notify/MCP/server layers — see the SPEC.
Bastion-first; in-cluster packaging later via
**[kpm-selfhosted](https://github.com/hrodrig/kpm-selfhosted)** (same product vs
operator split as [kwd](https://github.com/hrodrig/kwd) / [kzero](https://github.com/hrodrig/kzero) / [groot](https://github.com/hrodrig/groot)).

![kpm — Kubernetes Pods Metrics](docs/kpm-hero-oss.jpg)

**Related tools (same maintainer):**
- **[kpm](https://github.com/hrodrig/kpm)** — Kubernetes Pods Metrics (this repo; deploy: [kpm-selfhosted](https://github.com/hrodrig/kpm-selfhosted))
- **[kwd](https://github.com/hrodrig/kwd)** — Kubernetes workload readiness watchdog ([live traffic](https://gghstats.hermesrodriguez.com/hrodrig/kwd); deploy: [kwd-selfhosted](https://github.com/hrodrig/kwd-selfhosted))
- **[kzero](https://github.com/hrodrig/kzero)** — bastion-first declarative workload reset ([live traffic](https://gghstats.hermesrodriguez.com/hrodrig/kzero); deploy: [kzero-selfhosted](https://github.com/hrodrig/kzero-selfhosted))
- **[groot](https://github.com/hrodrig/groot)** — Kubernetes diagnostics archive ([live traffic](https://gghstats.hermesrodriguez.com/hrodrig/groot); deploy: [groot-selfhosted](https://github.com/hrodrig/groot-selfhosted))
- **[pgwd](https://github.com/hrodrig/pgwd)** — PostgreSQL connection watchdog ([live traffic](https://gghstats.hermesrodriguez.com/hrodrig/pgwd); deploy: [pgwd-selfhosted](https://github.com/hrodrig/pgwd-selfhosted))
- **[gghstats](https://github.com/hrodrig/gghstats)** — GitHub repo traffic beyond 14 days ([live demo](https://gghstats.hermesrodriguez.com); deploy: [gghstats-selfhosted](https://github.com/hrodrig/gghstats-selfhosted))

**Releases** will ship **binaries**, **`.deb`** / **`.rpm`**, and
**`ghcr.io/hrodrig/kpm`** (Cosign + SBOM via GoReleaser) once tagged from
`main`. Behavior contract: **[SPECIFICATIONS.md](SPECIFICATIONS.md)**.

## Table of contents

- [Early development disclaimer](#early-development-disclaimer)
- [Status](#status)
- [Quick start](#quick-start)
- [Development](#development)
- [License](#license)

---

## Status

| Area | State |
|------|--------|
| Repo mold (Make, CI, GoReleaser, man, whitelist `.gitignore`) | **Done** (scaffold) |
| `kpm version` / `--print-sample-config` | **Done** |
| Core: sample → SQLite, backup, export | Pending (SPEC draft v0.3) |
| Recommend / capacity / TUI | Pending |
| Notifications / MCP / server charts | Pending (optional layers) |
| `kpm-selfhosted` | Deferred |

[↑ Back to top](#top)

---

## Quick start

```bash
make build
./bin/kpm version
./bin/kpm --print-sample-config > kpm.yml
./bin/kpm --help
```

[↑ Back to top](#top)

---

## Development

```bash
make lint
make test
make cover-check
```

Before a release tag: **`make release-check`**.

[↑ Back to top](#top)

---

## License

[MIT](./LICENSE)

[↑ Back to top](#top)
