// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health contains tests for snapshot and snapshot cache.
package health

import (
	"context"
	"testing"
	"time"
)

// --------------------------------------------------------------------------
// NewSnapshot tests
// --------------------------------------------------------------------------

func TestNewSnapshot(t *testing.T) {
	t.Run("initializes_with_defaults", func(t *testing.T) {
		// Initializes_with_defaults should create empty maps and Up status.
		snap := NewSnapshot()

		if snap.OverallStatus != StatusUp {
			t.Fatalf("expected StatusUp, got %v", snap.OverallStatus)
		}
		if snap.Checks == nil {
			t.Fatal("expected non-nil Checks map")
		}
		if snap.Dependencies == nil {
			t.Fatal("expected non-nil Dependencies map")
		}
		if snap.Timestamp.IsZero() {
			t.Fatal("expected non-zero Timestamp")
		}
	})
}

// --------------------------------------------------------------------------
// Snapshot.ComputeOverallStatus tests
// --------------------------------------------------------------------------

func TestSnapshot_ComputeOverallStatus(t *testing.T) {
	t.Run("returns_up_when_all_healthy", func(t *testing.T) {
		// Returns_up_when_all_healthy should compute Up when no issues.
		snap := NewSnapshot()
		snap.Checks["c1"] = &CheckResult{Status: StatusUp}
		snap.Checks["c2"] = &CheckResult{Status: StatusUp}

		snap.ComputeOverallStatus()

		if snap.OverallStatus != StatusUp {
			t.Fatalf("expected StatusUp, got %v", snap.OverallStatus)
		}
	})

	t.Run("returns_worst_check_status", func(t *testing.T) {
		// Returns_worst_check_status should reflect the most severe status.
		snap := NewSnapshot()
		snap.Checks["c1"] = &CheckResult{Status: StatusUp}
		snap.Checks["c2"] = &CheckResult{Status: StatusDown}

		snap.ComputeOverallStatus()

		if snap.OverallStatus != StatusDown {
			t.Fatalf("expected StatusDown, got %v", snap.OverallStatus)
		}
	})

	t.Run("includes_dependency_status", func(t *testing.T) {
		// Includes_dependency_status should consider dependencies in overall status.
		snap := NewSnapshot()
		snap.Checks["c1"] = &CheckResult{Status: StatusUp}
		snap.Dependencies["d1"] = &DepState{Status: StatusDegraded}

		snap.ComputeOverallStatus()

		if snap.OverallStatus != StatusDegraded {
			t.Fatalf("expected StatusDegraded, got %v", snap.OverallStatus)
		}
	})
}

// --------------------------------------------------------------------------
// NewSnapshotCache tests
// --------------------------------------------------------------------------

func TestNewSnapshotCache(t *testing.T) {
	t.Run("creates_cache_with_ttl", func(t *testing.T) {
		// Creates_cache_with_ttl should set provided TTL.
		reg := NewRegistry()
		cache := NewSnapshotCache(reg, nil, 10*time.Second)

		if cache == nil {
			t.Fatal("expected non-nil cache")
		}
	})

	t.Run("uses_default_ttl_when_zero", func(t *testing.T) {
		// Uses_default_ttl_when_zero should default to 5 seconds.
		reg := NewRegistry()
		cache := NewSnapshotCache(reg, nil, 0)

		if cache == nil {
			t.Fatal("expected non-nil cache")
		}
	})
}

// --------------------------------------------------------------------------
// snapshotCache.Get tests
// --------------------------------------------------------------------------

func TestSnapshotCache_Get(t *testing.T) {
	t.Run("returns_cached_snapshot", func(t *testing.T) {
		// Returns_cached_snapshot should return same instance within TTL.
		reg := NewRegistry()
		cache := NewSnapshotCache(reg, nil, time.Hour)

		snap1 := cache.Get(context.Background())
		snap2 := cache.Get(context.Background())

		if snap1 != snap2 {
			t.Fatal("expected same snapshot instance from cache")
		}
	})

	t.Run("refreshes_after_ttl", func(t *testing.T) {
		// Refreshes_after_ttl should create new snapshot when expired.
		reg := NewRegistry()
		cache := NewSnapshotCache(reg, nil, 1*time.Millisecond)

		snap1 := cache.Get(context.Background())
		time.Sleep(5 * time.Millisecond)
		snap2 := cache.Get(context.Background())

		if snap1 == snap2 {
			t.Fatal("expected new snapshot after TTL")
		}
	})

	t.Run("includes_dependency_states", func(t *testing.T) {
		// Includes_dependency_states should include state store data.
		reg := NewRegistry()
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("my-dep")
		store.SetStatus("my-dep", StatusUp, "ok")

		cache := NewSnapshotCache(reg, store, time.Hour)
		snap := cache.Get(context.Background())

		if snap.Dependencies["my-dep"] == nil {
			t.Fatal("expected dependency in snapshot")
		}
	})
}

// --------------------------------------------------------------------------
// snapshotCache.GetReadiness tests
// --------------------------------------------------------------------------

func TestSnapshotCache_GetReadiness(t *testing.T) {
	t.Run("returns_readiness_snapshot", func(t *testing.T) {
		// Returns_readiness_snapshot should cache readiness checks separately.
		reg := NewRegistry()
		cache := NewSnapshotCache(reg, nil, time.Hour)

		snap1 := cache.GetReadiness(context.Background())
		snap2 := cache.GetReadiness(context.Background())

		if snap1 != snap2 {
			t.Fatal("expected same readiness snapshot from cache")
		}
	})

	t.Run("refreshes_readiness_after_ttl", func(t *testing.T) {
		// Refreshes_readiness_after_ttl should create new snapshot when expired.
		reg := NewRegistry()
		cache := NewSnapshotCache(reg, nil, 1*time.Millisecond)

		snap1 := cache.GetReadiness(context.Background())
		time.Sleep(5 * time.Millisecond)
		snap2 := cache.GetReadiness(context.Background())

		if snap1 == snap2 {
			t.Fatal("expected new readiness snapshot after TTL")
		}
	})
}

// --------------------------------------------------------------------------
// snapshotCache.Invalidate tests
// --------------------------------------------------------------------------

func TestSnapshotCache_Invalidate(t *testing.T) {
	t.Run("clears_cached_snapshots", func(t *testing.T) {
		// Clears_cached_snapshots should force refresh on next Get.
		reg := NewRegistry()
		cache := NewSnapshotCache(reg, nil, time.Hour)

		snap1 := cache.Get(context.Background())
		cache.Invalidate()
		snap2 := cache.Get(context.Background())

		if snap1 == snap2 {
			t.Fatal("expected new snapshot after invalidate")
		}
	})
}

// --------------------------------------------------------------------------
// snapshotCache.SetTTL tests
// --------------------------------------------------------------------------

func TestSnapshotCache_SetTTL(t *testing.T) {
	t.Run("updates_ttl", func(t *testing.T) {
		// Updates_ttl should change cache duration.
		reg := NewRegistry()
		cache := NewSnapshotCache(reg, nil, time.Hour)

		cache.SetTTL(1 * time.Millisecond)
		snap1 := cache.Get(context.Background())
		time.Sleep(5 * time.Millisecond)
		snap2 := cache.Get(context.Background())

		if snap1 == snap2 {
			t.Fatal("expected refresh with new short TTL")
		}
	})
}
