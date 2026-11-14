# Take the store home (analyze-local)

<a id="top"></a>

You sampled on a bastion or in-cluster. You want charts, `recommend`, or MCP on a
laptop **without** the cluster. That works: point `kpm serve` / `kpm mcp` at a
**consistent** SQLite copy. The failure mode is almost always a **hot file copy**
of `store.path` while the sampler still writes.

## Do not copy the live `.db` with `cp` / `scp` / `rsync`

While `kpm sample` (daemon) holds the store open, SQLite may keep recent pages in
a **WAL** (`metrics.db-wal`). Copying only `metrics.db` (or copying mid-write)
often yields:

```text
Error: database disk image is malformed
*** in database main ***
Tree … invalid page number …
```

`PRAGMA integrity_check` fails. Charts and MCP then fail to open the store.

## Preferred: `kpm backup`

On the machine that owns the live store:

```bash
kpm backup --config /etc/kpm/kpm.yaml -o /tmp/metrics-home.db
```

`kpm backup` uses SQLite **`VACUUM INTO`**: a consistent snapshot even while the
store is open. Take **`/tmp/metrics-home.db`** home (USB, scp, sync). Leave the
live `-wal` / `-shm` on the bastion; you do not need them for the backup file.

Default `-o` writes alongside `store.path` when omitted — see `kpm backup --help`.

## Alternatives (same idea)

| Method | Notes |
|--------|--------|
| `sqlite3 "$STORE" ".backup '/tmp/metrics-home.db'"` | Online backup API; good if `kpm` is not on PATH |
| Stop the sampler, then `PRAGMA wal_checkpoint(TRUNCATE);`, then copy `.db` | Only when you can pause writes; still prefer `kpm backup` |

## On the laptop

1. Place the backup file somewhere durable (example: `~/kpm-dbs/prod.db`).
2. Use a **dedicated** profile (do not point at a live bastion path):

```yaml
cluster:
  name: "prod"           # label only — offline
  environment: "offline"
store:
  path: "/Users/you/kpm-dbs/prod.db"
sample:
  interval_seconds: 0    # no sample loop
server:
  enabled: true
  listen: "127.0.0.1:9740"
# Optional MCP over HTTP (same listen) or use stdio: kpm mcp --config …
# mcp:
#   enabled: true
```

3. Verify before analyze:

```bash
sqlite3 ~/kpm-dbs/prod.db 'PRAGMA integrity_check;'
# expect: ok
sqlite3 ~/kpm-dbs/prod.db 'SELECT COUNT(*) FROM samples;'
```

4. Run facades (kubeconfig optional; live Metrics 503 is fine):

```bash
kpm serve --config ~/kpm-dbs/prod.yaml
# charts: http://127.0.0.1:9740/
# or stdio MCP for an agent:
kpm mcp --config ~/kpm-dbs/prod.yaml
```

See [mcp.md](./mcp.md) for Cursor / Claude client wiring. Operator deploy shapes:
[kpm-selfhosted](https://github.com/hrodrig/kpm-selfhosted).

## Offline recommend vs node capacity (do not confuse)

| Command | Needs cluster? | Writes | Table |
|---------|----------------|--------|--------|
| **`kpm recommend --record`** | **No** (SQLite only) | Rightsizing advice | `recommendations` |
| **`kpm capacity --record-nodes`** | **Yes** (live Metrics / nodes) | Node allocatable / request / util | `node_snapshots` |

You can run **`recommend --record` on the laptop** against the take-home DB (or on
the bastion before `kpm backup`). Charts and MCP `list_recommendations` stay empty
until someone records.

**Node** cards / `GET /api/v1/nodes` need a prior **`capacity --record-nodes`** on a
host that can reach the cluster — a store-only laptop copy does not invent that.

```bash
# bastion or laptop with the copied store (no kube):
kpm recommend --config ~/kpm-dbs/prod.yaml --record

# bastion with kubeconfig (node snapshots):
kpm capacity --config /etc/kpm/kpm.yaml --nodes --record-nodes
```

## Checklist

- [ ] Produced the file with **`kpm backup`** (or `.backup`), not a raw hot `cp`
- [ ] `PRAGMA integrity_check` → `ok` on the laptop
- [ ] Laptop YAML: `store.path` → that file, `sample.interval_seconds: 0`
- [ ] `kpm serve` and/or `kpm mcp` against that config
- [ ] Optional: `kpm recommend --record` for rightsizing badges / MCP recs
- [ ] Optional (needs kube): `kpm capacity --record-nodes` for Nodes panel

[↑ Back to top](#top)
