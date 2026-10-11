package cluster

import (
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func writeKubeconfig(t *testing.T) string {
	t.Helper()
	cfg := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"test": {Server: "https://127.0.0.1:6443"},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"ctx-a": {Cluster: "test", AuthInfo: "u"},
			"ctx-b": {Cluster: "test", AuthInfo: "u"},
		},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{"u": {Token: "t"}},
		CurrentContext: "ctx-a",
	}
	kubeconfig := filepath.Join(t.TempDir(), "config")
	if err := clientcmd.WriteToFile(cfg, kubeconfig); err != nil {
		t.Fatal(err)
	}
	return kubeconfig
}

func TestLoadRESTConfigExplicitPath(t *testing.T) {
	kc := writeKubeconfig(t)
	restCfg, err := LoadRESTConfig(kc, "")
	if err != nil {
		t.Fatalf("LoadRESTConfig: %v", err)
	}
	if restCfg.Host == "" {
		t.Fatal("expected non-empty host")
	}
}

func TestLoadRESTConfigSelectsContext(t *testing.T) {
	kc := writeKubeconfig(t)
	if _, err := LoadRESTConfig(kc, "ctx-b"); err != nil {
		t.Fatalf("LoadRESTConfig(ctx-b): %v", err)
	}
}

func TestLoadRESTConfigMissingPath(t *testing.T) {
	if _, err := LoadRESTConfig("/nonexistent/kubeconfig", ""); err == nil {
		t.Fatal("expected error for missing kubeconfig")
	}
}

func TestLoadRESTConfigFromKubeconfigEnv(t *testing.T) {
	kc := writeKubeconfig(t)
	t.Setenv("KUBECONFIG", kc)
	restCfg, err := LoadRESTConfig("", "")
	if err != nil {
		t.Fatalf("LoadRESTConfig(env): %v", err)
	}
	if restCfg.Host == "" {
		t.Fatal("expected non-empty host from KUBECONFIG env")
	}
}

func TestNewClientsets(t *testing.T) {
	kc := writeKubeconfig(t)
	restCfg, err := LoadRESTConfig(kc, "")
	if err != nil {
		t.Fatal(err)
	}
	cs, err := NewClientset(restCfg)
	if err != nil {
		t.Fatalf("NewClientset: %v", err)
	}
	if cs == nil {
		t.Fatal("expected non-nil clientset")
	}
	mcs, err := NewMetricsClientset(restCfg)
	if err != nil {
		t.Fatalf("NewMetricsClientset: %v", err)
	}
	if mcs == nil {
		t.Fatal("expected non-nil metrics clientset")
	}
}
