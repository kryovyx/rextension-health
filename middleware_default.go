// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"net/http"

	rx "github.com/kryovyx/rextension"
	rxroute "github.com/kryovyx/rextension/route"
)

// DependencyGateFactory returns a PerRouteMiddleware that attaches the
// dependency gate to exactly the routes that declare dependencies.
//
// This is the shape the whole per-route middleware primitive exists for
// (D18b/c/P4.12), and the reason is worth stating plainly, because the bug it
// fixes was invisible.
//
// # What was wrong
//
// The gate was one global middleware. To decide whether it applied to a
// request, it built a route identifier from the *live URL*:
//
//	routeID := RouteID(r.Method, r.URL.Path)   // "GET:/users/42"
//
// and looked that up in an index keyed at registration time by the route's
// *pattern*:
//
//	routeID := RouteID(rt.Method(), rt.Path()) // "GET:/users/{id}"
//
// Those never match for a parameterized route. So the gate silently did
// nothing for every route with a path parameter — a route declaring
// `Dependencies() → [database (hard)]` was served normally with the database
// down, for as long as the extension had existed. Nothing logged it, and any
// test using a static path passed.
//
// # What replaces it
//
// The framework calls this factory once per route, when the table is built.
// The route is handed over directly, so there is no identifier to construct
// and nothing to look up:
//
//   - A route that is not a HealthDepRoute gets nil — no middleware attached,
//     so it costs nothing rather than costing a lookup that always misses.
//   - A route that is one has its dependencies read **here, once**, and closed
//     over. There is no per-request map lookup and no way for the pattern and
//     the URL to disagree, because neither is involved.
func DependencyGateFactory(cfg MiddlewareConfig) rx.PerRouteMiddleware {
	return func(info rx.RouteInfo) rx.Middleware {
		rt := info.Route
		hdr, ok := rt.(HealthDepRoute)
		if !ok {
			return nil // not gated — nothing attached
		}

		// Read once, at freeze (D18c). The route's answer cannot change
		// between requests, and a request cannot see a half-updated one.
		deps := hdr.Dependencies()
		if len(deps) == 0 {
			return nil
		}

		routeID := RouteID(rt.Method(), rt.Path())
		return dependencyGate(cfg, routeID, deps)
	}
}

// DependencyGateMiddleware creates a standard HTTP middleware that gates
// requests based on dependency health.
//
// Deprecated: prefer DependencyGateFactory. This form has to discover a
// route's dependencies at request time, which is what made the gate silently
// inoperative for every parameterized route — see DependencyGateFactory.
//
// Retained for an application that composes the middleware itself; it now
// reads the matched route from the request context rather than reconstructing
// an identifier from the URL, so it is at least correct.
func DependencyGateMiddleware(cfg MiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The matched route, not a reconstruction from the URL. The
			// router puts it in the context precisely so middleware need not
			// re-derive it.
			rt, found := rxroute.GetMatchedRoute(r)
			if !found {
				next.ServeHTTP(w, r)
				return
			}
			hdr, ok := rt.(HealthDepRoute)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			deps := hdr.Dependencies()
			if len(deps) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			dependencyGate(cfg, RouteID(rt.Method(), rt.Path()), deps)(next).ServeHTTP(w, r)
		})
	}
}

// dependencyGate is the gate itself, with the route's dependencies already
// resolved.
func dependencyGate(cfg MiddlewareConfig, routeID string, deps []DepRequirement) rx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dsc := &DepStateContext{
				RouteID:      routeID,
				Dependencies: make(map[string]*DepState),
				DegradedDeps: make([]string, 0),
			}

			var states map[string]*DepState
			if cfg.UseCache && cfg.SnapshotCache != nil {
				snap := cfg.SnapshotCache.Get(r.Context())
				states = snap.Dependencies
			} else if cfg.StateStore != nil {
				states = cfg.StateStore.GetAll()
			}

			var resolver rx.Resolver
			if cfg.Resolver != nil {
				resolver, _ = cfg.Resolver.(rx.Resolver)
			}

			for _, dep := range deps {
				state := states[dep.DepID]

				if state == nil || shouldExecutePassiveCheck(cfg, dep.DepID, state) {
					if ps := executePassiveCheckIfNeeded(r.Context(), cfg, dep.DepID, resolver); ps != nil {
						state = ps
					}
				}

				unknown := state == nil
				if unknown {
					state = NewDepState(dep.DepID)
					state.Status = StatusUnknown
					state.Message = "no data yet"
				}
				dsc.Dependencies[dep.DepID] = state

				// Decide what an unknown dependency counts as (O14).
				//
				// The comparison below is `state.Status > dep.MinStatus`, and
				// StatusUnknown happens to sort after StatusDown, so an
				// unknown dependency used to fall through to "refuse" — not
				// as a decision, but as a consequence of the order the
				// constants were declared in. Reordering them would have
				// silently inverted it.
				//
				// It is now explicit, and defaults to serving: refusing means
				// 503ing every request for the first check interval after
				// every boot, because a check has not run yet rather than
				// because anything is wrong. The synchronous check pass in
				// OnStart is what makes that safe — see Config.TreatUnknownAs.
				effective := state.Status
				if unknown || state.Status == StatusUnknown {
					effective = cfg.TreatUnknownAs
					if cfg.Logger != nil {
						cfg.Logger.Warn("Dependency %s has no state yet; gating %s %s as %s",
							dep.DepID, r.Method, r.URL.Path, effective)
					}
				}

				if effective > dep.MinStatus {
					if dep.Type == RequirementHard {
						statusCode := cfg.FailureStatusCode
						if statusCode == 0 {
							statusCode = http.StatusServiceUnavailable
						}
						detail := cfg.FailureMessage
						if detail == "" {
							detail = "a dependency this endpoint requires is currently unavailable"
						}

						// An RFC 9457 problem document rather than a
						// plain-text line (D30/O9).
						//
						// The dependency's identifier is NOT disclosed. "cache
						// is down", "payments-api is degraded" names the
						// internal service topology to any caller who probes
						// endpoints during an outage. The identifier and its
						// state are logged instead, and the /status endpoint
						// exposes them to whoever is authorised to see it.
						p := rx.NewProblem(statusCode, rx.ProblemDependencyUnavailable, detail)
						if cfg.Logger != nil {
							cfg.Logger.Warn("Dependency gate refused %s %s: dependency %s is %s",
								r.Method, r.URL.Path, dep.DepID, state.Status)
						}
						p.Write(w, r)
						return
					}
					dsc.DegradedDeps = append(dsc.DegradedDeps, dep.DepID)
				}
			}

			r = r.WithContext(context.WithValue(r.Context(), ContextKeyDepStates, dsc))
			next.ServeHTTP(w, r)
		})
	}
}

// shouldExecutePassiveCheck determines if a passive or on-demand check should
// be executed on-demand.
func shouldExecutePassiveCheck(cfg MiddlewareConfig, depID string, state *DepState) bool {
	if cfg.Registry == nil {
		return false
	}
	check := cfg.Registry.Get(depID)
	if check == nil {
		return false
	}
	return check.Mode() == CheckModePassive || check.Mode() == CheckModeOnDemand
}

// executePassiveCheckIfNeeded executes a passive check on-demand using the cache.
func executePassiveCheckIfNeeded(ctx context.Context, cfg MiddlewareConfig, depID string, resolver rx.Resolver) *DepState {
	if cfg.Registry == nil || cfg.CheckCache == nil {
		return nil
	}
	check := cfg.Registry.Get(depID)
	if check == nil || (check.Mode() != CheckModePassive && check.Mode() != CheckModeOnDemand) {
		return nil
	}
	result := cfg.CheckCache.GetOrExecute(ctx, check)
	if result == nil {
		return nil
	}
	state := NewDepState(depID)
	state.Status = result.Status
	state.Message = result.Message
	state.LastCheck = result.Timestamp
	return state
}

// RouteResolverMiddleware creates a standard HTTP middleware that injects the
// resolved route ID into the request context.
//
// Deprecated: no longer used by this extension and no longer needed (P4.12).
// It existed so the dependency gate could read a route identifier it had not
// computed itself — and the identifier it computed, RouteID(method,
// r.URL.Path), was built from the live URL, which never matches a
// parameterized route's registered pattern. That mismatch is what made the
// gate silently inoperative.
//
// The gate is handed the matched route directly now, so nothing has to derive
// an identifier from a URL. Retained only for an application that reads
// ContextKeyRouteID.
func RouteResolverMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The matched route's *pattern*, not the request URL. A route
			// identifier built from the URL is different for every distinct
			// URL of one parameterized route, which is what made this
			// unusable for lookups.
			if rt, found := rxroute.GetMatchedRoute(r); found {
				r = r.WithContext(context.WithValue(r.Context(),
					ContextKeyRouteID, RouteID(rt.Method(), rt.Path())))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// DepContextMiddleware creates a standard HTTP middleware that injects dependency
// states into the request context for the specified dep IDs.
// Use this for non-gating scenarios where handlers want to check dep state for
// fallback logic without blocking the request.
func DepContextMiddleware(stateStore DepStateStore, depIDs ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dsc := &DepStateContext{
				Dependencies: make(map[string]*DepState),
				DegradedDeps: make([]string, 0),
			}
			for _, depID := range depIDs {
				state := stateStore.Get(depID)
				if state != nil {
					dsc.Dependencies[depID] = state
					if state.Status == StatusDegraded {
						dsc.DegradedDeps = append(dsc.DegradedDeps, depID)
					}
				}
			}
			r = r.WithContext(context.WithValue(r.Context(), ContextKeyDepStates, dsc))
			next.ServeHTTP(w, r)
		})
	}
}
