// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health contains tests for the HealthExtension implementation.
package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/kryovyx/rextension"
	rxevent "github.com/kryovyx/rextension/event"
	rxroute "github.com/kryovyx/rextension/route"
)

// --------------------------------------------------------------------------
// Mock implementations for testing
// --------------------------------------------------------------------------

// mockRex is a minimal Rex implementation for testing.
type mockRex struct {
	container               rextension.Container
	bus                     rxevent.EventBus
	logger                  rextension.Logger
	extensions              []rextension.Extension
	registeredRoutes        []rxroute.Route
	routerRoutes            map[string][]rxroute.Route
	createdRouters          []string
	createRouterErr         error
	registerRouteErr        error
	registerRouteErrOnCall  int // 0 means always fail, > 0 means fail on that call number
	registerRouteCallCount  int
	registerRouterErr       error
	registerRouterErrOnCall int // 0 means always fail, > 0 means fail on that call number
	registerRouterCallCount int
	usedMiddlewares         []func(http.Handler) http.Handler

	routerMiddlewares []mockRouterMW
	perRoute          []mockPerRouteMW
	perRouter         []mockPerRouterMW
}

func newMockRex() *mockRex {
	return &mockRex{
		container:    newMockContainer(),
		bus:          newMockBus(),
		logger:       &mockLoggerImpl{},
		routerRoutes: make(map[string][]rxroute.Route),
	}
}

func (m *mockRex) WithOptions(options ...rextension.Option) rextension.Rex { return m }
func (m *mockRex) WithExtensions(ext ...rextension.Extension) rextension.Rex {
	m.extensions = append(m.extensions, ext...)
	return m
}
func (m *mockRex) WithLogger(l rextension.Logger) rextension.Rex { m.logger = l; return m }
func (m *mockRex) Container() rextension.Container               { return m.container }
func (m *mockRex) RegisterRoute(rt rextension.Route) error {
	m.registerRouteCallCount++
	if m.registerRouteErr != nil {
		// If errOnCall == 0, always fail. If errOnCall > 0, fail only on that call.
		if m.registerRouteErrOnCall == 0 || m.registerRouteCallCount == m.registerRouteErrOnCall {
			return m.registerRouteErr
		}
	}
	if rt, ok := rt.(rxroute.Route); ok {
		m.registeredRoutes = append(m.registeredRoutes, rt)
	}
	return nil
}
func (m *mockRex) RegisterRouteToRouter(rt rextension.Route, routerName string) error {
	m.registerRouterCallCount++
	if m.registerRouterErr != nil {
		// If errOnCall == 0, always fail. If errOnCall > 0, fail only on that call.
		if m.registerRouterErrOnCall == 0 || m.registerRouterCallCount == m.registerRouterErrOnCall {
			return m.registerRouterErr
		}
	}
	if r, ok := rt.(rxroute.Route); ok {
		m.routerRoutes[routerName] = append(m.routerRoutes[routerName], r)
	}
	return nil
}
func (m *mockRex) CreateRouter(name string, cfg rextension.RouterConfig) error {
	if m.createRouterErr != nil {
		return m.createRouterErr
	}
	m.createdRouters = append(m.createdRouters, name)
	return nil
}
func (m *mockRex) Run() error                 { return nil }
func (m *mockRex) Stop() error                { return nil }
func (m *mockRex) EventBus() rxevent.EventBus { return m.bus }
func (m *mockRex) Use(mw rextension.Middleware) {
	m.usedMiddlewares = append(m.usedMiddlewares, mw)
}

// UseOnRouter and UsePerRoute record what the extension attached, and where.
func (m *mockRex) UseOnRouter(name string, mw rextension.Middleware, priority int) {
	m.routerMiddlewares = append(m.routerMiddlewares, mockRouterMW{name: name, mw: mw, priority: priority})
}

func (m *mockRex) UsePerRoute(f rextension.PerRouteMiddleware, priority int) {
	m.perRoute = append(m.perRoute, mockPerRouteMW{factory: f, priority: priority})
}

func (m *mockRex) UsePerRouter(f rextension.PerRouterMiddleware, priority int) {
	m.perRouter = append(m.perRouter, mockPerRouterMW{factory: f, priority: priority})
}

// mockPerRouterMW records a UsePerRouter call.
type mockPerRouterMW struct {
	factory  rextension.PerRouterMiddleware
	priority int
}

// mockRouterMW records a UseOnRouter call.
type mockRouterMW struct {
	name     string
	mw       rextension.Middleware
	priority int
}

// mockPerRouteMW records a UsePerRoute call.
type mockPerRouteMW struct {
	factory  rextension.PerRouteMiddleware
	priority int
}

func (m *mockRex) Logger() rextension.Logger { return m.logger }

var _ rextension.Rex = (*mockRex)(nil)

// mockLoggerImpl is a mock rextension.Logger for testing.
type mockLoggerImpl struct {
	level rextension.LogLevel
}

func (m *mockLoggerImpl) Info(format string, args ...interface{})                    {}
func (m *mockLoggerImpl) Warn(format string, args ...interface{})                    {}
func (m *mockLoggerImpl) Error(format string, args ...interface{})                   {}
func (m *mockLoggerImpl) Debug(format string, args ...interface{})                   {}
func (m *mockLoggerImpl) Trace(format string, args ...interface{})                   {}
func (m *mockLoggerImpl) SetLogLevel(level rextension.LogLevel)                      { m.level = level }
func (m *mockLoggerImpl) WithField(key string, value interface{}) rextension.Logger  { return m }
func (m *mockLoggerImpl) WithFields(fields map[string]interface{}) rextension.Logger { return m }
func (m *mockLoggerImpl) WithError(err error) rextension.Logger                      { return m }

var _ rextension.Logger = (*mockLoggerImpl)(nil)

// mockContainer is a minimal rextension.Container implementation for testing.
type mockContainer struct {
	instances map[interface{}]interface{}
}

func newMockContainer() *mockContainer {
	return &mockContainer{
		instances: make(map[interface{}]interface{}),
	}
}

func (m *mockContainer) Resolve(target interface{}) error    { return nil }
func (m *mockContainer) ResolveAll(target interface{}) error { return nil }
func (m *mockContainer) Singleton(factory any) error         { return nil }
func (m *mockContainer) Scoped(factory any) error            { return nil }
func (m *mockContainer) Transient(factory any) error         { return nil }
func (m *mockContainer) Instance(v any) error                { m.instances[v] = v; return nil }

// Unbind satisfies rextension.Container; the mock never needs to remove a registration.
func (m *mockContainer) Unbind(v any) (bool, error) { return false, nil }

var _ rextension.Container = (*mockContainer)(nil)

// mockBus is a minimal rxevent.EventBus implementation for testing.
type mockBus struct {
	handlers map[string][]rxevent.EventHandler
}

func newMockBus() *mockBus {
	return &mockBus{
		handlers: make(map[string][]rxevent.EventHandler),
	}
}

func (m *mockBus) Subscribe(eventType string, handler rxevent.EventHandler) {
	m.handlers[eventType] = append(m.handlers[eventType], handler)
}

func (m *mockBus) Emit(ev rxevent.Event) {
	for _, h := range m.handlers[ev.Type()] {
		h(ev)
	}
}

func (m *mockBus) SetLogger(l rxevent.BusLogger) {}
func (m *mockBus) Close()                        {}

var _ rxevent.EventBus = (*mockBus)(nil)

// mockRoute is a minimal rxroute.Route implementation for testing.
type mockRoute struct {
	method  string
	path    string
	handler rxroute.HandlerFunc
}

func (m *mockRoute) Method() string               { return m.method }
func (m *mockRoute) Path() string                 { return m.path }
func (m *mockRoute) Handler() rxroute.HandlerFunc { return m.handler }

var _ rxroute.Route = (*mockRoute)(nil)

// mockHealthDepRoute is a mock implementing both rxroute.Route and HealthDepRoute.
type mockHealthDepRoute struct {
	method string
	path   string
	deps   []DepRequirement
}

func (m *mockHealthDepRoute) Method() string                 { return m.method }
func (m *mockHealthDepRoute) Path() string                   { return m.path }
func (m *mockHealthDepRoute) Handler() rxroute.HandlerFunc   { return func(ctx rxroute.Context) {} }
func (m *mockHealthDepRoute) Dependencies() []DepRequirement { return m.deps }

var _ HealthDepRoute = (*mockHealthDepRoute)(nil)

// --------------------------------------------------------------------------
// NewHealthExtension tests
// --------------------------------------------------------------------------

func TestNewHealthExtension(t *testing.T) {
	t.Run("returns_extension_with_default_config", func(t *testing.T) {
		// Returns_extension_with_default_config should use defaults when nil config is provided.
		ext := NewHealthExtension(nil)

		if ext == nil {
			t.Fatal("expected non-nil extension")
		}
		he := ext.(*HealthExtension)
		if he.cfg.LivePath != "/live" {
			t.Fatalf("expected LivePath=/live, got %s", he.cfg.LivePath)
		}
		if he.cfg.ReadyPath != "/ready" {
			t.Fatalf("expected ReadyPath=/ready, got %s", he.cfg.ReadyPath)
		}
		if he.cfg.StatusPath != "/status" {
			t.Fatalf("expected StatusPath=/status, got %s", he.cfg.StatusPath)
		}
	})

	t.Run("returns_extension_with_custom_live_path", func(t *testing.T) {
		// Returns_extension_with_custom_live_path should override path when provided.
		cfg := &Config{LivePath: "/healthz"}
		ext := NewHealthExtension(cfg)

		he := ext.(*HealthExtension)
		if he.cfg.LivePath != "/healthz" {
			t.Fatalf("expected LivePath=/healthz, got %s", he.cfg.LivePath)
		}
	})

	t.Run("returns_extension_with_custom_ready_path", func(t *testing.T) {
		// Returns_extension_with_custom_ready_path should override path when provided.
		cfg := &Config{ReadyPath: "/readyz"}
		ext := NewHealthExtension(cfg)

		he := ext.(*HealthExtension)
		if he.cfg.ReadyPath != "/readyz" {
			t.Fatalf("expected ReadyPath=/readyz, got %s", he.cfg.ReadyPath)
		}
	})

	t.Run("returns_extension_with_custom_status_path", func(t *testing.T) {
		// Returns_extension_with_custom_status_path should override path when provided.
		cfg := &Config{StatusPath: "/health"}
		ext := NewHealthExtension(cfg)

		he := ext.(*HealthExtension)
		if he.cfg.StatusPath != "/health" {
			t.Fatalf("expected StatusPath=/health, got %s", he.cfg.StatusPath)
		}
	})

	t.Run("returns_extension_with_custom_snapshot_ttl", func(t *testing.T) {
		// Returns_extension_with_custom_snapshot_ttl should override TTL when provided.
		cfg := &Config{SnapshotTTL: 30 * time.Second}
		ext := NewHealthExtension(cfg)

		he := ext.(*HealthExtension)
		if he.cfg.SnapshotTTL != 30*time.Second {
			t.Fatalf("expected SnapshotTTL=30s, got %v", he.cfg.SnapshotTTL)
		}
	})

	t.Run("returns_extension_with_at_default_addr", func(t *testing.T) {
		// Returns_extension_with_at_default_addr should set AtDefaultAddr when true.
		cfg := &Config{AtDefaultAddr: true}
		ext := NewHealthExtension(cfg)

		he := ext.(*HealthExtension)
		if !he.cfg.AtDefaultAddr {
			t.Fatal("expected AtDefaultAddr=true")
		}
	})

	t.Run("returns_extension_with_custom_router_config", func(t *testing.T) {
		// Returns_extension_with_custom_router_config should override router address.
		cfg := &Config{Router: rextension.RouterConfig{Addr: ":9999"}}
		ext := NewHealthExtension(cfg)

		he := ext.(*HealthExtension)
		if he.cfg.Router.Addr != ":9999" {
			t.Fatalf("expected Router.Addr=:9999, got %s", he.cfg.Router.Addr)
		}
	})

	t.Run("returns_extension_with_state_store_config", func(t *testing.T) {
		// Returns_extension_with_state_store_config should override state store config.
		cfg := &Config{StateStoreConfig: DepStateStoreConfig{FailureThreshold: 10}}
		ext := NewHealthExtension(cfg)

		he := ext.(*HealthExtension)
		if he.cfg.StateStoreConfig.FailureThreshold != 10 {
			t.Fatalf("expected FailureThreshold=10, got %d", he.cfg.StateStoreConfig.FailureThreshold)
		}
	})

	t.Run("returns_extension_with_enable_dependency_gate", func(t *testing.T) {
		// Returns_extension_with_enable_dependency_gate should set EnableDependencyGate when true.
		cfg := &Config{EnableDependencyGate: true}
		ext := NewHealthExtension(cfg)

		he := ext.(*HealthExtension)
		if !he.cfg.EnableDependencyGate {
			t.Fatal("expected EnableDependencyGate=true")
		}
	})
}

// --------------------------------------------------------------------------
// WithHealth tests
// --------------------------------------------------------------------------

func TestWithHealth(t *testing.T) {
	t.Run("returns_option_function", func(t *testing.T) {
		// Returns_option_function should return a callable Option.
		opt := WithHealth(nil)

		if opt == nil {
			t.Fatal("expected non-nil option")
		}
	})

	t.Run("option_adds_extension_to_rex", func(t *testing.T) {
		// Option_adds_extension_to_rex should register the extension.
		r := newMockRex()
		opt := WithHealth(nil)
		opt(r)

		if len(r.extensions) != 1 {
			t.Fatalf("expected 1 extension, got %d", len(r.extensions))
		}
	})

	t.Run("option_with_custom_config", func(t *testing.T) {
		// Option_with_custom_config should pass config to extension.
		r := newMockRex()
		cfg := &Config{LivePath: "/healthz"}
		opt := WithHealth(cfg)
		opt(r)

		if len(r.extensions) != 1 {
			t.Fatalf("expected 1 extension, got %d", len(r.extensions))
		}
		he := r.extensions[0].(*HealthExtension)
		if he.cfg.LivePath != "/healthz" {
			t.Fatalf("expected LivePath=/healthz, got %s", he.cfg.LivePath)
		}
	})
}

// --------------------------------------------------------------------------
// OnInitialize tests
// --------------------------------------------------------------------------

func TestHealthExtension_OnInitialize(t *testing.T) {
	t.Run("initializes_core_components", func(t *testing.T) {
		// Initializes_core_components should create stateStore, registry, etc.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		err := ext.OnInitialize(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if ext.stateStore == nil {
			t.Fatal("expected non-nil stateStore")
		}
		if ext.registry == nil {
			t.Fatal("expected non-nil registry")
		}
		if ext.snapshotCache == nil {
			t.Fatal("expected non-nil snapshotCache")
		}
		if ext.routeDepMap == nil {
			t.Fatal("expected non-nil routeDepMap")
		}
		if ext.checkCache == nil {
			t.Fatal("expected non-nil checkCache")
		}
	})

	t.Run("sets_router_name_for_dedicated_router", func(t *testing.T) {
		// Sets_router_name_for_dedicated_router should use HealthRouterName when not at default addr.
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()

		err := ext.OnInitialize(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if ext.routerName != HealthRouterName {
			t.Fatalf("expected routerName=%s, got %s", HealthRouterName, ext.routerName)
		}
	})

	t.Run("sets_router_name_for_default_router", func(t *testing.T) {
		// Sets_router_name_for_default_router should use DefaultRouterName when at default addr.
		ext := NewHealthExtension(&Config{AtDefaultAddr: true}).(*HealthExtension)
		r := newMockRex()

		err := ext.OnInitialize(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if ext.routerName != rextension.DefaultRouterName {
			t.Fatalf("expected routerName=%s, got %s", rextension.DefaultRouterName, ext.routerName)
		}
	})

	t.Run("creates_dedicated_router", func(t *testing.T) {
		// Creates_dedicated_router should call CreateRouter when not at default addr.
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()

		err := ext.OnInitialize(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(r.createdRouters) != 1 {
			t.Fatalf("expected 1 created router, got %d", len(r.createdRouters))
		}
		if r.createdRouters[0] != HealthRouterName {
			t.Fatalf("expected router name %s, got %s", HealthRouterName, r.createdRouters[0])
		}
	})

	t.Run("skips_router_creation_when_at_default_addr", func(t *testing.T) {
		// Skips_router_creation_when_at_default_addr should not call CreateRouter.
		ext := NewHealthExtension(&Config{AtDefaultAddr: true}).(*HealthExtension)
		r := newMockRex()

		err := ext.OnInitialize(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(r.createdRouters) != 0 {
			t.Fatalf("expected 0 created routers, got %d", len(r.createdRouters))
		}
	})

	t.Run("ignores_router_already_exists_error", func(t *testing.T) {
		// Two extensions may both want a router; whichever runs second reuses
		// it rather than aborting.
		//
		// Matched with errors.Is against rextension.ErrRouterExists, so the
		// error text no longer has to say "already exists" — this test used to
		// construct errors.New("router already exists") and pass only because
		// the extension did strings.Contains on it (D39).
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()
		r.createRouterErr = fmt.Errorf("%w: health", rextension.ErrRouterExists)

		err := ext.OnInitialize(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("does_not_match_on_message_text", func(t *testing.T) {
		// The other half of D39: an unrelated error that happens to contain
		// "already exists" must not be swallowed.
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()
		r.createRouterErr = errors.New("the config file already exists")

		if err := ext.OnInitialize(context.Background(), r); err == nil {
			t.Fatal("an unrelated error containing \"already exists\" was swallowed")
		}
	})

	t.Run("returns_error_on_router_creation_failure", func(t *testing.T) {
		// Returns_error_on_router_creation_failure should propagate non-exists errors.
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()
		r.createRouterErr = errors.New("connection refused")

		err := ext.OnInitialize(context.Background(), r)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	// The route-registered subscription is gone (P4.12).
	//
	// It built an index from route identifier to dependencies which the gate
	// then looked up per request — keyed by the route's pattern, looked up by
	// the live URL, so it never matched for a parameterized route and the
	// gate silently did nothing. The gate is handed the route directly now.
	t.Run("does_not_subscribe_to_route_registered", func(t *testing.T) {
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		bus := r.bus.(*mockBus)
		if n := len(bus.handlers[rxevent.EventTypeRouterRouteRegistered]); n != 0 {
			t.Fatalf("the gate must not discover routes through events; %d handler(s) registered", n)
		}
	})

	t.Run("registers_the_gate_per_route_when_enabled", func(t *testing.T) {
		ext := NewHealthExtension(&Config{
			AtDefaultAddr:        true,
			EnableDependencyGate: true,
		}).(*HealthExtension)
		r := newMockRex()

		if err := ext.OnInitialize(context.Background(), r); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		if len(r.perRoute) != 1 {
			t.Fatalf("expected 1 per-route factory, got %d", len(r.perRoute))
		}
		// One factory, not two middlewares: RouteResolverMiddleware existed
		// only to give the gate an identifier it no longer needs.
		if len(r.usedMiddlewares) != 0 {
			t.Fatalf("expected no global middleware, got %d", len(r.usedMiddlewares))
		}
		if got := r.perRoute[0].priority; got != rextension.PriorityHealthGate {
			t.Errorf("priority = %d, want PriorityHealthGate (%d)", got, rextension.PriorityHealthGate)
		}
	})

	t.Run("does_not_register_the_gate_when_disabled", func(t *testing.T) {
		ext := NewHealthExtension(&Config{
			AtDefaultAddr:        true,
			EnableDependencyGate: false,
		}).(*HealthExtension)
		r := newMockRex()

		if err := ext.OnInitialize(context.Background(), r); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(r.perRoute) != 0 {
			t.Fatalf("expected 0 per-route factories, got %d", len(r.perRoute))
		}
		if len(r.usedMiddlewares) != 0 {
			t.Fatalf("expected 0 middlewares, got %d", len(r.usedMiddlewares))
		}
	})

	// The two "event_handler_*" subtests that stood here drove the
	// route-registered subscription. There is no subscription: the framework
	// hands each route to the per-route factory when the table is built.
	// The equivalent coverage is in gate_test.go, which exercises the factory
	// directly — including the parameterized-route case that the index-based
	// gate silently never handled.

}

// --------------------------------------------------------------------------
// OnStart tests
// --------------------------------------------------------------------------

func TestHealthExtension_OnStart(t *testing.T) {
	t.Run("registers_routes_at_default_addr", func(t *testing.T) {
		// Registers_routes_at_default_addr should call RegisterRoute for each endpoint.
		ext := NewHealthExtension(&Config{AtDefaultAddr: true}).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(r.registeredRoutes) != 3 {
			t.Fatalf("expected 3 registered routes, got %d", len(r.registeredRoutes))
		}
	})

	t.Run("registers_routes_to_dedicated_router", func(t *testing.T) {
		// Registers_routes_to_dedicated_router should call RegisterRouteToRouter.
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(r.routerRoutes[HealthRouterName]) != 3 {
			t.Fatalf("expected 3 routes on health router, got %d", len(r.routerRoutes[HealthRouterName]))
		}
	})

	t.Run("starts_health_check_ticker_when_interval_configured", func(t *testing.T) {
		// Starts_health_check_ticker_when_interval_configured should call registry.Start.
		ext := NewHealthExtension(&Config{
			AtDefaultAddr: true,
			CheckInterval: 10 * time.Second,
		}).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		// The ticker started; stop it to avoid leaking goroutine
		ext.registry.Stop()
	})

	t.Run("returns_error_on_live_route_registration_failure", func(t *testing.T) {
		// Returns_error_on_live_route_registration_failure should propagate error.
		ext := NewHealthExtension(&Config{AtDefaultAddr: true}).(*HealthExtension)
		r := newMockRex()
		r.registerRouteErr = errors.New("registration failed")

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns_error_on_ready_route_registration_failure", func(t *testing.T) {
		// Returns_error_on_ready_route_registration_failure should propagate error.
		ext := NewHealthExtension(&Config{AtDefaultAddr: true}).(*HealthExtension)
		r := newMockRex()
		r.registerRouteErr = errors.New("ready registration failed")
		r.registerRouteErrOnCall = 2 // Fail on second call (ready route)

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns_error_on_status_route_registration_failure", func(t *testing.T) {
		// Returns_error_on_status_route_registration_failure should propagate error.
		ext := NewHealthExtension(&Config{AtDefaultAddr: true}).(*HealthExtension)
		r := newMockRex()
		r.registerRouteErr = errors.New("status registration failed")
		r.registerRouteErrOnCall = 3 // Fail on third call (status route)

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns_error_on_dedicated_router_route_failure", func(t *testing.T) {
		// Returns_error_on_dedicated_router_route_failure should propagate error.
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()
		r.registerRouterErr = errors.New("registration failed")

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns_error_on_dedicated_router_ready_route_failure", func(t *testing.T) {
		// Returns_error_on_dedicated_router_ready_route_failure should propagate error.
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()
		r.registerRouterErr = errors.New("ready registration failed")
		r.registerRouterErrOnCall = 2 // Fail on second call (ready route)

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns_error_on_dedicated_router_status_route_failure", func(t *testing.T) {
		// Returns_error_on_dedicated_router_status_route_failure should propagate error.
		ext := NewHealthExtension(&Config{AtDefaultAddr: false}).(*HealthExtension)
		r := newMockRex()
		r.registerRouterErr = errors.New("status registration failed")
		r.registerRouterErrOnCall = 3 // Fail on third call (status route)

		ext.OnInitialize(context.Background(), r)
		err := ext.OnStart(context.Background(), r)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// --------------------------------------------------------------------------
// OnReady tests
// --------------------------------------------------------------------------

func TestHealthExtension_OnReady(t *testing.T) {
	t.Run("returns_nil", func(t *testing.T) {
		// Returns_nil should be a no-op.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		err := ext.OnReady(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// OnStop tests
// --------------------------------------------------------------------------

func TestHealthExtension_OnStop(t *testing.T) {
	t.Run("stops_health_check_ticker", func(t *testing.T) {
		// Stops_health_check_ticker should call registry.Stop.
		ext := NewHealthExtension(&Config{
			AtDefaultAddr: true,
			CheckInterval: 10 * time.Second,
		}).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)
		ext.OnStart(context.Background(), r)
		err := ext.OnStop(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// OnShutdown tests
// --------------------------------------------------------------------------

func TestHealthExtension_OnShutdown(t *testing.T) {
	t.Run("returns_nil", func(t *testing.T) {
		// Returns_nil should complete shutdown cleanly.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)
		err := ext.OnShutdown(context.Background(), r)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// Accessor method tests
// --------------------------------------------------------------------------

func TestHealthExtension_Registry(t *testing.T) {
	t.Run("returns_registry", func(t *testing.T) {
		// Returns_registry should return the initialized registry.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		if ext.Registry() == nil {
			t.Fatal("expected non-nil registry")
		}
	})
}

func TestHealthExtension_StateStore(t *testing.T) {
	t.Run("returns_state_store", func(t *testing.T) {
		// Returns_state_store should return the initialized stateStore.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		if ext.StateStore() == nil {
			t.Fatal("expected non-nil stateStore")
		}
	})
}

func TestHealthExtension_SnapshotCache(t *testing.T) {
	t.Run("returns_snapshot_cache", func(t *testing.T) {
		// Returns_snapshot_cache should return the initialized snapshotCache.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		if ext.SnapshotCache() == nil {
			t.Fatal("expected non-nil snapshotCache")
		}
	})
}

func TestHealthExtension_RouteDepMap(t *testing.T) {
	t.Run("returns_route_dep_map", func(t *testing.T) {
		// Returns_route_dep_map should return the initialized routeDepMap.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		if ext.RouteDepMap() == nil {
			t.Fatal("expected non-nil routeDepMap")
		}
	})
}

func TestHealthExtension_CheckCache(t *testing.T) {
	t.Run("returns_check_cache", func(t *testing.T) {
		// Returns_check_cache should return the initialized checkCache.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		if ext.CheckCache() == nil {
			t.Fatal("expected non-nil checkCache")
		}
	})
}

func TestHealthExtension_MiddlewareConfig(t *testing.T) {
	t.Run("returns_configured_middleware_config", func(t *testing.T) {
		// Returns_configured_middleware_config should return valid MiddlewareConfig.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)
		cfg := ext.MiddlewareConfig()

		if cfg.RouteDepMap == nil {
			t.Fatal("expected non-nil RouteDepMap in config")
		}
		if cfg.StateStore == nil {
			t.Fatal("expected non-nil StateStore in config")
		}
		if cfg.SnapshotCache == nil {
			t.Fatal("expected non-nil SnapshotCache in config")
		}
		if cfg.Registry == nil {
			t.Fatal("expected non-nil Registry in config")
		}
		if cfg.CheckCache == nil {
			t.Fatal("expected non-nil CheckCache in config")
		}
		if !cfg.UseCache {
			t.Fatal("expected UseCache=true")
		}
	})
}

// --------------------------------------------------------------------------
// RegisterCheck tests
// --------------------------------------------------------------------------

func TestHealthExtension_RegisterCheck(t *testing.T) {
	t.Run("registers_check_to_registry", func(t *testing.T) {
		// Registers_check_to_registry should add check to the internal registry.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		check := NewCheck("test-check", func(ctx context.Context, resolver rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		})
		ext.RegisterCheck(check)

		if ext.registry.Get("test-check") == nil {
			t.Fatal("expected check to be registered")
		}
	})
}

// --------------------------------------------------------------------------
// RegisterCheckFunc tests
// --------------------------------------------------------------------------

func TestHealthExtension_RegisterCheckFunc(t *testing.T) {
	t.Run("registers_check_function_to_registry", func(t *testing.T) {
		// Registers_check_function_to_registry should create and add check.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		ext.RegisterCheckFunc("func-check", func(ctx context.Context, resolver rextension.Resolver) *CheckResult {
			return NewCheckResult(StatusUp, "ok", 0)
		})

		if ext.registry.Get("func-check") == nil {
			t.Fatal("expected check to be registered")
		}
	})

	t.Run("registers_check_function_with_options", func(t *testing.T) {
		// Registers_check_function_with_options should apply options to the check.
		ext := NewHealthExtension(nil).(*HealthExtension)
		r := newMockRex()

		ext.OnInitialize(context.Background(), r)

		ext.RegisterCheckFunc("func-check-opts",
			func(ctx context.Context, resolver rextension.Resolver) *CheckResult {
				return NewCheckResult(StatusUp, "ok", 0)
			},
			WithTimeout(10*time.Second),
			WithTags("critical"),
		)

		check := ext.registry.Get("func-check-opts")
		if check == nil {
			t.Fatal("expected check to be registered")
		}
		if check.Timeout() != 10*time.Second {
			t.Fatalf("expected Timeout=10s, got %v", check.Timeout())
		}
		if len(check.Tags()) != 1 || check.Tags()[0] != "critical" {
			t.Fatalf("expected tags=[critical], got %v", check.Tags())
		}
	})
}

// ---------------------------------------------------------------------------
// Config options
// ---------------------------------------------------------------------------

func TestWithTreatUnknownAs(t *testing.T) {
	if got := NewConfig(WithTreatUnknownAs(StatusDown)).TreatUnknownAs; got != StatusDown {
		t.Fatalf("TreatUnknownAs = %v", got)
	}
	// The default serves rather than refuses: a dependency nobody declared is
	// not evidence that a route depending on it is broken.
	if got := NewDefaultConfig().TreatUnknownAs; got != StatusUp {
		t.Fatalf("default TreatUnknownAs = %v, want StatusUp", got)
	}
}

// Checks are declared rather than registered, because OnInitialize runs from
// Run now (D13): an application can no longer resolve the registry and call
// Register before Run, since the registry does not exist yet.
func TestWithCheck_and_WithChecks(t *testing.T) {
	a := &mockHealthCheck{name: "a"}
	b := &mockHealthCheck{name: "b"}
	c := &mockHealthCheck{name: "c"}

	cfg := NewConfig(WithCheck(a), WithChecks(b, c))
	if len(cfg.Checks) != 3 {
		t.Fatalf("Checks = %d, want 3", len(cfg.Checks))
	}
}

// A nil check is dropped rather than stored, so nothing has to nil-check it at
// execution time — where the panic would be a startup crash on a health check.
func TestWithCheck_drops_nils(t *testing.T) {
	cfg := NewConfig(
		WithCheck(nil),
		WithChecks(nil, &mockHealthCheck{name: "real"}, nil),
	)
	if len(cfg.Checks) != 1 {
		t.Fatalf("Checks = %d, want only the non-nil one", len(cfg.Checks))
	}
	if cfg.Checks[0].Name() != "real" {
		t.Fatalf("Checks[0] = %q", cfg.Checks[0].Name())
	}
}

func TestWithChecks_with_no_arguments(t *testing.T) {
	if got := len(NewConfig(WithChecks()).Checks); got != 0 {
		t.Fatalf("Checks = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// runInitialChecks (O14)
// ---------------------------------------------------------------------------

// Every declared check runs once before any listener binds, so the gate has a
// known state to work from rather than gating on "unknown" for the first
// request to each dependency.
func TestRunInitialChecks_records_every_result_before_serving(t *testing.T) {
	up := &mockHealthCheck{name: "up"}
	down := &mockHealthCheck{
		name:       "down",
		resultFunc: func() *CheckResult { return &CheckResult{Status: StatusDown, Message: "refused"} },
	}
	degraded := &mockHealthCheck{
		name:       "degraded",
		resultFunc: func() *CheckResult { return &CheckResult{Status: StatusDegraded, Message: "slow"} },
	}

	ext := NewHealthExtension(NewConfig(WithChecks(up, down, degraded))).(*HealthExtension)
	r := newMockRex()
	if err := ext.OnInitialize(context.Background(), r); err != nil {
		t.Fatalf("OnInitialize: %v", err)
	}

	// A failing dependency is not a startup failure: refusing to boot while a
	// dependency restarts is worse than booting and refusing the affected
	// routes. What matters is that the state is known.
	if err := ext.runInitialChecks(context.Background(), r.Logger()); err != nil {
		t.Fatalf("a failing check must not fail the boot, got %v", err)
	}

	for name, want := range map[string]Status{
		"up": StatusUp, "down": StatusDown, "degraded": StatusDegraded,
	} {
		state := ext.stateStore.Get(name)
		if state == nil {
			t.Errorf("%s has no recorded state after the initial pass", name)
			continue
		}
		if got := state.Clone().Status; got != want {
			t.Errorf("%s recorded as %v, want %v", name, got, want)
		}
	}
	if up.execCount != 1 {
		t.Errorf("the up check ran %d times, want 1", up.execCount)
	}
}

// A check producing no result at all leaves its dependency unknown, which is
// then TreatUnknownAs's decision — and worth a warning, because it is the one
// case the initial pass could not resolve.
func TestRunInitialChecks_warns_about_a_check_with_no_result(t *testing.T) {
	silent := &mockHealthCheck{name: "silent", resultFunc: func() *CheckResult { return nil }}

	ext := NewHealthExtension(NewConfig(WithCheck(silent))).(*HealthExtension)
	r := newMockRex()
	if err := ext.OnInitialize(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := ext.runInitialChecks(context.Background(), r.Logger()); err != nil {
		t.Fatalf("runInitialChecks: %v", err)
	}
}

// With nothing declared there is nothing to run.
func TestRunInitialChecks_with_no_checks(t *testing.T) {
	ext := NewHealthExtension(NewConfig()).(*HealthExtension)
	r := newMockRex()
	if err := ext.OnInitialize(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := ext.runInitialChecks(context.Background(), r.Logger()); err != nil {
		t.Fatalf("runInitialChecks: %v", err)
	}
}
