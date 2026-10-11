# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- `SPECIFICATIONS.md` / sample config: daemon (`interval_seconds > 0`) **always**
  persists metrics; notifications, MCP, and HTTP server are each optional
  facades when enabled (§2.3, §15.4).
- Notifications: if `enabled` and `rules` empty/omitted → built-in CPU+mem
  util-vs-request @ **80%** plus one-time English startup log (§6.0, §15.5).
- Docs: [docs/measurement-and-rightsizing.md](docs/measurement-and-rightsizing.md)
  (measurement + recommend strategy, Mermaid); short pointer + overview diagram
  in README; note that higher `retention_days` improves statistical quality
  (production: **> 45** days recommended).
- `SPECIFICATIONS.md`: **draft v0.3** — closed open points (notify
  predicate/lifecycle, MCP stdio+HTTP bearer, embedded server UI, csv/tsv
  export); added adjacent **TUI** (`kpm tui` / `capacity --tui`): preselected
  pods, horizontal heatmap bars, sort by name/util/allocation/ratio. Phasing:
  TUI in P2 with capacity.
- `configs/kpm.sample.yml`: `tui` block + notify field comments for v0.3.
- README aligned with the hrodrig CLI family header (badges, nav line, early-dev
  disclaimer, Related tools with gghstats/selfhosted links, OSS hero image).
- `.gitignore`: drop duplicate `!.goreleaser.yaml` whitelist entry.


## [0.1.0] - 2026-10-10

### Added

- Repository scaffold aligned with the [kwd](https://github.com/hrodrig/kwd) mold:
  whitelist `.gitignore`, `GNUmakefile` / BSD `Makefile`, CI / Security / CodeQL /
  Release workflows, GoReleaser (Homebrew deferred), Docker (distroless), man pages,
  `AGENTS.md`, stub `SPECIFICATIONS.md`, sample config embed.
- CLI scaffold: `kpm version`, `kpm --print-sample-config`, bare `kpm` → help.
- Product sampler, SQLite store, and recommend path deferred until SDD / SPEC.
