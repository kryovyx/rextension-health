// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"testing"
	"time"

	"github.com/kryovyx/dix"
)

// --------------------------------------------------------------------------
// CheckMode tests
// --------------------------------------------------------------------------

func TestCheckModeString(t *testing.T) {
	t.Run("returns_active_string", func(t *testing.T) {
		// Returns_active_string should return "ACTIVE" for CheckModeActive.
		if CheckModeActive.String() != "ACTIVE" {
			t.Errorf("expected ACTIVE, got %s", CheckModeActive.String())
		}
	})

	t.Run("returns_passive_string", func(t *testing.T) {
		// Returns_passive_string should return "PASSIVE" for CheckModePassive.
		if CheckModePassive.String() != "PASSIVE" {
			t.Errorf("expected PASSIVE, got %s", CheckModePassive.String())
		}
	})

	t.Run("returns_on_demand_string", func(t *testing.T) {
		// Returns_on_demand_string should return "ON_DEMAND" for CheckModeOnDemand.
		if CheckModeOnDemand.String() != "ON_DEMAND" {
			t.Errorf("expected ON_DEMAND, got %s", CheckModeOnDemand.String())
		}
	})

	t.Run("returns_unknown_for_invalid_mode", func(t *testing.T) {
		// Returns_unknown_for_invalid_mode should return "UNKNOWN" for invalid mode.
		invalidMode := CheckMode(99)
		if invalidMode.String() != "UNKNOWN" {
			t.Errorf("expected UNKNOWN, got %s", invalidMode.String())
		}
	})
}

// --------------------------------------------------------------------------
// NewCheck tests
// --------------------------------------------------------------------------

func TestNewCheck(t *testing.T) {
	t.Run("creates_check_with_defaults", func(t *testing.T) {
		// Creates_check_with_defaults should use default values.
		check := NewCheck("test", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		})

		if check.Name() != "test" {
			t.Errorf("expected name 'test', got %s", check.Name())
		}
		if check.Timeout() != 5*time.Second {
			t.Errorf("expected default timeout 5s, got %v", check.Timeout())
		}
		if !check.IsReadiness() {
			t.Error("expected default readiness true")
		}
		if check.Mode() != CheckModeActive {
			t.Errorf("expected default mode Active, got %v", check.Mode())
		}
		if check.CacheTTL() != 30*time.Second {
			t.Errorf("expected default cacheTTL 30s, got %v", check.CacheTTL())
		}
	})

	t.Run("creates_check_with_custom_timeout", func(t *testing.T) {
		// Creates_check_with_custom_timeout should override timeout.
		check := NewCheck("test",
			func(ctx context.Context, resolver dix.Resolver) *CheckResult {
				return NewCheckResult(StatusUp, "ok", 0)
			},
			WithTimeout(10*time.Second),
		)

		if check.Timeout() != 10*time.Second {
			t.Errorf("expected timeout 10s, got %v", check.Timeout())
		}
	})

	t.Run("creates_check_with_readiness_false", func(t *testing.T) {
		// Creates_check_with_readiness_false should disable readiness.
		check := NewCheck("test",
			func(ctx context.Context, resolver dix.Resolver) *CheckResult {
				return NewCheckResult(StatusUp, "ok", 0)
			},
			WithReadiness(false),
		)

		if check.IsReadiness() {
			t.Error("expected readiness false")
		}
	})

	t.Run("creates_check_with_tags", func(t *testing.T) {
		// Creates_check_with_tags should add tags.
		check := NewCheck("test",
			func(ctx context.Context, resolver dix.Resolver) *CheckResult {
				return NewCheckResult(StatusUp, "ok", 0)
			},
			WithTags("db", "critical"),
		)

		if len(check.Tags()) != 2 {
			t.Errorf("expected 2 tags, got %d", len(check.Tags()))
		}
	})

	t.Run("creates_check_with_passive_mode", func(t *testing.T) {
		// Creates_check_with_passive_mode should set mode to Passive.
		check := NewCheck("test",
			func(ctx context.Context, resolver dix.Resolver) *CheckResult {
				return NewCheckResult(StatusUp, "ok", 0)
			},
			WithCheckMode(CheckModePassive),
		)

		if check.Mode() != CheckModePassive {
			t.Errorf("expected mode Passive, got %v", check.Mode())
		}
	})

	t.Run("creates_check_with_cache_ttl", func(t *testing.T) {
		// Creates_check_with_cache_ttl should set cacheTTL.
		check := NewCheck("test",
			func(ctx context.Context, resolver dix.Resolver) *CheckResult {
				return NewCheckResult(StatusUp, "ok", 0)
			},
			WithCacheTTL(60*time.Second),
		)

		if check.CacheTTL() != 60*time.Second {
			t.Errorf("expected cacheTTL 60s, got %v", check.CacheTTL())
		}
	})

	t.Run("on_demand_mode_forces_zero_cache_ttl", func(t *testing.T) {
		// On_demand_mode_forces_zero_cache_ttl should ignore cacheTTL option.
		check := NewCheck("test",
			func(ctx context.Context, resolver dix.Resolver) *CheckResult {
				return NewCheckResult(StatusUp, "ok", 0)
			},
			WithCheckMode(CheckModeOnDemand),
			WithCacheTTL(60*time.Second), // Should be ignored
		)

		if check.CacheTTL() != 0 {
			t.Errorf("expected cacheTTL 0 for OnDemand mode, got %v", check.CacheTTL())
		}
	})
}

// --------------------------------------------------------------------------
// Check Execute tests
// --------------------------------------------------------------------------

func TestCheckExecute(t *testing.T) {
	t.Run("executes_and_returns_result", func(t *testing.T) {
		// Executes_and_returns_result should execute check function.
		check := NewCheck("test", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			time.Sleep(10 * time.Millisecond)
			return NewCheckResult(StatusUp, "healthy", 0)
		})

		result := check.Execute(context.Background())
		if result.Status != StatusUp {
			t.Errorf("expected StatusUp, got %v", result.Status)
		}
		if result.Duration < 10*time.Millisecond {
			t.Errorf("expected duration >= 10ms, got %v", result.Duration)
		}
	})

	t.Run("returns_down_on_timeout", func(t *testing.T) {
		// Returns_down_on_timeout should return StatusDown when check times out.
		check := NewCheck("slow",
			func(ctx context.Context, resolver dix.Resolver) *CheckResult {
				time.Sleep(200 * time.Millisecond)
				return NewCheckResult(StatusUp, "ok", 0)
			},
			WithTimeout(50*time.Millisecond),
		)

		result := check.Execute(context.Background())
		if result.Status != StatusDown {
			t.Errorf("expected StatusDown due to timeout, got %v", result.Status)
		}
		if result.Message != "check timed out" {
			t.Errorf("expected 'check timed out', got %s", result.Message)
		}
	})

	t.Run("recovers_from_panic", func(t *testing.T) {
		// Recovers_from_panic should return StatusDown after panic.
		check := NewCheck("panicky", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			panic("test panic")
		})

		result := check.Execute(context.Background())
		if result.Status != StatusDown {
			t.Errorf("expected StatusDown after panic, got %v", result.Status)
		}
	})
}

// --------------------------------------------------------------------------
// Registry tests
// --------------------------------------------------------------------------

func TestRegistry(t *testing.T) {
	t.Run("registers_and_retrieves_check", func(t *testing.T) {
		// Registers_and_retrieves_check should store and return check.
		registry := NewRegistry()

		check1 := NewCheck("db", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithTags("database"))

		registry.Register(check1)

		if registry.Get("db") == nil {
			t.Error("db check not found")
		}
	})

	t.Run("returns_all_checks", func(t *testing.T) {
		// Returns_all_checks should return all registered checks.
		registry := NewRegistry()

		check1 := NewCheck("db", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		})
		check2 := NewCheck("cache", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		})

		registry.Register(check1)
		registry.Register(check2)

		all := registry.GetAll()
		if len(all) != 2 {
			t.Errorf("expected 2 checks, got %d", len(all))
		}
	})

	t.Run("returns_readiness_checks_only", func(t *testing.T) {
		// Returns_readiness_checks_only should filter by readiness.
		registry := NewRegistry()

		check1 := NewCheck("db", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}) // Default readiness true

		check2 := NewCheck("optional", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithReadiness(false))

		registry.Register(check1)
		registry.Register(check2)

		readiness := registry.GetReadinessChecks()
		if len(readiness) != 1 {
			t.Errorf("expected 1 readiness check, got %d", len(readiness))
		}
	})

	t.Run("returns_checks_by_tags", func(t *testing.T) {
		// Returns_checks_by_tags should filter by tag.
		registry := NewRegistry()

		check1 := NewCheck("db", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithTags("database"))

		check2 := NewCheck("cache", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithTags("cache"))

		registry.Register(check1)
		registry.Register(check2)

		byTag := registry.GetByTags("database")
		if len(byTag) != 1 {
			t.Errorf("expected 1 check with tag 'database', got %d", len(byTag))
		}
	})

	t.Run("returns_active_checks_only", func(t *testing.T) {
		// Returns_active_checks_only should filter by Active mode.
		registry := NewRegistry()

		check1 := NewCheck("active-check", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModeActive))

		check2 := NewCheck("passive-check", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModePassive))

		registry.Register(check1)
		registry.Register(check2)

		activeChecks := registry.GetActiveChecks()
		if len(activeChecks) != 1 {
			t.Errorf("expected 1 active check, got %d", len(activeChecks))
		}
		if activeChecks[0].Name() != "active-check" {
			t.Errorf("expected active-check, got %s", activeChecks[0].Name())
		}
	})

	t.Run("returns_passive_checks_only", func(t *testing.T) {
		// Returns_passive_checks_only should filter by Passive mode.
		registry := NewRegistry()

		check1 := NewCheck("active-check", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModeActive))

		check2 := NewCheck("passive-check", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModePassive))

		registry.Register(check1)
		registry.Register(check2)

		passiveChecks := registry.GetPassiveChecks()
		if len(passiveChecks) != 1 {
			t.Errorf("expected 1 passive check, got %d", len(passiveChecks))
		}
		if passiveChecks[0].Name() != "passive-check" {
			t.Errorf("expected passive-check, got %s", passiveChecks[0].Name())
		}
	})
}

func TestRegistryUnregister(t *testing.T) {
	t.Run("removes_check", func(t *testing.T) {
		// Removes_check should remove check from registry.
		registry := NewRegistry()
		check := NewCheck("temp", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		})

		registry.Register(check)
		if registry.Get("temp") == nil {
			t.Error("check should exist after register")
		}

		registry.Unregister("temp")
		if registry.Get("temp") != nil {
			t.Error("check should not exist after unregister")
		}
	})
}

func TestRegistryExecuteAll(t *testing.T) {
	t.Run("executes_all_checks", func(t *testing.T) {
		// Executes_all_checks should run all checks concurrently.
		registry := NewRegistry()

		registry.Register(NewCheck("fast1", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}))
		registry.Register(NewCheck("fast2", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusDegraded, "slow", 0)
		}))

		results := registry.ExecuteAll(context.Background())
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
		if results["fast1"].Status != StatusUp {
			t.Errorf("expected StatusUp for fast1")
		}
		if results["fast2"].Status != StatusDegraded {
			t.Errorf("expected StatusDegraded for fast2")
		}
	})
}

func TestRegistryExecuteByTags(t *testing.T) {
	t.Run("executes_checks_matching_tags", func(t *testing.T) {
		// Executes_checks_matching_tags should run only matching checks.
		registry := NewRegistry()

		registry.Register(NewCheck("db", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithTags("database")))
		registry.Register(NewCheck("cache", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithTags("cache")))

		results := registry.ExecuteByTags(context.Background(), "database")
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
		if results["db"] == nil {
			t.Error("expected db result")
		}
	})
}

func TestRegistryExecuteCheck(t *testing.T) {
	t.Run("executes_single_check_by_name", func(t *testing.T) {
		// Executes_single_check_by_name should run specific check.
		registry := NewRegistry()

		registry.Register(NewCheck("target", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "healthy", 0)
		}))

		result := registry.ExecuteCheck(context.Background(), "target")
		if result.Status != StatusUp {
			t.Errorf("expected StatusUp, got %v", result.Status)
		}
	})

	t.Run("returns_unknown_for_missing_check", func(t *testing.T) {
		// Returns_unknown_for_missing_check should return StatusUnknown.
		registry := NewRegistry()

		result := registry.ExecuteCheck(context.Background(), "nonexistent")
		if result.Status != StatusUnknown {
			t.Errorf("expected StatusUnknown, got %v", result.Status)
		}
		if result.Message != "check not found" {
			t.Errorf("expected 'check not found', got %s", result.Message)
		}
	})
}

func TestRegistrySetResolver(t *testing.T) {
	t.Run("sets_resolver_on_new_and_existing_checks", func(t *testing.T) {
		// Sets_resolver_on_new_and_existing_checks should update all checks.
		registry := NewRegistry()

		// Register check before resolver
		check := NewCheck("test", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		})
		registry.Register(check)

		// Set resolver (should update existing checks)
		mockResolver := &mockTestResolver{}
		registry.SetResolver(mockResolver)

		// Verify resolver is set on the check
		// Note: We can't directly access the resolver, but we can verify no panic
		registry.Get("test").Execute(context.Background())
	})
}

func TestRegistrySetLogger(t *testing.T) {
	t.Run("sets_logger_for_registry", func(t *testing.T) {
		// Sets_logger_for_registry should not panic.
		registry := NewRegistry()
		registry.SetLogger(&mockLoggerImpl{})
		// No panic means success
	})
}

func TestRegistryStart(t *testing.T) {
	t.Run("starts_ticker_and_executes_checks", func(t *testing.T) {
		// Starts_ticker_and_executes_checks should run checks on start.
		registry := NewRegistry()
		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())

		check := NewCheck("db", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 10*time.Millisecond)
		}, WithCheckMode(CheckModeActive))
		registry.Register(check)

		registry.Start(100*time.Millisecond, stateStore)
		defer registry.Stop()

		// Wait for at least one tick
		time.Sleep(150 * time.Millisecond)

		// Check should have been executed
		state := stateStore.Get("db")
		if state == nil || state.Status != StatusUp {
			t.Error("expected check to be executed and status to be Up")
		}
	})

	t.Run("does_nothing_when_already_running", func(t *testing.T) {
		// Does_nothing_when_already_running should not start a second ticker.
		registry := NewRegistry()
		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())

		registry.Start(100*time.Millisecond, stateStore)
		defer registry.Stop()

		// Call Start again - should return early
		registry.Start(100*time.Millisecond, stateStore)
		// No panic or double-run means success
	})

	t.Run("reports_failure_on_check_failure", func(t *testing.T) {
		// Reports_failure_on_check_failure should call ReportFailure.
		registry := NewRegistry()
		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())

		check := NewCheck("failing-db", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusDown, "connection failed", 10*time.Millisecond)
		}, WithCheckMode(CheckModeActive))
		registry.Register(check)

		registry.Start(100*time.Millisecond, stateStore)
		defer registry.Stop()

		// Wait for at least one execution
		time.Sleep(50 * time.Millisecond)

		// Check should have been executed and reported failure
		state := stateStore.Get("failing-db")
		if state == nil || state.Status != StatusDown {
			t.Error("expected check failure to be reported")
		}
	})

	t.Run("logs_check_results_when_logger_set", func(t *testing.T) {
		// Logs_check_results_when_logger_set should trace log each check result.
		registry := NewRegistry()
		registry.SetLogger(&mockLoggerImpl{})
		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())

		check := NewCheck("logged-check", func(ctx context.Context, resolver dix.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 10*time.Millisecond)
		}, WithCheckMode(CheckModeActive))
		registry.Register(check)

		registry.Start(100*time.Millisecond, stateStore)
		defer registry.Stop()

		// Wait for at least one execution
		time.Sleep(50 * time.Millisecond)

		// No panic means logger was invoked successfully
	})
}

func TestRegistryStop(t *testing.T) {
	t.Run("stops_ticker_gracefully", func(t *testing.T) {
		// Stops_ticker_gracefully should close the stop channel.
		registry := NewRegistry()
		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())

		registry.Start(10*time.Millisecond, stateStore)
		registry.Stop()

		// Should complete without panic or deadlock
	})

	t.Run("does_nothing_when_not_running", func(t *testing.T) {
		// Does_nothing_when_not_running should return early.
		registry := NewRegistry()
		registry.Stop() // Should not panic
	})
}

// mockTestResolver implements dix.Resolver for testing.
type mockTestResolver struct{}

func (m *mockTestResolver) Resolve(target interface{}) error    { return nil }
func (m *mockTestResolver) ResolveAll(target interface{}) error { return nil }

// --------------------------------------------------------------------------
// DependencyCheck tests
// --------------------------------------------------------------------------

func TestDependencyCheck(t *testing.T) {
	t.Run("reflects_state_from_store", func(t *testing.T) {
		// Reflects_state_from_store should return state from DepStateStore.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.ReportSuccess("db", 50*time.Millisecond)

		check := NewDependencyCheck("db-check", "db", store, true)

		result := check.Execute(context.Background())
		if result.Status != StatusUp {
			t.Errorf("expected StatusUp, got %v", result.Status)
		}
	})

	t.Run("returns_name", func(t *testing.T) {
		// Returns_name should return the check name.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		check := NewDependencyCheck("my-check", "db", store, true)

		if check.Name() != "my-check" {
			t.Errorf("expected name 'my-check', got %s", check.Name())
		}
	})

	t.Run("returns_timeout", func(t *testing.T) {
		// Returns_timeout should return 1 second.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		check := NewDependencyCheck("check", "db", store, true)

		if check.Timeout() != time.Second {
			t.Errorf("expected timeout 1s, got %v", check.Timeout())
		}
	})

	t.Run("returns_readiness", func(t *testing.T) {
		// Returns_readiness should return the configured value.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		checkTrue := NewDependencyCheck("check", "db", store, true)
		checkFalse := NewDependencyCheck("check", "db", store, false)

		if !checkTrue.IsReadiness() {
			t.Error("expected readiness true")
		}
		if checkFalse.IsReadiness() {
			t.Error("expected readiness false")
		}
	})

	t.Run("returns_tags", func(t *testing.T) {
		// Returns_tags should return the configured tags.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		check := NewDependencyCheck("check", "db", store, true, "critical", "database")

		if len(check.Tags()) != 2 {
			t.Errorf("expected 2 tags, got %d", len(check.Tags()))
		}
	})

	t.Run("returns_active_mode", func(t *testing.T) {
		// Returns_active_mode should return CheckModeActive.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		check := NewDependencyCheck("check", "db", store, true)

		if check.Mode() != CheckModeActive {
			t.Errorf("expected CheckModeActive, got %v", check.Mode())
		}
	})

	t.Run("returns_zero_cache_ttl", func(t *testing.T) {
		// Returns_zero_cache_ttl should return 0.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		check := NewDependencyCheck("check", "db", store, true)

		if check.CacheTTL() != 0 {
			t.Errorf("expected CacheTTL 0, got %v", check.CacheTTL())
		}
	})

	t.Run("returns_up_when_no_data", func(t *testing.T) {
		// Returns_up_when_no_data should return StatusUp when state is nil.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		// Don't report anything for "unknown-dep"
		check := NewDependencyCheck("check", "unknown-dep", store, true)

		result := check.Execute(context.Background())
		if result.Status != StatusUp {
			t.Errorf("expected StatusUp for missing data, got %v", result.Status)
		}
		if result.Message != "no data yet" {
			t.Errorf("expected 'no data yet', got %s", result.Message)
		}
	})
}
