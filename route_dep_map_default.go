// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"sync"
)

// routeDepMap is the default implementation of RouteDepMap.
type routeDepMap struct {
	deps map[string][]DepRequirement
	mu   sync.RWMutex
}

// NewRouteDepMap creates a new route dependency map.
func NewRouteDepMap() RouteDepMap {
	return &routeDepMap{
		deps: make(map[string][]DepRequirement),
	}
}

// Register adds dependency requirements for a route.
func (m *routeDepMap) Register(routeID string, requirements []DepRequirement) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deps[routeID] = requirements
}

// Get returns the dependency requirements for a route.
func (m *routeDepMap) Get(routeID string) []DepRequirement {
	m.mu.RLock()
	defer m.mu.RUnlock()
	reqs, ok := m.deps[routeID]
	if !ok {
		return nil
	}
	// Return a copy
	result := make([]DepRequirement, len(reqs))
	copy(result, reqs)
	return result
}

// Remove removes the dependency requirements for a route.
func (m *routeDepMap) Remove(routeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.deps, routeID)
}

// GetAll returns all route-dependency mappings.
func (m *routeDepMap) GetAll() map[string][]DepRequirement {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string][]DepRequirement, len(m.deps))
	for k, v := range m.deps {
		cp := make([]DepRequirement, len(v))
		copy(cp, v)
		result[k] = cp
	}
	return result
}
