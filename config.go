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
	LivePath string
	// ReadyPath is the HTTP path for readiness checks. Default: "/ready".
	ReadyPath string
	// StatusPath is the HTTP path for full status. Default: "/status".
	StatusPath string
	// AtDefaultAddr serves health endpoints on the default router when true.
	AtDefaultAddr bool
	// Router config for the dedicated health router when AtDefaultAddr is false.
	//
	// TLS is opt-in: ListenSSL defaults to false, and the router serves TLS only
	// once a certificate source is configured - CertFile/KeyFile, or TLSConfig for
	// per-handshake selection so the certificate can be replaced without a restart.
	Router rx.RouterConfig
	// SnapshotTTL is the cache duration for health snapshots. Default: 5s.
	SnapshotTTL time.Duration
	// CheckInterval is how often registered health checks are executed. Default: 10s.
	// Set to 0 to disable automatic checking (checks only run on-demand).
	CheckInterval time.Duration
	// StateStoreConfig configures the dependency state store.
	StateStoreConfig DepStateStoreConfig
	// EnableDependencyGate enables the dependency gate middleware.
	EnableDependencyGate bool

	// TreatUnknownAs is the status an unknown dependency is gated as.
	//
	// Defaults to StatusUp — the gate **serves** when it does not yet know
	// (O14). The reasoning differs deliberately from the security extension's
	// fail-closed stance (D28), and the difference is the point:
	//
	//   - Authorization failing open grants access nobody granted. There is no
	//     acceptable version of that.
	//   - Availability-gating failing closed 503s every request for the first
	//     check interval after every boot and every deploy — because a check
	//     has not run yet, not because anything is wrong. A rolling deploy
	//     then fails its own readiness probes and rolls back.
	//
	// What makes serving safe rather than merely convenient is the
	// synchronous check pass in OnStart: every registered check runs once,
	// before the listeners bind, so by the time a request can arrive the
	// states are real. "Unknown" then means a check that has genuinely never
	// produced a result, which is worth a warning rather than a refusal.
	//
	// Set it to StatusDown for an application that must refuse rather than
	// guess.
	//
	// This replaces an implicit dependence on StatusUnknown's ordinal
	// position: the gate compared `state.Status > dep.MinStatus`, and Unknown
	// happened to sort after Down, so unknown fell through to "refuse". That
	// was not a decision, it was the order the constants were declared in —
	// and reordering them would have silently inverted the behaviour.
	TreatUnknownAs Status

	// Checks are the health checks to register at startup.
	//
	// Declaring them here is the supported way to wire checks, because the
	// registry no longer exists before the extension's OnInitialize runs
	// (D13/O2). Reaching into the container for it beforehand — which is what
	// applications used to do — now resolves nothing.
	//
	// Use WithCheck or WithChecks rather than setting this directly.
	Checks []HealthCheck
}

// NewDefaultConfig returns the default configuration for the health extension.
func NewDefaultConfig() *Config {
	return &Config{
		LivePath:             "/live",
		ReadyPath:            "/ready",
		StatusPath:           "/status",
		AtDefaultAddr:        false,
		Router:               rx.RouterConfig{Addr: ":9091", BaseURL: "/", ListenSSL: false, CertFile: nil, KeyFile: nil, TLSConfig: nil},
		SnapshotTTL:          5 * time.Second,
		CheckInterval:        10 * time.Second,
		StateStoreConfig:     DefaultDepStateStoreConfig(),
		EnableDependencyGate: true,
		TreatUnknownAs:       StatusUp,
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

// WithTreatUnknownAs sets the status an unknown dependency is gated as.
// See Config.TreatUnknownAs for why the default serves rather than refuses.
func WithTreatUnknownAs(status Status) ConfigOption {
	return func(c *Config) { c.TreatUnknownAs = status }
}

// WithCheck declares a health check to register at startup.
//
// This exists because OnInitialize now runs from Run rather than at
// registration time (D13), so an application can no longer resolve the
// registry from the container and call Register before Run — the registry does
// not exist yet. Declaring the check hands it to the extension, which registers
// it once it has built the registry.
//
// For wiring that needs more than a check — a route that depends on the state
// store, say — implement a small application Extension and do it in its
// OnInitialize (O2).
func WithCheck(check HealthCheck) ConfigOption {
	return func(c *Config) {
		if check != nil {
			c.Checks = append(c.Checks, check)
		}
	}
}

// WithChecks declares several health checks at once.
func WithChecks(checks ...HealthCheck) ConfigOption {
	return func(c *Config) {
		for _, check := range checks {
			if check != nil {
				c.Checks = append(c.Checks, check)
			}
		}
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
