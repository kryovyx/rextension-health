// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"encoding/json"
	"testing"
	"time"
)

// --------------------------------------------------------------------------
// Status tests
// --------------------------------------------------------------------------

func TestStatusString(t *testing.T) {
	t.Run("returns_up", func(t *testing.T) {
		// Returns_up should return "UP" for StatusUp.
		if StatusUp.String() != "UP" {
			t.Errorf("expected UP, got %s", StatusUp.String())
		}
	})

	t.Run("returns_degraded", func(t *testing.T) {
		// Returns_degraded should return "DEGRADED" for StatusDegraded.
		if StatusDegraded.String() != "DEGRADED" {
			t.Errorf("expected DEGRADED, got %s", StatusDegraded.String())
		}
	})

	t.Run("returns_down", func(t *testing.T) {
		// Returns_down should return "DOWN" for StatusDown.
		if StatusDown.String() != "DOWN" {
			t.Errorf("expected DOWN, got %s", StatusDown.String())
		}
	})

	t.Run("returns_unknown_for_invalid_status", func(t *testing.T) {
		// Returns_unknown_for_invalid_status should return "UNKNOWN" for invalid status.
		invalidStatus := Status(99)
		if invalidStatus.String() != "UNKNOWN" {
			t.Errorf("expected UNKNOWN, got %s", invalidStatus.String())
		}
	})
}

func TestStatusIsHealthy(t *testing.T) {
	t.Run("up_is_healthy", func(t *testing.T) {
		// Up_is_healthy should return true.
		if !StatusUp.IsHealthy() {
			t.Error("StatusUp should be healthy")
		}
	})

	t.Run("degraded_is_healthy", func(t *testing.T) {
		// Degraded_is_healthy should return true.
		if !StatusDegraded.IsHealthy() {
			t.Error("StatusDegraded should be healthy")
		}
	})

	t.Run("down_is_not_healthy", func(t *testing.T) {
		// Down_is_not_healthy should return false.
		if StatusDown.IsHealthy() {
			t.Error("StatusDown should not be healthy")
		}
	})
}

func TestStatusIsUp(t *testing.T) {
	t.Run("up_is_up", func(t *testing.T) {
		// Up_is_up should return true.
		if !StatusUp.IsUp() {
			t.Error("StatusUp should be up")
		}
	})

	t.Run("degraded_is_not_up", func(t *testing.T) {
		// Degraded_is_not_up should return false.
		if StatusDegraded.IsUp() {
			t.Error("StatusDegraded should not be up")
		}
	})

	t.Run("down_is_not_up", func(t *testing.T) {
		// Down_is_not_up should return false.
		if StatusDown.IsUp() {
			t.Error("StatusDown should not be up")
		}
	})
}

func TestStatusUnmarshalJSON(t *testing.T) {
	t.Run("unmarshals_up", func(t *testing.T) {
		// Unmarshals_up should parse "UP" correctly.
		var s Status
		err := json.Unmarshal([]byte(`"UP"`), &s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != StatusUp {
			t.Errorf("expected StatusUp, got %v", s)
		}
	})

	t.Run("unmarshals_degraded", func(t *testing.T) {
		// Unmarshals_degraded should parse "DEGRADED" correctly.
		var s Status
		err := json.Unmarshal([]byte(`"DEGRADED"`), &s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != StatusDegraded {
			t.Errorf("expected StatusDegraded, got %v", s)
		}
	})

	t.Run("unmarshals_down", func(t *testing.T) {
		// Unmarshals_down should parse "DOWN" correctly.
		var s Status
		err := json.Unmarshal([]byte(`"DOWN"`), &s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != StatusDown {
			t.Errorf("expected StatusDown, got %v", s)
		}
	})

	t.Run("defaults_to_unknown_for_invalid", func(t *testing.T) {
		// Defaults_to_unknown_for_invalid should return StatusUnknown.
		var s Status
		err := json.Unmarshal([]byte(`"INVALID"`), &s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != StatusUnknown {
			t.Errorf("expected StatusUnknown, got %v", s)
		}
	})
}

// --------------------------------------------------------------------------
// DepState tests
// --------------------------------------------------------------------------

func TestNewDepState(t *testing.T) {
	t.Run("creates_state_with_id", func(t *testing.T) {
		// Creates_state_with_id should initialize state correctly.
		state := NewDepState("test-dep")
		if state.ID != "test-dep" {
			t.Errorf("expected ID 'test-dep', got %s", state.ID)
		}
		if state.Status != StatusUp {
			t.Errorf("expected StatusUp, got %v", state.Status)
		}
	})
}

func TestDepStateClone(t *testing.T) {
	t.Run("creates_independent_copy", func(t *testing.T) {
		// Creates_independent_copy should not share state.
		state := NewDepState("test-dep")
		state.SetMeta("key", "value")
		state.SetStatus(StatusDegraded, "test message")

		clone := state.Clone()
		if clone.ID != state.ID {
			t.Errorf("clone ID mismatch")
		}
		if clone.Status != state.Status {
			t.Errorf("clone status mismatch")
		}
		if clone.GetMeta("key") != "value" {
			t.Errorf("clone metadata mismatch")
		}

		// Modifying clone should not affect original
		clone.SetMeta("key", "modified")
		if state.GetMeta("key") == "modified" {
			t.Error("original was modified through clone")
		}
	})
}

func TestDepStateSetMeta(t *testing.T) {
	t.Run("initializes_metadata_map", func(t *testing.T) {
		// Initializes_metadata_map should create map if nil.
		state := NewDepState("test")
		state.Metadata = nil // Force nil
		state.SetMeta("key", "value")

		if state.GetMeta("key") != "value" {
			t.Errorf("expected value, got %s", state.GetMeta("key"))
		}
	})
}

// --------------------------------------------------------------------------
// CircuitState tests
// --------------------------------------------------------------------------

func TestCircuitStateString(t *testing.T) {
	t.Run("returns_closed", func(t *testing.T) {
		// Returns_closed should return "CLOSED" for CircuitClosed.
		if CircuitClosed.String() != "CLOSED" {
			t.Errorf("expected CLOSED, got %s", CircuitClosed.String())
		}
	})

	t.Run("returns_open", func(t *testing.T) {
		// Returns_open should return "OPEN" for CircuitOpen.
		if CircuitOpen.String() != "OPEN" {
			t.Errorf("expected OPEN, got %s", CircuitOpen.String())
		}
	})

	t.Run("returns_half_open", func(t *testing.T) {
		// Returns_half_open should return "HALF_OPEN" for CircuitHalfOpen.
		if CircuitHalfOpen.String() != "HALF_OPEN" {
			t.Errorf("expected HALF_OPEN, got %s", CircuitHalfOpen.String())
		}
	})

	t.Run("returns_unknown_for_invalid_state", func(t *testing.T) {
		// Returns_unknown_for_invalid_state should return "UNKNOWN".
		invalidState := CircuitState(99)
		if invalidState.String() != "UNKNOWN" {
			t.Errorf("expected UNKNOWN, got %s", invalidState.String())
		}
	})
}

// --------------------------------------------------------------------------
// DepStateStore tests
// --------------------------------------------------------------------------

func TestDepStateStore(t *testing.T) {
	t.Run("registers_dependency", func(t *testing.T) {
		// Registers_dependency should create and store state.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		state := store.Register("db")
		if state == nil {
			t.Fatal("Register returned nil")
		}
		if state.ID != "db" {
			t.Errorf("expected ID 'db', got %s", state.ID)
		}
	})

	t.Run("get_returns_same_data", func(t *testing.T) {
		// Get_returns_same_data should return the registered state.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		store.Register("db")

		got := store.Get("db")
		if got == nil {
			t.Fatal("Get returned nil")
		}
		if got.ID != "db" {
			t.Errorf("expected ID 'db', got %s", got.ID)
		}
	})

	t.Run("register_returns_existing_state", func(t *testing.T) {
		// Register_returns_existing_state should return the same state on second call.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		state1 := store.Register("db")
		state1.SetStatus(StatusDegraded, "test")

		state2 := store.Register("db")
		if state2.Status != StatusDegraded {
			t.Error("expected existing state to be returned")
		}
	})
}

func TestDepStateStoreReportSuccess(t *testing.T) {
	t.Run("updates_state_on_success", func(t *testing.T) {
		// Updates_state_on_success should update status and counts.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		store.ReportSuccess("api", 100*time.Millisecond)

		state := store.Get("api")
		if state == nil {
			t.Fatal("state is nil after ReportSuccess")
		}
		if state.Status != StatusUp {
			t.Errorf("expected StatusUp, got %v", state.Status)
		}
		if state.SuccessCount != 1 {
			t.Errorf("expected SuccessCount 1, got %d", state.SuccessCount)
		}
		if state.LastLatency != 100*time.Millisecond {
			t.Errorf("expected LastLatency 100ms, got %v", state.LastLatency)
		}
	})

	t.Run("resets_failure_count_on_success", func(t *testing.T) {
		// Resets_failure_count_on_success should clear consecutive failures.
		cfg := DepStateStoreConfig{
			FailureThreshold:  3,
			DegradedThreshold: 2,
			WindowDuration:    30 * time.Second,
		}
		store := NewDepStateStore(cfg)

		// Report some failures
		store.ReportFailure("api", "error")
		store.ReportFailure("api", "error")

		// Report success
		store.ReportSuccess("api", 50*time.Millisecond)

		state := store.Get("api")
		if state.Status != StatusUp {
			t.Errorf("expected StatusUp after success, got %v", state.Status)
		}
	})
}

func TestDepStateStoreReportFailure(t *testing.T) {
	t.Run("tracks_failure_progression", func(t *testing.T) {
		// Tracks_failure_progression should follow threshold-based degradation.
		cfg := DepStateStoreConfig{
			FailureThreshold:  3,
			DegradedThreshold: 2,
			WindowDuration:    30 * time.Second,
		}
		store := NewDepStateStore(cfg)

		// First failure - still UP
		store.ReportFailure("api", "timeout")
		state := store.Get("api")
		if state.Status != StatusUp {
			t.Errorf("expected StatusUp after 1 failure, got %v", state.Status)
		}

		// Second failure - DEGRADED
		store.ReportFailure("api", "timeout")
		state = store.Get("api")
		if state.Status != StatusDegraded {
			t.Errorf("expected StatusDegraded after 2 failures, got %v", state.Status)
		}

		// Third failure - DOWN
		store.ReportFailure("api", "timeout")
		state = store.Get("api")
		if state.Status != StatusDown {
			t.Errorf("expected StatusDown after 3 failures, got %v", state.Status)
		}
	})
}

func TestDepStateStoreSetStatus(t *testing.T) {
	t.Run("sets_status_directly", func(t *testing.T) {
		// Sets_status_directly should override the current status.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		store.SetStatus("cache", StatusDegraded, "high latency")

		state := store.Get("cache")
		if state == nil {
			t.Fatal("state is nil after SetStatus")
		}
		if state.Status != StatusDegraded {
			t.Errorf("expected StatusDegraded, got %v", state.Status)
		}
		if state.Message != "high latency" {
			t.Errorf("expected message 'high latency', got %s", state.Message)
		}
	})
}

func TestDepStateStoreGetAll(t *testing.T) {
	t.Run("returns_all_states", func(t *testing.T) {
		// Returns_all_states should return all registered states.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		store.Register("db")
		store.Register("cache")
		store.Register("api")

		all := store.GetAll()
		if len(all) != 3 {
			t.Errorf("expected 3 entries, got %d", len(all))
		}
	})
}

func TestDepStateStoreRemove(t *testing.T) {
	t.Run("removes_state", func(t *testing.T) {
		// Removes_state should delete the state from store.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		store.Register("temp")
		store.Remove("temp")

		if store.Get("temp") != nil {
			t.Error("expected nil after Remove")
		}
	})
}

func TestDepStateStoreSetCircuitState(t *testing.T) {
	t.Run("updates_circuit_state", func(t *testing.T) {
		// Updates_circuit_state should set the CircuitState field.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		store.Register("db")
		store.SetCircuitState("db", CircuitOpen)

		state := store.Get("db")
		if state == nil {
			t.Fatal("expected state to exist")
		}
		if state.CircuitState != CircuitOpen {
			t.Errorf("expected CircuitOpen, got %v", state.CircuitState)
		}
	})

	t.Run("does_nothing_when_state_not_found", func(t *testing.T) {
		// Does_nothing_when_state_not_found should not panic.
		cfg := DefaultDepStateStoreConfig()
		store := NewDepStateStore(cfg)

		// Should not panic when state doesn't exist
		store.SetCircuitState("nonexistent", CircuitOpen)

		// Verify nothing was created
		if store.Get("nonexistent") != nil {
			t.Error("expected nil state for nonexistent")
		}
	})
}
