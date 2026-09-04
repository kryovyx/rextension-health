// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health covers the dependency gate as a per-route middleware
// (D18b/c, P4.12).
//
// The central test here is TestGate_fires_for_a_parameterized_route. Before
// P4.12 the gate was one global middleware that decided applicability by
// building a route identifier from the live URL — "GET:/users/42" — and
// looking it up in an index keyed at registration by the route's pattern —
// "GET:/users/{id}". Those never match, so the gate silently did nothing for
// every route with a path parameter.
//
// The bug survived because every existing test used a static path, where the
// URL and the pattern happen to be identical. That is worth remembering when
// reading the tests below: the parameterized case is not an edge case, it is
// the case that was broken.
package health

import (
	"net/http"
	"net/http/httptest"
	"testing"

	rx "github.com/kryovyx/rextension"
	rxroute "github.com/kryovyx/rextension/route"
)

// gateConfig builds a MiddlewareConfig with the given dependency states.
func gateConfig(t *testing.T, states map[string]Status) MiddlewareConfig {
	t.Helper()
	store := NewDepStateStore(DefaultDepStateStoreConfig())
	for id, status := range states {
		store.SetStatus(id, status, "test")
	}
	return MiddlewareConfig{
		StateStore:        store,
		UseCache:          false,
		FailureStatusCode: http.StatusServiceUnavailable,
	}
}

// serveThrough composes the factory's middleware for rt and dispatches req.
func serveThrough(t *testing.T, cfg MiddlewareConfig, rt rxroute.Route, req *http.Request) (*httptest.ResponseRecorder, bool) {
	t.Helper()

	factory := DependencyGateFactory(cfg)
	mw := factory(rx.RouteInfo{Route: rt, Router: "default"})

	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	var handler http.Handler = next
	if mw != nil {
		handler = mw(next)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, withMatchedRoute(req, rt))
	return rec, reached
}

// ---------------------------------------------------------------------------
// The bug
// ---------------------------------------------------------------------------

// TestGate_fires_for_a_parameterized_route is the P4.12 acceptance test.
//
// A route declaring a hard dependency on a database that is down must be
// refused — and it was not, for as long as the extension had existed, if its
// path contained a parameter.
func TestGate_fires_for_a_parameterized_route(t *testing.T) {
	cfg := gateConfig(t, map[string]Status{"database": StatusDown})
	route := &gateRoute{
		method: http.MethodGet,
		path:   "/users/{id}", // the pattern
		deps:   []DepRequirement{NewHardRequirement("database")},
	}

	// The request URL is "/users/42" — a different string from the pattern.
	// That difference is the entire bug.
	rec, reached := serveThrough(t, cfg, route, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if reached {
		t.Fatal("the handler ran with a hard dependency down; the gate did not fire for a parameterized route")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// TestGate_fires_for_every_shape_of_path covers the parameterized case
// alongside the static one that always worked, and a wildcard.
func TestGate_fires_for_every_shape_of_path(t *testing.T) {
	cases := []struct {
		pattern string
		url     string
	}{
		{"/static", "/static"},       // always worked
		{"/users/{id}", "/users/42"}, // never worked
		{"/users/{id}/posts/{pid}", "/users/1/posts/9"},
		{"/files/*", "/files/deep/nested/thing.txt"}, // never worked
		{"/a/{b}/c", "/a/anything/c"},
	}

	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			cfg := gateConfig(t, map[string]Status{"database": StatusDown})
			route := &gateRoute{
				method: http.MethodGet,
				path:   tc.pattern,
				deps:   []DepRequirement{NewHardRequirement("database")},
			}

			rec, reached := serveThrough(t, cfg, route, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if reached {
				t.Fatalf("the gate did not fire for pattern %q requested as %q", tc.pattern, tc.url)
			}
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", rec.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Attachment
// ---------------------------------------------------------------------------

// TestGate_attaches_nothing_to_an_ungated_route is the assertable half of the
// P2.9 criterion: a route that declares no dependencies has *no* health
// middleware in its chain, rather than one that runs and does nothing.
func TestGate_attaches_nothing_to_an_ungated_route(t *testing.T) {
	factory := DependencyGateFactory(gateConfig(t, nil))

	t.Run("not a HealthDepRoute", func(t *testing.T) {
		if mw := factory(rx.RouteInfo{Route: &bareRoute{method: http.MethodGet, path: "/plain"}, Router: "default"}); mw != nil {
			t.Fatal("middleware was attached to a route that declares no dependencies")
		}
	})

	t.Run("empty dependency list", func(t *testing.T) {
		rt := &gateRoute{method: http.MethodGet, path: "/empty", deps: nil}
		if mw := factory(rx.RouteInfo{Route: rt, Router: "default"}); mw != nil {
			t.Fatal("middleware was attached to a route with an empty dependency list")
		}
	})

	t.Run("declares dependencies", func(t *testing.T) {
		rt := &gateRoute{method: http.MethodGet, path: "/gated",
			deps: []DepRequirement{NewHardRequirement("db")}}
		if mw := factory(rx.RouteInfo{Route: rt, Router: "default"}); mw == nil {
			t.Fatal("no middleware attached to a route that does declare dependencies")
		}
	})
}

// TestGate_reads_dependencies_once is the D18c criterion: the route's
// configuration is read at freeze, not per request.
func TestGate_reads_dependencies_once(t *testing.T) {
	cfg := gateConfig(t, map[string]Status{"db": StatusUp})

	counting := &countingDepRoute{
		deps: []DepRequirement{NewHardRequirement("db")},
	}

	factory := DependencyGateFactory(cfg)
	mw := factory(rx.RouteInfo{Route: counting, Router: "default"})
	if mw == nil {
		t.Fatal("no middleware attached")
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for range 20 {
		req := withMatchedRoute(httptest.NewRequest(http.MethodGet, "/users/1", nil), counting)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	if counting.calls != 1 {
		t.Fatalf("Dependencies() was called %d times; it must be read once, at freeze", counting.calls)
	}
}

// countingDepRoute counts how often its dependencies are read.
type countingDepRoute struct {
	gateRoute
	calls int
}

func (r *countingDepRoute) Dependencies() []DepRequirement {
	r.calls++
	return r.gateRoute.deps
}

// ---------------------------------------------------------------------------
// Behaviour
// ---------------------------------------------------------------------------

// TestGate_allows_a_healthy_dependency keeps fail-fast from becoming
// fail-always.
func TestGate_allows_a_healthy_dependency(t *testing.T) {
	cfg := gateConfig(t, map[string]Status{"database": StatusUp})
	route := &gateRoute{method: http.MethodGet, path: "/users/{id}",
		deps: []DepRequirement{NewHardRequirement("database")}}

	rec, reached := serveThrough(t, cfg, route, httptest.NewRequest(http.MethodGet, "/users/42", nil))
	if !reached {
		t.Fatal("the handler was refused with a healthy dependency")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// TestGate_soft_dependency_continues_and_marks_context. A soft dependency does
// not refuse the request; it tells the handler so it can degrade.
func TestGate_soft_dependency_continues_and_marks_context(t *testing.T) {
	cfg := gateConfig(t, map[string]Status{"cache": StatusDown})
	route := &gateRoute{method: http.MethodGet, path: "/users/{id}",
		deps: []DepRequirement{NewSoftRequirement("cache")}}

	factory := DependencyGateFactory(cfg)
	mw := factory(rx.RouteInfo{Route: route, Router: "default"})
	if mw == nil {
		t.Fatal("no middleware attached")
	}

	var degraded []string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsc := GetDepStateContext(r.Context()); dsc != nil {
			degraded = dsc.DegradedDeps
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := withMatchedRoute(httptest.NewRequest(http.MethodGet, "/users/42", nil), route)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("a soft dependency refused the request: status %d", rec.Code)
	}
	if len(degraded) != 1 || degraded[0] != "cache" {
		t.Fatalf("degraded dependencies = %v, want [cache]", degraded)
	}
}

// TestGate_refusal_is_a_problem_document and does not name the dependency.
//
// "cache is down" or "payments-api is degraded" maps the internal service
// topology for any caller who probes endpoints during an outage (D30).
func TestGate_refusal_is_a_problem_document(t *testing.T) {
	cfg := gateConfig(t, map[string]Status{"payments-api": StatusDown})
	route := &gateRoute{method: http.MethodPost, path: "/orders",
		deps: []DepRequirement{NewHardRequirement("payments-api")}}

	rec, _ := serveThrough(t, cfg, route, httptest.NewRequest(http.MethodPost, "/orders", nil))

	if ct := rec.Header().Get("Content-Type"); ct != rx.ProblemMediaType {
		t.Fatalf("Content-Type = %q, want %q", ct, rx.ProblemMediaType)
	}
	body := rec.Body.String()
	if !containsStr(body, rx.ProblemDependencyUnavailable) {
		t.Errorf("the problem type should be %q: %s", rx.ProblemDependencyUnavailable, body)
	}
	if containsStr(body, "payments-api") {
		t.Errorf("the problem document names the failing dependency, mapping the internal topology: %s", body)
	}
}

// containsStr is a small substring helper.
func containsStr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
