// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"encoding/json"
	"net/http"

	rxroute "github.com/kryovyx/rextension/route"
)

// newReadyRoute creates the /ready endpoint route.
func newReadyRoute(path string, cache SnapshotCache) rxroute.Route {
	if path == "" {
		path = "/ready"
	}
	return rxroute.New("GET", path, func(ctx rxroute.Context) {
		snap := cache.GetReadiness(ctx)

		resp := ReadyResponse{
			Status: snap.OverallStatus.String(),
			Checks: snap.Checks,
		}

		statusCode := http.StatusOK
		if snap.OverallStatus == StatusDown {
			statusCode = http.StatusServiceUnavailable
		}

		ctx.JSON(statusCode, resp)
	})
}

// ReadyHandler creates a standalone http.Handler for the /ready endpoint.
func ReadyHandler(cache SnapshotCache) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := cache.GetReadiness(r.Context())

		resp := ReadyResponse{
			Status: snap.OverallStatus.String(),
			Checks: snap.Checks,
		}

		w.Header().Set("Content-Type", "application/json")
		statusCode := http.StatusOK
		if snap.OverallStatus == StatusDown {
			statusCode = http.StatusServiceUnavailable
		}
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(resp)
	})
}
