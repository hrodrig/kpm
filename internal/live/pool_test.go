package live

import "testing"

// A pool is defined by the taints that gate it, and grouping is for reading: the
// capacity of a dedicated pool is real and must add up like any other.
func TestGroupNodesByPool(t *testing.T) {
	nodes := []NodeCapacity{
		{Name: "gen-0", CPUAllocMcores: 4000, CPUReqMcores: 2000, Pods: 5},
		{Name: "gpu-0", Taints: []string{"dedicated=gpu:NoSchedule"}, CPUAllocMcores: 8000, CPUReqMcores: 1000, Pods: 2},
		{Name: "gpu-1", Taints: []string{"dedicated=gpu:NoSchedule"}, CPUAllocMcores: 8000, Unschedulable: true},
	}

	pools := GroupNodesByPool(nodes)
	if len(pools) != 2 {
		t.Fatalf("pools: got %d want 2", len(pools))
	}
	if pools[0].Pool != GeneralPool {
		t.Fatalf("first pool: got %q want %q (sorted by name)", pools[0].Pool, GeneralPool)
	}
	if pools[0].Nodes != 1 || pools[0].CPUAllocMcores != 4000 || pools[0].Pods != 5 {
		t.Fatalf("general pool: %+v", pools[0])
	}

	gpu := pools[1]
	if gpu.Pool != "dedicated=gpu:NoSchedule" {
		t.Fatalf("pool name: %q", gpu.Pool)
	}
	if gpu.Nodes != 2 || gpu.Cordoned != 1 {
		t.Fatalf("nodes/cordoned: %+v", gpu)
	}
	// 8000 + 8000: taints never remove capacity from a total.
	if gpu.CPUAllocMcores != 16000 {
		t.Fatalf("alloc: got %v want 16000 (a tainted node still has capacity)", gpu.CPUAllocMcores)
	}
}

// The key has to be stable regardless of the order the taints arrive in, or the
// same pool would show up as two groups.
func TestPoolKeySortsTaints(t *testing.T) {
	n := NodeCapacity{Taints: []string{"b.example/x:NoSchedule", "a.example/y:NoExecute"}}
	if got, want := PoolKey(n), "a.example/y:NoExecute,b.example/x:NoSchedule"; got != want {
		t.Fatalf("PoolKey = %q want %q", got, want)
	}
}

func TestPoolKeyGeneral(t *testing.T) {
	if got := PoolKey(NodeCapacity{Name: "n"}); got != GeneralPool {
		t.Fatalf("PoolKey = %q want %q", got, GeneralPool)
	}
}

// Grouping must not mutate the caller's slice while sorting.
func TestGroupNodesByPoolDoesNotMutate(t *testing.T) {
	taints := []string{"b:NoSchedule", "a:NoSchedule"}
	nodes := []NodeCapacity{{Name: "n", Taints: taints}}
	GroupNodesByPool(nodes)
	if nodes[0].Taints[0] != "b:NoSchedule" {
		t.Fatalf("caller's taints reordered: %v", nodes[0].Taints)
	}
}
