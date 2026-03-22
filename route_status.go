// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"encoding/json"
	"net/http"

	rxroute "github.com/kryovyx/rextension/route"
)

// newStatusRoute creates the /status endpoint route.
func newStatusRoute(path string, cache SnapshotCache, stateStore DepStateStore) rxroute.Route {
	if path == "" {
		path = "/status"
	}
	return rxroute.New("GET", path, func(ctx rxroute.Context) {
		snap := cache.Get(ctx)

		resp := StatusResponse{
			Status:       snap.OverallStatus.String(),
			Timestamp:    snap.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			Checks:       snap.Checks,
			Dependencies: snap.Dependencies,
		}

		// Add fresh dependency states
		if stateStore != nil {
			resp.Dependencies = stateStore.GetAll()
		}

		statusCode := http.StatusOK
		if snap.OverallStatus == StatusDown {
			statusCode = http.StatusServiceUnavailable
		}

		ctx.JSON(statusCode, resp)
	})
}

// StatusHandler creates a standalone http.Handler for the /status endpoint.
// Useful for exposing health on a separate port without Rex routing.
func StatusHandler(cache SnapshotCache, stateStore DepStateStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := cache.Get(r.Context())

		resp := StatusResponse{
			Status:       snap.OverallStatus.String(),
			Timestamp:    snap.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			Checks:       snap.Checks,
			Dependencies: snap.Dependencies,
		}

		if stateStore != nil {
			resp.Dependencies = stateStore.GetAll()
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
