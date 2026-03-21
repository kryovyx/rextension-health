// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health provides a Rex extension for comprehensive health checking,
// dependency state management, and circuit breaker patterns.
//
// This file defines the core status types and dependency state structures.
package health

import (
	"sync"
	"time"
)

// Status represents the health state of a dependency or service.
type Status int

const (
	// StatusUp indicates the dependency is fully operational.
	StatusUp Status = iota
	// StatusDegraded indicates the dependency is operational but with issues.
	StatusDegraded
	// StatusDown indicates the dependency is not operational.
	StatusDown
	// StatusUnknown indicates the dependency state has not been determined yet.
	StatusUnknown
)

// String returns the string representation of the status.
func (s Status) String() string {
	switch s {
	case StatusUp:
		return "UP"
	case StatusDegraded:
		return "DEGRADED"
	case StatusDown:
		return "DOWN"
	case StatusUnknown:
		return "UNKNOWN"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON implements json.Marshaler to serialize Status as a string.
func (s Status) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler to deserialize Status from a string.
func (s *Status) UnmarshalJSON(data []byte) error {
	str := string(data)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	switch str {
	case "UP":
		*s = StatusUp
	case "DEGRADED":
		*s = StatusDegraded
	case "DOWN":
		*s = StatusDown
	default:
		*s = StatusUnknown
	}
	return nil
}

// IsHealthy returns true if the status is UP or DEGRADED.
func (s Status) IsHealthy() bool {
	return s == StatusUp || s == StatusDegraded
}

// IsUp returns true if the status is UP.
func (s Status) IsUp() bool {
	return s == StatusUp
}

// DepState holds the current state and metadata for a single dependency.
type DepState struct {
	ID           string            `json:"id"`
	Status       Status            `json:"status"`
	Message      string            `json:"message,omitempty"`
	LastCheck    time.Time         `json:"last_check"`
	LastSuccess  *time.Time        `json:"last_success"`
	LastFailure  *time.Time        `json:"last_failure"`
	FailureCount int64             `json:"failure_count"`
	SuccessCount int64             `json:"success_count"`
	LastLatency  time.Duration     `json:"last_latency,omitempty"`
	AvgLatency   time.Duration     `json:"avg_latency,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CircuitState CircuitState      `json:"circuit_state,omitempty"`
	mu           sync.RWMutex
}

// NewDepState creates a new dependency state with the given ID.
func NewDepState(id string) *DepState {
	return &DepState{
		ID:           id,
		Status:       StatusUp,
		LastCheck:    time.Now(),
		Metadata:     make(map[string]string),
		CircuitState: CircuitClosed,
	}
}

// Clone creates a thread-safe copy of the state.
func (d *DepState) Clone() *DepState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	meta := make(map[string]string, len(d.Metadata))
	for k, v := range d.Metadata {
		meta[k] = v
	}
	var lastSuccess, lastFailure *time.Time
	if d.LastSuccess != nil {
		t := *d.LastSuccess
		lastSuccess = &t
	}
	if d.LastFailure != nil {
		t := *d.LastFailure
		lastFailure = &t
	}
	return &DepState{
		ID:           d.ID,
		Status:       d.Status,
		Message:      d.Message,
		LastCheck:    d.LastCheck,
		LastSuccess:  lastSuccess,
		LastFailure:  lastFailure,
		FailureCount: d.FailureCount,
		SuccessCount: d.SuccessCount,
		LastLatency:  d.LastLatency,
		AvgLatency:   d.AvgLatency,
		Metadata:     meta,
		CircuitState: d.CircuitState,
	}
}

// SetStatus updates the status and timestamp.
func (d *DepState) SetStatus(status Status, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Status = status
	d.Message = message
	d.LastCheck = time.Now()
}

// SetMeta sets a metadata key-value pair.
func (d *DepState) SetMeta(key, value string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Metadata == nil {
		d.Metadata = make(map[string]string)
	}
	d.Metadata[key] = value
}

// GetMeta returns the value for a metadata key.
func (d *DepState) GetMeta(key string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Metadata[key]
}

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed allows requests to pass through.
	CircuitClosed CircuitState = iota
	// CircuitOpen blocks all requests.
	CircuitOpen
	// CircuitHalfOpen allows limited requests for probing.
	CircuitHalfOpen
)

// String returns the string representation of the circuit state.
func (c CircuitState) String() string {
	switch c {
	case CircuitClosed:
		return "CLOSED"
	case CircuitOpen:
		return "OPEN"
	case CircuitHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}
