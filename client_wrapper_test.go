// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health contains tests for the HTTP client wrapper.
package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --------------------------------------------------------------------------
// HTTPClientOption tests
// --------------------------------------------------------------------------

func TestHTTPClientOptions(t *testing.T) {
	t.Run("WithHTTPTimeout_sets_timeout", func(t *testing.T) {
		// WithHTTPTimeout_sets_timeout should configure request timeout.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		client := WrapHTTPClient(nil, "test", store, WithHTTPTimeout(5*time.Second))
		if client == nil {
			t.Fatal("expected non-nil client")
		}
	})

	t.Run("WithRetries_sets_retry_config", func(t *testing.T) {
		// WithRetries_sets_retry_config should configure retry behavior.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		client := WrapHTTPClient(nil, "test", store, WithRetries(3, 100*time.Millisecond))
		if client == nil {
			t.Fatal("expected non-nil client")
		}
	})

	t.Run("WithCircuitBreaker_attaches_breaker", func(t *testing.T) {
		// WithCircuitBreaker_attaches_breaker should set circuit breaker.
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
		client := WrapHTTPClient(nil, "test", nil, WithCircuitBreaker(cb))
		if client == nil {
			t.Fatal("expected non-nil client")
		}
	})
}

// --------------------------------------------------------------------------
// WrapHTTPClient tests
// --------------------------------------------------------------------------

func TestWrapHTTPClient(t *testing.T) {
	t.Run("uses_default_client_when_nil", func(t *testing.T) {
		// Uses_default_client_when_nil should not panic with nil client.
		client := WrapHTTPClient(nil, "dep", nil)
		if client == nil {
			t.Fatal("expected non-nil wrapped client")
		}
	})

	t.Run("wraps_provided_client", func(t *testing.T) {
		// Wraps_provided_client should use the given http.Client.
		httpClient := &http.Client{Timeout: 10 * time.Second}
		client := WrapHTTPClient(httpClient, "dep", nil)
		if client == nil {
			t.Fatal("expected non-nil wrapped client")
		}
	})
}

// --------------------------------------------------------------------------
// wrappedHTTPClient.Do tests
// --------------------------------------------------------------------------

func TestWrappedHTTPClient_Do(t *testing.T) {
	t.Run("reports_success_on_2xx", func(t *testing.T) {
		// Reports_success_on_2xx should update state store with success.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("api")
		client := WrapHTTPClient(server.Client(), "api", store, WithHTTPTimeout(5*time.Second))

		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		resp, err := client.Do(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		state := store.Get("api")
		if state == nil || state.Status != StatusUp {
			t.Fatal("expected state to be Up after success")
		}
	})

	t.Run("reports_failure_on_5xx", func(t *testing.T) {
		// Reports_failure_on_5xx should update state store with failure.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("api")
		client := WrapHTTPClient(server.Client(), "api", store, WithHTTPTimeout(5*time.Second))

		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		client.Do(context.Background(), req)

		state := store.Get("api")
		if state == nil || state.FailureCount < 1 {
			t.Fatal("expected failure to be recorded")
		}
	})

	t.Run("returns_error_when_circuit_open", func(t *testing.T) {
		// Returns_error_when_circuit_open should fail fast when breaker is open.
		cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour})
		cb.Failure() // Open the circuit

		client := WrapHTTPClient(nil, "blocked", nil, WithCircuitBreaker(cb))
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		_, err := client.Do(context.Background(), req)

		if err == nil {
			t.Fatal("expected CircuitOpenError")
		}
		if _, ok := err.(*CircuitOpenError); !ok {
			t.Fatalf("expected CircuitOpenError, got %T", err)
		}
	})

	t.Run("reports_failure_on_network_error", func(t *testing.T) {
		// Reports_failure_on_network_error should handle connection errors.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("api")
		// Use a non-routable address to trigger network error
		client := WrapHTTPClient(nil, "api", store, WithHTTPTimeout(10*time.Millisecond))

		req, _ := http.NewRequest(http.MethodGet, "http://192.0.2.1:1", nil)
		_, err := client.Do(context.Background(), req)

		if err == nil {
			t.Fatal("expected network error")
		}
		state := store.Get("api")
		if state == nil || state.FailureCount < 1 {
			t.Fatal("expected failure to be recorded on network error")
		}
	})

	t.Run("notifies_circuit_breaker_on_success", func(t *testing.T) {
		// Notifies_circuit_breaker_on_success should call cb.Success().
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cb := NewCircuitBreaker(CircuitBreakerConfig{
			FailureThreshold: 1,
			SuccessThreshold: 1,
			Timeout:          1 * time.Millisecond,
			HalfOpenMaxCalls: 2, // Need 2: one for Do's Allow check, one for the success
		})
		cb.Failure()
		time.Sleep(5 * time.Millisecond) // -> half-open

		client := WrapHTTPClient(server.Client(), "api", nil, WithCircuitBreaker(cb))
		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		_, err := client.Do(context.Background(), req)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cb.State() != CircuitClosed {
			t.Fatalf("expected CircuitClosed after success, got %v", cb.State())
		}
	})

	t.Run("notifies_circuit_breaker_on_failure", func(t *testing.T) {
		// Notifies_circuit_breaker_on_failure should call cb.Failure().
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour})
		client := WrapHTTPClient(server.Client(), "api", nil, WithCircuitBreaker(cb))

		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		client.Do(context.Background(), req)

		if cb.State() != CircuitOpen {
			t.Fatalf("expected CircuitOpen after failure, got %v", cb.State())
		}
	})

	t.Run("retries_on_5xx_with_backoff", func(t *testing.T) {
		// Retries_on_5xx_with_backoff should retry and sleep between attempts.
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			if requestCount < 3 {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("api")
		client := WrapHTTPClient(server.Client(), "api", store,
			WithRetries(3, 1*time.Millisecond),
			WithHTTPTimeout(5*time.Second),
		)

		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		resp, err := client.Do(context.Background(), req)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if requestCount != 3 {
			t.Fatalf("expected 3 requests (2 retries), got %d", requestCount)
		}
	})
}

// --------------------------------------------------------------------------
// wrappedHTTPClient.Get tests
// --------------------------------------------------------------------------

func TestWrappedHTTPClient_Get(t *testing.T) {
	t.Run("performs_get_request", func(t *testing.T) {
		// Performs_get_request should issue GET and return response.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := WrapHTTPClient(server.Client(), "get-test", nil)
		resp, err := client.Get(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("returns_error_for_invalid_url", func(t *testing.T) {
		// Returns_error_for_invalid_url should fail for bad URL.
		client := WrapHTTPClient(nil, "get-test", nil)
		_, err := client.Get(context.Background(), "://invalid")
		if err == nil {
			t.Fatal("expected error for invalid URL")
		}
	})
}

// --------------------------------------------------------------------------
// wrappedHTTPClient.Post tests
// --------------------------------------------------------------------------

func TestWrappedHTTPClient_Post(t *testing.T) {
	t.Run("performs_post_request", func(t *testing.T) {
		// Performs_post_request should issue POST with content type.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("expected application/json, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		client := WrapHTTPClient(server.Client(), "post-test", nil)
		resp, err := client.Post(context.Background(), server.URL, "application/json", []byte(`{}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("returns_error_for_invalid_url", func(t *testing.T) {
		// Returns_error_for_invalid_url should fail for bad URL.
		client := WrapHTTPClient(nil, "post-test", nil)
		_, err := client.Post(context.Background(), "://invalid", "text/plain", nil)
		if err == nil {
			t.Fatal("expected error for invalid URL")
		}
	})
}

// --------------------------------------------------------------------------
// CircuitOpenError tests
// --------------------------------------------------------------------------

func TestCircuitOpenError(t *testing.T) {
	t.Run("returns_error_message", func(t *testing.T) {
		// Returns_error_message should include dependency ID.
		err := &CircuitOpenError{DepID: "my-service"}
		msg := err.Error()
		if msg != "circuit breaker open for dependency: my-service" {
			t.Fatalf("unexpected error message: %s", msg)
		}
	})
}

// --------------------------------------------------------------------------
// NewDepReporter tests
// --------------------------------------------------------------------------

func TestNewDepReporter(t *testing.T) {
	t.Run("creates_reporter", func(t *testing.T) {
		// Creates_reporter should return a valid DepReporter.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		reporter := NewDepReporter("dep", store, nil)
		if reporter == nil {
			t.Fatal("expected non-nil reporter")
		}
	})
}

// --------------------------------------------------------------------------
// depReporter.ReportSuccess tests
// --------------------------------------------------------------------------

func TestDepReporter_ReportSuccess(t *testing.T) {
	t.Run("updates_state_store", func(t *testing.T) {
		// Updates_state_store should report success to state store.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("dep")
		reporter := NewDepReporter("dep", store, nil)

		reporter.ReportSuccess(10 * time.Millisecond)

		state := store.Get("dep")
		if state == nil || state.Status != StatusUp {
			t.Fatal("expected state to be Up")
		}
	})

	t.Run("notifies_circuit_breaker", func(t *testing.T) {
		// Notifies_circuit_breaker should call cb.Success().
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			FailureThreshold: 1,
			SuccessThreshold: 1,
			Timeout:          1 * time.Millisecond,
			HalfOpenMaxCalls: 1,
		})
		cb.Failure()
		time.Sleep(5 * time.Millisecond) // -> half-open

		reporter := NewDepReporter("dep", nil, cb)
		cb.Allow() // Take the half-open slot
		reporter.ReportSuccess(time.Millisecond)

		if cb.State() != CircuitClosed {
			t.Fatalf("expected CircuitClosed after success, got %v", cb.State())
		}
	})
}

// --------------------------------------------------------------------------
// depReporter.ReportFailure tests
// --------------------------------------------------------------------------

func TestDepReporter_ReportFailure(t *testing.T) {
	t.Run("updates_state_store", func(t *testing.T) {
		// Updates_state_store should report failure to state store.
		store := NewDepStateStore(DefaultDepStateStoreConfig())
		store.Register("dep")
		reporter := NewDepReporter("dep", store, nil)

		reporter.ReportFailure("timeout")

		state := store.Get("dep")
		if state == nil || state.FailureCount < 1 {
			t.Fatal("expected failure to be recorded")
		}
	})

	t.Run("notifies_circuit_breaker", func(t *testing.T) {
		// Notifies_circuit_breaker should call cb.Failure().
		cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour})
		reporter := NewDepReporter("dep", nil, cb)

		reporter.ReportFailure("error")

		if cb.State() != CircuitOpen {
			t.Fatalf("expected CircuitOpen after failure, got %v", cb.State())
		}
	})
}
