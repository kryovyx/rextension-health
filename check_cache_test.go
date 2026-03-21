// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health contains tests for the check cache.
package health

import (
	"context"
	"testing"
	"time"
)

// mockHealthCheck implements HealthCheck for testing.
type mockHealthCheck struct {
	name       string
	mode       CheckMode
	cacheTTL   time.Duration
	execCount  int
	resultFunc func() *CheckResult
}

func (m *mockHealthCheck) Name() string              { return m.name }
func (m *mockHealthCheck) Timeout() time.Duration    { return time.Second }
func (m *mockHealthCheck) IsReadiness() bool         { return false }
func (m *mockHealthCheck) Tags() []string            { return nil }
func (m *mockHealthCheck) Mode() CheckMode           { return m.mode }
func (m *mockHealthCheck) CacheTTL() time.Duration   { return m.cacheTTL }
func (m *mockHealthCheck) SetResolver(r interface{}) {}
func (m *mockHealthCheck) Execute(ctx context.Context) *CheckResult {
	m.execCount++
	if m.resultFunc != nil {
		return m.resultFunc()
	}
	return &CheckResult{Status: StatusUp, Message: "ok"}
}

// --------------------------------------------------------------------------
// NewCheckCache tests
// --------------------------------------------------------------------------

func TestNewCheckCache(t *testing.T) {
	t.Run("creates_empty_cache", func(t *testing.T) {
		// Creates_empty_cache should return a usable cache instance.
		cache := NewCheckCache(nil, nil)
		if cache == nil {
			t.Fatal("expected non-nil cache")
		}
	})
}

// --------------------------------------------------------------------------
// checkCache.GetOrExecute tests
// --------------------------------------------------------------------------

func TestCheckCache_GetOrExecute(t *testing.T) {
	t.Run("executes_active_check_without_caching", func(t *testing.T) {
		// Executes_active_check_without_caching should always run check for active mode.
		cache := NewCheckCache(nil, nil)
		check := &mockHealthCheck{
			name: "active",
			mode: CheckModeActive,
		}

		// Execute twice
		cache.GetOrExecute(context.Background(), check)
		cache.GetOrExecute(context.Background(), check)

		if check.execCount != 2 {
			t.Fatalf("expected 2 executions, got %d", check.execCount)
		}
	})

	t.Run("caches_passive_check_result", func(t *testing.T) {
		// Caches_passive_check_result should return cached result for passive checks.
		cache := NewCheckCache(nil, nil)
		check := &mockHealthCheck{
			name:     "passive",
			mode:     CheckModePassive,
			cacheTTL: 1 * time.Hour,
		}

		// First call executes
		cache.GetOrExecute(context.Background(), check)
		// Second call should use cache
		cache.GetOrExecute(context.Background(), check)

		if check.execCount != 1 {
			t.Fatalf("expected 1 execution (cached), got %d", check.execCount)
		}
	})

	t.Run("re_executes_on_cache_expiry", func(t *testing.T) {
		// Re_executes_on_cache_expiry should execute again when TTL expires.
		cache := NewCheckCache(nil, nil)
		check := &mockHealthCheck{
			name:     "short-ttl",
			mode:     CheckModePassive,
			cacheTTL: 1 * time.Millisecond,
		}

		cache.GetOrExecute(context.Background(), check)
		time.Sleep(5 * time.Millisecond)
		cache.GetOrExecute(context.Background(), check)

		if check.execCount != 2 {
			t.Fatalf("expected 2 executions after TTL expiry, got %d", check.execCount)
		}
	})

	t.Run("does_not_cache_with_zero_ttl", func(t *testing.T) {
		// Does_not_cache_with_zero_ttl should not store result when TTL is 0.
		cache := NewCheckCache(nil, nil)
		check := &mockHealthCheck{
			name:     "no-cache",
			mode:     CheckModeOnDemand,
			cacheTTL: 0,
		}

		cache.GetOrExecute(context.Background(), check)
		cache.GetOrExecute(context.Background(), check)

		if check.execCount != 2 {
			t.Fatalf("expected 2 executions (no cache), got %d", check.execCount)
		}
	})

	t.Run("updates_state_store_on_success", func(t *testing.T) {
		// Updates_state_store_on_success should report success to state store.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("test-dep")
		cache := NewCheckCache(store, nil)
		check := &mockHealthCheck{
			name:     "test-dep",
			mode:     CheckModePassive,
			cacheTTL: time.Hour,
			resultFunc: func() *CheckResult {
				return &CheckResult{Status: StatusUp, Message: "healthy", Duration: time.Millisecond}
			},
		}

		cache.GetOrExecute(context.Background(), check)

		state := store.Get("test-dep")
		if state == nil || state.Status != StatusUp {
			t.Fatal("expected state store to be updated with Up status")
		}
	})

	t.Run("updates_state_store_on_failure", func(t *testing.T) {
		// Updates_state_store_on_failure should report failure to state store.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("fail-dep")
		cache := NewCheckCache(store, nil)
		check := &mockHealthCheck{
			name:     "fail-dep",
			mode:     CheckModePassive,
			cacheTTL: time.Hour,
			resultFunc: func() *CheckResult {
				return &CheckResult{Status: StatusDown, Message: "failed"}
			},
		}

		cache.GetOrExecute(context.Background(), check)

		state := store.Get("fail-dep")
		if state == nil || state.Status != StatusDown {
			t.Fatal("expected state store to be updated with Down status")
		}
	})

	t.Run("logs_when_logger_present", func(t *testing.T) {
		// Logs_when_logger_present should use logger for trace messages.
		cache := NewCheckCache(nil, &mockLoggerImpl{})

		// Test active check path
		activeCheck := &mockHealthCheck{
			name: "active-logged",
			mode: CheckModeActive,
		}
		cache.GetOrExecute(context.Background(), activeCheck)

		// Test passive check with cache hit
		passiveCheck := &mockHealthCheck{
			name:     "passive-logged",
			mode:     CheckModePassive,
			cacheTTL: time.Hour,
		}
		cache.GetOrExecute(context.Background(), passiveCheck) // Cache miss
		cache.GetOrExecute(context.Background(), passiveCheck) // Cache hit

		// No panic means success - logger was invoked
	})
}

// --------------------------------------------------------------------------
// checkCache.Invalidate tests
// --------------------------------------------------------------------------

func TestCheckCache_Invalidate(t *testing.T) {
	t.Run("removes_cached_result", func(t *testing.T) {
		// Removes_cached_result should clear specific cache entry.
		cache := NewCheckCache(nil, nil)
		check := &mockHealthCheck{
			name:     "to-invalidate",
			mode:     CheckModePassive,
			cacheTTL: time.Hour,
		}

		cache.GetOrExecute(context.Background(), check)
		cache.Invalidate("to-invalidate")
		cache.GetOrExecute(context.Background(), check)

		if check.execCount != 2 {
			t.Fatalf("expected 2 executions after invalidate, got %d", check.execCount)
		}
	})
}

// --------------------------------------------------------------------------
// checkCache.Clear tests
// --------------------------------------------------------------------------

func TestCheckCache_Clear(t *testing.T) {
	t.Run("removes_all_cached_results", func(t *testing.T) {
		// Removes_all_cached_results should empty the entire cache.
		cache := NewCheckCache(nil, nil)
		check1 := &mockHealthCheck{name: "c1", mode: CheckModePassive, cacheTTL: time.Hour}
		check2 := &mockHealthCheck{name: "c2", mode: CheckModePassive, cacheTTL: time.Hour}

		cache.GetOrExecute(context.Background(), check1)
		cache.GetOrExecute(context.Background(), check2)
		cache.Clear()
		cache.GetOrExecute(context.Background(), check1)
		cache.GetOrExecute(context.Background(), check2)

		if check1.execCount != 2 || check2.execCount != 2 {
			t.Fatalf("expected 2 executions each after clear, got %d and %d", check1.execCount, check2.execCount)
		}
	})
}
