# MCP — kpm as a tool for AI agents

<a id="top"></a>

`kpm mcp` serves the SQLite store to an MCP client over **stdio**. No cluster, no network, no writes:
the agent reads what you already sampled.

That is the whole value proposition. An agent answering "why is this pod over-provisioned" normally has
to shell out to `kubectl`, parse YAML, and hope the cluster is reachable. Here it calls a tool and gets
numbers that already have history behind them.

Everything on this page was verified against `kpm v0.9.0` by driving the server over stdio and reading
the responses — the examples are captured output, not a sketch of one.

## What it needs

Two things, and the second one is the usual reason it "does nothing":

1. **An absolute path to a config file.** `--config` is required in practice: without it, kpm looks for
   `kpm.yaml` in the client's working directory, which is almost never where you think it is. There is
   no MCP-specific default.
2. **A store with samples in it.** `kpm mcp` reads only the store. If you never ran `kpm sample`, every
   tool returns an empty set — correctly, and unhelpfully.

```yaml
# /Users/you/.config/kpm/kpm.yaml
cluster:
  name: prod-eu
client:
  id: bastion-01
store:
  path: /Users/you/.local/share/kpm/metrics.db
```

## Client configuration

Claude Code and Cursor both take the same `mcpServers` shape — `.mcp.json` at the project root, or
`.cursor/mcp.json` for Cursor. Any other MCP client that spawns a command over stdio uses the same
`command` + `args` pair.

```json
{
  "mcpServers": {
    "kpm": {
      "command": "/usr/local/bin/kpm",
      "args": ["mcp", "--config", "/Users/you/.config/kpm/kpm.yaml"]
    }
  }
}
```

**Both paths must be absolute.** A client does not run a shell, so nothing expands `~`: passing
`--config ~/.config/kpm/kpm.yaml` fails with `open ~/.config/kpm/kpm.yaml: no such file or directory`,
even though the same string works when you type it in a terminal. Run `which kpm` and use the result.

## Tools

| Tool | Arguments | Returns |
|---|---|---|
| `get_status` | — | version, store path, sample interval, notify/mcp flags, cluster name. No secrets. |
| `list_samples` | `limit`, `namespace`, `workload`, `since`, `until` | raw sample rows with `cpu_util_ratio` / `mem_util_ratio` computed, plus `truncated` (there is a hard row cap) |
| `summarize_window` | `namespace`, `workload`, `since`, `until` | util ratios aggregated over a window, from the store — **not** the live Metrics API |
| `list_recommendations` | `grain` (`workload` or `pod`) | the latest rightsizing snapshots already recorded |

`get_status` is cheap and answers "is this wired up at all", which is the first question worth asking.

## A real exchange

```console
$ kpm mcp --config /Users/you/.config/kpm/kpm.yaml
# stderr: kpm mcp: stdio tools on store /Users/you/.local/share/kpm/metrics.db
```

Then, per JSON-RPC over stdin/stdout:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_samples","arguments":{"limit":2}}}
```

```json
{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"logging":{},"tools":{"listChanged":true}},"protocolVersion":"2025-06-18","serverInfo":{"name":"kpm","version":"v0.9.0"}}}
{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"{\"count\":1,\"rows\":[{\"container\":\"api\",\"cpu_request_mcores\":500,\"cpu_util_mcores\":120,\"cpu_util_ratio\":0.24,\"mem_request_bytes\":536870912,\"mem_util_bytes\":134217728,\"mem_util_ratio\":0.25,\"namespace\":\"default\",\"owner_kind\":\"Deployment\",\"owner_name\":\"api\",\"pod\":\"api-0\",\"sampled_at\":\"2026-10-17T10:00:00Z\"}],\"truncated\":false}"}]}}
```

**Protocol logs go to stderr only**, so stdout stays clean for the client. If you probe by hand, keep
stdin open until the reply arrives — piping a single line and closing immediately can exit before the
server writes anything, which looks like a hang or a dead server and is neither.

The row above comes from a one-row probe store, so the numbers are illustrative; the envelope, field
names and computed ratios are exactly what the server returns.

## What it does not do

- **No cluster access.** It never talks to the API server, so it works on a bastion with no route to
  the cluster — and it cannot answer anything your store does not already contain.
- **No writes.** Read-only tools, by construction. An agent cannot mutate your metrics.
- **No sampling.** `kpm sample` fills the store; the MCP server only reads it.
- **No authentication**, because stdio is a pipe between two local processes. Expose the HTTP surface
  (`kpm serve`) instead if you need remote access — and read its allowlist configuration, which fails
  closed on an unparseable CIDR.

## Security note

The store contains cluster names, namespaces, pod names, node names and resource figures. That is
infrastructure topology. Point an agent at it deliberately, and treat the config file's `store.path` as
the boundary: the server reads exactly that file and nothing else.

[↑ Back to top](#top)
