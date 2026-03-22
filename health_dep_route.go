// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	rxroute "github.com/kryovyx/rextension/route"
)

// HealthDepRoute extends rxroute.Route with dependency declaration.
// Routes implementing this interface will have their dependencies
// automatically registered with the health extension.
type HealthDepRoute interface {
	rxroute.Route
	// Dependencies returns the list of dependency requirements for this route.
	Dependencies() []DepRequirement
}

// healthDepRoute is a wrapper that adds dependency information to a route.
type healthDepRoute struct {
	rxroute.Route
	deps []DepRequirement
}

// NewHealthDepRoute wraps a route with dependency requirements.
func NewHealthDepRoute(rt rxroute.Route, deps ...DepRequirement) HealthDepRoute {
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
func NewRouteWithDeps(method, path string, handler rxroute.HandlerFunc, deps ...DepRequirement) HealthDepRoute {
	return &healthDepRoute{
		Route: rxroute.New(method, path, handler),
		deps:  deps,
	}
}
