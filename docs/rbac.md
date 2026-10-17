# RBAC for kpm

<a id="top"></a>

What kpm reads from a cluster, and the smallest role that allows it. Derived from the API calls in the
tree, not from what it might plausibly need: no `watch`, no `namespaces`, no writes, no secrets.

Two entry points matter: the **sampler** (`kpm sample`, or the daemon) and the **live views**
(`kpm capacity`, `kpm capacity fit`, `kpm tui`). Both are read-only.

## What is actually called

| API group | Resource | Verb | Used by |
|-----------|----------|------|---------|
| `""` (core) | `pods` | list | sampler, `capacity`, `capacity fit` |
| `""` (core) | `nodes` | list | `capacity --nodes`, `--record-nodes`, `capacity fit` |
| `apps` | `deployments`, `statefulsets`, `daemonsets`, `replicasets` | get | owner resolution, `capacity fit --for` |
| `metrics.k8s.io` | `pods` | list | sampler, `capacity` (not `capacity fit` — see below) |

`metrics.k8s.io` is served by **metrics-server**. Without it, `kpm capacity` and `kpm sample` cannot
see utilisation and exit `2`; everything offline (`recommend`, `export`, `restore`, most of `doctor`)
still works from the store.

`kpm capacity fit` deliberately does **not** read the Metrics API: it needs requests, not usage, so it
works on a cluster without metrics-server.

## The role

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kpm-readonly
rules:
  - apiGroups: [""]
    resources: ["nodes", "pods"]
    verbs: ["get", "list"]
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
    verbs: ["get"]
  - apiGroups: ["metrics.k8s.io"]
    resources: ["pods"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kpm-readonly
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kpm-readonly
subjects:
  - kind: ServiceAccount
    name: kpm
    namespace: kube-system
```

Replace the subject with whatever identity runs kpm in your setup — a ServiceAccount in a CronJob, or
your own user for local runs.

### Why a ClusterRole and not a namespaced Role

`nodes` is a cluster-scoped resource, so a `Role` cannot grant it at all. And a node's "requested" CPU
is the sum of **every** pod scheduled there: a role limited to one namespace would produce a node view
that silently undercounts, which is the failure mode the 0.9 aggregation work exists to fix. If you
only care about one namespace, use `scope.namespaces` in the config instead of narrowing RBAC — that
filters what is reported without lying about what is reserved.

### Why `replicasets: get`

Owner resolution walks `pod → ReplicaSet → Deployment` to attribute a pod to its workload. `get` is
enough; kpm never lists ReplicaSets.

## Verify before you deploy

```sh
kubectl auth can-i list pods --all-namespaces --as=system:serviceaccount:kube-system:kpm
kubectl auth can-i list nodes --as=system:serviceaccount:kube-system:kpm
kubectl auth can-i list pods.metrics.k8s.io --as=system:serviceaccount:kube-system:kpm
```

Each should print `yes`. If a permission is missing, kpm **fails closed**: the denial surfaces as
exit `2` (cluster / Metrics API / RBAC) rather than as an empty table.

## What needs no RBAC at all

`kpm recommend`, `kpm export`, `kpm restore`, `kpm backup`, and the offline half of `kpm doctor` read
only the SQLite store. That is the point of sampling: the rightsizing analysis runs where the cluster
is not reachable.

[↑ Back to top](#top)
