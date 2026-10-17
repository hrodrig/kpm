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
| `#24` | [#87](https://github.com/hrodrig/kpm/issues/87) | Warn when `retention_days` < `recommend.window_days` at load |
| `#30` | [#88](https://github.com/hrodrig/kpm/issues/88) | Group rate-limit buckets by IPv6 `/64` |
| `#31` | [#89](https://github.com/hrodrig/kpm/issues/89) | Encode SMTP `Subject` (RFC 2047), reject CR/LF, `require_starttls` |
| `#32` | [#90](https://github.com/hrodrig/kpm/issues/90) | Fail the release unless the tag is on `main`; scope workflow permissions |

`#25`–`#28` are native to this band and not yet filed as issues.

---

## Band checklist

### Docs / planning

- [x] `docs/plan-0.8.0.md` (this file)
- [x] ROADMAP Current focus → 0.8.x Honesty; band table + 0.8/0.9 sections updated
- [x] SPEC notes: retention guidance unchanged (§2.2 still "prefer"), rate-limit `/64` grouping
      and the allowlist exception in §7, `require_starttls` + SMTP header handling in §6
- [x] `configs/kpm.sample.yml` notes for `require_starttls` and the `/64` grouping
- [x] man pages — n/a, no CLI/help string changed in this batch (the man pages do not enumerate
      sink or rate-limit sub-keys)
- [ ] CHANGELOG `[Unreleased]` → `[0.8.0]` on version cut only

### `#24` — retention vs window (issue #87)

- [x] Advisory warning when `store.retention_days < recommend.window_days` — **warn, not fail-closed**:
      `SPECIFICATIONS.md` §2.2 states the relation as guidance ("prefer"), so a hard error would
      contradict the contract and stop collection over a quality nit
- [x] Warning names both values (via `slog.Warn`, same channel as `httpserver/trusted.go`)
- [x] Cross-check `SPECIFICATIONS.md:95` and `docs/measurement-and-rightsizing.md:76` still match
- [x] Config tests: equal values stay silent, thin retention warns and still loads, default (14 vs 7) silent
- [x] Commit `98821bb`

### `#30` — IPv6 rate-limit grouping (issue #88)

- [x] IPv6 buckets keyed by `/64`; IPv4 and IPv4-mapped IPv6 stay per-address
- [x] IPv4-mapped IPv6 normalised so one host cannot hold two buckets
- [x] Allowlist **left unchanged** — grouping there would *widen* an operator's intent
      (a `/128` entry would start admitting a whole `/64`), the opposite direction from the
      rate-limit fix, so the two must not share the prefix logic
- [x] Tests: two addresses in the same `/64` share a bucket (identity + middleware 429);
      a different `/64` keeps its own budget
- [x] SPEC §7 + sample config document the prefix length and the allowlist exception
- [x] Commit `12e6729`

### `#31` — SMTP headers and TLS policy (issue #89)

- [x] `Subject` encoded with `mime.QEncoding`; ASCII passes through unchanged
- [x] CR/LF rejected in `from`, recipients and subject before the transport is reached
- [x] `require_starttls` option; `use_tls: false` without it stays opportunistic but is documented
      as such, and `require_starttls` fails closed against a relay that does not advertise it
- [x] Tests: CRLF subject and header fields rejected; non-ASCII encoded, ASCII untouched;
      fake relay proves the refusal path *and* that the advertise path attempts the upgrade
- [x] `sendImplicitTLS` and the STARTTLS-required path share one `deliver()`
- [x] Commit `0380cd4`

### `#32` — release guard (issue #90)

- [x] Guard step fails the run when the tag commit is not an ancestor of `main`
- [x] Workflow-level `permissions:` reduced to `contents: read`
- [x] cosign (`id-token: write`) and GHCR push behaviour unchanged (job-level block untouched)
- [x] **Sizing resolved — NOT release prep.** `go-release-hygiene` treats a Dockerfile / Makefile /
      `.goreleaser` change as release prep because it changes the produced artifact. `release.yml`
      is the release *gate*, not an artifact, and the repo's own precedent is a workflow-only commit
      with no VERSION bump (`581af40 chore(ci): include configs package in unit tests`, the `#67`
      fix). This change follows that precedent.
- [x] Verified the guard discriminates: main reachable → pass, develop-only or branch commit → fail,
      and all seven existing tags (`v0.1.0` … `v0.7.0`) are reachable from `main`, so no past
      release would have been blocked
- [x] Commit `c69e151`

### Native band items (not yet filed)

- [ ] `#25` Separate CPU vs memory `target_utilization`
- [ ] `#26` Window coverage by days-with-samples
- [ ] `#27` Percentile / `--score` docs first-class
- [ ] `#28` `kpm doctor` (metrics-server / store / history)

### Gates / band exit

- [x] `make lint` + `make cover-check` ≥ 80% (80.6% after the pulled-forward items)
- [x] SPEC Honesty sections updated (retention wording, rate-limit `/64`, SMTP sink)
- [ ] Feature PRs → `develop` green
- [ ] Solo `chore/release-0.8.0` (VERSION, CHANGELOG, badge, both man `.TH`)
- [ ] `make release-check` green
- [ ] PR `develop` → `main` + annotated tag `v0.8.0` (operator OK)
- [ ] Sync `main` → `develop`

[↑ Back to top](#top)

---

## Merge order

1. Docs ROADMAP + this plan
2. `#24` retention warning — smallest, sets the doc/gate agreement
3. `#31` SMTP headers — self-contained in `internal/notify`
4. `#30` IPv6 `/64` — touches rate limit + SPEC together
5. `#32` release guard — standalone workflow change, no VERSION bump
6. Version cut `0.8.0`

[↑ Back to top](#top)
