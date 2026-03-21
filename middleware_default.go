// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
"context"
"fmt"
"net/http"

"github.com/kryovyx/dix"
)

// DependencyGateMiddleware creates a standard HTTP middleware that gates requests
// based on dependency health.
// Hard dependencies that are down cause an immediate 503 response.
// Soft dependencies that are degraded are marked in the request context for
// handler fallback logic.
//
// For passive checks (CheckModePassive), the middleware executes the check
// on-demand with result caching rather than relying on background polling.
func DependencyGateMiddleware(cfg MiddlewareConfig) func(http.Handler) http.Handler {
return func(next http.Handler) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
routeID := RouteID(r.Method, r.URL.Path)
if routeID == "" {
routeID, _ = r.Context().Value(ContextKeyRouteID).(string)
}

if routeID == "" || cfg.RouteDepMap == nil {
next.ServeHTTP(w, r)
return
}

deps := cfg.RouteDepMap.Get(routeID)
if len(deps) == 0 {
next.ServeHTTP(w, r)
return
}

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

var resolver dix.Resolver
if cfg.Resolver != nil {
resolver, _ = cfg.Resolver.(dix.Resolver)
}

for _, dep := range deps {
state := states[dep.DepID]

if state == nil || shouldExecutePassiveCheck(cfg, dep.DepID, state) {
if ps := executePassiveCheckIfNeeded(r.Context(), cfg, dep.DepID, resolver); ps != nil {
state = ps
}
}

if state == nil {
state = NewDepState(dep.DepID)
state.Status = StatusUnknown
state.Message = "no data yet"
}
dsc.Dependencies[dep.DepID] = state

if state.Status > dep.MinStatus {
if dep.Type == RequirementHard {
statusCode := cfg.FailureStatusCode
if statusCode == 0 {
statusCode = http.StatusServiceUnavailable
}
msg := cfg.FailureMessage
if msg == "" {
msg = fmt.Sprintf("Dependency %s is %s", dep.DepID, state.Status)
}
http.Error(w, msg, statusCode)
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
func executePassiveCheckIfNeeded(ctx context.Context, cfg MiddlewareConfig, depID string, resolver dix.Resolver) *DepState {
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
// Apply this early in the middleware stack so the route ID is available to
// downstream middleware (e.g. DependencyGateMiddleware).
func RouteResolverMiddleware() func(http.Handler) http.Handler {
return func(next http.Handler) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
routeID := RouteID(r.Method, r.URL.Path)
if routeID != "" {
r = r.WithContext(context.WithValue(r.Context(), ContextKeyRouteID, routeID))
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
