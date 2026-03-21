// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"time"

	"github.com/kryovyx/dix"
)

// CheckMode determines when a health check is executed.
type CheckMode int

const (
	// CheckModeActive runs the check periodically at CheckInterval (concurrent polling).
	// Use for critical dependencies that must be monitored continuously.
	CheckModeActive CheckMode = iota
	// CheckModePassive runs the check on-demand when a route needs it (lazy evaluation).
	// Results are cached for CacheTTL. Use for optional deps or expensive checks.
	CheckModePassive
	// CheckModeOnDemand is like Passive but never caches (CacheTTL forced to zero).
	// Each gate triggers a fresh execution.
	CheckModeOnDemand
)

// String returns the string representation of the check mode.
func (m CheckMode) String() string {
	switch m {
	case CheckModeActive:
		return "ACTIVE"
	case CheckModePassive:
		return "PASSIVE"
	case CheckModeOnDemand:
		return "ON_DEMAND"
	default:
		return "UNKNOWN"
	}
}

// CheckResult represents the result of a health check execution.
type CheckResult struct {
	Status    Status            `json:"status"`
	Message   string            `json:"message,omitempty"`
	Duration  time.Duration     `json:"duration"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NewCheckResult creates a new check result.
func NewCheckResult(status Status, message string, duration time.Duration) *CheckResult {
	return &CheckResult{
		Status:    status,
		Message:   message,
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

// HealthCheck defines the interface for health check implementations.
type HealthCheck interface {
	// Name returns the unique name of this check.
	Name() string
	// Execute performs the health check and returns the result.
	Execute(ctx context.Context) *CheckResult
	// Timeout returns the maximum duration for this check.
	Timeout() time.Duration
	// IsReadiness returns true if this check affects readiness.
	IsReadiness() bool
	// Tags returns the tags associated with this check.
	Tags() []string
	// Mode returns the check mode (Active, Passive, or OnDemand).
	Mode() CheckMode
	// CacheTTL returns how long to cache results (for Passive checks).
	CacheTTL() time.Duration
}

// CheckFunc is a function type that implements a health check.
// The resolver provides access to dependencies registered in the DI container.
type CheckFunc func(ctx context.Context, resolver dix.Resolver) *CheckResult

// checkImpl is the default HealthCheck implementation.
type checkImpl struct {
	name      string
	fn        CheckFunc
	resolver  dix.Resolver
	timeout   time.Duration
	readiness bool
	tags      []string
	mode      CheckMode
	cacheTTL  time.Duration
}

// CheckOption configures a health check.
type CheckOption func(*checkImpl)

// WithTimeout sets the timeout for a health check.
func WithTimeout(d time.Duration) CheckOption {
	return func(c *checkImpl) {
		c.timeout = d
	}
}

// WithReadiness marks the check as affecting readiness.
func WithReadiness(readiness bool) CheckOption {
	return func(c *checkImpl) {
		c.readiness = readiness
	}
}

// WithTags adds tags to the health check.
func WithTags(tags ...string) CheckOption {
	return func(c *checkImpl) {
		c.tags = append(c.tags, tags...)
	}
}

// WithCheckMode sets the check mode (Active, Passive, or OnDemand).
// Active checks run periodically at CheckInterval.
// Passive checks run on-demand when gating and cache results for CacheTTL.
// OnDemand checks run on-demand with caching disabled (CacheTTL forced to zero).
func WithCheckMode(mode CheckMode) CheckOption {
	return func(c *checkImpl) {
		c.mode = mode
	}
}

// WithCacheTTL sets how long to cache results for Passive checks.
// Default is 30 seconds. Ignored when mode is OnDemand (always zero).
func WithCacheTTL(ttl time.Duration) CheckOption {
	return func(c *checkImpl) {
		c.cacheTTL = ttl
	}
}

// NewCheck creates a new health check with the given name and function.
func NewCheck(name string, fn CheckFunc, opts ...CheckOption) HealthCheck {
	c := &checkImpl{
		name:      name,
		fn:        fn,
		timeout:   5 * time.Second,
		readiness: true,
		tags:      make([]string, 0),
		mode:      CheckModeActive,  // Default to active
		cacheTTL:  30 * time.Second, // Default cache TTL for passive
	}
	for _, opt := range opts {
		opt(c)
	}
	// Force on-demand checks to never cache regardless of option order
	if c.mode == CheckModeOnDemand {
		c.cacheTTL = 0
	}
	return c
}

func (c *checkImpl) Name() string            { return c.name }
func (c *checkImpl) Timeout() time.Duration  { return c.timeout }
func (c *checkImpl) IsReadiness() bool       { return c.readiness }
func (c *checkImpl) Tags() []string          { return c.tags }
func (c *checkImpl) Mode() CheckMode         { return c.mode }
func (c *checkImpl) CacheTTL() time.Duration { return c.cacheTTL }

// SetResolver sets the resolver for this health check.
func (c *checkImpl) SetResolver(resolver dix.Resolver) {
	c.resolver = resolver
}

func (c *checkImpl) Execute(ctx context.Context) *CheckResult {
	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()
	resultCh := make(chan *CheckResult, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- NewCheckResult(StatusDown, "panic during check", time.Since(start))
			}
		}()
		resultCh <- c.fn(ctx, c.resolver)
	}()

	select {
	case result := <-resultCh:
		result.Duration = time.Since(start)
		result.Timestamp = time.Now()
		return result
	case <-ctx.Done():
		return NewCheckResult(StatusDown, "check timed out", time.Since(start))
	}
}

// DependencyCheck creates a check that monitors a dependency from the state store.
type DependencyCheck struct {
	depID     string
	store     DepStateStore
	name      string
	readiness bool
	tags      []string
}

// NewDependencyCheck creates a check that reflects dependency state.
func NewDependencyCheck(name, depID string, store DepStateStore, readiness bool, tags ...string) HealthCheck {
	return &DependencyCheck{
		name:      name,
		depID:     depID,
		store:     store,
		readiness: readiness,
		tags:      tags,
	}
}

func (d *DependencyCheck) Name() string            { return d.name }
func (d *DependencyCheck) Timeout() time.Duration  { return time.Second }
func (d *DependencyCheck) IsReadiness() bool       { return d.readiness }
func (d *DependencyCheck) Tags() []string          { return d.tags }
func (d *DependencyCheck) Mode() CheckMode         { return CheckModeActive }
func (d *DependencyCheck) CacheTTL() time.Duration { return 0 }

func (d *DependencyCheck) Execute(ctx context.Context) *CheckResult {
	state := d.store.Get(d.depID)
	if state == nil {
		return NewCheckResult(StatusUp, "no data yet", 0)
	}
	result := NewCheckResult(state.Status, state.Message, state.LastLatency)
	result.Metadata = state.Metadata
	return result
}
