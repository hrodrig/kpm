package server

import "testing"

func TestGrainKey(t *testing.T) {
	cases := []struct {
		ns, kind, name, pod, container, want string
	}{
		{"app", "Deployment", "foo", "p", "c", "app/Deployment/foo/c"},
		{"app", "", "foo", "p", "c", "app/Workload/foo/c"},
		{"app", "Deployment", "foo", "p", "", "app/Deployment/foo"},
		{"app", "Deployment", "foo", "p", "  ", "app/Deployment/foo"},
		{"app", "", "", "pod-0", "", "app/Pod/pod-0"},
		{"app", "", "", "pod-0", "main", "app/Pod/pod-0/main"},
	}
	for _, tc := range cases {
		got := grainKey(tc.ns, tc.kind, tc.name, tc.pod, tc.container)
		if got != tc.want {
			t.Fatalf("grainKey(%q,%q,%q,%q,%q)=%q want %q",
				tc.ns, tc.kind, tc.name, tc.pod, tc.container, got, tc.want)
		}
	}
}
