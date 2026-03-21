// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"sync"
	"time"
)

// Snapshot holds a point-in-time view of all health check results.
type Snapshot struct {
	Timestamp     time.Time               `json:"timestamp"`
	OverallStatus Status                  `json:"overall_status"`
	Checks        map[string]*CheckResult `json:"checks"`
	Dependencies  map[string]*DepState    `json:"dependencies,omitempty"`
}

// NewSnapshot creates a new health snapshot.
func NewSnapshot() *Snapshot {
	return &Snapshot{
		Timestamp:     time.Now(),
		OverallStatus: StatusUp,
		Checks:        make(map[string]*CheckResult),
		Dependencies:  make(map[string]*DepState),
	}
}

// ComputeOverallStatus computes the overall status from all check results.
func (s *Snapshot) ComputeOverallStatus() {
	worst := StatusUp
	for _, result := range s.Checks {
		if result.Status > worst {
			worst = result.Status
		}
	}
	for _, dep := range s.Dependencies {
		if dep.Status > worst {
			worst = dep.Status
		}
	}
	s.OverallStatus = worst
}

// SnapshotCache provides cached health snapshots with TTL.
type SnapshotCache interface {
	// Get returns the current cached snapshot, refreshing if stale.
	Get(ctx context.Context) *Snapshot
	// GetReadiness returns a snapshot for readiness checks only.
	GetReadiness(ctx context.Context) *Snapshot
	// Invalidate clears the cache, forcing a refresh on next Get.
	Invalidate()
	// SetTTL sets the cache TTL.
	SetTTL(ttl time.Duration)
}

// snapshotCache is the default implementation of SnapshotCache.
type snapshotCache struct {
	registry   Registry
	stateStore DepStateStore
	ttl        time.Duration

	snapshot          *Snapshot
	readinessSnapshot *Snapshot
	lastRefresh       time.Time
	lastReadiness     time.Time
	mu                sync.RWMutex
}

// NewSnapshotCache creates a new snapshot cache.
func NewSnapshotCache(registry Registry, stateStore DepStateStore, ttl time.Duration) SnapshotCache {
	if ttl == 0 {
		ttl = 5 * time.Second
	}
	return &snapshotCache{
		registry:   registry,
		stateStore: stateStore,
		ttl:        ttl,
	}
}

// Get returns the current cached snapshot, refreshing if stale.
func (c *snapshotCache) Get(ctx context.Context) *Snapshot {
	c.mu.RLock()
	if c.snapshot != nil && time.Since(c.lastRefresh) < c.ttl {
		snap := c.snapshot
		c.mu.RUnlock()
		return snap
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.snapshot != nil && time.Since(c.lastRefresh) < c.ttl {
		return c.snapshot
	}

	c.snapshot = c.buildSnapshot(ctx, false)
	c.lastRefresh = time.Now()
	return c.snapshot
}

// GetReadiness returns a snapshot for readiness checks only.
func (c *snapshotCache) GetReadiness(ctx context.Context) *Snapshot {
	c.mu.RLock()
	if c.readinessSnapshot != nil && time.Since(c.lastReadiness) < c.ttl {
		snap := c.readinessSnapshot
		c.mu.RUnlock()
		return snap
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.readinessSnapshot != nil && time.Since(c.lastReadiness) < c.ttl {
		return c.readinessSnapshot
	}

	c.readinessSnapshot = c.buildSnapshot(ctx, true)
	c.lastReadiness = time.Now()
	return c.readinessSnapshot
}

// buildSnapshot creates a new snapshot from current state.
func (c *snapshotCache) buildSnapshot(ctx context.Context, readinessOnly bool) *Snapshot {
	snap := NewSnapshot()

	// Execute checks
	var results map[string]*CheckResult
	if readinessOnly {
		results = c.registry.ExecuteReadiness(ctx)
	} else {
		results = c.registry.ExecuteAll(ctx)
	}
	snap.Checks = results

	// Add dependency states
	if c.stateStore != nil {
		snap.Dependencies = c.stateStore.GetAll()
	}

	snap.ComputeOverallStatus()
	return snap
}

// Invalidate clears the cache, forcing a refresh on next Get.
func (c *snapshotCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot = nil
	c.readinessSnapshot = nil
	c.lastRefresh = time.Time{}
	c.lastReadiness = time.Time{}
}

// SetTTL sets the cache TTL.
func (c *snapshotCache) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = ttl
}
