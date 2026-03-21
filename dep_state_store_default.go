// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"sync"
	"time"
)

// depStateStore is the default implementation of DepStateStore.
type depStateStore struct {
	states            map[string]*DepState
	mu                sync.RWMutex
	failureThreshold  int
	degradedThreshold int
	windowDuration    time.Duration
}

// NewDepStateStore creates a new dependency state store.
func NewDepStateStore(cfg DepStateStoreConfig) DepStateStore {
	return &depStateStore{
		states:            make(map[string]*DepState),
		failureThreshold:  cfg.FailureThreshold,
		degradedThreshold: cfg.DegradedThreshold,
		windowDuration:    cfg.WindowDuration,
	}
}

// Get returns the current state of a dependency.
func (s *depStateStore) Get(id string) *DepState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if state, ok := s.states[id]; ok {
		return state.Clone()
	}
	return nil
}

// GetAll returns a copy of all dependency states.
func (s *depStateStore) GetAll() map[string]*DepState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*DepState, len(s.states))
	for k, v := range s.states {
		result[k] = v.Clone()
	}
	return result
}

// Register creates or returns an existing dependency state entry.
func (s *depStateStore) Register(id string) *DepState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, ok := s.states[id]; ok {
		return state
	}
	state := NewDepState(id)
	s.states[id] = state
	return state
}

// ReportSuccess records a successful operation for a dependency.
func (s *depStateStore) ReportSuccess(id string, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[id]
	if !ok {
		state = NewDepState(id)
		s.states[id] = state
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.SuccessCount++
	now := time.Now()
	state.LastSuccess = &now
	state.LastLatency = latency
	state.LastCheck = now

	// Update average latency (exponential moving average)
	if state.AvgLatency == 0 {
		state.AvgLatency = latency
	} else {
		alpha := 0.3
		state.AvgLatency = time.Duration(alpha*float64(latency) + (1-alpha)*float64(state.AvgLatency))
	}

	// Reset failure tracking on success
	state.FailureCount = 0
	state.Status = StatusUp
	state.Message = ""
	state.CircuitState = CircuitClosed
}

// ReportFailure records a failed operation for a dependency.
func (s *depStateStore) ReportFailure(id string, errType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[id]
	if !ok {
		state = NewDepState(id)
		s.states[id] = state
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.FailureCount++
	now := time.Now()
	state.LastFailure = &now
	state.LastCheck = now
	state.Message = errType

	// Update status based on failure count
	if state.FailureCount >= int64(s.failureThreshold) {
		state.Status = StatusDown
		state.CircuitState = CircuitOpen
	} else if state.FailureCount >= int64(s.degradedThreshold) {
		state.Status = StatusDegraded
	}
}

// SetStatus explicitly sets the status of a dependency.
func (s *depStateStore) SetStatus(id string, status Status, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[id]
	if !ok {
		state = NewDepState(id)
		s.states[id] = state
	}
	state.SetStatus(status, message)
}

// SetCircuitState updates the circuit breaker state for a dependency.
func (s *depStateStore) SetCircuitState(id string, circuitState CircuitState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[id]
	if !ok {
		return // Don't create state just for circuit state
	}
	state.mu.Lock()
	state.CircuitState = circuitState
	state.mu.Unlock()
}

// Remove removes a dependency from the store.
func (s *depStateStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, id)
}
