package config

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Load reads and validates kpm configuration from YAML at path. An empty path
// falls back to DefaultConfigPath.
func Load(path string) (*Config, error) {
	cfgPath := path
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = DefaultConfigPath
	}

	v := viper.New()
	v.SetConfigFile(cfgPath)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("KPM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	bindConfigEnv(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %q: %w", cfgPath, err)
	}

	var raw rawConfig
	if err := v.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	cfg := &Config{
		Cluster:       raw.Cluster,
		Kube:          raw.Kube,
		Client:        raw.Client,
		Store:         raw.Store,
		Sample:        raw.Sample,
		Scope:         raw.Scope,
		Recommend:     raw.Recommend,
		TUI:           raw.TUI,
		Notifications: raw.Notifications,
		MCP:           raw.MCP,
		Server:        raw.Server,
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

type rawConfig struct {
	Cluster       Cluster       `mapstructure:"cluster"`
	Kube          Kube          `mapstructure:"kube"`
	Client        Client        `mapstructure:"client"`
	Store         Store         `mapstructure:"store"`
	Sample        Sample        `mapstructure:"sample"`
	Scope         Scope         `mapstructure:"scope"`
	Recommend     Recommend     `mapstructure:"recommend"`
	TUI           TUI           `mapstructure:"tui"`
	Notifications Notifications `mapstructure:"notifications"`
	MCP           MCP           `mapstructure:"mcp"`
	Server        Server        `mapstructure:"server"`
}

func bindConfigEnv(v *viper.Viper) {
	for _, key := range []string{
		"cluster.name",
		"cluster.environment",
		"cluster.description",
		"kube.context",
		"client.id",
		"store.path",
		"store.retention_days",
		"sample.interval_seconds",
		"mcp.enabled",
		"mcp.listen",
		"mcp.token_env",
		"server.enabled",
		"server.listen",
		"server.token_env",
	} {
		_ = v.BindEnv(key)
	}
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Store.Path) == "" {
		c.Store.Path = "~/.local/share/kpm/metrics.db"
	}
	if c.Store.RetentionDays == 0 {
		c.Store.RetentionDays = 14
	}
	// interval_seconds may be 0 (one-shot); leave as-is.
	if c.Recommend.WindowDays == 0 {
		c.Recommend.WindowDays = 7
	}
	if c.Recommend.Percentile == 0 {
		c.Recommend.Percentile = 95
	}
	if c.Recommend.MinSamples == 0 {
		c.Recommend.MinSamples = 24
	}
	if c.Recommend.TargetUtilization == 0 {
		c.Recommend.TargetUtilization = 0.70
	}
	setRatio := func(p *float64, def float64) {
		if *p == 0 {
			*p = def
		}
	}
	setRatio(&c.Recommend.CPUOversizeRatio, 0.35)
	setRatio(&c.Recommend.CPUUndersizeRatio, 0.85)
	setRatio(&c.Recommend.CPULimitOversizeRatio, 0.35)
	setRatio(&c.Recommend.CPULimitUndersizeRatio, 0.85)
	setRatio(&c.Recommend.MemoryOversizeRatio, 0.35)
	setRatio(&c.Recommend.MemoryUndersizeRatio, 0.85)
	setRatio(&c.Recommend.MemoryLimitOversizeRatio, 0.35)
	setRatio(&c.Recommend.MemoryLimitUndersizeRatio, 0.85)
	if c.TUI.RefreshSeconds == 0 {
		c.TUI.RefreshSeconds = 5
	}
	if strings.TrimSpace(c.TUI.DefaultSort) == "" {
		c.TUI.DefaultSort = "cpu.ratio"
	}
	if strings.TrimSpace(c.TUI.Baseline) == "" {
		c.TUI.Baseline = "request"
	}
	c.applyHTTPListenDefaults()
}

func (c *Config) applyHTTPListenDefaults() {
	if c.MCP.Enabled && strings.TrimSpace(c.MCP.Listen) == "" {
		c.MCP.Listen = "127.0.0.1:9740"
	}
	if c.Server.Enabled && strings.TrimSpace(c.Server.Listen) == "" {
		if c.MCP.Enabled && strings.TrimSpace(c.MCP.Listen) != "" {
			c.Server.Listen = c.MCP.Listen
		} else {
			c.Server.Listen = "127.0.0.1:9740"
		}
	}
}

// HTTPListen returns the shared daemon/serve bind address.
// Prefer server.listen when server.enabled; else mcp.listen; else default.
func (c *Config) HTTPListen() string {
	if c == nil {
		return "127.0.0.1:9740"
	}
	if c.Server.Enabled {
		if l := strings.TrimSpace(c.Server.Listen); l != "" {
			return l
		}
	}
	if c.MCP.Enabled {
		if l := strings.TrimSpace(c.MCP.Listen); l != "" {
			return l
		}
	}
	return "127.0.0.1:9740"
}

// ServerAuthRequired reports whether JSON API bearer is mandatory.
func (c *Config) ServerAuthRequired(listen string) bool {
	if c == nil {
		return false
	}
	if c.Server.Auth != nil && *c.Server.Auth {
		return true
	}
	return !listenIsLoopback(listen)
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Store.Path) == "" {
		return fmt.Errorf("store.path must be non-empty")
	}
	if c.Store.RetentionDays < 1 {
		return fmt.Errorf("store.retention_days must be >= 1 (got %d)", c.Store.RetentionDays)
	}
	// Retention shorter than the recommend window truncates the very history the
	// window measures, so percentiles come out noisier than the operator asked
	// for. SPECIFICATIONS.md §2.2 states the relation as guidance ("prefer
	// retention_days >= recommend.window_days"), not as a hard requirement, so
	// this warns instead of failing closed: a collector that refuses to run is
	// worse than one that runs with a noisy p95. Advisory only — no exit code.
	if c.Store.RetentionDays < c.Recommend.WindowDays {
		slog.Warn("store.retention_days is shorter than recommend.window_days; percentiles will be computed over truncated history",
			"retention_days", c.Store.RetentionDays,
			"window_days", c.Recommend.WindowDays)
	}
	if c.Sample.IntervalSeconds < 0 {
		return fmt.Errorf("sample.interval_seconds must be >= 0 (got %d)", c.Sample.IntervalSeconds)
	}
	if c.Recommend.Percentile < 1 || c.Recommend.Percentile > 100 {
		return fmt.Errorf("recommend.percentile must be in 1..100 (got %d)", c.Recommend.Percentile)
	}
	if c.Recommend.TargetUtilization <= 0 || c.Recommend.TargetUtilization > 1 {
		return fmt.Errorf("recommend.target_utilization must be in (0,1] (got %v)", c.Recommend.TargetUtilization)
	}
	switch strings.TrimSpace(c.TUI.Baseline) {
	case "request", "limit":
	default:
		return fmt.Errorf("tui.baseline must be request or limit (got %q)", c.TUI.Baseline)
	}
	if _, err := c.ExpandedStorePath(); err != nil {
		return err
	}
	if err := c.validateNotifications(); err != nil {
		return err
	}
	if err := c.validateMCP(); err != nil {
		return err
	}
	if err := c.validateServer(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateMCP() error {
	m := c.MCP
	if !m.Enabled {
		return nil
	}
	listen := strings.TrimSpace(m.Listen)
	if listen == "" {
		return fmt.Errorf("mcp.enabled requires listen (or default)")
	}
	if _, _, err := net.SplitHostPort(listen); err != nil {
		return fmt.Errorf("mcp.listen: %w", err)
	}
	if !listenIsLoopback(listen) {
		if strings.TrimSpace(m.TokenEnv) == "" {
			return fmt.Errorf("mcp.token_env required when mcp.listen is non-loopback")
		}
	}
	if err := validateMCPRateLimit(m.RateLimit); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateServer() error {
	s := c.Server
	if !s.Enabled {
		return nil
	}
	listen := strings.TrimSpace(s.Listen)
	if listen == "" {
		return fmt.Errorf("server.enabled requires listen (or default)")
	}
	if _, _, err := net.SplitHostPort(listen); err != nil {
		return fmt.Errorf("server.listen: %w", err)
	}
	needTok := c.ServerAuthRequired(listen)
	if needTok && strings.TrimSpace(s.TokenEnv) == "" && strings.TrimSpace(c.MCP.TokenEnv) == "" {
		return fmt.Errorf("server.token_env (or mcp.token_env) required when server auth is required")
	}
	return nil
}

func validateMCPRateLimit(rl MCPRateLimit) error {
	period := strings.TrimSpace(rl.Period)
	if period != "" {
		d, err := time.ParseDuration(period)
		if err != nil {
			return fmt.Errorf("mcp.rate_limit.period: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("mcp.rate_limit.period must be > 0")
		}
	}
	if rl.Requests < 0 {
		return fmt.Errorf("mcp.rate_limit.requests must be >= 0")
	}
	if rl.Burst < 0 {
		return fmt.Errorf("mcp.rate_limit.burst must be >= 0")
	}
	return nil
}

// MCPRateLimitEnabled reports whether per-IP rate limiting is on (default true).
func (m MCP) MCPRateLimitEnabled() bool {
	if m.RateLimit.Enabled != nil {
		return *m.RateLimit.Enabled
	}
	return true
}

// MCPRateLimitResolved returns concrete rate-limit knobs with gghstats defaults.
func (m MCP) MCPRateLimitResolved() (requests int, period time.Duration, burst int) {
	requests = m.RateLimit.Requests
	if requests == 0 {
		requests = 120
	}
	burst = m.RateLimit.Burst
	if burst == 0 {
		burst = 20
	}
	period = time.Minute
	if p := strings.TrimSpace(m.RateLimit.Period); p != "" {
		if d, err := time.ParseDuration(p); err == nil && d > 0 {
			period = d
		}
	}
	return requests, period, burst
}

func listenIsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Config) validateNotifications() error {
	n := c.Notifications
	if !n.Enabled {
		return nil
	}
	if len(n.Sinks) == 0 {
		return fmt.Errorf("notifications.enabled requires at least one sinks[] entry")
	}
	for i, s := range n.Sinks {
		if err := validateNotifySink(i, s); err != nil {
			return err
		}
	}
	return validateNotifyRules(n.Rules)
}

func validateNotifySink(i int, s NotifySink) error {
	switch strings.ToLower(strings.TrimSpace(s.Type)) {
	case "slack":
		if strings.TrimSpace(s.WebhookURLEnv) == "" {
			return fmt.Errorf("notifications.sinks[%d]: slack requires webhook_url_env", i)
		}
	case "webhook":
		return validateWebhookSink(i, s)
	case "smtp":
		return validateSMTPSink(i, s)
	default:
		return fmt.Errorf("notifications.sinks[%d]: unknown type %q (want slack, webhook, smtp)", i, s.Type)
	}
	return nil
}

func validateWebhookSink(i int, s NotifySink) error {
	if strings.TrimSpace(s.URLEnv) == "" {
		return fmt.Errorf("notifications.sinks[%d]: webhook requires url_env", i)
	}
	body := strings.ToLower(strings.TrimSpace(s.Body))
	if body == "" {
		body = "generic"
	}
	switch body {
	case "generic", "discord", "teams":
		return nil
	default:
		return fmt.Errorf("notifications.sinks[%d]: webhook.body must be generic, discord, or teams (got %q)", i, s.Body)
	}
}

func validateSMTPSink(i int, s NotifySink) error {
	for _, pair := range []struct {
		name, val string
	}{
		{"host_env", s.HostEnv},
		{"port_env", s.PortEnv},
		{"from_env", s.FromEnv},
		{"to_env", s.ToEnv},
	} {
		if strings.TrimSpace(pair.val) == "" {
			return fmt.Errorf("notifications.sinks[%d]: smtp requires %s", i, pair.name)
		}
	}
	return nil
}

func validateNotifyRules(rules []NotifyRule) error {
	for i, r := range rules {
		if strings.TrimSpace(r.Name) == "" {
			return fmt.Errorf("notifications.rules[%d]: name must be non-empty", i)
		}
		if strings.TrimSpace(r.Metric) == "" {
			return fmt.Errorf("notifications.rules[%d]: metric must be non-empty", i)
		}
		if err := validateNotifyMetric(r); err != nil {
			return fmt.Errorf("notifications.rules[%d]: %w", i, err)
		}
	}
	return nil
}

func validateNotifyMetric(r NotifyRule) error {
	m := strings.TrimSpace(r.Metric)
	switch m {
	case "cpu.util_vs_request", "mem.util_vs_request",
		"cpu.util_vs_limit", "mem.util_vs_limit":
		if r.ThresholdRatio <= 0 {
			return fmt.Errorf("threshold_ratio must be > 0 for metric %s", m)
		}
	case "cpu.util_mcores", "mem.util_bytes":
		if r.Threshold <= 0 {
			return fmt.Errorf("threshold must be > 0 for metric %s", m)
		}
	default:
		return fmt.Errorf("unsupported metric %q", m)
	}
	return nil
}
