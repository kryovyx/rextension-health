// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health provides a Rex extension for comprehensive health checking,
// dependency state management, circuit breaker patterns, and request gating.
//
// This file defines the RouteDepMap interface for route-to-dependency mapping.
package health

// RequirementType defines how a dependency failure affects the route.
type RequirementType int

const (
	// RequirementHard means the route fails fast if dependency is down.
	RequirementHard RequirementType = iota
	// RequirementSoft means the route continues but may use fallback.
	RequirementSoft
)

// String returns the string representation of the requirement type.
func (r RequirementType) String() string {
	switch r {
	case RequirementHard:
		return "HARD"
	case RequirementSoft:
		return "SOFT"
	default:
		return "UNKNOWN"
	}
}

// DepRequirement represents a dependency requirement for a route.
type DepRequirement struct {
	DepID     string          `json:"dep_id"`
	Type      RequirementType `json:"type"`
	MinStatus Status          `json:"min_status"`
}

// NewHardRequirement creates a hard dependency requirement.
func NewHardRequirement(depID string) DepRequirement {
	return DepRequirement{
		DepID:     depID,
		Type:      RequirementHard,
		MinStatus: StatusUp,
	}
}

// NewSoftRequirement creates a soft dependency requirement.
func NewSoftRequirement(depID string) DepRequirement {
	return DepRequirement{
		DepID:     depID,
		Type:      RequirementSoft,
		MinStatus: StatusDegraded,
	}
}

// WithMinStatus sets the minimum required status.
func (d DepRequirement) WithMinStatus(status Status) DepRequirement {
	d.MinStatus = status
	return d
}

// RouteDepMap manages the mapping of routes to their dependency requirements.
type RouteDepMap interface {
	// Register adds dependency requirements for a route.
	Register(routeID string, requirements []DepRequirement)
	// Get returns the dependency requirements for a route.
	Get(routeID string) []DepRequirement
	// Remove removes the dependency requirements for a route.
	Remove(routeID string)
	// GetAll returns all route-dependency mappings.
	GetAll() map[string][]DepRequirement
}

// RouteID generates a unique route identifier.
func RouteID(method, path string) string {
	return method + ":" + path
}
