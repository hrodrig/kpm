package config

import (
	"fmt"
	"strings"

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
		Cluster:   raw.Cluster,
		Kube:      raw.Kube,
		Client:    raw.Client,
		Store:     raw.Store,
		Sample:    raw.Sample,
		Scope:     raw.Scope,
		Recommend: raw.Recommend,
		TUI:       raw.TUI,
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

type rawConfig struct {
	Cluster   Cluster   `mapstructure:"cluster"`
	Kube      Kube      `mapstructure:"kube"`
	Client    Client    `mapstructure:"client"`
	Store     Store     `mapstructure:"store"`
	Sample    Sample    `mapstructure:"sample"`
	Scope     Scope     `mapstructure:"scope"`
	Recommend Recommend `mapstructure:"recommend"`
	TUI       TUI       `mapstructure:"tui"`
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
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Store.Path) == "" {
		return fmt.Errorf("store.path must be non-empty")
	}
	if c.Store.RetentionDays < 1 {
		return fmt.Errorf("store.retention_days must be >= 1 (got %d)", c.Store.RetentionDays)
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
	return nil
}
