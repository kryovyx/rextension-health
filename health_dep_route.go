// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"github.com/kryovyx/rex/route"
)

// HealthDepRoute extends route.Route with dependency declaration.
// Routes implementing this interface will have their dependencies
// automatically registered with the health extension.
type HealthDepRoute interface {
	route.Route
	// Dependencies returns the list of dependency requirements for this route.
	Dependencies() []DepRequirement
}

// healthDepRoute is a wrapper that adds dependency information to a route.
type healthDepRoute struct {
	route.Route
	deps []DepRequirement
}

// NewHealthDepRoute wraps a route with dependency requirements.
func NewHealthDepRoute(rt route.Route, deps ...DepRequirement) HealthDepRoute {
	return &healthDepRoute{
		Route: rt,
		deps:  deps,
	}
}

// Dependencies returns the dependency requirements for this route.
func (r *healthDepRoute) Dependencies() []DepRequirement {
	return r.deps
}

// NewRouteWithDeps creates a new route with dependency requirements.
func NewRouteWithDeps(method, path string, handler route.HandlerFunc, deps ...DepRequirement) HealthDepRoute {
	return &healthDepRoute{
		Route: route.New(method, path, handler),
		deps:  deps,
	}
}
