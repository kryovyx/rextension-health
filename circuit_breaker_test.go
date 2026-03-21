// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health contains tests for the circuit breaker implementation.
package health

import (
	"testing"
	"time"
)

// --------------------------------------------------------------------------
// DefaultCircuitBreakerConfig tests
// --------------------------------------------------------------------------

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	t.Run("returns_sensible_defaults", func(t *testing.T) {
		// Returns_sensible_defaults should set appropriate threshold values.
		cfg := DefaultCircuitBreakerConfig()

		if cfg.FailureThreshold != 5 {
			t.Fatalf("expected FailureThreshold=5, got %d", cfg.FailureThreshold)
		}
		if cfg.SuccessThreshold != 2 {
			t.Fatalf("expected SuccessThreshold=2, got %d", cfg.SuccessThreshold)
		}
		if cfg.Timeout != 30*time.Second {
			t.Fatalf("expected Timeout=30s, got %v", cfg.Timeout)
		}
		if cfg.HalfOpenMaxCalls != 1 {
			t.Fatalf("expected HalfOpenMaxCalls=1, got %d", cfg.HalfOpenMaxCalls)
		}
	})
}

// --------------------------------------------------------------------------
// NewCircuitBreaker tests
// --------------------------------------------------------------------------

func TestNewCircuitBreaker(t *testing.T) {
	t.Run("starts_in_closed_state", func(t *testing.T) {
		// Starts_in_closed_state should initialize circuit as closed.
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

		if cb.State() != CircuitClosed {
			t.Fatalf("expected CircuitClosed, got %v", cb.State())
		}
	})
}

// --------------------------------------------------------------------------
// NewCircuitBreakerWithStore tests
// --------------------------------------------------------------------------

func TestNewCircuitBreakerWithStore(t *testing.T) {
	t.Run("registers_dependency_in_store", func(t *testing.T) {
		// Registers_dependency_in_store should add dep to state store.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		cb := NewCircuitBreakerWithStore(DefaultCircuitBreakerConfig(), store, "my-service")

		if cb.State() != CircuitClosed {
			t.Fatalf("expected CircuitClosed, got %v", cb.State())
		}
		if store.Get("my-service") == nil {
			t.Fatal("expected dependency to be registered in store")
		}
	})

	t.Run("handles_nil_store", func(t *testing.T) {
		// Handles_nil_store should not panic when store is nil.
		cb := NewCircuitBreakerWithStore(DefaultCircuitBreakerConfig(), nil, "")
		if cb.State() != CircuitClosed {
			t.Fatalf("expected CircuitClosed, got %v", cb.State())
		}
	})
}

// --------------------------------------------------------------------------
// circuitBreaker.Allow tests
// --------------------------------------------------------------------------

func TestCircuitBreaker_Allow(t *testing.T) {
	t.Run("allows_when_closed", func(t *testing.T) {
		// Allows_when_closed should permit requests in closed state.
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

		if !cb.Allow() {
			t.Fatal("expected Allow=true when closed")
		}
	})

	t.Run("denies_when_open", func(t *testing.T) {
		// Denies_when_open should block requests when circuit is open.
		cfg := CircuitBreakerConfig{FailureThreshold: 2, Timeout: time.Hour}
		cb := NewCircuitBreaker(cfg)

		cb.Failure()
		cb.Failure()

		if cb.Allow() {
			t.Fatal("expected Allow=false when open")
		}
	})

	t.Run("allows_limited_calls_when_half_open", func(t *testing.T) {
		// Allows_limited_calls_when_half_open should allow up to HalfOpenMaxCalls.
		cfg := CircuitBreakerConfig{
			FailureThreshold: 1,
			Timeout:          1 * time.Millisecond,
			HalfOpenMaxCalls: 1,
		}
		cb := NewCircuitBreaker(cfg)

		cb.Failure()
		time.Sleep(5 * time.Millisecond) // Wait for transition to half-open

		if !cb.Allow() {
			t.Fatal("expected first call allowed in half-open")
		}
		if cb.Allow() {
			t.Fatal("expected second call denied in half-open")
		}
	})
}

// --------------------------------------------------------------------------
// circuitBreaker.Success tests
// --------------------------------------------------------------------------

func TestCircuitBreaker_Success(t *testing.T) {
	t.Run("closes_circuit_after_threshold", func(t *testing.T) {
		// Closes_circuit_after_threshold should transition from half-open to closed.
		cfg := CircuitBreakerConfig{
			FailureThreshold: 1,
			SuccessThreshold: 2,
			Timeout:          1 * time.Millisecond,
			HalfOpenMaxCalls: 5,
		}
		cb := NewCircuitBreaker(cfg)

		cb.Failure()
		time.Sleep(5 * time.Millisecond) // -> half-open

		cb.Allow()
		cb.Success()
		cb.Allow()
		cb.Success()

		if cb.State() != CircuitClosed {
			t.Fatalf("expected CircuitClosed after successes, got %v", cb.State())
		}
	})

	t.Run("resets_failure_count_when_closed", func(t *testing.T) {
		// Resets_failure_count_when_closed should clear failures on success.
		cfg := CircuitBreakerConfig{FailureThreshold: 3, Timeout: time.Hour}
		cb := NewCircuitBreaker(cfg)

		cb.Failure()
		cb.Failure()
		cb.Success()
		cb.Failure() // Should not open circuit (count was reset)

		if cb.State() != CircuitClosed {
			t.Fatal("expected circuit to remain closed after reset")
		}
	})
}

// --------------------------------------------------------------------------
// circuitBreaker.Failure tests
// --------------------------------------------------------------------------

func TestCircuitBreaker_Failure(t *testing.T) {
	t.Run("opens_circuit_after_threshold", func(t *testing.T) {
		// Opens_circuit_after_threshold should transition to open state.
		cfg := CircuitBreakerConfig{FailureThreshold: 2, Timeout: time.Hour}
		cb := NewCircuitBreaker(cfg)

		cb.Failure()
		if cb.State() != CircuitClosed {
			t.Fatal("expected still closed after 1 failure")
		}

		cb.Failure()
		if cb.State() != CircuitOpen {
			t.Fatalf("expected CircuitOpen, got %v", cb.State())
		}
	})

	t.Run("returns_to_open_from_half_open_on_failure", func(t *testing.T) {
		// Returns_to_open_from_half_open_on_failure should reopen circuit.
		cfg := CircuitBreakerConfig{
			FailureThreshold: 1,
			Timeout:          1 * time.Millisecond,
			HalfOpenMaxCalls: 1,
		}
		cb := NewCircuitBreaker(cfg)

		cb.Failure()
		time.Sleep(5 * time.Millisecond) // -> half-open
		cb.Allow()
		cb.Failure()

		if cb.State() != CircuitOpen {
			t.Fatalf("expected CircuitOpen after half-open failure, got %v", cb.State())
		}
	})
}

// --------------------------------------------------------------------------
// circuitBreaker.Reset tests
// --------------------------------------------------------------------------

func TestCircuitBreaker_Reset(t *testing.T) {
	t.Run("resets_to_closed", func(t *testing.T) {
		// Resets_to_closed should return circuit to initial state.
		cfg := CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour}
		cb := NewCircuitBreaker(cfg)

		cb.Failure()
		if cb.State() != CircuitOpen {
			t.Fatal("expected open before reset")
		}

		cb.Reset()
		if cb.State() != CircuitClosed {
			t.Fatalf("expected CircuitClosed after reset, got %v", cb.State())
		}
	})

	t.Run("syncs_state_to_store", func(t *testing.T) {
		// Syncs_state_to_store should update dep state on reset.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		cfg := CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour}
		cb := NewCircuitBreakerWithStore(cfg, store, "my-dep")

		cb.Failure()
		cb.Reset()

		state := store.Get("my-dep")
		if state == nil {
			t.Fatal("expected state in store")
		}
		if state.Status != StatusUp {
			t.Errorf("expected StatusUp after reset, got %s", state.Status)
		}
		if state.CircuitState != CircuitClosed {
			t.Errorf("expected CircuitClosed after reset, got %v", state.CircuitState)
		}
	})
}

// --------------------------------------------------------------------------
// syncState tests (via circuit transitions)
// --------------------------------------------------------------------------

func TestCircuitBreaker_SyncState(t *testing.T) {
	t.Run("syncs_status_down_when_open", func(t *testing.T) {
		// Syncs_status_down_when_open should set status to Down when circuit opens.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		cfg := CircuitBreakerConfig{FailureThreshold: 2, Timeout: time.Hour}
		cb := NewCircuitBreakerWithStore(cfg, store, "ext-api")

		cb.Failure()
		cb.Failure() // Opens circuit

		state := store.Get("ext-api")
		if state == nil {
			t.Fatal("expected state in store")
		}
		if state.Status != StatusDown {
			t.Errorf("expected StatusDown when open, got %s", state.Status)
		}
		if state.CircuitState != CircuitOpen {
			t.Errorf("expected CircuitOpen, got %v", state.CircuitState)
		}
	})

	t.Run("syncs_status_degraded_when_elevated_failures", func(t *testing.T) {
		// Syncs_status_degraded_when_elevated_failures should mark degraded.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		cfg := CircuitBreakerConfig{FailureThreshold: 4, Timeout: time.Hour}
		cb := NewCircuitBreakerWithStore(cfg, store, "cache")

		// Half of failure threshold should trigger degraded
		cb.Failure()
		cb.Failure()

		state := store.Get("cache")
		if state == nil {
			t.Fatal("expected state in store")
		}
		if state.Status != StatusDegraded {
			t.Errorf("expected StatusDegraded at half failures, got %s", state.Status)
		}
	})

	t.Run("syncs_status_degraded_when_half_open", func(t *testing.T) {
		// Syncs_status_degraded_when_half_open should mark degraded on transition.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		cfg := CircuitBreakerConfig{
			FailureThreshold: 1,
			Timeout:          1 * time.Millisecond,
			HalfOpenMaxCalls: 1,
		}
		cb := NewCircuitBreakerWithStore(cfg, store, "db")

		cb.Failure()                     // Opens circuit
		time.Sleep(5 * time.Millisecond) // Wait for timeout
		cb.Allow()                       // Triggers half-open check

		state := store.Get("db")
		if state == nil {
			t.Fatal("expected state in store")
		}
		if state.Status != StatusDegraded {
			t.Errorf("expected StatusDegraded when half-open, got %s", state.Status)
		}
		if state.CircuitState != CircuitHalfOpen {
			t.Errorf("expected CircuitHalfOpen, got %v", state.CircuitState)
		}
	})

	t.Run("syncs_status_down_on_half_open_failure", func(t *testing.T) {
		// Syncs_status_down_on_half_open_failure should return to Down.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		cfg := CircuitBreakerConfig{
			FailureThreshold: 1,
			Timeout:          1 * time.Millisecond,
			HalfOpenMaxCalls: 1,
		}
		cb := NewCircuitBreakerWithStore(cfg, store, "ext")

		cb.Failure()                     // Opens circuit
		time.Sleep(5 * time.Millisecond) // Wait for timeout -> half-open
		cb.Allow()                       // Get permission
		cb.Failure()                     // Fail in half-open -> back to open

		state := store.Get("ext")
		if state == nil {
			t.Fatal("expected state in store")
		}
		if state.Status != StatusDown {
			t.Errorf("expected StatusDown after half-open failure, got %s", state.Status)
		}
		if state.CircuitState != CircuitOpen {
			t.Errorf("expected CircuitOpen, got %v", state.CircuitState)
		}
	})

	t.Run("syncs_status_up_on_success_close", func(t *testing.T) {
		// Syncs_status_up_on_success_close should set Up when circuit closes.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		cfg := CircuitBreakerConfig{
			FailureThreshold: 1,
			SuccessThreshold: 1,
			Timeout:          1 * time.Millisecond,
			HalfOpenMaxCalls: 2,
		}
		cb := NewCircuitBreakerWithStore(cfg, store, "svc")

		cb.Failure()                     // Opens circuit
		time.Sleep(5 * time.Millisecond) // Wait for timeout -> half-open
		cb.Allow()                       // Get permission
		cb.Success()                     // Success closes circuit

		state := store.Get("svc")
		if state == nil {
			t.Fatal("expected state in store")
		}
		if state.Status != StatusUp {
			t.Errorf("expected StatusUp after success, got %s", state.Status)
		}
		if state.CircuitState != CircuitClosed {
			t.Errorf("expected CircuitClosed, got %v", state.CircuitState)
		}
	})

	t.Run("handles_nil_state_in_store", func(t *testing.T) {
		// Handles_nil_state_in_store should not panic.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		cfg := CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour}
		// Create breaker but don't register in store first
		cb := NewCircuitBreaker(cfg)
		// Manually set store and depID without registering
		cbInternal := cb.(*circuitBreaker)
		cbInternal.stateStore = store
		cbInternal.depID = "unregistered"

		// This should not panic
		cb.Failure()
	})
}
