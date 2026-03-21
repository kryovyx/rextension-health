// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"time"
)

// DepStateStore defines the interface for managing dependency states.
type DepStateStore interface {
	// Get returns the current state of a dependency.
	Get(id string) *DepState
	// GetAll returns all dependency states.
	GetAll() map[string]*DepState
	// Register creates or returns an existing dependency state entry.
	Register(id string) *DepState
	// ReportSuccess records a successful operation for a dependency.
	ReportSuccess(id string, latency time.Duration)
	// ReportFailure records a failed operation for a dependency.
	ReportFailure(id string, errType string)
	// SetStatus explicitly sets the status of a dependency.
	SetStatus(id string, status Status, message string)
	// SetCircuitState updates the circuit breaker state for a dependency.
	SetCircuitState(id string, circuitState CircuitState)
	// Remove removes a dependency from the store.
	Remove(id string)
}

// DepStateStoreConfig configures the behavior of the state store.
type DepStateStoreConfig struct {
	// FailureThreshold is the number of consecutive failures to mark as DOWN.
	FailureThreshold int
	// DegradedThreshold is the number of consecutive failures to mark as DEGRADED.
	DegradedThreshold int
	// WindowDuration is the time window for failure counting.
	WindowDuration time.Duration
}

// DefaultDepStateStoreConfig returns default configuration.
func DefaultDepStateStoreConfig() DepStateStoreConfig {
	return DepStateStoreConfig{
		FailureThreshold:  5,
		DegradedThreshold: 2,
		WindowDuration:    30 * time.Second,
	}
}
