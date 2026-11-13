package httpserver

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"testing"
)

func TestListenTCPDisablesMultipathTCP(t *testing.T) {
	var lc net.ListenConfig
	lc.SetMultipathTCP(false)
	if lc.MultipathTCP() {
		t.Fatal("ListenConfig.MultipathTCP() true after SetMultipathTCP(false)")
	}

	ln, err := listenTCP(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listenTCP: %v", err)
	}
	defer ln.Close()
	if ln.Addr() == nil {
		t.Fatal("nil listen addr")
	}
}

func TestModuleDefaultGODEBUGDisablesMultipathTCP(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", "{{.DefaultGODEBUG}}", "./cmd/kpm")
	cmd.Dir = "../.."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if !strings.Contains(got, "multipathtcp=0") {
		t.Fatalf("DefaultGODEBUG %q missing multipathtcp=0 (go.mod godebug)", got)
	}
}
