# kpm compared with similar tools

> **Snapshot 2026-10-17**, against kpm `v0.9.0`. Peer versions, star counts, and
> licenses below were read from GitHub that day. Re-check upstream before you
> bet a design on them. This is not a market survey: hosted rightsizing
> products (Kubecost, PerfectScale, CAST AI, StormForge, and the rest) are
> named once and left out of the matrix.

The one-screen version is in the [README](../README.md#compared-to-similar-tools).

## TL;DR

- **kpm** samples the Metrics API and pod requests into a **local SQLite file**, then recommends offline. It never applies a change.
- **`kubectl top`** and **[kube-capacity](https://github.com/robscott/kube-capacity)** answer “what does the cluster look like right now?” They do not keep history.
- **[KRR](https://github.com/robusta-dev/krr)** answers “what should the request be?” from **Prometheus** you already run.
- **[VPA](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler)** is a controller. **[Goldilocks](https://github.com/FairwindsOps/goldilocks)** is a page over the VPA objects that controller writes.
- Prefer **kpm** when you want weeks of samples on a bastion or a laptop and a suggestion you paste into Git. Prefer **Prometheus / VPA** when that stack is already the source of truth.

## kpm is / kpm is not

| kpm **is** | kpm **is not** |
|------------|----------------|
| A history of metrics-server samples plus the requests and limits on the pod | A Prometheus, cAdvisor, or kube-state-metrics replacement |
| An offline recommendation (`p95` over the stored window, per resource) | An applier. Nothing in kpm runs `kubectl apply` or patches a workload |
| A live capacity table and TUI (`kpm capacity`, `kpm tui`) for the moment you ask | A scheduler. `capacity fit` says whether a workload lands; it does not place it |
| Charts and a read-only MCP over a store you can copy | A multi-cluster product. One process, one cluster |
| Advisory. You own the Git change | A cost model (dollars, spot, bin-packing savings) |

`GET /metrics` on the daemon is **process** counters (ticks, errors). It does not re-export pod CPU and memory.

## Snapshot

| | **kpm** | [kube-capacity](https://github.com/robscott/kube-capacity) | [KRR](https://github.com/robusta-dev/krr) | [Goldilocks](https://github.com/FairwindsOps/goldilocks) | [VPA](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler) |
|--|---------|--------------------------------------------------------------|---------------------------------------------|----------------------------------------------------------|-------------------------------------------------------------------------------------|
| Runtime | Go, single binary | Go, single binary | Python | Go, in-cluster | Go controller |
| Release checked | **0.9.0** (pre-1.0) | **v0.8.0** (2024-02-21). Repo pushed 2025-11-21; no newer tag | **v1.31.0** (2026-10-14) | **v4.16.2** (2026-09-15) | Active in `kubernetes/autoscaler` (pushed 2026-10-15). The repo’s latest tag that day was a Helm chart, not a VPA binary version |
| GitHub stars | ~1 | ~2.7k | ~4.8k | ~3.4k | ~9.0k (whole autoscaler repo) |
| License | MIT | Apache-2.0 | MIT | Apache-2.0 | Apache-2.0 |
| Needs Prometheus | No | No | Yes | No. It needs VPA | No, unless you point the recommender at a Prometheus history provider |
| Needs a controller in the cluster | No | No | No | Yes (itself, plus VPA) | Yes |
| Changes workloads | Never | Never | The CLI prints a recommendation | No. VPA does, when its update mode is not `Off` | Yes, in `Initial`, `Recreate`, or `Auto` |

Star counts are adoption, not a quality score. kube-capacity, KRR, and Goldilocks are the tools people already run. kpm is early.

## What each one answers

| Question | **kpm** | kube-capacity | KRR | Goldilocks | VPA |
|----------|---------|---------------|-----|------------|-----|
| What is using CPU and memory right now? | `capacity` / `tui` (live). History is whatever you sampled | Yes, live | No. It recommends | No | No |
| What did this workload do over the last weeks? | SQLite, default retention 14 days, recommend window 7 | No | Whatever Prometheus still holds | Only what VPA has already reduced to a recommendation | The recommender’s own history |
| What request should I set? | Offline `p95` vs your target utilization. CPU and memory targets can differ | No | Yes, from Prometheus | Shows the VPA target | Yes |
| Requests and limits next to util? | Yes, on the sample and on the live table | Yes. This is the table kpm’s live view borrowed | Requests from kube-state-metrics, usage from Prometheus | VPA’s target, lower, upper | The recommendation object |
| Can I read it with the cluster disconnected? | Yes, from the store (`recommend`, `serve`) | No | No | No | No |
| Where does the advice go? | You copy a patch into Git. kpm does not apply it | Nowhere | You apply what the CLI printed | You copy the VPA number | The controller can edit the pod |

`kubectl top` is the baseline the table does not repeat: one instant of usage, no request, no limit, no history, no recommendation. It ships with kubectl.

## When to pick kpm

- You do not run Prometheus, and you do not want to, for this question.
- You want the samples on a bastion or a laptop. Copy the SQLite file, run `kpm serve` or `kpm recommend`, leave the cluster alone.
- The recommendation must stay advisory. A controller that rewrites requests is the wrong default.
- You also want the live “does this pod fit, and what is the node holding?” table without a second tool.

## When not kpm

- **You already have Prometheus** with container usage and kube-state-metrics, and you want a recommendation today. Use **KRR**. Standing up kpm beside that store duplicates the history.
- **You want the cluster to keep requests current** without a human pasting YAML. Use **VPA** with an update mode other than `Off`. Know that it will recreate or resize pods.
- **You want a dashboard inside the cluster** of those VPA numbers, for people who will not open a CLI. Use **Goldilocks**.
- **You only need the number right now**, once. Use **`kubectl top`** or **kube-capacity**. Sampling for a week is wasted motion.
- **You need dollars, GPU, ephemeral-storage, or network.** kpm does not model them. A cost product or the Prometheus you already have is the right place.
- **You need one view across many clusters.** kpm is one process, one cluster. Repeat it, or use a store that already aggregates.

## Notes on each peer

### kubectl top

Built in. Reads the Metrics API. Prints current CPU and memory for nodes or pods. It cannot show the request the scheduler is holding, and the number is gone when you close the terminal. Use it to see if a pod is hot. Do not use it to decide next quarter’s requests.

### kube-capacity

The live table this space grew up with: node and pod requests, limits, and (with metrics-server) utilization, plus label and taint filters. kpm’s `capacity` uses the same quantities and does not copy its source. kube-capacity’s last tagged release is **v0.8.0 (2024-02-21)**. Krew installs it as `kubectl resource-capacity`.

Use kube-capacity when you want that table and nothing else. Use kpm `capacity` when you are already sampling and you want the same glance plus a store. Neither tool remembers yesterday unless something else writes it down. kpm is the something else.

### KRR

[Robusta KRR](https://github.com/robusta-dev/krr) is a Python CLI: “Prometheus-based Kubernetes Resource Recommendations.” It asks Prometheus for history and prints a recommended request. It does not install a controller and it does not keep its own copy of the samples.

Use KRR when Prometheus retention already covers the window you care about. kpm’s reason to exist is the case where that sentence is false: the history has to live in a file you control, taken from metrics-server on a timer.

### Goldilocks and VPA

VPA, in [kubernetes/autoscaler](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler), watches usage and writes a recommendation. Update mode `Off` only recommends. `Initial`, `Recreate`, and `Auto` change pod requests. That is a different product decision from kpm, which has no update mode because it has no write path to the cluster.

Goldilocks creates VPA objects (namespaces you label) and renders their recommendations. It is the in-cluster UI for VPA, not a second recommender and not a local archive. If VPA is already installed and trusted, Goldilocks is the page. Adding kpm there means a second opinion from a different history (metrics-server samples you took), which is useful only if you want that second opinion on purpose.

### Prometheus, kube-state-metrics, Grafana

The full stack when usage, requests, alerts, and several clusters already live in one TSDB. kpm does not scrape exporters and does not draw Grafana dashboards. `kpm serve` charts the SQLite file. If the organization standard is “every number comes from Prometheus,” kpm is the wrong system of record. It can still sit on a bastion as a private notebook for one cluster.

### kube-resource-report

[hjacobs/kube-resource-report](https://github.com/hjacobs/kube-resource-report) generated a static HTML report of requests versus usage. The repository is **archived**. Last release **20.7.3 (2020-07-02)**, GPL-3.0. It is the historical “write a report from the cluster” idea, not a tool to adopt now.

## One line each

| Tool | In one sentence |
|------|-----------------|
| **kpm** | Metrics-server history in SQLite, an offline recommendation, a live table, and no writes to the cluster. |
| **kubectl top** | The usage number for this instant. |
| **kube-capacity** | The live requests / limits / util table. Last release February 2024. |
| **KRR** | Recommendations from Prometheus, printed by a Python CLI. |
| **Goldilocks** | A dashboard of VPA recommendations inside the cluster. |
| **VPA** | The controller that computes those recommendations and can apply them. |
