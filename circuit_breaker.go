// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"sync"
	"time"
)

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures to open the circuit.
	FailureThreshold int
	// SuccessThreshold is the number of successes to close the circuit from half-open.
	SuccessThreshold int
	// Timeout is the duration the circuit stays open before transitioning to half-open.
	Timeout time.Duration
	// HalfOpenMaxCalls is the maximum concurrent calls allowed in half-open state.
	HalfOpenMaxCalls int
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker interface {
	// State returns the current circuit state.
	State() CircuitState
	// Allow checks if a request should be allowed.
	Allow() bool
	// Success records a successful call.
	Success()
	// Failure records a failed call.
	Failure()
	// Reset resets the circuit breaker to closed state.
	Reset()
}

// circuitBreaker is the default implementation.
type circuitBreaker struct {
	cfg           CircuitBreakerConfig
	state         CircuitState
	failureCount  int
	successCount  int
	lastFailure   time.Time
	halfOpenCalls int
	mu            sync.Mutex
	stateStore    DepStateStore
	depID         string
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(cfg CircuitBreakerConfig) CircuitBreaker {
	return &circuitBreaker{
		cfg:   cfg,
		state: CircuitClosed,
	}
}

// NewCircuitBreakerWithStore creates a circuit breaker that syncs with a DepStateStore.
func NewCircuitBreakerWithStore(cfg CircuitBreakerConfig, store DepStateStore, depID string) CircuitBreaker {
	cb := &circuitBreaker{
		cfg:        cfg,
		state:      CircuitClosed,
		stateStore: store,
		depID:      depID,
	}
	// Register dependency
	if store != nil && depID != "" {
		store.Register(depID)
	}
	return cb
}

// State returns the current circuit state.
func (cb *circuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.checkState()
	return cb.state
}

// Allow checks if a request should be allowed.
func (cb *circuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.checkState()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		return false
	case CircuitHalfOpen:
		if cb.halfOpenCalls < cb.cfg.HalfOpenMaxCalls {
			cb.halfOpenCalls++
			return true
		}
		return false
	}
	return false
}

// Success records a successful call.
func (cb *circuitBreaker) Success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.cfg.SuccessThreshold {
			cb.state = CircuitClosed
			cb.failureCount = 0
			cb.successCount = 0
			cb.halfOpenCalls = 0
			cb.syncState(StatusUp, "")
		}
	} else if cb.state == CircuitClosed {
		cb.failureCount = 0
	}
}

// Failure records a failed call.
func (cb *circuitBreaker) Failure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailure = time.Now()

	if cb.state == CircuitHalfOpen {
		// Any failure in half-open returns to open
		cb.state = CircuitOpen
		cb.successCount = 0
		cb.halfOpenCalls = 0
		cb.syncState(StatusDown, "circuit open after half-open failure")
	} else if cb.failureCount >= cb.cfg.FailureThreshold {
		cb.state = CircuitOpen
		cb.syncState(StatusDown, "circuit open due to failures")
	} else if cb.failureCount >= cb.cfg.FailureThreshold/2 {
		cb.syncState(StatusDegraded, "elevated failure rate")
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *circuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenCalls = 0
	cb.syncState(StatusUp, "")
}

// checkState checks if the circuit should transition (e.g., open -> half-open).
func (cb *circuitBreaker) checkState() {
	if cb.state == CircuitOpen && time.Since(cb.lastFailure) >= cb.cfg.Timeout {
		cb.state = CircuitHalfOpen
		cb.halfOpenCalls = 0
		cb.successCount = 0
		cb.syncState(StatusDegraded, "circuit half-open")
	}
}

// syncState updates the associated DepStateStore.
func (cb *circuitBreaker) syncState(status Status, message string) {
	if cb.stateStore != nil && cb.depID != "" {
		cb.stateStore.SetStatus(cb.depID, status, message)
		cb.stateStore.SetCircuitState(cb.depID, cb.state)
	}
}
