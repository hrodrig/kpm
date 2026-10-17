# Plan 0.8.0 — Honesty (retention · trust signals · pulled-forward guards)

<a id="top"></a>

Band plan for **0.8.x Honesty**. Roadmap IDs: `#24`–`#28` (native to this band) plus
`#30`, `#31`, `#32` pulled forward from the 0.9.x Freeze parked list.
Parent: [ROADMAP.md](./ROADMAP.md). Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md).

**Depends on:** 0.7.x Scale shipped (`v0.7.0`) on `main` and synced back to `develop`.

**Goal:** Make the surfaces that *report* confidence honest. The 0.7.x band made the store and the
serve path usable at scale; this band makes sure the numbers kpm prints are not quietly unsupported
— retention shorter than the window being measured, a rate limit that an IPv6 client can rotate
past, an SMTP header that accepts injected CR/LF, and a release job that will happily publish a tag
that never touched `main`.

**Checklist convention:** `[x]` done · `[ ]` remaining — **do not delete** rows.

**Why the pulled-forward items are here:** all three were parked under 0.9.x Freeze in the band map
(`audits-and-others/BANDS-0.7-to-1.0.md`). They moved into 0.8.x because each is a closed-scope
correctness gap with a one-file centre of gravity, and leaving them in Freeze risked shipping
`v1.0.0` with them still open. 0.9.x `#22` keeps the remaining security-pass and RBAC items.

**Out of band for 0.8:** E2E smoke (`#20`), docs freeze (`#21`), the rest of the security pass
(`#22`), Homebrew (`#23`), CLI/schema freeze notes (`#29`), async notify queue
(`#ice-notify-async`).

---

## Open issues (this band)

| Roadmap ID | Issue | Item |
|------------|-------|------|
| `#24` | [#87](https://github.com/hrodrig/kpm/issues/87) | Validate `retention_days` ≥ `recommend.window_days` at load |
| `#30` | [#88](https://github.com/hrodrig/kpm/issues/88) | Group rate-limit buckets by IPv6 `/64` |
| `#31` | [#89](https://github.com/hrodrig/kpm/issues/89) | Encode SMTP `Subject` (RFC 2047), reject CR/LF, `require_starttls` |
| `#32` | [#90](https://github.com/hrodrig/kpm/issues/90) | Fail the release unless the tag is on `main`; scope workflow permissions |

`#25`–`#28` are native to this band and not yet filed as issues.

---

## Band checklist

### Docs / planning

- [x] `docs/plan-0.8.0.md` (this file)
- [ ] ROADMAP Current focus → 0.8.x Honesty; band table + 0.8/0.9 sections updated
- [ ] SPEC notes for the retention gate and the IPv6 prefix length
- [ ] `configs/kpm.sample.yml` notes for `require_starttls` and the `/64` grouping
- [ ] man pages when CLI/help strings change
- [ ] CHANGELOG `[Unreleased]` → `[0.8.0]` on version cut only

### `#24` — retention vs window (issue #87)

- [ ] Validation fails closed when `store.retention_days < recommend.window_days`
- [ ] Error message names both values and the file that sets them
- [ ] Cross-check `SPECIFICATIONS.md:95` and `docs/measurement-and-rightsizing.md:76` still match
- [ ] Config tests: equal values pass, shorter retention rejected, default (14 vs 7) passes

### `#30` — IPv6 rate-limit grouping (issue #88)

- [ ] IPv6 buckets keyed by `/64`; IPv4 keeps per-address
- [ ] IPv4-mapped IPv6 normalised so one host cannot hold two buckets
- [ ] Allowlist CIDR matching uses the same prefix logic
- [ ] Tests: two addresses in the same `/64` share a bucket; different `/64` do not
- [ ] SPEC + sample config document the prefix length

### `#31` — SMTP headers and TLS policy (issue #89)

- [ ] `Subject` encoded with `mime.QEncoding`; CR/LF rejected or stripped post-encode
- [ ] `From` / `To` config values rejected on embedded CR/LF
- [ ] `require_starttls` option; `UseTLS: false` without it stays opportunistic but says so
- [ ] Tests: CRLF subject rejected; non-ASCII subject round-trips; STARTTLS required fails closed

### `#32` — release guard (issue #90)

- [ ] Guard step fails the run when the tag commit is not an ancestor of `main`
- [ ] Workflow-level `permissions:` removed or reduced to `contents: read`
- [ ] cosign (`id-token: write`) and GHCR push behaviour unchanged
- [ ] **Release prep:** this is a CI workflow change, so VERSION bump + dated version-surface sync
      (CHANGELOG, README badge, both man `.TH`, BSD ports) land in the same train

### Native band items (not yet filed)

- [ ] `#25` Separate CPU vs memory `target_utilization`
- [ ] `#26` Window coverage by days-with-samples
- [ ] `#27` Percentile / `--score` docs first-class
- [ ] `#28` `kpm doctor` (metrics-server / store / history)

### Gates / band exit

- [ ] `make lint` + `make cover-check` ≥ 80%
- [ ] SPEC Honesty sections updated (status / CLI map as needed)
- [ ] Feature PRs → `develop` green
- [ ] Solo `chore/release-0.8.0` (VERSION, CHANGELOG, badge, both man `.TH`)
- [ ] `make release-check` green
- [ ] PR `develop` → `main` + annotated tag `v0.8.0` (operator OK)
- [ ] Sync `main` → `develop`

[↑ Back to top](#top)

---

## Merge order (suggested)

1. Docs ROADMAP + this plan
2. `#24` retention gate — smallest, unblocks nothing but sets the doc/gate agreement
3. `#31` SMTP headers — self-contained in `internal/notify`
4. `#30` IPv6 `/64` — touches rate limit + allowlist + SPEC together
5. `#32` release guard — **release prep**, carries the VERSION bump with it
6. Version cut `0.8.0`

[↑ Back to top](#top)
