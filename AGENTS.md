# Agent Guidelines (kpm)

Context and instructions for AI coding agents working on **kpm** (Kubernetes
Pods Metrics). See [agents.md](https://agents.md/) for the format.

## Project overview

- **What it is:** Go CLI whose **core** samples control-plane pod metrics into
  **SQLite** (plus backup/export). Adjacent: recommend, `capacity`, interactive
  **TUI** (heatmap pod list). Optional: notifications (webhook), read-only
  **MCP**, HTTP **server** charts. **Scaffold only** at `VERSION` 0.1.0.
- **Entrypoint:** `cmd/kpm/main.go`. Packages today: `internal/cli` (Cobra),
  `internal/exitcode`, `configs` (embedded sample YAML).
- **Config contract:** [SPECIFICATIONS.md](SPECIFICATIONS.md) **draft v0.3**.
  Example profile: `configs/kpm.sample.yml`.
- **Kubernetes client:** planned **`client-go`** (no `kubectl` subprocess).

## Scope (product vs operator)

- **This repo (`kpm`):** Go CLI, config, tests, CI, **`make release-check`**,
  binaries, `.deb`/`.rpm`, **`ghcr.io/hrodrig/kpm`**, cosign + SBOM — same split
  as **kwd** / **kzero** / **groot** / **pgwd**.
- **Not here:** cron/systemd, Helm CronJob, operator runbooks → future
  **kpm-selfhosted**. Do not add `run/` deployment trees here.
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
- Behavior contract: **SPECIFICATIONS.md** (stub until SDD)
- Shipped history: **CHANGELOG.md**
- Local GSD / SDD / AID / GSM: use `.planning/` (gitignored) when the operator
  starts that pass — do not invent product behavior only in README.

## Deferrals (do not invent in code yet)

- Full recommend engine, SQLite schema, Metrics API sampler
- Homebrew tap (`homebrew-kpm`)
- FreeBSD/OpenBSD port skeletons
- `kpm-selfhosted`
