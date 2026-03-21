// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"sync"
	"time"

	"github.com/kryovyx/dix"
	"github.com/kryovyx/rextension"
)

// registry is the default implementation of Registry.
type registry struct {
	checks   map[string]HealthCheck
	mu       sync.RWMutex
	stopChan chan struct{}
	running  bool
	resolver dix.Resolver
	logger   rextension.Logger
}

// NewRegistry creates a new health check registry.
func NewRegistry() Registry {
	return &registry{
		checks: make(map[string]HealthCheck),
	}
}

// Register adds a health check to the registry.
func (r *registry) Register(check HealthCheck) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Set resolver on the check if it supports it
	if setter, ok := check.(interface{ SetResolver(dix.Resolver) }); ok && r.resolver != nil {
		setter.SetResolver(r.resolver)
	}
	r.checks[check.Name()] = check
}

// Unregister removes a health check by name.
func (r *registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.checks, name)
}

// Get returns a health check by name.
func (r *registry) Get(name string) HealthCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.checks[name]
}

// GetAll returns all registered health checks.
func (r *registry) GetAll() []HealthCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]HealthCheck, 0, len(r.checks))
	for _, check := range r.checks {
		result = append(result, check)
	}
	return result
}

// GetByTags returns checks matching any of the given tags.
func (r *registry) GetByTags(tags ...string) []HealthCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}
	var result []HealthCheck
	for _, check := range r.checks {
		for _, t := range check.Tags() {
			if tagSet[t] {
				result = append(result, check)
				break
			}
		}
	}
	return result
}

// GetReadinessChecks returns all checks affecting readiness.
func (r *registry) GetReadinessChecks() []HealthCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []HealthCheck
	for _, check := range r.checks {
		if check.IsReadiness() {
			result = append(result, check)
		}
	}
	return result
}

// GetActiveChecks returns all checks with mode Active.
func (r *registry) GetActiveChecks() []HealthCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []HealthCheck
	for _, check := range r.checks {
		if check.Mode() == CheckModeActive {
			result = append(result, check)
		}
	}
	return result
}

// GetPassiveChecks returns all checks with mode Passive.
func (r *registry) GetPassiveChecks() []HealthCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []HealthCheck
	for _, check := range r.checks {
		if check.Mode() == CheckModePassive {
			result = append(result, check)
		}
	}
	return result
}

// executeChecks runs the given checks concurrently.
func executeChecks(ctx context.Context, checks []HealthCheck) map[string]*CheckResult {
	results := make(map[string]*CheckResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, check := range checks {
		wg.Add(1)
		go func(c HealthCheck) {
			defer wg.Done()
			result := c.Execute(ctx)
			mu.Lock()
			results[c.Name()] = result
			mu.Unlock()
		}(check)
	}
	wg.Wait()
	return results
}

// ExecuteAll executes all checks and returns their results.
func (r *registry) ExecuteAll(ctx context.Context) map[string]*CheckResult {
	return executeChecks(ctx, r.GetAll())
}

// ExecuteReadiness executes only readiness checks.
func (r *registry) ExecuteReadiness(ctx context.Context) map[string]*CheckResult {
	return executeChecks(ctx, r.GetReadinessChecks())
}

// ExecuteByTags executes checks matching any of the given tags.
func (r *registry) ExecuteByTags(ctx context.Context, tags ...string) map[string]*CheckResult {
	return executeChecks(ctx, r.GetByTags(tags...))
}

// ExecuteCheck executes a single check by name and returns the result.
func (r *registry) ExecuteCheck(ctx context.Context, name string) *CheckResult {
	check := r.Get(name)
	if check == nil {
		return NewCheckResult(StatusUnknown, "check not found", 0)
	}
	return check.Execute(ctx)
}

// Start begins automatic periodic health check execution for Active checks only.
// Passive checks are not run by the ticker - they run on-demand during gating.
// Results are automatically reported to the provided DepStateStore.
func (r *registry) Start(interval time.Duration, stateStore DepStateStore) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.stopChan = make(chan struct{})
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Trace("health: starting active check ticker interval=%s", interval)
	}

	go r.runTicker(interval, stateStore)
}

// Stop halts automatic health check execution.
func (r *registry) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return
	}
	r.running = false
	close(r.stopChan)
	if r.logger != nil {
		r.logger.Trace("health: stopped active check ticker")
	}
}

// runTicker executes health checks periodically and reports to state store.
func (r *registry) runTicker(interval time.Duration, stateStore DepStateStore) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	r.executeAndReport(stateStore)

	for {
		select {
		case <-ticker.C:
			r.executeAndReport(stateStore)
		case <-r.stopChan:
			return
		}
	}
}

// executeAndReport runs Active checks only and reports results to the state store.
func (r *registry) executeAndReport(stateStore DepStateStore) {
	ctx := context.Background()
	// Only execute Active checks in the ticker
	checks := r.GetActiveChecks()
	if r.logger != nil {
		r.logger.Trace("health: executing %d active checks", len(checks))
	}
	results := executeChecks(ctx, checks)

	for name, result := range results {
		if result.Status == StatusUp {
			stateStore.ReportSuccess(name, result.Duration)
		} else {
			stateStore.ReportFailure(name, result.Message)
		}
		// Also set status directly for immediate reflection
		stateStore.SetStatus(name, result.Status, result.Message)
		if r.logger != nil {
			r.logger.Trace("health: active check %s status=%s duration=%s", name, result.Status, result.Duration)
		}
	}
}

// SetResolver sets the DI resolver for all health checks.
// Must be called before registering checks, or existing checks won't have the resolver.
func (r *registry) SetResolver(resolver dix.Resolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolver = resolver
	// Update existing checks
	for _, check := range r.checks {
		if setter, ok := check.(interface{ SetResolver(dix.Resolver) }); ok {
			setter.SetResolver(resolver)
		}
	}
}

// SetLogger sets the logger for internal trace/debug logs.
func (r *registry) SetLogger(l rextension.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = l
}
