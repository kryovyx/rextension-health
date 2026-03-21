// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health provides a Rex extension for comprehensive health checking.
//
// This file defines the response types for health endpoints.
package health

// LiveResponse represents the response for /live endpoint.
type LiveResponse struct {
	Status string `json:"status"`
}

// ReadyResponse represents the response for /ready endpoint.
type ReadyResponse struct {
	Status string                  `json:"status"`
	Checks map[string]*CheckResult `json:"checks,omitempty"`
}

// StatusResponse represents the response for /status endpoint.
type StatusResponse struct {
	Status       string                  `json:"status"`
	Timestamp    string                  `json:"timestamp"`
	Checks       map[string]*CheckResult `json:"checks,omitempty"`
	Dependencies map[string]*DepState    `json:"dependencies,omitempty"`
}
