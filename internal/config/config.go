// Package config loads and validates the kpm YAML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultConfigPath is the default config file path (kwd-style relative).
const DefaultConfigPath = "kpm.yaml"

// Config is the root kpm configuration (core keys for band 0.2.x).
type Config struct {
	Cluster       Cluster       `yaml:"cluster" mapstructure:"cluster"`
	Kube          Kube          `yaml:"kube" mapstructure:"kube"`
	Client        Client        `yaml:"client" mapstructure:"client"`
	Store         Store         `yaml:"store" mapstructure:"store"`
	Sample        Sample        `yaml:"sample" mapstructure:"sample"`
	Scope         Scope         `yaml:"scope" mapstructure:"scope"`
	Recommend     Recommend     `yaml:"recommend" mapstructure:"recommend"`
	TUI           TUI           `yaml:"tui" mapstructure:"tui"`
	Notifications Notifications `yaml:"notifications" mapstructure:"notifications"`
	MCP           MCP           `yaml:"mcp" mapstructure:"mcp"`
	Server        Server        `yaml:"server" mapstructure:"server"`
}

// Server is optional JSON API (+ later charts) on the shared HTTP mux (SPEC §8).
type Server struct {
	Enabled  bool   `yaml:"enabled" mapstructure:"enabled"`
	Listen   string `yaml:"listen" mapstructure:"listen"`
	TokenEnv string `yaml:"token_env" mapstructure:"token_env"`
	// Auth nil → require bearer off-loopback only; true → always require token_env.
	Auth *bool `yaml:"auth" mapstructure:"auth"`
}

// MCP is optional read-only MCP + daemon HTTP ops (SPEC §7).
type MCP struct {
	Enabled        bool         `yaml:"enabled" mapstructure:"enabled"`
	Listen         string       `yaml:"listen" mapstructure:"listen"`
	TokenEnv       string       `yaml:"token_env" mapstructure:"token_env"`
	TrustedProxies []string     `yaml:"trusted_proxies" mapstructure:"trusted_proxies"`
	AllowlistCIDRs []string     `yaml:"allowlist_cidrs" mapstructure:"allowlist_cidrs"`
	RateLimit      MCPRateLimit `yaml:"rate_limit" mapstructure:"rate_limit"`
}

// MCPRateLimit is per-IP token-bucket limiting on the daemon HTTP mux (gghstats mold).
// Enabled nil → default true when mcp.enabled; ops paths are always exempt.
type MCPRateLimit struct {
	Enabled  *bool  `yaml:"enabled" mapstructure:"enabled"`
	Requests int    `yaml:"requests" mapstructure:"requests"`
	Period   string `yaml:"period" mapstructure:"period"` // Go duration, e.g. "1m"
	Burst    int    `yaml:"burst" mapstructure:"burst"`
}

// Notifications is optional daemon threshold alerting (SPEC §6).
type Notifications struct {
	Enabled bool         `yaml:"enabled" mapstructure:"enabled"`
	Rules   []NotifyRule `yaml:"rules" mapstructure:"rules"`
	Sinks   []NotifySink `yaml:"sinks" mapstructure:"sinks"`
}

// NotifyRule is one sustained-threshold rule.
type NotifyRule struct {
	Name              string   `yaml:"name" mapstructure:"name"`
	Namespaces        []string `yaml:"namespaces" mapstructure:"namespaces"`
	Metric            string   `yaml:"metric" mapstructure:"metric"`
	ThresholdRatio    float64  `yaml:"threshold_ratio" mapstructure:"threshold_ratio"`
	Threshold         float64  `yaml:"threshold" mapstructure:"threshold"`
	HoldForSeconds    int      `yaml:"hold_for_seconds" mapstructure:"hold_for_seconds"`
	ClearAfterSeconds int      `yaml:"clear_after_seconds" mapstructure:"clear_after_seconds"`
	// RepeatSeconds nil → default 3600; pointer 0 → no repeat (SPEC §6.2).
	RepeatSeconds *int `yaml:"repeat_seconds" mapstructure:"repeat_seconds"`
	// NotifyOnResolve nil means default true (SPEC §6.2).
	NotifyOnResolve *bool `yaml:"notify_on_resolve" mapstructure:"notify_on_resolve"`
}

// NotifySink is one fan-out destination (gghstats-school).
type NotifySink struct {
	Type          string            `yaml:"type" mapstructure:"type"`
	WebhookURLEnv string            `yaml:"webhook_url_env" mapstructure:"webhook_url_env"`
	URLEnv        string            `yaml:"url_env" mapstructure:"url_env"`
	Body          string            `yaml:"body" mapstructure:"body"`
	HeadersEnv    map[string]string `yaml:"headers_env" mapstructure:"headers_env"`
	HostEnv       string            `yaml:"host_env" mapstructure:"host_env"`
	PortEnv       string            `yaml:"port_env" mapstructure:"port_env"`
	UserEnv       string            `yaml:"user_env" mapstructure:"user_env"`
	PasswordEnv   string            `yaml:"password_env" mapstructure:"password_env"`
	FromEnv       string            `yaml:"from_env" mapstructure:"from_env"`
	ToEnv         string            `yaml:"to_env" mapstructure:"to_env"`
	UseTLS        bool              `yaml:"use_tls" mapstructure:"use_tls"`
}

// Client is the sampling identity ("who samples"). ID empty resolves via the
// cascade in internal/identity (KPM_CLIENT_ID → client.id → hostname → …).
type Client struct {
	ID string `yaml:"id" mapstructure:"id"`
}

// Cluster is cluster metadata ("what is being sampled").
type Cluster struct {
	Name        string `yaml:"name" mapstructure:"name"`
	Environment string `yaml:"environment" mapstructure:"environment"`
	Description string `yaml:"description" mapstructure:"description"`
}

// Kube selects the Kubernetes target by kubeconfig context.
type Kube struct {
	// Context is the kubeconfig context; empty means current-context.
	Context string `yaml:"context" mapstructure:"context"`
}

// Store holds the local SQLite path and retention.
type Store struct {
	Path          string `yaml:"path" mapstructure:"path"`
	RetentionDays int    `yaml:"retention_days" mapstructure:"retention_days"`
}

// Sample controls the capture loop.
type Sample struct {
	// IntervalSeconds is 0 for one-shot, >0 for daemon sample loop.
	IntervalSeconds int `yaml:"interval_seconds" mapstructure:"interval_seconds"`
}

// Scope filters which pods are sampled / shown live.
type Scope struct {
	Namespaces      []string `yaml:"namespaces" mapstructure:"namespaces"`
	NamespaceLabels string   `yaml:"namespace_labels" mapstructure:"namespace_labels"`
	PodLabels       string   `yaml:"pod_labels" mapstructure:"pod_labels"`
	NodeLabels      string   `yaml:"node_labels" mapstructure:"node_labels"`
}

// Recommend holds offline rightsizing knobs (validated lightly in 0.2.x).
type Recommend struct {
	WindowDays                int     `yaml:"window_days" mapstructure:"window_days"`
	Percentile                int     `yaml:"percentile" mapstructure:"percentile"`
	MinSamples                int     `yaml:"min_samples" mapstructure:"min_samples"`
	TargetUtilization         float64 `yaml:"target_utilization" mapstructure:"target_utilization"`
	CPUOversizeRatio          float64 `yaml:"cpu_oversize_ratio" mapstructure:"cpu_oversize_ratio"`
	CPUUndersizeRatio         float64 `yaml:"cpu_undersize_ratio" mapstructure:"cpu_undersize_ratio"`
	CPULimitOversizeRatio     float64 `yaml:"cpu_limit_oversize_ratio" mapstructure:"cpu_limit_oversize_ratio"`
	CPULimitUndersizeRatio    float64 `yaml:"cpu_limit_undersize_ratio" mapstructure:"cpu_limit_undersize_ratio"`
	MemoryOversizeRatio       float64 `yaml:"memory_oversize_ratio" mapstructure:"memory_oversize_ratio"`
	MemoryUndersizeRatio      float64 `yaml:"memory_undersize_ratio" mapstructure:"memory_undersize_ratio"`
	MemoryLimitOversizeRatio  float64 `yaml:"memory_limit_oversize_ratio" mapstructure:"memory_limit_oversize_ratio"`
	MemoryLimitUndersizeRatio float64 `yaml:"memory_limit_undersize_ratio" mapstructure:"memory_limit_undersize_ratio"`
}

// TUI holds interactive heatmap defaults (unused until 0.3.x).
type TUI struct {
	RefreshSeconds int      `yaml:"refresh_seconds" mapstructure:"refresh_seconds"`
	DefaultSort    string   `yaml:"default_sort" mapstructure:"default_sort"`
	Baseline       string   `yaml:"baseline" mapstructure:"baseline"`
	Pods           []string `yaml:"pods" mapstructure:"pods"`
	PodLabels      string   `yaml:"pod_labels" mapstructure:"pod_labels"`
}

// ExpandedStorePath returns store.path with leading ~ expanded.
func (c *Config) ExpandedStorePath() (string, error) {
	return ExpandHome(c.Store.Path)
}

// ExpandHome expands a leading ~/ or ~ to the user home directory.
func ExpandHome(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("store.path must be non-empty")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand store.path: %w", err)
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}
