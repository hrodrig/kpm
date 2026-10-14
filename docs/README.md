# Docs

Product behavior lives in root [SPECIFICATIONS.md](../SPECIFICATIONS.md). This
folder holds README assets, operator guides, and future VHS tapes — same role
as `docs/` in [kwd](https://github.com/hrodrig/kwd).

| Asset | Role |
|-------|------|
| [ROADMAP.md](./ROADMAP.md) | Semver bands to v1.0; global work IDs `#1`–`#23` |
| [plan-0.2.0.md](./plan-0.2.0.md) | Core band merge order + success criteria |
| [measurement-and-rightsizing.md](./measurement-and-rightsizing.md) | Measurement + recommend strategy (Mermaid); linked from README |
| [kpm-hero-oss.jpg](./kpm-hero-oss.jpg) | README hero (1280×720), family style |
| [kpm-serve-analyze-local-dark.png](./kpm-serve-analyze-local-dark.png) | README — `kpm serve` charts (analyze-local) |
| [kpm-tui-cpu-request-13pods.png](./kpm-tui-cpu-request-13pods.png) | README — live TUI spectrum |
| [kpm-flow-diagram.png](./kpm-flow-diagram.png) | README — sample → SQLite → facades |
| [demo.tape](./demo.tape) → [demo.gif](./demo.gif) | VHS terminal demo; re-record on VERSION bump |

```bash
make build && export PATH="$(pwd)/bin:$$PATH" && bash -c "vhs docs/demo.tape"
```
