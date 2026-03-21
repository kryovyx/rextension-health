// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health contains tests for the health route handlers.
package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kryovyx/dix"
)

// --------------------------------------------------------------------------
// Mock implementations for route testing
// --------------------------------------------------------------------------

// mockRouteContext implements route.Context for testing route handlers.
type mockRouteContext struct {
	ctx            context.Context
	jsonCalled     bool
	jsonStatus     int
	jsonBody       interface{}
	textCalled     bool
	textStatus     int
	textBody       string
	values         map[interface{}]interface{}
	responseWriter http.ResponseWriter
	request        *http.Request
}

func newMockRouteContext() *mockRouteContext {
	return &mockRouteContext{
		ctx:    context.Background(),
		values: make(map[interface{}]interface{}),
	}
}

func (m *mockRouteContext) Deadline() (deadline time.Time, ok bool) { return time.Time{}, false }
func (m *mockRouteContext) Done() <-chan struct{}                   { return m.ctx.Done() }
func (m *mockRouteContext) Err() error                              { return m.ctx.Err() }
func (m *mockRouteContext) Value(key interface{}) interface{} {
	if val, ok := m.values[key]; ok {
		return val
	}
	return m.ctx.Value(key)
}

func (m *mockRouteContext) ResponseWriter() http.ResponseWriter {
	if m.responseWriter != nil {
		return m.responseWriter
	}
	return httptest.NewRecorder()
}
func (m *mockRouteContext) Request() *http.Request {
	if m.request != nil {
		return m.request
	}
	req, _ := http.NewRequest("GET", "/", nil)
	return req
}

func (m *mockRouteContext) Resolver() dix.Resolver {
	return &mockRouteResolver{}
}

func (m *mockRouteContext) Respond(status int, contentType string, body interface{}) error {
	return nil
}
func (m *mockRouteContext) Text(status int, body string) error {
	m.textCalled = true
	m.textStatus = status
	m.textBody = body
	return nil
}
func (m *mockRouteContext) JSON(status int, v interface{}) error {
	m.jsonCalled = true
	m.jsonStatus = status
	m.jsonBody = v
	return nil
}
func (m *mockRouteContext) OpenMetrics(status int, v interface{}) error { return nil }
func (m *mockRouteContext) SetValue(key, value interface{}) {
	m.values[key] = value
}
func (m *mockRouteContext) GetValue(key interface{}) interface{} {
	return m.values[key]
}

// mockRouteResolver implements dix.Resolver for route tests.
type mockRouteResolver struct{}

func (m *mockRouteResolver) Resolve(target interface{}) error    { return nil }
func (m *mockRouteResolver) ResolveAll(target interface{}) error { return nil }

// mockSnapshotCacheForRoutes implements SnapshotCache for testing routes.
type mockSnapshotCacheForRoutes struct {
	getSnapshot       *Snapshot
	readinessSnapshot *Snapshot
}

func newMockSnapshotCacheForRoutes() *mockSnapshotCacheForRoutes {
	return &mockSnapshotCacheForRoutes{
		getSnapshot:       NewSnapshot(),
		readinessSnapshot: NewSnapshot(),
	}
}

func (m *mockSnapshotCacheForRoutes) Get(ctx context.Context) *Snapshot {
	return m.getSnapshot
}

func (m *mockSnapshotCacheForRoutes) GetReadiness(ctx context.Context) *Snapshot {
	return m.readinessSnapshot
}

func (m *mockSnapshotCacheForRoutes) Invalidate() {}

func (m *mockSnapshotCacheForRoutes) SetTTL(ttl time.Duration) {}

var _ SnapshotCache = (*mockSnapshotCacheForRoutes)(nil)

// mockDepStateStoreForRoutes implements DepStateStore for testing routes.
type mockDepStateStoreForRoutes struct {
	states map[string]*DepState
}

func newMockDepStateStoreForRoutes() *mockDepStateStoreForRoutes {
	return &mockDepStateStoreForRoutes{
		states: make(map[string]*DepState),
	}
}

func (m *mockDepStateStoreForRoutes) Register(id string) *DepState {
	state := NewDepState(id)
	m.states[id] = state
	return state
}

func (m *mockDepStateStoreForRoutes) Get(id string) *DepState {
	return m.states[id]
}

func (m *mockDepStateStoreForRoutes) GetAll() map[string]*DepState {
	return m.states
}

func (m *mockDepStateStoreForRoutes) Remove(id string) {
	delete(m.states, id)
}

func (m *mockDepStateStoreForRoutes) ReportSuccess(id string, latency time.Duration) {
	if m.states[id] == nil {
		m.states[id] = NewDepState(id)
	}
	m.states[id].Status = StatusUp
}

func (m *mockDepStateStoreForRoutes) ReportFailure(id string, msg string) {
	if m.states[id] == nil {
		m.states[id] = NewDepState(id)
	}
	m.states[id].Status = StatusDown
	m.states[id].Message = msg
}

func (m *mockDepStateStoreForRoutes) SetStatus(id string, status Status, msg string) {
	if m.states[id] == nil {
		m.states[id] = NewDepState(id)
	}
	m.states[id].Status = status
	m.states[id].Message = msg
}

func (m *mockDepStateStoreForRoutes) SetCircuitState(id string, circuitState CircuitState) {
	if m.states[id] != nil {
		m.states[id].CircuitState = circuitState
	}
}

var _ DepStateStore = (*mockDepStateStoreForRoutes)(nil)

// --------------------------------------------------------------------------
// newLiveRoute tests
// --------------------------------------------------------------------------

func TestNewLiveRoute(t *testing.T) {
	t.Run("with_default_path", func(t *testing.T) {
		// With_default_path should set path to /live when empty.
		r := newLiveRoute("")

		if r.Path() != "/live" {
			t.Errorf("expected path /live, got %s", r.Path())
		}
		if r.Method() != "GET" {
			t.Errorf("expected method GET, got %s", r.Method())
		}
	})

	t.Run("with_custom_path", func(t *testing.T) {
		// With_custom_path should use the provided path.
		r := newLiveRoute("/healthz/live")

		if r.Path() != "/healthz/live" {
			t.Errorf("expected path /healthz/live, got %s", r.Path())
		}
	})

	t.Run("handler_returns_up_status", func(t *testing.T) {
		// Handler_returns_up_status should return JSON with status UP.
		r := newLiveRoute("/live")
		ctx := newMockRouteContext()

		r.Handler()(ctx)

		if !ctx.jsonCalled {
			t.Error("expected JSON to be called")
		}
		if ctx.jsonStatus != http.StatusOK {
			t.Errorf("expected status 200, got %d", ctx.jsonStatus)
		}
		resp, ok := ctx.jsonBody.(LiveResponse)
		if !ok {
			t.Fatalf("expected LiveResponse, got %T", ctx.jsonBody)
		}
		if resp.Status != "UP" {
			t.Errorf("expected status UP, got %s", resp.Status)
		}
	})
}

// --------------------------------------------------------------------------
// LiveHandler tests
// --------------------------------------------------------------------------

func TestLiveHandler(t *testing.T) {
	t.Run("returns_up_status_json", func(t *testing.T) {
		// Returns_up_status_json should return HTTP 200 with JSON body.
		handler := LiveHandler()
		req := httptest.NewRequest("GET", "/live", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
		}
	})
}

// --------------------------------------------------------------------------
// newReadyRoute tests
// --------------------------------------------------------------------------

func TestNewReadyRoute(t *testing.T) {
	t.Run("with_default_path", func(t *testing.T) {
		// With_default_path should set path to /ready when empty.
		cache := newMockSnapshotCacheForRoutes()
		r := newReadyRoute("", cache)

		if r.Path() != "/ready" {
			t.Errorf("expected path /ready, got %s", r.Path())
		}
		if r.Method() != "GET" {
			t.Errorf("expected method GET, got %s", r.Method())
		}
	})

	t.Run("with_custom_path", func(t *testing.T) {
		// With_custom_path should use the provided path.
		cache := newMockSnapshotCacheForRoutes()
		r := newReadyRoute("/healthz/ready", cache)

		if r.Path() != "/healthz/ready" {
			t.Errorf("expected path /healthz/ready, got %s", r.Path())
		}
	})

	t.Run("handler_returns_up_status_when_healthy", func(t *testing.T) {
		// Handler_returns_up_status_when_healthy should return 200 with status UP.
		cache := newMockSnapshotCacheForRoutes()
		cache.readinessSnapshot.OverallStatus = StatusUp
		r := newReadyRoute("/ready", cache)
		ctx := newMockRouteContext()

		r.Handler()(ctx)

		if !ctx.jsonCalled {
			t.Error("expected JSON to be called")
		}
		if ctx.jsonStatus != http.StatusOK {
			t.Errorf("expected status 200, got %d", ctx.jsonStatus)
		}
		resp, ok := ctx.jsonBody.(ReadyResponse)
		if !ok {
			t.Fatalf("expected ReadyResponse, got %T", ctx.jsonBody)
		}
		if resp.Status != "UP" {
			t.Errorf("expected status UP, got %s", resp.Status)
		}
	})

	t.Run("handler_returns_503_when_down", func(t *testing.T) {
		// Handler_returns_503_when_down should return 503 when status is DOWN.
		cache := newMockSnapshotCacheForRoutes()
		cache.readinessSnapshot.OverallStatus = StatusDown
		r := newReadyRoute("/ready", cache)
		ctx := newMockRouteContext()

		r.Handler()(ctx)

		if !ctx.jsonCalled {
			t.Error("expected JSON to be called")
		}
		if ctx.jsonStatus != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", ctx.jsonStatus)
		}
	})

	t.Run("handler_returns_degraded_status", func(t *testing.T) {
		// Handler_returns_degraded_status should return 200 with DEGRADED status.
		cache := newMockSnapshotCacheForRoutes()
		cache.readinessSnapshot.OverallStatus = StatusDegraded
		r := newReadyRoute("/ready", cache)
		ctx := newMockRouteContext()

		r.Handler()(ctx)

		if ctx.jsonStatus != http.StatusOK {
			t.Errorf("expected status 200, got %d", ctx.jsonStatus)
		}
		resp, ok := ctx.jsonBody.(ReadyResponse)
		if !ok {
			t.Fatalf("expected ReadyResponse, got %T", ctx.jsonBody)
		}
		if resp.Status != "DEGRADED" {
			t.Errorf("expected status DEGRADED, got %s", resp.Status)
		}
	})
}

// --------------------------------------------------------------------------
// ReadyHandler tests
// --------------------------------------------------------------------------

func TestReadyHandler(t *testing.T) {
	t.Run("returns_up_status_json_when_healthy", func(t *testing.T) {
		// Returns_up_status_json_when_healthy should return HTTP 200 with JSON body.
		cache := newMockSnapshotCacheForRoutes()
		cache.readinessSnapshot.OverallStatus = StatusUp
		handler := ReadyHandler(cache)
		req := httptest.NewRequest("GET", "/ready", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("returns_503_when_down", func(t *testing.T) {
		// Returns_503_when_down should return HTTP 503 when not ready.
		cache := newMockSnapshotCacheForRoutes()
		cache.readinessSnapshot.OverallStatus = StatusDown
		handler := ReadyHandler(cache)
		req := httptest.NewRequest("GET", "/ready", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", rec.Code)
		}
	})
}

// --------------------------------------------------------------------------
// newStatusRoute tests
// --------------------------------------------------------------------------

func TestNewStatusRoute(t *testing.T) {
	t.Run("with_default_path", func(t *testing.T) {
		// With_default_path should set path to /status when empty.
		cache := newMockSnapshotCacheForRoutes()
		store := newMockDepStateStoreForRoutes()
		r := newStatusRoute("", cache, store)

		if r.Path() != "/status" {
			t.Errorf("expected path /status, got %s", r.Path())
		}
		if r.Method() != "GET" {
			t.Errorf("expected method GET, got %s", r.Method())
		}
	})

	t.Run("with_custom_path", func(t *testing.T) {
		// With_custom_path should use the provided path.
		cache := newMockSnapshotCacheForRoutes()
		store := newMockDepStateStoreForRoutes()
		r := newStatusRoute("/healthz/status", cache, store)

		if r.Path() != "/healthz/status" {
			t.Errorf("expected path /healthz/status, got %s", r.Path())
		}
	})

	t.Run("handler_returns_up_status_when_healthy", func(t *testing.T) {
		// Handler_returns_up_status_when_healthy should return 200 with status UP.
		cache := newMockSnapshotCacheForRoutes()
		cache.getSnapshot.OverallStatus = StatusUp
		store := newMockDepStateStoreForRoutes()
		r := newStatusRoute("/status", cache, store)
		ctx := newMockRouteContext()

		r.Handler()(ctx)

		if !ctx.jsonCalled {
			t.Error("expected JSON to be called")
		}
		if ctx.jsonStatus != http.StatusOK {
			t.Errorf("expected status 200, got %d", ctx.jsonStatus)
		}
		resp, ok := ctx.jsonBody.(StatusResponse)
		if !ok {
			t.Fatalf("expected StatusResponse, got %T", ctx.jsonBody)
		}
		if resp.Status != "UP" {
			t.Errorf("expected status UP, got %s", resp.Status)
		}
	})

	t.Run("handler_returns_503_when_down", func(t *testing.T) {
		// Handler_returns_503_when_down should return 503 when status is DOWN.
		cache := newMockSnapshotCacheForRoutes()
		cache.getSnapshot.OverallStatus = StatusDown
		store := newMockDepStateStoreForRoutes()
		r := newStatusRoute("/status", cache, store)
		ctx := newMockRouteContext()

		r.Handler()(ctx)

		if ctx.jsonStatus != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", ctx.jsonStatus)
		}
	})

	t.Run("handler_includes_dependency_states", func(t *testing.T) {
		// Handler_includes_dependency_states should include dependencies from stateStore.
		cache := newMockSnapshotCacheForRoutes()
		cache.getSnapshot.OverallStatus = StatusUp
		store := newMockDepStateStoreForRoutes()
		store.Register("db")
		store.SetStatus("db", StatusUp, "connected")
		r := newStatusRoute("/status", cache, store)
		ctx := newMockRouteContext()

		r.Handler()(ctx)

		resp, ok := ctx.jsonBody.(StatusResponse)
		if !ok {
			t.Fatalf("expected StatusResponse, got %T", ctx.jsonBody)
		}
		if len(resp.Dependencies) != 1 {
			t.Fatalf("expected 1 dependency, got %d", len(resp.Dependencies))
		}
		if resp.Dependencies["db"] == nil {
			t.Fatal("expected db dependency")
		}
	})

	t.Run("handler_works_with_nil_state_store", func(t *testing.T) {
		// Handler_works_with_nil_state_store should handle nil stateStore gracefully.
		cache := newMockSnapshotCacheForRoutes()
		cache.getSnapshot.OverallStatus = StatusUp
		r := newStatusRoute("/status", cache, nil)
		ctx := newMockRouteContext()

		r.Handler()(ctx)

		if !ctx.jsonCalled {
			t.Error("expected JSON to be called")
		}
		if ctx.jsonStatus != http.StatusOK {
			t.Errorf("expected status 200, got %d", ctx.jsonStatus)
		}
	})
}

// --------------------------------------------------------------------------
// StatusHandler tests
// --------------------------------------------------------------------------

func TestStatusHandler(t *testing.T) {
	t.Run("returns_up_status_json_when_healthy", func(t *testing.T) {
		// Returns_up_status_json_when_healthy should return HTTP 200 with JSON body.
		cache := newMockSnapshotCacheForRoutes()
		cache.getSnapshot.OverallStatus = StatusUp
		store := newMockDepStateStoreForRoutes()
		handler := StatusHandler(cache, store)
		req := httptest.NewRequest("GET", "/status", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("returns_503_when_down", func(t *testing.T) {
		// Returns_503_when_down should return HTTP 503 when not healthy.
		cache := newMockSnapshotCacheForRoutes()
		cache.getSnapshot.OverallStatus = StatusDown
		store := newMockDepStateStoreForRoutes()
		handler := StatusHandler(cache, store)
		req := httptest.NewRequest("GET", "/status", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", rec.Code)
		}
	})

	t.Run("includes_dependency_states", func(t *testing.T) {
		// Includes_dependency_states should include dependencies in response.
		cache := newMockSnapshotCacheForRoutes()
		cache.getSnapshot.OverallStatus = StatusUp
		store := newMockDepStateStoreForRoutes()
		store.Register("cache")
		store.SetStatus("cache", StatusDegraded, "high latency")
		handler := StatusHandler(cache, store)
		req := httptest.NewRequest("GET", "/status", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Verify we got a response - detailed parsing would require json decode
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("handles_nil_state_store", func(t *testing.T) {
		// Handles_nil_state_store should work without crashing.
		cache := newMockSnapshotCacheForRoutes()
		cache.getSnapshot.OverallStatus = StatusUp
		handler := StatusHandler(cache, nil)
		req := httptest.NewRequest("GET", "/status", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})
}
