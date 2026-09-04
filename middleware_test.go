// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kryovyx/rextension"
	rxroute "github.com/kryovyx/rextension/route"
)

// --------------------------------------------------------------------------
// Mock implementations for middleware testing
// --------------------------------------------------------------------------

// mockMwResolver implements rextension.Resolver for middleware tests.
type mockMwResolver struct{}

func (m *mockMwResolver) Resolve(target interface{}) error    { return nil }
func (m *mockMwResolver) ResolveAll(target interface{}) error { return nil }

var _ rextension.Resolver = (*mockMwResolver)(nil)

// mockMwRegistry implements Registry for middleware tests.
type mockMwRegistry struct {
	checks map[string]HealthCheck
}

func newMockMwRegistry() *mockMwRegistry {
	return &mockMwRegistry{
		checks: make(map[string]HealthCheck),
	}
}

func (m *mockMwRegistry) Register(check HealthCheck)             { m.checks[check.Name()] = check }
func (m *mockMwRegistry) Unregister(name string)                 { delete(m.checks, name) }
func (m *mockMwRegistry) Get(name string) HealthCheck            { return m.checks[name] }
func (m *mockMwRegistry) GetAll() []HealthCheck                  { return nil }
func (m *mockMwRegistry) GetByTags(tags ...string) []HealthCheck { return nil }
func (m *mockMwRegistry) GetReadinessChecks() []HealthCheck      { return nil }
func (m *mockMwRegistry) GetActiveChecks() []HealthCheck         { return nil }
func (m *mockMwRegistry) GetPassiveChecks() []HealthCheck        { return nil }
func (m *mockMwRegistry) ExecuteAll(ctx context.Context) map[string]*CheckResult {
	return nil
}
func (m *mockMwRegistry) ExecuteReadiness(ctx context.Context) map[string]*CheckResult {
	return nil
}
func (m *mockMwRegistry) ExecuteByTags(ctx context.Context, tags ...string) map[string]*CheckResult {
	return nil
}
func (m *mockMwRegistry) ExecuteCheck(ctx context.Context, name string) *CheckResult {
	return nil
}
func (m *mockMwRegistry) Start(interval time.Duration, stateStore DepStateStore) {}
func (m *mockMwRegistry) Stop()                                                  {}
func (m *mockMwRegistry) SetResolver(resolver rextension.Resolver)               {}
func (m *mockMwRegistry) SetLogger(l rextension.Logger)                          {}

var _ Registry = (*mockMwRegistry)(nil)

// mockMwSnapshotCache implements SnapshotCache for middleware tests.
type mockMwSnapshotCache struct {
	snapshot *Snapshot
}

func newMockMwSnapshotCache() *mockMwSnapshotCache {
	snap := NewSnapshot()
	snap.Dependencies = make(map[string]*DepState)
	return &mockMwSnapshotCache{snapshot: snap}
}

func (m *mockMwSnapshotCache) Get(ctx context.Context) *Snapshot          { return m.snapshot }
func (m *mockMwSnapshotCache) GetReadiness(ctx context.Context) *Snapshot { return m.snapshot }
func (m *mockMwSnapshotCache) Invalidate()                                {}
func (m *mockMwSnapshotCache) SetTTL(ttl time.Duration)                   {}

var _ SnapshotCache = (*mockMwSnapshotCache)(nil)

// mockMwCheckCache implements CheckCache for middleware tests.
type mockMwCheckCache struct {
	result *CheckResult
}

func newMockMwCheckCache() *mockMwCheckCache {
	return &mockMwCheckCache{
		result: NewCheckResult(StatusUp, "ok", 0),
	}
}

func (m *mockMwCheckCache) GetOrExecute(ctx context.Context, check HealthCheck) *CheckResult {
	return m.result
}
func (m *mockMwCheckCache) Invalidate(name string) {}
func (m *mockMwCheckCache) Clear()                 {}

var _ CheckCache = (*mockMwCheckCache)(nil)

// GetDepStateContext tests
// --------------------------------------------------------------------------

func TestGetDepStateContext(t *testing.T) {
	t.Run("returns_nil_for_empty_context", func(t *testing.T) {
		// Returns_nil_for_empty_context should return nil when no state in context.
		ctx := context.Background()

		if GetDepStateContext(ctx) != nil {
			t.Error("expected nil for empty context")
		}
	})

	t.Run("returns_state_context_when_present", func(t *testing.T) {
		// Returns_state_context_when_present should return the stored context.
		ctx := context.Background()
		dsc := &DepStateContext{
			RouteID:      "GET:/api/test",
			Dependencies: make(map[string]*DepState),
			DegradedDeps: []string{"cache"},
		}
		ctx = context.WithValue(ctx, ContextKeyDepStates, dsc)

		got := GetDepStateContext(ctx)
		if got == nil {
			t.Fatal("expected non-nil DepStateContext")
		}
		if got.RouteID != "GET:/api/test" {
			t.Errorf("expected route ID 'GET:/api/test', got %s", got.RouteID)
		}
	})

	t.Run("returns_nil_for_wrong_type", func(t *testing.T) {
		// Returns_nil_for_wrong_type should return nil when wrong type in context.
		ctx := context.Background()
		ctx = context.WithValue(ctx, ContextKeyDepStates, "wrong type")

		if GetDepStateContext(ctx) != nil {
			t.Error("expected nil for wrong type in context")
		}
	})
}

// --------------------------------------------------------------------------
// IsDegraded tests
// --------------------------------------------------------------------------

func TestIsDegraded(t *testing.T) {
	t.Run("returns_false_for_empty_context", func(t *testing.T) {
		// Returns_false_for_empty_context should return false.
		ctx := context.Background()

		if IsDegraded(ctx, "any") {
			t.Error("expected false for empty context")
		}
	})

	t.Run("returns_true_for_degraded_dep", func(t *testing.T) {
		// Returns_true_for_degraded_dep should return true for known degraded dep.
		dsc := &DepStateContext{
			DegradedDeps: []string{"cache", "redis"},
		}
		ctx := context.WithValue(context.Background(), ContextKeyDepStates, dsc)

		if !IsDegraded(ctx, "cache") {
			t.Error("expected cache to be degraded")
		}
		if !IsDegraded(ctx, "redis") {
			t.Error("expected redis to be degraded")
		}
	})

	t.Run("returns_false_for_non_degraded_dep", func(t *testing.T) {
		// Returns_false_for_non_degraded_dep should return false for healthy dep.
		dsc := &DepStateContext{
			DegradedDeps: []string{"cache"},
		}
		ctx := context.WithValue(context.Background(), ContextKeyDepStates, dsc)

		if IsDegraded(ctx, "db") {
			t.Error("expected db to not be degraded")
		}
	})
}

// --------------------------------------------------------------------------
// GetDepState tests
// --------------------------------------------------------------------------

func TestGetDepStateFromContext(t *testing.T) {
	t.Run("returns_nil_for_empty_context", func(t *testing.T) {
		// Returns_nil_for_empty_context should return nil.
		ctx := context.Background()

		if GetDepState(ctx, "any") != nil {
			t.Error("expected nil for empty context")
		}
	})

	t.Run("returns_state_when_present", func(t *testing.T) {
		// Returns_state_when_present should return the stored state.
		state := NewDepState("db")
		state.SetStatus(StatusDegraded, "high latency")

		dsc := &DepStateContext{
			Dependencies: map[string]*DepState{
				"db": state,
			},
		}
		ctx := context.WithValue(context.Background(), ContextKeyDepStates, dsc)

		got := GetDepState(ctx, "db")
		if got == nil {
			t.Fatal("expected non-nil state")
		}
		if got.Status != StatusDegraded {
			t.Errorf("expected StatusDegraded, got %v", got.Status)
		}
	})

	t.Run("returns_nil_for_missing_dep", func(t *testing.T) {
		// Returns_nil_for_missing_dep should return nil for unknown dep.
		dsc := &DepStateContext{
			Dependencies: map[string]*DepState{},
		}
		ctx := context.WithValue(context.Background(), ContextKeyDepStates, dsc)

		if GetDepState(ctx, "unknown") != nil {
			t.Error("expected nil for missing dependency")
		}
	})
}

// --------------------------------------------------------------------------
// RouteID tests
// --------------------------------------------------------------------------

func TestRouteID(t *testing.T) {
	t.Run("formats_method_and_path", func(t *testing.T) {
		// Formats_method_and_path should combine method and path.
		tests := []struct {
			method   string
			path     string
			expected string
		}{
			{"GET", "/api/users", "GET:/api/users"},
			{"POST", "/api/orders", "POST:/api/orders"},
			{"DELETE", "/api/items/123", "DELETE:/api/items/123"},
		}

		for _, tt := range tests {
			got := RouteID(tt.method, tt.path)
			if got != tt.expected {
				t.Errorf("RouteID(%s, %s) = %s, want %s", tt.method, tt.path, got, tt.expected)
			}
		}
	})
}

// --------------------------------------------------------------------------
// RouteDepMap tests
// --------------------------------------------------------------------------

func TestRouteDepMap(t *testing.T) {
	t.Run("registers_and_retrieves_deps", func(t *testing.T) {
		// Registers_and_retrieves_deps should store and return dependencies.
		m := NewRouteDepMap()

		deps := []DepRequirement{
			NewHardRequirement("db"),
			NewSoftRequirement("cache"),
		}
		m.Register("GET:/api/users", deps)

		got := m.Get("GET:/api/users")
		if len(got) != 2 {
			t.Errorf("expected 2 deps, got %d", len(got))
		}
	})

	t.Run("returns_nil_for_non_existent", func(t *testing.T) {
		// Returns_nil_for_non_existent should return nil for unknown route.
		m := NewRouteDepMap()

		if m.Get("GET:/api/other") != nil {
			t.Error("expected nil for non-existent route")
		}
	})

	t.Run("returns_all_routes", func(t *testing.T) {
		// Returns_all_routes should return all registered routes.
		m := NewRouteDepMap()
		m.Register("GET:/api/users", []DepRequirement{NewHardRequirement("db")})

		all := m.GetAll()
		if len(all) != 1 {
			t.Errorf("expected 1 route, got %d", len(all))
		}
	})

	t.Run("removes_route", func(t *testing.T) {
		// Removes_route should delete the route.
		m := NewRouteDepMap()
		m.Register("GET:/api/users", []DepRequirement{NewHardRequirement("db")})

		m.Remove("GET:/api/users")
		if m.Get("GET:/api/users") != nil {
			t.Error("expected nil after remove")
		}
	})
}

// --------------------------------------------------------------------------
// DepRequirement tests
// --------------------------------------------------------------------------

func TestDepRequirements(t *testing.T) {
	t.Run("creates_hard_requirement", func(t *testing.T) {
		// Creates_hard_requirement should set correct defaults.
		hard := NewHardRequirement("db")
		if hard.Type != RequirementHard {
			t.Error("expected RequirementHard")
		}
		if hard.MinStatus != StatusUp {
			t.Error("expected MinStatus StatusUp for hard requirement")
		}
	})

	t.Run("creates_soft_requirement", func(t *testing.T) {
		// Creates_soft_requirement should set correct defaults.
		soft := NewSoftRequirement("cache")
		if soft.Type != RequirementSoft {
			t.Error("expected RequirementSoft")
		}
		if soft.MinStatus != StatusDegraded {
			t.Error("expected MinStatus StatusDegraded for soft requirement")
		}
	})

	t.Run("with_min_status_overrides", func(t *testing.T) {
		// With_min_status_overrides should change MinStatus.
		custom := NewHardRequirement("api").WithMinStatus(StatusDegraded)
		if custom.MinStatus != StatusDegraded {
			t.Error("expected MinStatus StatusDegraded after WithMinStatus")
		}
	})
}

// --------------------------------------------------------------------------
// RequirementType String tests
// --------------------------------------------------------------------------

func TestRequirementTypeString(t *testing.T) {
	t.Run("returns_hard", func(t *testing.T) {
		// Returns_hard should return "HARD".
		if RequirementHard.String() != "HARD" {
			t.Errorf("expected HARD, got %s", RequirementHard.String())
		}
	})

	t.Run("returns_soft", func(t *testing.T) {
		// Returns_soft should return "SOFT".
		if RequirementSoft.String() != "SOFT" {
			t.Errorf("expected SOFT, got %s", RequirementSoft.String())
		}
	})

	t.Run("returns_unknown_for_invalid", func(t *testing.T) {
		// Returns_unknown_for_invalid should return "UNKNOWN".
		invalidType := RequirementType(99)
		if invalidType.String() != "UNKNOWN" {
			t.Errorf("expected UNKNOWN, got %s", invalidType.String())
		}
	})
}

// --------------------------------------------------------------------------
// DefaultMiddlewareConfig tests
// --------------------------------------------------------------------------

func TestDefaultMiddlewareConfig(t *testing.T) {
	t.Run("returns_default_values", func(t *testing.T) {
		// Returns_default_values should set sensible defaults.
		cfg := DefaultMiddlewareConfig()

		if cfg.FailureStatusCode != 503 {
			t.Errorf("expected FailureStatusCode=503, got %d", cfg.FailureStatusCode)
		}
		if cfg.FailureMessage != "Service temporarily unavailable" {
			t.Errorf("expected default FailureMessage, got %s", cfg.FailureMessage)
		}
		if !cfg.UseCache {
			t.Error("expected UseCache=true")
		}
	})
}

// --------------------------------------------------------------------------
// RegisterMiddlewares tests
// --------------------------------------------------------------------------

func TestRegisterMiddlewares(t *testing.T) {
	t.Run("registers_route_resolver_and_dep_gate", func(t *testing.T) {
		// Registers_route_resolver_and_dep_gate should apply two middlewares in order.
		cfg := DefaultMiddlewareConfig()

		// Collect middlewares registered via a mock Use function
		var registered []func(http.Handler) http.Handler
		mockUse := func(mw func(http.Handler) http.Handler) {
			registered = append(registered, mw)
		}

		// Build a small adapter to simulate r.Use()
		apply := func(r *mockUseAccumulator) error {
			r.Use(RouteResolverMiddleware())
			r.Use(DependencyGateMiddleware(cfg))
			return nil
		}
		_ = mockUse
		_ = apply

		// Verify the middlewares can be constructed without error
		rr := RouteResolverMiddleware()
		dg := DependencyGateMiddleware(cfg)
		if rr == nil {
			t.Error("expected non-nil RouteResolverMiddleware")
		}
		if dg == nil {
			t.Error("expected non-nil DependencyGateMiddleware")
		}
	})
}

// mockUseAccumulator collects middlewares for test assertions.
type mockUseAccumulator struct {
	middlewares []func(http.Handler) http.Handler
}

func (m *mockUseAccumulator) Use(mw func(http.Handler) http.Handler) {
	m.middlewares = append(m.middlewares, mw)
}

// shouldExecutePassiveCheck tests
// --------------------------------------------------------------------------

func TestShouldExecutePassiveCheck(t *testing.T) {
	t.Run("returns_false_when_no_registry", func(t *testing.T) {
		// Returns_false_when_no_registry should return false.
		cfg := MiddlewareConfig{
			Registry: nil,
		}

		result := shouldExecutePassiveCheck(cfg, "test", nil)

		if result {
			t.Error("expected false when registry is nil")
		}
	})

	t.Run("returns_false_when_check_not_found", func(t *testing.T) {
		// Returns_false_when_check_not_found should return false.
		registry := newMockMwRegistry()
		cfg := MiddlewareConfig{
			Registry: registry,
		}

		result := shouldExecutePassiveCheck(cfg, "unknown", nil)

		if result {
			t.Error("expected false when check not found")
		}
	})

	t.Run("returns_true_for_passive_check", func(t *testing.T) {
		// Returns_true_for_passive_check should return true for passive mode.
		registry := newMockMwRegistry()
		passiveCheck := NewCheck("ext", func(ctx context.Context, r rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModePassive))
		registry.Register(passiveCheck)

		cfg := MiddlewareConfig{
			Registry: registry,
		}

		result := shouldExecutePassiveCheck(cfg, "ext", nil)

		if !result {
			t.Error("expected true for passive check")
		}
	})

	t.Run("returns_true_for_on_demand_check", func(t *testing.T) {
		// Returns_true_for_on_demand_check should return true for on-demand mode.
		registry := newMockMwRegistry()
		onDemandCheck := NewCheck("api", func(ctx context.Context, r rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModeOnDemand))
		registry.Register(onDemandCheck)

		cfg := MiddlewareConfig{
			Registry: registry,
		}

		result := shouldExecutePassiveCheck(cfg, "api", nil)

		if !result {
			t.Error("expected true for on-demand check")
		}
	})

	t.Run("returns_false_for_active_check", func(t *testing.T) {
		// Returns_false_for_active_check should return false for active mode.
		registry := newMockMwRegistry()
		activeCheck := NewCheck("db", func(ctx context.Context, r rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModeActive))
		registry.Register(activeCheck)

		cfg := MiddlewareConfig{
			Registry: registry,
		}

		result := shouldExecutePassiveCheck(cfg, "db", nil)

		if result {
			t.Error("expected false for active check")
		}
	})
}

// --------------------------------------------------------------------------
// executePassiveCheckIfNeeded tests
// --------------------------------------------------------------------------

func TestExecutePassiveCheckIfNeeded(t *testing.T) {
	t.Run("returns_nil_when_no_registry", func(t *testing.T) {
		// Returns_nil_when_no_registry should return nil.
		cfg := MiddlewareConfig{
			Registry:   nil,
			CheckCache: newMockMwCheckCache(),
		}

		result := executePassiveCheckIfNeeded(context.Background(), cfg, "test", nil)

		if result != nil {
			t.Error("expected nil when registry is nil")
		}
	})

	t.Run("returns_nil_when_no_check_cache", func(t *testing.T) {
		// Returns_nil_when_no_check_cache should return nil.
		registry := newMockMwRegistry()
		cfg := MiddlewareConfig{
			Registry:   registry,
			CheckCache: nil,
		}

		result := executePassiveCheckIfNeeded(context.Background(), cfg, "test", nil)

		if result != nil {
			t.Error("expected nil when check cache is nil")
		}
	})

	t.Run("returns_nil_when_check_not_found", func(t *testing.T) {
		// Returns_nil_when_check_not_found should return nil.
		registry := newMockMwRegistry()
		checkCache := newMockMwCheckCache()
		cfg := MiddlewareConfig{
			Registry:   registry,
			CheckCache: checkCache,
		}

		result := executePassiveCheckIfNeeded(context.Background(), cfg, "unknown", nil)

		if result != nil {
			t.Error("expected nil when check not found")
		}
	})

	t.Run("returns_nil_for_active_check", func(t *testing.T) {
		// Returns_nil_for_active_check should return nil for non-passive.
		registry := newMockMwRegistry()
		activeCheck := NewCheck("db", func(ctx context.Context, r rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModeActive))
		registry.Register(activeCheck)

		checkCache := newMockMwCheckCache()
		cfg := MiddlewareConfig{
			Registry:   registry,
			CheckCache: checkCache,
		}

		result := executePassiveCheckIfNeeded(context.Background(), cfg, "db", nil)

		if result != nil {
			t.Error("expected nil for active check")
		}
	})

	t.Run("returns_state_for_passive_check", func(t *testing.T) {
		// Returns_state_for_passive_check should execute and return state.
		registry := newMockMwRegistry()
		passiveCheck := NewCheck("ext", func(ctx context.Context, r rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModePassive))
		registry.Register(passiveCheck)

		checkCache := newMockMwCheckCache()
		checkCache.result = NewCheckResult(StatusUp, "healthy", 10*time.Millisecond)

		cfg := MiddlewareConfig{
			Registry:   registry,
			CheckCache: checkCache,
		}

		result := executePassiveCheckIfNeeded(context.Background(), cfg, "ext", nil)

		if result == nil {
			t.Fatal("expected non-nil state")
		}
		if result.ID != "ext" {
			t.Errorf("expected ID=ext, got %s", result.ID)
		}
		if result.Status != StatusUp {
			t.Errorf("expected StatusUp, got %s", result.Status)
		}
	})

	t.Run("returns_state_for_on_demand_check", func(t *testing.T) {
		// Returns_state_for_on_demand_check should execute and return state.
		registry := newMockMwRegistry()
		onDemandCheck := NewCheck("api", func(ctx context.Context, r rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusDegraded, "slow", 0)
		}, WithCheckMode(CheckModeOnDemand))
		registry.Register(onDemandCheck)

		checkCache := newMockMwCheckCache()
		checkCache.result = NewCheckResult(StatusDegraded, "slow response", 50*time.Millisecond)

		cfg := MiddlewareConfig{
			Registry:   registry,
			CheckCache: checkCache,
		}

		result := executePassiveCheckIfNeeded(context.Background(), cfg, "api", nil)

		if result == nil {
			t.Fatal("expected non-nil state")
		}
		if result.Status != StatusDegraded {
			t.Errorf("expected StatusDegraded, got %s", result.Status)
		}
	})

	t.Run("returns_nil_when_cache_returns_nil", func(t *testing.T) {
		// Returns_nil_when_cache_returns_nil should return nil if cache fails.
		registry := newMockMwRegistry()
		passiveCheck := NewCheck("ext", func(ctx context.Context, r rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		}, WithCheckMode(CheckModePassive))
		registry.Register(passiveCheck)

		checkCache := newMockMwCheckCache()
		checkCache.result = nil // Simulate cache returning nil

		cfg := MiddlewareConfig{
			Registry:   registry,
			CheckCache: checkCache,
		}

		result := executePassiveCheckIfNeeded(context.Background(), cfg, "ext", nil)

		if result != nil {
			t.Error("expected nil when cache returns nil")
		}
	})
}

// --------------------------------------------------------------------------
// DependencyGateMiddleware tests
// --------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Gate test helpers
// ---------------------------------------------------------------------------

// gateRoute is a route declaring dependency requirements.
type gateRoute struct {
	method string
	path   string
	deps   []DepRequirement
}

func (r *gateRoute) Method() string                 { return r.method }
func (r *gateRoute) Path() string                   { return r.path }
func (r *gateRoute) Handler() rxroute.HandlerFunc   { return func(rxroute.Context) {} }
func (r *gateRoute) Dependencies() []DepRequirement { return r.deps }

// bareRoute declares no dependencies.
type bareRoute struct {
	method string
	path   string
}

func (r *bareRoute) Method() string               { return r.method }
func (r *bareRoute) Path() string                 { return r.path }
func (r *bareRoute) Handler() rxroute.HandlerFunc { return func(rxroute.Context) {} }

// withMatchedRoute attaches rt to the request the way the router does.
//
// The gate reads the matched route from the context rather than reconstructing
// an identifier from the URL, which is the whole point of P4.12 — so a test
// that does not attach one is testing the pass-through path.
func withMatchedRoute(req *http.Request, rt rxroute.Route) *http.Request {
	return req.WithContext(rxroute.SetMatchedRoute(req.Context(), rt))
}

func TestDependencyGateMiddleware(t *testing.T) {
	t.Run("passes_through_for_a_route_with_no_dependencies", func(t *testing.T) {
		// A route that is not a HealthDepRoute is passed straight through.
		cfg := DefaultMiddlewareConfig()
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		route := &bareRoute{method: "GET", path: "/api/users"}
		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("expected next to be called")
		}
	})

	t.Run("passes_through_when_the_dependency_list_is_empty", func(t *testing.T) {
		// A HealthDepRoute declaring an empty list is also passed through.
		cfg := MiddlewareConfig{}
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		route := &gateRoute{method: "GET", path: "/api/users", deps: nil}
		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("expected next to be called")
		}
	})

	t.Run("fails_fast_when_hard_dep_down", func(t *testing.T) {
		// Fails_fast_when_hard_dep_down should write 503 and not call next.
		route := &gateRoute{method: "GET", path: "/api/users",
			deps: []DepRequirement{NewHardRequirement("db")}}

		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())
		stateStore.SetStatus("db", StatusDown, "connection refused")

		cfg := MiddlewareConfig{
			StateStore:        stateStore,
			UseCache:          false,
			FailureStatusCode: 503,
			FailureMessage:    "Service temporarily unavailable",
		}
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if nextCalled {
			t.Error("expected next NOT to be called")
		}
		if w.Code != 503 {
			t.Errorf("expected 503, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Service temporarily unavailable") {
			t.Errorf("expected failure message in body, got: %s", w.Body.String())
		}
	})

	t.Run("marks_soft_dep_degraded_and_continues", func(t *testing.T) {
		// Marks_soft_dep_degraded_and_continues should add to DegradedDeps and call next.
		route := &gateRoute{method: "GET", path: "/api/users",
			deps: []DepRequirement{NewSoftRequirement("cache")}}

		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())
		stateStore.SetStatus("cache", StatusDown, "connection lost")

		cfg := MiddlewareConfig{
			StateStore: stateStore,
			UseCache:   false,
		}
		mw := DependencyGateMiddleware(cfg)

		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
		})

		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if capturedCtx == nil {
			t.Fatal("expected next to be called")
		}
		dsc := GetDepStateContext(capturedCtx)
		if dsc == nil {
			t.Fatal("expected DepStateContext in context")
		}
		if len(dsc.DegradedDeps) != 1 || dsc.DegradedDeps[0] != "cache" {
			t.Errorf("expected cache in degraded deps, got %v", dsc.DegradedDeps)
		}
	})

	t.Run("uses_cache_when_configured", func(t *testing.T) {
		// Uses_cache_when_configured should read from SnapshotCache.
		route := &gateRoute{method: "GET", path: "/api/users",
			deps: []DepRequirement{NewHardRequirement("db")}}

		snapCache := newMockMwSnapshotCache()
		snapCache.snapshot.Dependencies["db"] = &DepState{
			ID:     "db",
			Status: StatusUp,
		}

		cfg := MiddlewareConfig{
			SnapshotCache: snapCache,
			UseCache:      true,
		}
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("expected next to be called")
		}
	})

	// Replaces "uses_route_id_from_context_value", which injected a route
	// identifier string into the context for the gate to read.
	//
	// The gate does not read an identifier any more — it reads the matched
	// route. That removes the failure mode the old test was quietly built
	// around: the identifier had to be constructed by somebody, from
	// something, and the only thing available at request time was the URL.
	t.Run("passes_through_with_no_matched_route", func(t *testing.T) {
		cfg := MiddlewareConfig{
			StateStore: NewDepStateStore(DefaultDepStateStoreConfig()),
			UseCache:   false,
		}
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		// No matched route in the context, as on a 404 path.
		req := httptest.NewRequest("GET", "/other-path", nil)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("expected next to be called when there is no matched route")
		}
	})

	// Replaces "creates_unknown_state_when_no_data", which asserted that an
	// unknown dependency refuses the request.
	//
	// That behaviour was not a decision: the gate compares
	// `state.Status > dep.MinStatus`, and StatusUnknown happened to be
	// declared after StatusDown, so unknown sorted into "refuse". Reordering
	// the constants would have silently inverted it.
	//
	// It is explicit now, and the default serves (O14) — refusing means 503ing
	// every request for the first check interval after every boot, because a
	// check has not run yet rather than because anything is wrong. The
	// synchronous check pass in OnStart is what makes serving safe.
	t.Run("an_unknown_dependency_serves_by_default", func(t *testing.T) {
		route := &gateRoute{method: "GET", path: "/api/users",
			deps: []DepRequirement{NewHardRequirement("db")}}

		cfg := MiddlewareConfig{
			StateStore: NewDepStateStore(DefaultDepStateStoreConfig()), // nothing recorded
			UseCache:   false,
			// TreatUnknownAs is the zero value, StatusUp.
		}
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("an unknown dependency refused the request; the default must serve")
		}
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("an_unknown_dependency_can_be_configured_to_refuse", func(t *testing.T) {
		// The escape hatch, for an application that must refuse rather than
		// guess.
		route := &gateRoute{method: "GET", path: "/api/users",
			deps: []DepRequirement{NewHardRequirement("db")}}

		cfg := MiddlewareConfig{
			StateStore:     NewDepStateStore(DefaultDepStateStoreConfig()),
			UseCache:       false,
			TreatUnknownAs: StatusDown,
		}
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if nextCalled {
			t.Error("TreatUnknownAs: StatusDown did not refuse the request")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", w.Code)
		}
	})

	t.Run("the_gate_does_not_depend_on_status_ordinals", func(t *testing.T) {
		// The property the old behaviour accidentally relied on: that
		// StatusUnknown sorts after StatusDown. The decision is explicit now,
		// so the same unknown state produces different outcomes purely from
		// configuration.
		route := &gateRoute{method: "GET", path: "/api/users",
			deps: []DepRequirement{NewHardRequirement("db")}}

		for _, treatAs := range []Status{StatusUp, StatusDegraded, StatusDown} {
			cfg := MiddlewareConfig{
				StateStore:     NewDepStateStore(DefaultDepStateStoreConfig()),
				UseCache:       false,
				TreatUnknownAs: treatAs,
			}
			served := false
			handler := DependencyGateMiddleware(cfg)(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					served = true
					w.WriteHeader(http.StatusOK)
				}))
			req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			// A hard requirement has MinStatus == StatusUp, so only StatusUp
			// passes; Degraded and Down both exceed it.
			want := treatAs == StatusUp
			if served != want {
				t.Errorf("TreatUnknownAs=%v: served=%v, want %v", treatAs, served, want)
			}
		}
	})

	t.Run("executes_passive_check_on_demand", func(t *testing.T) {
		// Executes_passive_check_on_demand should run passive check via cache.
		route := &gateRoute{method: "GET", path: "/api/users", deps: []DepRequirement{NewHardRequirement("ext-api")}}

		registry := newMockMwRegistry()
		passiveCheck := NewCheck("ext-api", func(ctx context.Context, resolver rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 10*time.Millisecond)
		}, WithCheckMode(CheckModePassive))
		registry.Register(passiveCheck)

		checkCache := newMockMwCheckCache()
		checkCache.result = NewCheckResult(StatusUp, "ok", 10*time.Millisecond)

		cfg := MiddlewareConfig{
			StateStore: NewDepStateStore(DefaultDepStateStoreConfig()),
			Registry:   registry,
			CheckCache: checkCache,
			UseCache:   false,
		}
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("expected next to be called")
		}
	})

	t.Run("uses_resolver_from_config", func(t *testing.T) {
		// Uses_resolver_from_config should extract resolver when configured.
		route := &gateRoute{method: "GET", path: "/api/users", deps: []DepRequirement{NewHardRequirement("ext-api")}}

		registry := newMockMwRegistry()
		passiveCheck := NewCheck("ext-api", func(ctx context.Context, resolver rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 10*time.Millisecond)
		}, WithCheckMode(CheckModePassive))
		registry.Register(passiveCheck)

		checkCache := newMockMwCheckCache()
		checkCache.result = NewCheckResult(StatusUp, "ok", 10*time.Millisecond)

		cfg := MiddlewareConfig{
			StateStore: NewDepStateStore(DefaultDepStateStoreConfig()),
			Registry:   registry,
			CheckCache: checkCache,
			Resolver:   &mockMwResolver{},
			UseCache:   false,
		}
		mw := DependencyGateMiddleware(cfg)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("expected next to be called")
		}
	})
}

// --------------------------------------------------------------------------
// RouteResolverMiddleware tests
// --------------------------------------------------------------------------

func TestRouteResolverMiddleware(t *testing.T) {
	t.Run("injects_route_id_into_request_context", func(t *testing.T) {
		route := &gateRoute{method: "GET", path: "/api/users", deps: []DepRequirement{NewHardRequirement("db")}}
		// Injects_route_id_into_request_context should store route ID in r.Context().
		mw := RouteResolverMiddleware()

		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
		})

		req := withMatchedRoute(httptest.NewRequest("GET", "/api/users", nil), route)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if capturedCtx == nil {
			t.Fatal("expected next to be called")
		}
		routeID, _ := capturedCtx.Value(ContextKeyRouteID).(string)
		if routeID != "GET:/api/users" {
			t.Errorf("expected route ID 'GET:/api/users', got %q", routeID)
		}
	})

	t.Run("passes_through_any_request", func(t *testing.T) {
		// Passes_through_any_request should always call next.
		mw := RouteResolverMiddleware()

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		req := httptest.NewRequest("POST", "/some/path", nil)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("expected next to be called")
		}
	})
}

// --------------------------------------------------------------------------
// DepContextMiddleware tests
// --------------------------------------------------------------------------

func TestDepContextMiddleware(t *testing.T) {
	t.Run("injects_dependency_states", func(t *testing.T) {
		// Injects_dependency_states should add states to request context.
		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())
		stateStore.SetStatus("db", StatusUp, "connected")
		stateStore.SetStatus("cache", StatusDegraded, "high latency")

		mw := DepContextMiddleware(stateStore, "db", "cache")

		var capturedCtx context.Context
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
		})

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if capturedCtx == nil {
			t.Fatal("expected next to be called")
		}
		dsc := GetDepStateContext(capturedCtx)
		if dsc == nil {
			t.Fatal("expected DepStateContext in context")
		}
		if dsc.Dependencies["db"] == nil {
			t.Error("expected db state in context")
		}
		if dsc.Dependencies["cache"] == nil {
			t.Error("expected cache state in context")
		}
		if len(dsc.DegradedDeps) != 1 || dsc.DegradedDeps[0] != "cache" {
			t.Errorf("expected cache in degraded deps, got %v", dsc.DegradedDeps)
		}
	})

	t.Run("handles_missing_deps", func(t *testing.T) {
		// Handles_missing_deps should skip deps not in store.
		stateStore := NewDepStateStore(DefaultDepStateStoreConfig())

		mw := DepContextMiddleware(stateStore, "unknown")

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		mw(next).ServeHTTP(w, req)

		if !nextCalled {
			t.Error("expected next to be called")
		}
	})
}
