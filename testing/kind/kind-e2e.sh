#!/usr/bin/env bash
# kind-based end-to-end: a real API server, a real metrics-server, real nodes, real load.
#
# This covers the half the offline smoke cannot: that the sampler reads pods, metrics and
# nodes from a live cluster, and that the capacity commands answer with real allocatable
# values. It needs Docker and kind, so it stays a deliberate target
# (`make test-e2e-kind`) instead of a gate on every push.
#
# Three things here are not incidental:
#
#   - kind's kubelets present self-signed certificates, so metrics-server needs
#     --kubelet-insecure-tls or it never scrapes anything.
#   - the cluster carries a workload that actually burns CPU and memory. An idle nginx
#     reports cpu_util=0, which proves only that the sampler did not crash -- not that
#     the Metrics API was read at all. The assertions below require non-zero sampled
#     utilization, so a broken metrics path fails here instead of passing quietly.
#   - requests are set on both workloads, so the node's requested totals are real and
#     `capacity fit` has something to subtract.
#
# Environment:
#   KPM_KIND_CLUSTER   cluster name (default kpm-e2e)
#   KPM_KIND_KEEP=1    keep the cluster after the run, for poking at it
set -euo pipefail

CLUSTER="${KPM_KIND_CLUSTER:-kpm-e2e}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="$(mktemp -d)"
KUBECONFIG_FILE="$WORK/kubeconfig"
STORE="$WORK/metrics.db"
CONFIG="$WORK/kpm.yaml"
BIN="$WORK/kpm"
NODE="$CLUSTER-control-plane"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cleanup() {
  local rc=$?
  echo
  echo "--- cleanup (exit $rc) ---"
  if [ "${KPM_KIND_KEEP:-0}" = "1" ]; then
    echo "keeping cluster $CLUSTER and $WORK"
  else
    kind delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
    rm -rf "$WORK"
  fi
  exit $rc
}
trap cleanup EXIT

for tool in kind kubectl docker go; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is required but not installed"
done

echo "== build =="
(cd "$ROOT" && go build -trimpath -o "$BIN" ./cmd/kpm) || fail "build"

echo "== kind cluster ($CLUSTER) =="
if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  fail "cluster $CLUSTER already exists; delete it (kind delete cluster --name $CLUSTER) or set KPM_KIND_CLUSTER"
fi
kind create cluster --name "$CLUSTER" --wait 120s
kind export kubeconfig --name "$CLUSTER" --kubeconfig "$KUBECONFIG_FILE"

kc() { kubectl --kubeconfig "$KUBECONFIG_FILE" "$@"; }

echo "== metrics-server (--kubelet-insecure-tls) =="
kc apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml >/dev/null
kc patch deployment metrics-server -n kube-system --type=json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]' >/dev/null
kc wait --for=condition=Available deployment/metrics-server -n kube-system --timeout=180s >/dev/null

echo "== workloads =="
# api: an ordinary service that mostly idles. Its near-zero utilization is the control.
kc create deployment api --image=nginx:alpine --replicas=1 >/dev/null
kc set resources deployment/api --requests=cpu=100m,memory=64Mi --limits=cpu=200m,memory=128Mi >/dev/null

# load: something that actually consumes, so the Metrics API has real numbers to serve.
# Command, args and resources go in one patch: applying them separately causes extra
# rollouts, and a pod left over from an earlier revision is easy to sample by mistake.
kc create deployment load --image=polinux/stress --replicas=1 >/dev/null
kc patch deployment load --type=json -p '[
  {"op":"add","path":"/spec/template/spec/containers/0/command","value":["stress"]},
  {"op":"add","path":"/spec/template/spec/containers/0/args","value":["--cpu","1","--vm","1","--vm-bytes","128M","--vm-hang","0"]},
  {"op":"add","path":"/spec/template/spec/containers/0/resources","value":{"requests":{"cpu":"500m","memory":"256Mi"},"limits":{"cpu":"1","memory":"512Mi"}}}
]' >/dev/null

kc rollout status deployment/api --timeout=120s >/dev/null
kc rollout status deployment/load --timeout=180s >/dev/null

# metrics-server scrapes on its own schedule; until it has, kpm would legitimately
# report the pods as skipped.
echo "== waiting for metrics =="
reported=0
for _ in $(seq 1 60); do
  top="$(kc top pods 2>/dev/null || true)"
  if echo "$top" | grep -q '^api' && echo "$top" | grep -q '^load'; then
    reported=1
    break
  fi
  sleep 3
done
[ "$reported" = "1" ] || fail "metrics-server never reported both pods"

kc top pods
load_cpu="$(kc top pods 2>/dev/null | awk '/^load/ {gsub("m","",$2); print $2; exit}')"
[ -n "$load_cpu" ] || fail "could not read the load pod CPU"
[ "$load_cpu" -gt 50 ] || fail "the load pod is not actually loading anything (cpu=$load_cpu m)"
echo "  load pod burning $load_cpu m of CPU — real utilization to sample"

cat > "$CONFIG" <<YAML
cluster:
  name: $CLUSTER
client:
  id: kind-e2e
store:
  path: $STORE
sample:
  interval_seconds: 0
YAML

echo "== kpm sample (one shot) =="
sample_out="$("$BIN" sample --config "$CONFIG" --kubeconfig "$KUBECONFIG_FILE")" || fail "sample exited non-zero"
echo "$sample_out"
echo "$sample_out" | grep -qE "sampled [1-9][0-9]* containers" || fail "sample recorded no containers"

echo "== kpm export =="
"$BIN" export --config "$CONFIG" --format csv -o "$WORK/out.csv" >/dev/null || fail "export"
rows="$(($(wc -l < "$WORK/out.csv") - 1))"
echo "  rows: $rows"
[ "$rows" -ge 2 ] || fail "expected at least two containers, got $rows"
grep -q "api" "$WORK/out.csv" || fail "exported rows do not mention the api pod"
grep -q "load" "$WORK/out.csv" || fail "exported rows do not mention the load pod"

# The assertion that matters: real numbers crossed the Metrics API into the store.
# CSV columns: 5=pod 10=cpu_util_mcores 11=mem_util_bytes 12=cpu_request_mcores.
# The pod carries the ReplicaSet hash, so match on the prefix the deployment creates.
awk -F, 'NR>1 && $5 ~ /^load/ {printf "  load row: cpu_util=%sm mem_util=%sB cpu_req=%sm\n", $10, $11, $12; exit}' "$WORK/out.csv"
awk -F, 'NR>1 && $5 ~ /^load/ { if ($10+0 > 50 && $11+0 > 0) ok=1 } END { exit ok?0:1 }' "$WORK/out.csv" ||
  fail "the sampler stored no real utilization for the load pod"
# Utilization is half of it: the requests have to be there too, or capacity fit has
# nothing to subtract and the fit numbers are fiction.
awk -F, 'NR>1 && $5 ~ /^load/ { if ($12+0 > 0) req=1 } END { exit req?0:1 }' "$WORK/out.csv" ||
  fail "the load pod was sampled without requests, so fit subtraction is untested"

echo "== kpm capacity --nodes --record-nodes =="
capacity_out="$("$BIN" capacity --config "$CONFIG" --kubeconfig "$KUBECONFIG_FILE" --nodes --record-nodes)" ||
  fail "capacity exited non-zero"
echo "$capacity_out"
echo "$capacity_out" | grep -q "$NODE" || fail "node table does not list $NODE"
echo "$capacity_out" | grep -q "api" || fail "pod table does not list the api workload"
echo "$capacity_out" | grep -q "load" || fail "pod table does not list the load workload"

echo "== kpm capacity fit (should fit) =="
"$BIN" capacity fit --config "$CONFIG" --kubeconfig "$KUBECONFIG_FILE" --cpu 100m --memory 64Mi ||
  fail "a 100m/64Mi pod must fit on an idle node"
"$BIN" capacity fit --config "$CONFIG" --kubeconfig "$KUBECONFIG_FILE" --for deployment/api ||
  fail "capacity fit --for deployment/api must succeed"

echo "== kpm capacity fit (should NOT fit, expect exit 4) =="
set +e
"$BIN" capacity fit --config "$CONFIG" --kubeconfig "$KUBECONFIG_FILE" --cpu 100 --memory 512Gi >/dev/null 2>&1
nofit=$?
set -e
[ "$nofit" = "4" ] || fail "a 100 CPU pod must exit 4 (NoFit), got $nofit"

echo "== kpm capacity fit (no such workload, expect 2) =="
set +e
"$BIN" capacity fit --config "$CONFIG" --kubeconfig "$KUBECONFIG_FILE" --for deployment/nope >/dev/null 2>&1
missing=$?
set -e
[ "$missing" = "2" ] || fail "an unknown workload must exit 2 (cluster), got $missing"

echo
echo "kind e2e: PASS"
