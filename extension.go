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
	"errors"
	"time"

	rx "github.com/kryovyx/rextension"
)

// HealthRouterName is the default name for the dedicated health router.
const HealthRouterName = "health"

// HealthExtension implements the Rex extension contract for health checking.
type HealthExtension struct {
	cfg           Config
	routerName    string
	resolver      rx.Resolver
	logger        rx.Logger
	stateStore    DepStateStore
	registry      Registry
	snapshotCache SnapshotCache
	routeDepMap   RouteDepMap
	checkCache    CheckCache
}

// NewHealthExtension constructs a health extension instance.
//
// A nil cfg takes the defaults. A non-nil cfg is used **verbatim** (D22/O6).
//
// It used to merge field by field — "copy this one if it is non-zero" — which
// looked like a convenience and behaved as a trap:
//
//   - Fields the merge simply forgot were silently ignored. Any value an
//     application set for them was discarded, and there was nothing to see:
//     no error, no log line, just the default.
//   - Fields copied *unconditionally* had the opposite problem. A partial
//     struct literal like &Config{LivePath: "/x"} zeroed them, so setting one
//     unrelated field turned another off.
//   - Zero and "unset" were indistinguishable, so a deliberate zero could not
//     be expressed at all.
//
// Build the config with NewConfig and the With* options; that is the shape
// that cannot drift out of step with the struct:
//
//	health.NewConfig(health.WithSomething(...))
//
// ⚠ A partial struct literal now leaves the rest of the fields at their zero
// values rather than at their defaults. That is the breaking half of this
// change, and it is deliberate: silently substituting a default for a field
// the caller wrote is what produced the traps above.
func NewHealthExtension(cfg *Config) rx.Extension {
	c := NewDefaultConfig()
	if cfg != nil {
		c = cfg
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

	// Register the declared checks (O2).
	//
	// This is how an application wires checks now that OnInitialize runs from
	// Run: it cannot resolve the registry beforehand and call Register,
	// because the registry is created here.
	for _, check := range e.cfg.Checks {
		e.registry.Register(check)
		logger.Debug("Registered declared health check %s", check.Name())
	}
	if n := len(e.cfg.Checks); n > 0 {
		logger.Info("Registered %d declared health check(s)", n)
	}

	// Expose via DI
	container := r.Container()
	container.Instance(e.stateStore)
	container.Instance(e.registry)
	container.Instance(e.snapshotCache)
	container.Instance(e.routeDepMap)

	// The route-registered subscription is gone (P4.12).
	//
	// It existed to build an index from route identifier to dependencies,
	// which the gate then looked up per request. The gate is handed the route
	// directly now, so there is no index to build and no lookup to get wrong
	// — and getting it wrong is exactly what happened: the index was keyed by
	// the route's pattern while the gate looked up the live URL, so the gate
	// never fired for a parameterized route.
	//
	// RouteDepMap is still populated below, and still exposed through the
	// container, for applications that inspect it. It is no longer on the
	// request path.

	logger.Info("Health extension initialized for router %s", e.routerName)

	// Create the dedicated health router if needed
	if needsDedicatedRouter {
		// errors.Is against an exported sentinel, replacing
		// strings.Contains(err.Error(), "already exists") — which coupled this
		// extension to the wording of another module's error message (D39).
		if err := r.CreateRouter(e.routerName, e.cfg.Router); err != nil {
			if !errors.Is(err, rx.ErrRouterExists) {
				logger.Error("Failed to create health router %s: %v", e.routerName, err)
				return err
			}
			logger.Debug("Health router %s already exists; reusing it", e.routerName)
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

	// One synchronous check pass, before anything binds (O14).
	//
	// This is what makes "an unknown dependency serves" safe rather than
	// merely convenient. Without it, every dependency is unknown for the
	// first check interval after every boot — up to ten seconds by default —
	// and the gate is deciding on no information at all. With it, by the time
	// a request can arrive the states are real, and "unknown" means a check
	// that has genuinely never produced a result.
	//
	// It is deliberately synchronous, which does add its own duration to
	// startup. That is the trade: a listener that binds a moment later, in
	// exchange for never serving on a guess. Individual checks have their own
	// timeouts, so a hanging dependency delays the boot by its timeout rather
	// than indefinitely.
	if err := e.runInitialChecks(ctx, logger); err != nil {
		return err
	}

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
		RouteDepMap:    e.routeDepMap,
		StateStore:     e.stateStore,
		SnapshotCache:  e.snapshotCache,
		Registry:       e.registry,
		CheckCache:     e.checkCache,
		Resolver:       e.resolver,
		Logger:         e.logger,
		TreatUnknownAs: e.cfg.TreatUnknownAs,
		UseCache:       true,
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

// runInitialChecks executes every registered check once and records the
// results, before any listener binds (O14).
//
// A failing check is **not** a startup failure. A dependency being down is
// exactly the condition the gate exists to handle at request time — refusing
// to start would mean an application could not boot while one of its
// dependencies was restarting, which is worse than booting and refusing the
// affected routes. What matters is that the state is known.
func (e *HealthExtension) runInitialChecks(ctx context.Context, logger rx.Logger) error {
	checks := e.registry.GetAll()
	if len(checks) == 0 {
		return nil
	}

	start := time.Now()
	results := e.registry.ExecuteAll(ctx)

	var down, degraded int
	for name, result := range results {
		if result == nil {
			continue
		}
		e.stateStore.SetStatus(name, result.Status, result.Message)
		switch result.Status {
		case StatusDown:
			down++
		case StatusDegraded:
			degraded++
		}
	}

	logger.Info("Ran %d initial health check(s) in %v: %d down, %d degraded",
		len(results), time.Since(start).Round(time.Millisecond), down, degraded)

	// Any check that produced no result at all leaves its dependency unknown,
	// which is what TreatUnknownAs then decides. Worth naming, because it is
	// the one case the initial pass could not resolve.
	for _, check := range checks {
		if _, ran := results[check.Name()]; !ran {
			logger.Warn("Health check %s produced no result during startup; its dependency will be gated as %s",
				check.Name(), e.cfg.TreatUnknownAs)
		}
	}
	return nil
}
