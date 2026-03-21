// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"time"

	rx "github.com/kryovyx/rextension"
)

// Config controls the health extension behavior.
type Config struct {
	// LivePath is the HTTP path for liveness checks. Default: "/live".
	LivePath string `default:"/live"`
	// ReadyPath is the HTTP path for readiness checks. Default: "/ready".
	ReadyPath string `default:"/ready"`
	// StatusPath is the HTTP path for full status. Default: "/status".
	StatusPath string `default:"/status"`
	// AtDefaultAddr serves health endpoints on the default router when true.
	AtDefaultAddr bool `default:"false"`
	// Router config for the dedicated health router when AtDefaultAddr is false.
	Router rx.RouterConfig
	// SnapshotTTL is the cache duration for health snapshots. Default: 5s.
	SnapshotTTL time.Duration `default:"5s"`
	// CheckInterval is how often registered health checks are executed. Default: 10s.
	// Set to 0 to disable automatic checking (checks only run on-demand).
	CheckInterval time.Duration `default:"10s"`
	// StateStoreConfig configures the dependency state store.
	StateStoreConfig DepStateStoreConfig
	// EnableDependencyGate enables the dependency gate middleware.
	EnableDependencyGate bool `default:"true"`
}

// NewDefaultConfig returns the default configuration for the health extension.
func NewDefaultConfig() *Config {
	return &Config{
		LivePath:             "/live",
		ReadyPath:            "/ready",
		StatusPath:           "/status",
		AtDefaultAddr:        false,
		Router:               rx.RouterConfig{Addr: ":9091", BaseURL: "/", SSLVerify: true, ListenSSL: true},
		SnapshotTTL:          5 * time.Second,
		CheckInterval:        10 * time.Second,
		StateStoreConfig:     DefaultDepStateStoreConfig(),
		EnableDependencyGate: true,
	}
}

// ConfigOption allows functional configuration.
type ConfigOption func(*Config)

// WithLivePath sets the live endpoint path.
func WithLivePath(path string) ConfigOption {
	return func(c *Config) {
		c.LivePath = path
	}
}

// WithReadyPath sets the ready endpoint path.
func WithReadyPath(path string) ConfigOption {
	return func(c *Config) {
		c.ReadyPath = path
	}
}

// WithStatusPath sets the status endpoint path.
func WithStatusPath(path string) ConfigOption {
	return func(c *Config) {
		c.StatusPath = path
	}
}

// WithAtDefaultAddr configures whether to use the default router.
func WithAtDefaultAddr(atDefault bool) ConfigOption {
	return func(c *Config) {
		c.AtDefaultAddr = atDefault
	}
}

// WithHealthRouter sets the dedicated health router config.
func WithHealthRouter(cfg rx.RouterConfig) ConfigOption {
	return func(c *Config) {
		c.Router = cfg
	}
}

// WithSnapshotTTL sets the snapshot cache TTL.
func WithSnapshotTTL(ttl time.Duration) ConfigOption {
	return func(c *Config) {
		c.SnapshotTTL = ttl
	}
}

// WithCheckInterval sets how often health checks are executed automatically.
// Set to 0 to disable automatic checking.
func WithCheckInterval(interval time.Duration) ConfigOption {
	return func(c *Config) {
		c.CheckInterval = interval
	}
}

// WithStateStoreConfig sets the state store configuration.
func WithStateStoreConfig(cfg DepStateStoreConfig) ConfigOption {
	return func(c *Config) {
		c.StateStoreConfig = cfg
	}
}

// WithDependencyGate enables or disables the dependency gate middleware.
func WithDependencyGate(enable bool) ConfigOption {
	return func(c *Config) {
		c.EnableDependencyGate = enable
	}
}

// NewConfig creates a config with the given options.
func NewConfig(opts ...ConfigOption) *Config {
	c := NewDefaultConfig()
	for _, opt := range opts {
		opt(c)
	}
	return c
}
