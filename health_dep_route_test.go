// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health contains tests for the HealthDepRoute implementation.
package health

import (
	"testing"

	rxroute "github.com/kryovyx/rextension/route"
)

// --------------------------------------------------------------------------
// NewHealthDepRoute tests
// --------------------------------------------------------------------------

func TestNewHealthDepRoute(t *testing.T) {
	t.Run("wraps_route_with_dependencies", func(t *testing.T) {
		// Wraps_route_with_dependencies should add deps to existing route.
		baseRoute := rxroute.New("GET", "/api/users", func(ctx rxroute.Context) {})

		deps := []DepRequirement{
			NewHardRequirement("db"),
			NewSoftRequirement("cache"),
		}

		wrapped := NewHealthDepRoute(baseRoute, deps...)

		if wrapped.Method() != "GET" {
			t.Errorf("expected method GET, got %s", wrapped.Method())
		}
		if wrapped.Path() != "/api/users" {
			t.Errorf("expected path /api/users, got %s", wrapped.Path())
		}
		gotDeps := wrapped.Dependencies()
		if len(gotDeps) != 2 {
			t.Errorf("expected 2 dependencies, got %d", len(gotDeps))
		}
	})

	t.Run("handler_delegates_to_base_route", func(t *testing.T) {
		// Handler_delegates_to_base_route should use the base route's handler.
		handlerCalled := false
		baseRoute := rxroute.New("POST", "/api/orders", func(ctx rxroute.Context) {
			handlerCalled = true
		})

		wrapped := NewHealthDepRoute(baseRoute)

		// Call the handler
		wrapped.Handler()(nil)

		if !handlerCalled {
			t.Error("expected base route handler to be called")
		}
	})
}

// --------------------------------------------------------------------------
// NewRouteWithDeps tests
// --------------------------------------------------------------------------

func TestNewRouteWithDeps(t *testing.T) {
	t.Run("creates_route_with_dependencies", func(t *testing.T) {
		// Creates_route_with_dependencies should create a new route with deps.
		handlerCalled := false
		rt := NewRouteWithDeps("DELETE", "/api/items/:id", func(ctx rxroute.Context) {
			handlerCalled = true
		}, NewHardRequirement("db"))

		if rt.Method() != "DELETE" {
			t.Errorf("expected method DELETE, got %s", rt.Method())
		}
		if rt.Path() != "/api/items/:id" {
			t.Errorf("expected path /api/items/:id, got %s", rt.Path())
		}

		deps := rt.Dependencies()
		if len(deps) != 1 {
			t.Errorf("expected 1 dependency, got %d", len(deps))
		}
		if deps[0].DepID != "db" {
			t.Errorf("expected depID db, got %s", deps[0].DepID)
		}

		// Call handler
		rt.Handler()(nil)
		if !handlerCalled {
			t.Error("expected handler to be called")
		}
	})

	t.Run("creates_route_without_dependencies", func(t *testing.T) {
		// Creates_route_without_dependencies should work with no deps.
		rt := NewRouteWithDeps("GET", "/api/health", func(ctx rxroute.Context) {})

		deps := rt.Dependencies()
		if len(deps) != 0 {
			t.Errorf("expected 0 dependencies, got %d", len(deps))
		}
	})
}
