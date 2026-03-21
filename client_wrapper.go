// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

package health

import (
	"context"
	"net/http"
	"time"
)

// HTTPClientOption configures the wrapped HTTP client.
type HTTPClientOption func(*wrappedHTTPClient)

// WithHTTPTimeout sets the request timeout.
func WithHTTPTimeout(timeout time.Duration) HTTPClientOption {
	return func(c *wrappedHTTPClient) {
		c.timeout = timeout
	}
}

// WithRetries configures retry behavior.
func WithRetries(maxRetries int, backoff time.Duration) HTTPClientOption {
	return func(c *wrappedHTTPClient) {
		c.maxRetries = maxRetries
		c.retryBackoff = backoff
	}
}

// WithCircuitBreaker attaches a circuit breaker.
func WithCircuitBreaker(cb CircuitBreaker) HTTPClientOption {
	return func(c *wrappedHTTPClient) {
		c.circuitBreaker = cb
	}
}

// wrappedHTTPClient wraps an HTTP client with health reporting.
type wrappedHTTPClient struct {
	client         *http.Client
	depID          string
	stateStore     DepStateStore
	circuitBreaker CircuitBreaker
	timeout        time.Duration
	maxRetries     int
	retryBackoff   time.Duration
}

// HTTPClient defines the interface for an HTTP client with health reporting.
type HTTPClient interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
	Get(ctx context.Context, url string) (*http.Response, error)
	Post(ctx context.Context, url, contentType string, body []byte) (*http.Response, error)
}

// WrapHTTPClient wraps an HTTP client for health reporting.
func WrapHTTPClient(client *http.Client, depID string, store DepStateStore, opts ...HTTPClientOption) HTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	c := &wrappedHTTPClient{
		client:       client,
		depID:        depID,
		stateStore:   store,
		timeout:      30 * time.Second,
		maxRetries:   0,
		retryBackoff: 100 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Do executes an HTTP request with health tracking.
func (c *wrappedHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Check circuit breaker
	if c.circuitBreaker != nil && !c.circuitBreaker.Allow() {
		return nil, &CircuitOpenError{DepID: c.depID}
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req = req.WithContext(ctx)

	var resp *http.Response
	var err error
	start := time.Now()

	// Execute with retries
	attempts := 1 + c.maxRetries
	for i := 0; i < attempts; i++ {
		resp, err = c.client.Do(req)
		if err == nil && resp.StatusCode < 500 {
			break
		}
		if i < attempts-1 {
			time.Sleep(c.retryBackoff * time.Duration(i+1))
		}
	}

	latency := time.Since(start)

	// Report result
	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		errType := "unknown"
		if err != nil {
			errType = err.Error()
		} else {
			errType = http.StatusText(resp.StatusCode)
		}
		c.reportFailure(errType)
		if c.circuitBreaker != nil {
			c.circuitBreaker.Failure()
		}
	} else {
		c.reportSuccess(latency)
		if c.circuitBreaker != nil {
			c.circuitBreaker.Success()
		}
	}

	return resp, err
}

// Get performs an HTTP GET request.
func (c *wrappedHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// Post performs an HTTP POST request.
func (c *wrappedHTTPClient) Post(ctx context.Context, url, contentType string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(ctx, req)
}

func (c *wrappedHTTPClient) reportSuccess(latency time.Duration) {
	if c.stateStore != nil && c.depID != "" {
		c.stateStore.ReportSuccess(c.depID, latency)
	}
}

func (c *wrappedHTTPClient) reportFailure(errType string) {
	if c.stateStore != nil && c.depID != "" {
		c.stateStore.ReportFailure(c.depID, errType)
	}
}

// CircuitOpenError is returned when the circuit breaker is open.
type CircuitOpenError struct {
	DepID string
}

func (e *CircuitOpenError) Error() string {
	return "circuit breaker open for dependency: " + e.DepID
}

// DepReporter provides a simple interface for reporting dependency status.
type DepReporter interface {
	// ReportSuccess reports a successful operation.
	ReportSuccess(latency time.Duration)
	// ReportFailure reports a failed operation.
	ReportFailure(errType string)
}

// depReporter is a simple reporter implementation.
type depReporter struct {
	depID      string
	stateStore DepStateStore
	cb         CircuitBreaker
}

// NewDepReporter creates a dependency reporter.
func NewDepReporter(depID string, store DepStateStore, cb CircuitBreaker) DepReporter {
	return &depReporter{
		depID:      depID,
		stateStore: store,
		cb:         cb,
	}
}

func (r *depReporter) ReportSuccess(latency time.Duration) {
	if r.stateStore != nil {
		r.stateStore.ReportSuccess(r.depID, latency)
	}
	if r.cb != nil {
		r.cb.Success()
	}
}

func (r *depReporter) ReportFailure(errType string) {
	if r.stateStore != nil {
		r.stateStore.ReportFailure(r.depID, errType)
	}
	if r.cb != nil {
		r.cb.Failure()
	}
}
