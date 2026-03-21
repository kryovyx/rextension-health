// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"time"

	"github.com/kryovyx/dix"
	"github.com/kryovyx/rextension"
)

// Registry defines the interface for managing health checks.
type Registry interface {
	// Register adds a health check to the registry.
	Register(check HealthCheck)
	// Unregister removes a health check by name.
	Unregister(name string)
	// Get returns a health check by name.
	Get(name string) HealthCheck
	// GetAll returns all registered health checks.
	GetAll() []HealthCheck
	// GetByTags returns checks matching any of the given tags.
	GetByTags(tags ...string) []HealthCheck
	// GetReadinessChecks returns all checks affecting readiness.
	GetReadinessChecks() []HealthCheck
	// GetActiveChecks returns all checks with mode Active.
	GetActiveChecks() []HealthCheck
	// GetPassiveChecks returns all checks with mode Passive.
	GetPassiveChecks() []HealthCheck
	// ExecuteAll executes all checks and returns their results.
	ExecuteAll(ctx context.Context) map[string]*CheckResult
	// ExecuteReadiness executes only readiness checks.
	ExecuteReadiness(ctx context.Context) map[string]*CheckResult
	// ExecuteByTags executes checks matching any of the given tags.
	ExecuteByTags(ctx context.Context, tags ...string) map[string]*CheckResult
	// ExecuteCheck executes a single check by name and returns the result.
	ExecuteCheck(ctx context.Context, name string) *CheckResult
	// Start begins automatic periodic health check execution for Active checks.
	// Results are reported to the provided DepStateStore.
	Start(interval time.Duration, stateStore DepStateStore)
	// Stop halts automatic health check execution.
	Stop()
	// SetResolver sets the DI resolver for all health checks to access dependencies.
	SetResolver(resolver dix.Resolver)
	// SetLogger sets the logger used for internal trace/debug logs.
	SetLogger(l rextension.Logger)
}
