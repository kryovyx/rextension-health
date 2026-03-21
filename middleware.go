// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health provides a Rex extension for comprehensive health checking.
//
// This file defines the middleware interfaces and context helpers.
package health

import (
	"context"

	rx "github.com/kryovyx/rextension"
)

// Context keys for dependency state in request context.
type contextKey string

const (
	// ContextKeyRouteID stores the resolved route ID in context.
	ContextKeyRouteID contextKey = "health:route_id"
	// ContextKeyDepStates stores dependency states in context.
	ContextKeyDepStates contextKey = "health:dep_states"
	// ContextKeyDegradedDeps stores degraded dependency IDs in context.
	ContextKeyDegradedDeps contextKey = "health:degraded_deps"
)

// DepStateContext holds dependency state information for a request.
type DepStateContext struct {
	RouteID      string
	Dependencies map[string]*DepState
	DegradedDeps []string
}

// GetDepStateContext retrieves the dependency state context from a request context.
func GetDepStateContext(ctx context.Context) *DepStateContext {
	if val := ctx.Value(ContextKeyDepStates); val != nil {
		if dsc, ok := val.(*DepStateContext); ok {
			return dsc
		}
	}
	return nil
}

// IsDegraded checks if a specific dependency is marked as degraded in context.
func IsDegraded(ctx context.Context, depID string) bool {
	dsc := GetDepStateContext(ctx)
	if dsc == nil {
		return false
	}
	for _, d := range dsc.DegradedDeps {
		if d == depID {
			return true
		}
	}
	return false
}

// GetDepState retrieves the state of a specific dependency from context.
func GetDepState(ctx context.Context, depID string) *DepState {
	dsc := GetDepStateContext(ctx)
	if dsc == nil {
		return nil
	}
	return dsc.Dependencies[depID]
}

// MiddlewareConfig configures the dependency gate middleware.
type MiddlewareConfig struct {
	// RouteDepMap for looking up route dependencies.
	RouteDepMap RouteDepMap
	// StateStore for getting dependency states.
	StateStore DepStateStore
	// SnapshotCache for cached state lookups.
	SnapshotCache SnapshotCache
	// Registry for looking up health checks (needed for passive checks).
	Registry Registry
	// CheckCache for executing and caching passive checks on-demand.
	CheckCache CheckCache
	// Resolver for executing checks that need DI.
	Resolver interface{}
	// FailureStatusCode is the HTTP status code for hard failures (default: 503).
	FailureStatusCode int
	// FailureMessage is the response message for hard failures.
	FailureMessage string
	// UseCache determines whether to use SnapshotCache or direct StateStore.
	UseCache bool
}

// DefaultMiddlewareConfig returns the default middleware configuration.
func DefaultMiddlewareConfig() MiddlewareConfig {
	return MiddlewareConfig{
		FailureStatusCode: 503,
		FailureMessage:    "Service temporarily unavailable",
		UseCache:          true,
	}
}

// RegisterMiddlewares registers the health middleware stack (route resolver +
// dependency gate) on the provided Rex instance's default router.
func RegisterMiddlewares(r rx.Rex, cfg MiddlewareConfig) error {
	r.Use(RouteResolverMiddleware())
	r.Use(DependencyGateMiddleware(cfg))
	return nil
}
