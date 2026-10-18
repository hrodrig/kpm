# Plan 1.0.0 — Harden and tag (no new surface)

> **Status: cutting.** Product surface through `v0.9.0` is frozen. This band does not add commands,
> flags, or schema. It closes the security tail that `#22` marked done after only the allowlist
> and the RBAC example, then cuts `v1.0.0`.

<a id="top"></a>

Band plan for **1.0.0**. Parent: [ROADMAP.md](./ROADMAP.md). Contract:
[SPECIFICATIONS.md](../SPECIFICATIONS.md). Previous band: [plan-0.9.0.md](./plan-0.9.0.md).

**Depends on:** 0.9.x truth + restore shipped (`v0.9.0`) on `main` and synced back to `develop`.

**Goal:** Tag a stable CLI whose frozen contract matches the code. `v0.9.0` already shipped
restore, capacity fit, the normative SPEC, E2E smoke, the Homebrew cask, and the RBAC example.
What is left is hardening that either locks a decision already in SPEC §7.2 or fixes a fail-open
path the 2026-10-17 static audit found. No new product surface.

**Checklist convention:** `[x]` done · `[ ]` remaining — **do not delete** rows.

**Decided, not a work item:** bearer stays **required off loopback** and **optional on loopback**
(`127.0.0.1`, `::1`, `localhost`). SPEC §7.2 already says this. A shared bastion does not isolate
local users; that is an accepted risk of the default bind, not a 1.0 behavior change. Requiring a
token on loopback after the tag would be a breaking config change.

## Work units

| # | Work unit | Status |
|---|-----------|--------|
| 1 | `#33` `trusted_proxies` fails closed on an unparseable entry | `[x]` on `develop` |
| 2 | `#34` `kpm doctor` RunE returns the report exit code when `Broken()` | `[x]` on `develop` |
| 3 | `#35` Sampler not starved by API reads (read pool, per-request timeout, lower batch cap) | `[x]` on `develop` |
| 4 | `#36` CI and release pins (Action SHAs, Grype without `curl \| sh`, pinned govulncheck, image digests, GoReleaser not `~> v2`, image SBOM) | `[x]` on `develop` |
| 5 | `#37` Defense in depth that does not change the frozen CLI | `[ ]` |

`#33`–`#37` continue the roadmap sequence. They are not GitHub issues until filed.
`#22` in 0.9.x covered the allowlist fail-closed fix and [docs/rbac.md](./rbac.md) only.

## Band checklist

### Before the tag

- `[x]` Bearer contract recorded in SPEC §7.2 (required off loopback, optional on loopback)
- `[x]` `#33` Invalid `mcp.trusted_proxies` entry refuses to start; empty list still ignores forwarded client-IP headers
- `[x]` `#34` `internal/cli` test: missing store exits 1 after printing the report; warnings exit 0; cluster-only failure exits 2
- `[x]` `#35` HTTP reads do not hold the sampler's only SQLite connection. Series scan stops at 10,000 rows; a batch accepts 20 workloads
- `[x]` `docs/plan-0.9.0.md` checklist matches what `v0.9.0` shipped
- `[x]` SPEC §14 P6 no longer says Planned

### `#36` — supply chain (same train as the tag, or the first 1.0.x if the tag cannot wait)

- `[x]` Actions pinned by SHA, including the release job
- `[x]` Grype installed without `curl | sh`
- `[x]` `govulncheck` not `@latest`
- `[x]` GoReleaser not a floating `~> v2`
- `[x]` Dockerfiles by digest
- `[x]` Image SBOM enabled (`.goreleaser.yaml` `sbom: true`)

### `#37` — lows, safe as 1.0.x

- `[ ]` Security headers on the UI (CSP, nosniff, frame-ancestors, Referrer-Policy)
- `[ ]` `IdleTimeout` and `MaxHeaderBytes`
- `[ ]` API 500s and `/readyz` do not return raw `err.Error()`
- `[ ]` 413 detected without `strings.Contains(err, "exceeds")`
- `[ ]` `BrowserGuard` does not treat `0.0.0.0` or an empty Host as loopback
- `[ ]` `kpm export -o` and the restore temp use `0600` and `O_EXCL`; backup chmod is not a window after `VACUUM INTO`
- `[ ]` systemd unit: dedicated user, `NoNewPrivileges`, `ProtectSystem`

### Gates

- `[ ]` `make release-check` green on the commit that will be tagged
- `[ ]` Annotated tag `v1.0.0` on `main` — **only with explicit operator approval**

## What this band deliberately does not do

- **New commands, flags, JSON fields, or schema versions.** SPEC §2.6 freezes that surface. A
  behavior correction is not a break; a new default that requires a token on loopback is.
- **json export, mTLS, history in the TUI, `kpm-selfhosted`.** Later 1.x. They do not block the tag.
- **Postgres, HPA, cost, multi-cluster, async notify.** Icebox.
- **Prometheus as a second utilization source, a VPA column, GPU, ephemeral-storage, network columns, dropping tainted nodes from the total.** Surveyed from kube-capacity and rejected. See the 1.x section.

## 1.x after the tag — capacity table

Does not block `v1.0.0`. New flags are a minor (1.1, 1.2, …): they add names, they do not change the ones §2.6 freezes. Source: the open kube-capacity queue (37 issues, 15 PRs), surveyed 2026-10-17. kpm 0.9 already took terminal pods and the init-container request fold.

| # | Item | From | Status |
|---|------|------|--------|
| `#38` | Print init containers and native sidecars as their own rows. The folded `max(sum, max init)` number stays the node total | [#157](https://github.com/robscott/kube-capacity/issues/157), [#182](https://github.com/robscott/kube-capacity/issues/182), [PR #206](https://github.com/robscott/kube-capacity/pull/206) | `[ ]` |
| `#39` | Util percent denominator: node allocatable, request, or limit. Live `capacity` today speaks in node percent; `recommend` already compares to the request | [#67](https://github.com/robscott/kube-capacity/issues/67). Not the Prometheus half of [PR #202](https://github.com/robscott/kube-capacity/pull/202) | `[ ]` |
| `#40` | When node util and the sum of pod util diverge, say so. metrics-server node numbers are not that sum | [#100](https://github.com/robscott/kube-capacity/issues/100) | `[ ]` |
| `#41` | Test the namespace rollup. kube-capacity prints `0` memory util on the total while the nodes underneath have numbers | [#196](https://github.com/robscott/kube-capacity/issues/196) | `[ ]` |
| `#42` | List pods whose containers have no request or limit. Same question as recommend, without waiting for history | [#103](https://github.com/robscott/kube-capacity/issues/103) | `[ ]` |
| `#43` | `capacity --nodes` restricted to a node name, repeatable | [#197](https://github.com/robscott/kube-capacity/issues/197) | `[ ]` |
| `#44` | If a label filter is added, repeated values must all apply | [#158](https://github.com/robscott/kube-capacity/issues/158), [PR #208](https://github.com/robscott/kube-capacity/pull/208) | `[ ]` |

Pending and `ImagePullBackOff` pods keep their reservation. [#125](https://github.com/robscott/kube-capacity/issues/125) asked to drop every non-Running pod. That under-counts what the scheduler still holds.

First slice, if only two land: `#38` and `#39`.

## Merge order

1. `fix/trusted-proxies-fail-closed` → `develop` (`#33`, `#34`).
2. `#35` and whatever of `#36` is in the tag train, each as its own PR into `develop`.
3. Solo `chore/release-1.0.0` for `VERSION`, CHANGELOG, badge, and both man `.TH` lines. No other diffs.
4. PR `develop` → `main`, green CI, annotated tag. Never without explicit operator approval.
5. `#37` and any `#36` item left behind ship as 1.0.x.

[↑ Back to top](#top)
