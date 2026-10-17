# Plan 0.9.0 — Capacity truth + restore (aggregation · backup round-trip · placement)

<a id="top"></a>

Band plan for **0.9.x**. Roadmap IDs: `#20`–`#23` and `#29` (the original Freeze list) plus the
capacity and restore work pulled forward from the audit and the kube-capacity survey (`#110`, `#111`).
Parent: [ROADMAP.md](./ROADMAP.md). Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md).

**Depends on:** 0.8.x Honesty shipped (`v0.8.0`) on `main` and synced back to `develop`.

**Goal:** Make the two hard questions answerable. *Are the numbers right?* — the 0.8.x band made the
confidence signals honest, but `capacity` was still aggregating requests in a way the scheduler does
not, and reporting a backup path that only ran one way. *Does this fit?* — a capacity tool that
answers "how much is free" is guessing when the operator is asking "can I move this workload here".

**Checklist convention:** `[x]` done · `[ ]` remaining — **do not delete** rows.

**Why these came forward:** the aggregation bugs were found by reading what users of kube-capacity
complain about most — seven of its thirty-seven open issues are "the numbers are wrong", and two of
them are defects kpm shared (`#53`/`#74` init containers, `#99` terminal pods). The incumbent has
carried both for years with the fixes sitting unmerged, which is not a reason for us to carry them
into `1.0.0`.

## Work units

| # | Work unit | Commit | Status |
|---|-----------|--------|--------|
| 1 | Terminal pods out of node totals and pod counts | `846b6df` | `[x]` |
| 2 | Init container requests counted (`max(sum, max init)`) | `36c5be4` | `[x]` |
| 3 | `kpm restore` — replace and merge | `b8dd6e7` | `[x]` |
| 4 | `kpm capacity fit` — workload-aware placement | `f078363` | `[x]` |
| 5 | Contract + man + README + CHANGELOG (three commits) | `bf4ab3e`, `0572728`, `1ddc2f5` | `[x]` |

Open issues closed by the band: **#110** (`restore`), **#111** (taints / placement).

## Band checklist

### Docs / planning

- `[x]` SPEC §2.4 — `kpm restore`, both modes and their guards
- `[x]` SPEC §4 — scheduler-shaped aggregation, terminal pods, pool identity, `capacity fit`
- `[x]` SPEC §11 CLI map — `restore` row
- `[x]` `contrib/man/man1/kpm.1` — `restore` and `capacity fit`
- `[x]` `README.md` status table
- `[x]` `CHANGELOG.md` Unreleased (Added + Fixed)
- `[x]` This plan + `ROADMAP.md`

### `#110` — restore

- `[x]` Replace: validate, temp file, rename, remove stale `-wal`/`-shm`, `--force` guard
- `[x]` Merge: natural-key dedupe per table (no UNIQUE on `samples`)
- `[x]` Merge idempotence — second run of the same file adds nothing
- `[x]` Older backup accepted (absent tables skipped, newer columns defaulted)
- `[x]` Newer schema refused
- `[ ]` **Persist per-node pool data.** `NodeSnapshot` has no taint column, so the server node panel
      does not see pool identity yet. Needs a schema decision (v7) — deliberately not taken here.

### `#111` — placement

- `[x]` `capacity fit --for kind/name` and `--cpu`/`--memory`
- `[x]` Filters: cordon, taints/tolerations, `nodeSelector`, required node affinity, capacity
- `[x]` Scheduler semantics: empty-effect toleration, `Exists` with empty key, `PreferNoSchedule`
      never blocks, ORed affinity terms, `Gt`/`Lt` on integers only
- `[x]` Unmodelled filters named on every run
- `[x]` Taints never change a total (`NodeCapacity.Taints`, `Unschedulable`)
- `[ ]` **Exit code when nothing fits.** The exit table has only cluster (2) and store (3);
      inventing a fourth is a SPEC decision. Today the output says `0 of N nodes fit`.

### Gates / band exit

- `[x]` `make lint` · `make test` · `make cover-check` green (coverage 80.4%)
- `[x]` Mutation check per behavioural fix (4/4 red before green)
- `[ ]` `#20` Integration/E2E smoke
- `[ ]` `#21` Docs freeze: SPEC → normative v1.0; sample config complete
- `[ ]` `#22` Security pass; minimal RBAC ClusterRole example in docs (not Helm)
- `[ ]` `#23` Homebrew tap, if still wanted
- `[ ]` `#29` CLI + SQLite schema freeze notes; 0.x → 1.0 migration notes
- `[ ]` `make release-check`

## What this band deliberately does not do

- **Filter capacity by taint.** A dedicated pool has real capacity and is fully available to the
  workload it exists for. Removing it from the totals would erase it from the report of the operator
  who pays for it. Pool identity is reported instead.
- **Model the whole scheduler.** Anti-affinity, topology spread, volume zone binding and
  fragmentation are out of scope and printed as such. A fit answer that silently ignored them would
  be worse than no fit command.
- **Migrate the schema for taints.** See `#110` above.

## Merge order

1. `feat/0.9-capacity-restore` → `develop` (PR #113), 7 commits, four independent slices.
2. Remaining Freeze items (`#20`–`#23`, `#29`) as their own PRs against `develop`.
3. `develop` → `main` + annotated tag when the Freeze list is empty — **never without explicit
   operator approval**.

[↑ Back to top](#top)
