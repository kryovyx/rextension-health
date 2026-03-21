// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health provides a Rex extension for comprehensive health checking,
// dependency state management, circuit breaker patterns, and request gating.
//
// The extension provides:
//   - Liveness, readiness, and status endpoints
//   - Dependency state tracking (UP/DEGRADED/DOWN)
//   - Health check registry with TTL-cached snapshots
//   - Middleware for dependency-based request gating
//   - Circuit breaker integration
//   - HTTP client wrappers with automatic state reporting
package health

import (
	"context"
	"strings"

	"github.com/kryovyx/dix"
	"github.com/kryovyx/rex/event"
	rx "github.com/kryovyx/rextension"
)

// HealthRouterName is the default name for the dedicated health router.
const HealthRouterName = "health"

// HealthExtension implements the Rex extension contract for health checking.
type HealthExtension struct {
	cfg           Config
	routerName    string
	resolver      dix.Resolver
	logger        rx.Logger
	stateStore    DepStateStore
	registry      Registry
	snapshotCache SnapshotCache
	routeDepMap   RouteDepMap
	checkCache    CheckCache
}

// NewHealthExtension constructs a health extension instance.
func NewHealthExtension(cfg *Config) rx.Extension {
	c := NewDefaultConfig()
	if cfg != nil {
		if cfg.LivePath != "" {
			c.LivePath = cfg.LivePath
		}
		if cfg.ReadyPath != "" {
			c.ReadyPath = cfg.ReadyPath
		}
		if cfg.StatusPath != "" {
			c.StatusPath = cfg.StatusPath
		}
		if cfg.SnapshotTTL != 0 {
			c.SnapshotTTL = cfg.SnapshotTTL
		}
		c.AtDefaultAddr = cfg.AtDefaultAddr
		if cfg.Router.Addr != "" {
			c.Router = cfg.Router
		}
		c.StateStoreConfig = cfg.StateStoreConfig
		c.EnableDependencyGate = cfg.EnableDependencyGate
	}
	return &HealthExtension{cfg: *c}
}

// WithHealth is a helper Option to attach the extension to Rex.
func WithHealth(cfg *Config) rx.Option {
	return rx.WithExtension(NewHealthExtension(cfg))
}

// OnInitialize sets up the health infrastructure and event subscriptions.
func (e *HealthExtension) OnInitialize(ctx context.Context, r rx.Rex) error {
	logger := r.Logger()
	e.logger = logger

	// Choose router for health endpoints
	e.routerName = rx.DefaultRouterName
	needsDedicatedRouter := !e.cfg.AtDefaultAddr
	if needsDedicatedRouter {
		e.routerName = HealthRouterName
	}

	// Store resolver for health checks
	e.resolver = r.Container()

	// Create core components
	e.stateStore = NewDepStateStore(e.cfg.StateStoreConfig)
	e.registry = NewRegistry()
	e.registry.SetResolver(e.resolver) // Set resolver so checks can access dependencies
	e.registry.SetLogger(logger)
	e.snapshotCache = NewSnapshotCache(e.registry, e.stateStore, e.cfg.SnapshotTTL)
	e.routeDepMap = NewRouteDepMap()
	e.checkCache = NewCheckCache(e.stateStore, logger)

	// Expose via DI
	container := r.Container()
	container.Instance(e.stateStore)
	container.Instance(e.registry)
	container.Instance(e.snapshotCache)
	container.Instance(e.routeDepMap)

	bus := r.EventBus()

	// Subscribe to route registration events
	bus.Subscribe(event.RouterRouteRegisteredEventType, func(ev event.Event) {
		if routeEv, ok := event.As[event.RouterRouteRegisteredEvent](ev); ok {
			rt := routeEv.Route
			// Check if route implements HealthDepRoute (which embeds route.Route)
			if hdr, ok := rt.(HealthDepRoute); ok {
				routeID := RouteID(rt.Method(), rt.Path())
				e.routeDepMap.Register(routeID, hdr.Dependencies())
				logger.Info("Registered dependencies for route %s", routeID)
			}
		}
	})

	logger.Info("Health extension initialized for router %s", e.routerName)

	// Create the dedicated health router if needed
	if needsDedicatedRouter {
		if err := r.CreateRouter(e.routerName, e.cfg.Router); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				logger.Error("Failed to create health router %s: %v", e.routerName, err)
				return err
			}
		}
	}

	// Register dependency gate middleware if enabled
	if e.cfg.EnableDependencyGate {
		mwCfg := e.MiddlewareConfig()
		if err := RegisterMiddlewares(r, mwCfg); err != nil {
			logger.Error("Failed to register health middlewares: %v", err)
			return err
		}
		logger.Info("Registered health dependency gate middlewares")
	}

	return nil
}

// OnStart registers the health routes and starts the health check ticker.
func (e *HealthExtension) OnStart(ctx context.Context, r rx.Rex) error {
	logger := r.Logger()

	// Start the health check ticker if interval is configured
	if e.cfg.CheckInterval > 0 {
		e.registry.Start(e.cfg.CheckInterval, e.stateStore)
		logger.Info("Started health check ticker with interval %s", e.cfg.CheckInterval)
	}

	// Create routes
	liveRoute := newLiveRoute(e.cfg.LivePath)
	readyRoute := newReadyRoute(e.cfg.ReadyPath, e.snapshotCache)
	statusRoute := newStatusRoute(e.cfg.StatusPath, e.snapshotCache, e.stateStore)

	var err error
	if e.cfg.AtDefaultAddr {
		if err = r.RegisterRoute(liveRoute); err != nil {
			logger.Error("Failed to register live route: %v", err)
			return err
		}
		if err = r.RegisterRoute(readyRoute); err != nil {
			logger.Error("Failed to register ready route: %v", err)
			return err
		}
		if err = r.RegisterRoute(statusRoute); err != nil {
			logger.Error("Failed to register status route: %v", err)
			return err
		}
	} else {
		if err = r.RegisterRouteToRouter(liveRoute, e.routerName); err != nil {
			logger.Error("Failed to register live route: %v", err)
			return err
		}
		if err = r.RegisterRouteToRouter(readyRoute, e.routerName); err != nil {
			logger.Error("Failed to register ready route: %v", err)
			return err
		}
		if err = r.RegisterRouteToRouter(statusRoute, e.routerName); err != nil {
			logger.Error("Failed to register status route: %v", err)
			return err
		}
	}

	logger.Info("Registered health routes on router %s (%s, %s, %s)",
		e.routerName, e.cfg.LivePath, e.cfg.ReadyPath, e.cfg.StatusPath)

	return nil
}

// OnReady is a no-op for health.
func (e *HealthExtension) OnReady(ctx context.Context, r rx.Rex) error { return nil }

// OnStop stops the health check ticker.
func (e *HealthExtension) OnStop(ctx context.Context, r rx.Rex) error {
	e.registry.Stop()
	r.Logger().Info("Stopped health check ticker")
	return nil
}

// OnShutdown cleans up resources.
func (e *HealthExtension) OnShutdown(ctx context.Context, r rx.Rex) error {
	logger := r.Logger()
	logger.Info("Health extension shutdown complete")
	return nil
}

// Registry returns the health check registry.
func (e *HealthExtension) Registry() Registry {
	return e.registry
}

// StateStore returns the dependency state store.
func (e *HealthExtension) StateStore() DepStateStore {
	return e.stateStore
}

// SnapshotCache returns the snapshot cache.
func (e *HealthExtension) SnapshotCache() SnapshotCache {
	return e.snapshotCache
}

// RouteDepMap returns the route dependency map.
func (e *HealthExtension) RouteDepMap() RouteDepMap {
	return e.routeDepMap
}

// CheckCache returns the check cache for passive checks.
func (e *HealthExtension) CheckCache() CheckCache {
	return e.checkCache
}

// MiddlewareConfig returns a configured MiddlewareConfig for the dependency gate.
func (e *HealthExtension) MiddlewareConfig() MiddlewareConfig {
	return MiddlewareConfig{
		RouteDepMap:   e.routeDepMap,
		StateStore:    e.stateStore,
		SnapshotCache: e.snapshotCache,
		Registry:      e.registry,
		CheckCache:    e.checkCache,
		Resolver:      e.resolver,
		UseCache:      true,
	}
}

// RegisterCheck registers a health check with the extension's registry.
// This is a convenience method that allows registering checks directly on the extension.
func (e *HealthExtension) RegisterCheck(check HealthCheck) {
	e.registry.Register(check)
}

// RegisterCheckFunc registers a health check function with the extension's registry.
// This is a convenience method for simple health checks without options.
func (e *HealthExtension) RegisterCheckFunc(name string, fn CheckFunc, opts ...CheckOption) {
	e.registry.Register(NewCheck(name, fn, opts...))
}
