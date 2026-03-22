// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"encoding/json"
	"net/http"

	rxroute "github.com/kryovyx/rextension/route"
)

// newLiveRoute creates the /live endpoint route.
func newLiveRoute(path string) rxroute.Route {
	if path == "" {
		path = "/live"
	}
	return rxroute.New("GET", path, func(ctx rxroute.Context) {
		resp := LiveResponse{Status: "UP"}
		ctx.JSON(http.StatusOK, resp)
	})
}

// LiveHandler creates a standalone http.Handler for the /live endpoint.
func LiveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LiveResponse{Status: "UP"})
	})
}
