# Upgrading kpm

<a id="top"></a>

Operator notes for moving a kpm installation between releases. The normative contract is
[SPECIFICATIONS.md §2.5 and §2.6](../SPECIFICATIONS.md); this page is how to apply it without
losing data.

**Short version:** run `kpm backup` before you upgrade, then upgrade. kpm migrates an older store
in place on first use, and refuses a store written by a newer build instead of guessing at it.

## 1. Back up first

```sh
kpm backup -o ~/kpm-backup-$(date +%Y%m%d).db
```

`backup` is a `VACUUM INTO` copy, so it is safe to run against a store the sampler is writing to.

The reason to bother: **schema migrations are one-way**. Once this build has upgraded a store, a
previous kpm cannot read it. The backup is the way back, and `kpm restore` is how you take it.

## 2. Upgrade

```sh
# package install
sudo apt install ./kpm_<version>_linux_amd64.deb     # or the .rpm

# or pick up the new binary however you installed it
kpm version
```

## 3. Let the store migrate

Nothing to run. The first command that opens the store — `sample`, `serve`, `recommend --record`,
`restore --merge` — migrates it in place:

- new tables and columns are added (`CREATE TABLE IF NOT EXISTS`, `addColumnIfMissing`);
- no existing row is rewritten and no column is dropped;
- `schema_meta.version` moves to the version this build expects.

It is additive and idempotent: running it twice changes nothing.

Confirm afterwards:

```sh
kpm doctor
```

`schema` reports the version on disk against the one the build expects.

## 4. If kpm refuses to open the store

```
store schema v8 is newer than this build understands (v7): upgrade kpm
```

The store was written by a newer kpm than the binary you are running — usually a downgrade, or two
hosts on different versions sharing one file through NFS. kpm refuses **before touching anything**: an
older build adding the columns it thinks are missing would corrupt a schema it cannot interpret.

Fix it by running the newer kpm, not by editing `schema_meta`. If you genuinely need to go back,
restore the backup from step 1 with a kpm at that version.

## 5. What changed that can move your numbers

Corrections are not breaks — they ship in a patch or a minor and are listed in
[CHANGELOG.md](../CHANGELOG.md) — but they do change output, so dashboards move.

| Since | Change | What you will see |
|-------|--------|-------------------|
| 0.9.0 | `capacity` counts init container requests | Node requests **rise** where pods have heavy init containers (`max(sum(containers), max(init))` is what the scheduler reserves). Free headroom was previously overstated. |
| 0.9.0 | `capacity` excludes `Succeeded`/`Failed` pods | Node requests and pod counts **fall** on clusters with completed Jobs. |
| 0.8.0 | `recommend` reports `days_with_samples` | A percentile resting on a fraction of the window is now visible instead of implied. |
| 0.8.0 | per-resource recommend targets | `cpu_target_utilization` / `memory_target_utilization` override the shared target per resource. |

If a node's "requested" total moved between those releases and nothing was deployed, that is the
first two rows: the old number was wrong in the direction that made a node look emptier than it was.

## 6. Schema versions at a glance

| v | Release | Adds |
|---|---------|------|
| 1 | 0.2.0 | `samples` |
| 2 | 0.3.0 | `recommendations` |
| 3 | 0.7.0 | owner index (no table change) |
| 4 | 0.7.0 | `node_snapshots` |
| 5 | 0.8.0 | per-resource recommend targets |
| 6 | 0.8.0 | window coverage |
| 7 | 0.9.0 | node pool identity (`taints`, `unschedulable`) |

`1.0.0` freezes v7. New columns still bump the version; the meaning of an existing table, column or
index changes only in a major release.

## 7. CLI compatibility

Frozen at `1.0.0`: command and flag names, TSV/CSV column order, JSON API field names, and the
[exit codes](../SPECIFICATIONS.md). Adding a command, flag or field is not a break; removing,
renaming or reordering one is.

One addition worth knowing if you upgraded from ≤ 0.9: **`kpm capacity fit` exits `4`** when no node
can run the workload. That is an answer, not a failure — `2` means kpm could not ask the cluster and
`3` means there is no data.

[↑ Back to top](#top)
