# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
