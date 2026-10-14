# Agent Guidelines (kpm)

Context and instructions for AI coding agents working on **kpm** (Kubernetes
Pods Metrics). See [agents.md](https://agents.md/) for the format.

## Project overview

- **What it is:** Go CLI whose **core** samples control-plane pod metrics into
  **SQLite** (plus backup/export). Adjacent: recommend, `capacity`, interactive
  **TUI** (heatmap pod list). Optional: notifications (webhook), read-only
  **MCP**, HTTP **server** charts. Core sample→SQLite shipped at `VERSION`
  0.2.0; adjacent recommend/capacity/TUI still planned.
- **Entrypoint:** `cmd/kpm/main.go`. Packages: `internal/cli` (Cobra),
  `internal/config`, `internal/identity` (`client.id` cascade),
  `internal/cluster`, `internal/store`, `internal/sample`,
  `internal/exitcode`, `configs` (embedded sample YAML).
- **Config contract:** [SPECIFICATIONS.md](SPECIFICATIONS.md) **draft v0.4**
  (Core normative). Path to v1: [docs/ROADMAP.md](docs/ROADMAP.md). Example:
  `configs/kpm.sample.yml`.
- **Kubernetes client:** **`client-go`** + Metrics API (no `kubectl` subprocess).

## Scope (product vs operator)

- **This repo (`kpm`):** Go CLI, config, tests, CI, **`make release-check`**,
  binaries, `.deb`/`.rpm`, **`ghcr.io/hrodrig/kpm`**, cosign + SBOM — same split
  as **kwd** / **kzero** / **groot** / **pgwd**.
- **Not here:** cron/systemd, Helm CronJob, operator runbooks →
  **[kpm-selfhosted](https://github.com/hrodrig/kpm-selfhosted)**. Do not add
  `run/` deployment trees here.
- **Not kwd:** readiness watchdog stays in [kwd](https://github.com/hrodrig/kwd).

## Setup and build

- Install deps: `go mod download`.
- Build: **`make build`** (reads **`VERSION`**, ldflags for
  version/commit/date/branch). Root **`GNUmakefile`** is canonical; on
  **FreeBSD** use **`gmake`** via the BSD **`Makefile`** stub.
- Toolchain: pinned in **`go.mod`** (currently `go 1.27.2`); CI follows
  `go-version-file`.

## Test and quality commands

- **Unit tests:** `make test` (`go test -race ./...`).
- **Coverage:** `make cover`; gate **`make cover-check`** (total statement
  coverage **≥ 80%** over `./internal/...`).
- **Lint:** `make lint` — `check-pins`, `check-man-version`, `gofmt -s`,
  `go vet ./...`, gocyclo ≤ 14 (`make lint-fix` rewrites formatting).
- **Security:** `make security` (govulncheck + gocyclo + grype) and
  `make docker-scan`.
- **Before release:** **`make release-check`**.

## Git flow

- Work on **topic branches** from `develop`; PR **into `develop`**. **Never**
  commit or push directly to `develop` or `main`.
- Release: PR **`develop` → `main`**, annotated tag **`v<semver>`** on **`main`**
  only. Then sync **`main` → `develop`** via PR.
- **Never** merge to `main`, create/push a release tag, or run `make release`
  without **explicit user approval**.
- **Commits:** show the proposed commit message and wait for user approval
  before `git commit`.
- **Language:** English only for code, comments, commit messages, docs, and
  UI strings.

## Planning docs

- User-facing: **README.md**
- Behavior contract: **SPECIFICATIONS.md** (draft until bands promote sections)
- Roadmap / band plans: **docs/ROADMAP.md**, **docs/plan-0.2.0.md**
- Shipped history: **CHANGELOG.md**
- Local GSD / SDD / AID / GSM: use `.planning/` (gitignored) when the operator
  starts that pass — do not invent product behavior only in README.

## Version bump (on `develop`, before merge/tag)

| # | Artifact | Action |
|---|----------|--------|
| 1 | **`VERSION`** | New semver without `v` |
| 2 | **`README.md`** | Static Version badge; Status table if needed |
| 3 | **`CHANGELOG.md`** | Move `[Unreleased]` → `[X.Y.Z]` |
| 4 | **`contrib/man/man1/*.1`** | `.TH` date + `kpm v<VERSION>` on **both** pages |
| 5 | **BSD ports** | `make port-freebsd-sync` + `make port-openbsd-sync` |
| 6 | **VHS** | Re-record `docs/demo.gif` from `docs/demo.tape` when help/version strings change |
| 7 | **SPEC / ROADMAP** | Status banner + CLI map + band tables match shipped surface (no stale “draft/planned”) |
| 8 | **Gate** | `make release-check` — only when asked |
| 9 | **Ship** | PR `develop` → `main`, tag — only with explicit operator OK |

**Band exit (feature train):** every band PR that ships behavior updates
`SPECIFICATIONS.md` (section status), man pages, `configs/kpm.sample.yml`, and
`CHANGELOG` **in the same train** — not a follow-up “someday”. Release cut
checks VHS + SPEC banner so docs never lag the binary.

## Deferrals (do not invent in code yet)

- Homebrew tap (`homebrew-kpm`)
- Official FreeBSD/OpenBSD ports submission (skeletons live under `contrib/`)
- Marketing / webs pins (`kpm-webs` / skill **`kpm-webs-release`**) — landing `kpm.hermesrodriguez.com` + get host (GitLab); pin after published tags when shipping webs

Operator deploy docs live in **[kpm-selfhosted](https://github.com/hrodrig/kpm-selfhosted)** (not this tree).
