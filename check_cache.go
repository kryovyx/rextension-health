// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"sync"
	"time"

	"github.com/kryovyx/rextension"
)

// CheckCache caches health check results for passive checks.
// On-demand checks skip caching (CacheTTL is forced to zero).
// Each check result is cached for its configured CacheTTL when applicable.
type CheckCache interface {
	// GetOrExecute returns a cached result if valid, otherwise executes the check.
	// For passive checks, this runs the check on-demand and caches the result.
	// For active checks, this just executes without caching (they use DepStateStore).
	GetOrExecute(ctx context.Context, check HealthCheck) *CheckResult
	// Invalidate removes a cached result for the given check name.
	Invalidate(name string)
	// Clear removes all cached results.
	Clear()
}

// cachedResult holds a check result with expiration.
type cachedResult struct {
	result    *CheckResult
	expiresAt time.Time
}

// checkCache is the default implementation of CheckCache.
type checkCache struct {
	cache      map[string]*cachedResult
	mu         sync.RWMutex
	stateStore DepStateStore
	logger     rextension.Logger
}

// NewCheckCache creates a new check cache.
// The stateStore is used to update dependency state after passive check execution.
func NewCheckCache(stateStore DepStateStore, l rextension.Logger) CheckCache {
	return &checkCache{
		cache:      make(map[string]*cachedResult),
		stateStore: stateStore,
		logger:     l,
	}
}

// GetOrExecute returns a cached result if valid, otherwise executes the check.
func (c *checkCache) GetOrExecute(ctx context.Context, check HealthCheck) *CheckResult {
	name := check.Name()

	// For active checks, just execute - they don't use this cache
	if check.Mode() == CheckModeActive {
		if c.logger != nil {
			c.logger.Trace("health: executing active check %s (no cache)", name)
		}
		return check.Execute(ctx)
	}

	// For passive/on-demand checks, check cache first
	c.mu.RLock()
	if cached, ok := c.cache[name]; ok && time.Now().Before(cached.expiresAt) {
		if c.logger != nil {
			c.logger.Trace("health: cache hit for check %s (status=%s)", name, cached.result.Status)
		}
		c.mu.RUnlock()
		return cached.result
	}
	c.mu.RUnlock()

	// Cache miss or expired - execute the check
	if c.logger != nil {
		c.logger.Trace("health: executing on-demand check %s (mode=%s)", name, check.Mode())
	}
	result := check.Execute(ctx)

	// Cache the result with TTL (may be zero for OnDemand)
	ttl := check.CacheTTL()
	if ttl > 0 {
		c.mu.Lock()
		c.cache[name] = &cachedResult{
			result:    result,
			expiresAt: time.Now().Add(ttl),
		}
		c.mu.Unlock()
	}

	// Update state store so it appears in /status
	if c.stateStore != nil {
		if result.Status == StatusUp {
			c.stateStore.ReportSuccess(name, result.Duration)
		} else {
			c.stateStore.ReportFailure(name, result.Message)
		}
		c.stateStore.SetStatus(name, result.Status, result.Message)
	}

	if c.logger != nil {
		c.logger.Trace("health: check %s executed (status=%s, ttl=%s)", name, result.Status, ttl)
	}

	return result
}

// Invalidate removes a cached result for the given check name.
func (c *checkCache) Invalidate(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, name)
}

// Clear removes all cached results.
func (c *checkCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*cachedResult)
}
