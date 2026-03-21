# Rex Health Extension (rextension-health)

A comprehensive health checking and dependency management extension for the Rex framework.

[![Go Version](https://img.shields.io/badge/go-1.26+-blue.svg)](https://golang.org/dl/)
[![Coverage](https://img.shields.io/badge/coverage-99.3%25-brightgreen.svg)](#)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## Overview

`rextension-health` is a Rex extension that provides:

- **Health endpoints**: Liveness, readiness, and detailed status endpoints
- **Dependency state tracking**: Monitor UP/DEGRADED/DOWN states for dependencies
- **Health check registry**: Register and execute custom health checks with TTL caching
- **Circuit breaker pattern**: Protect against cascading failures
- **Dependency gate middleware**: Block requests when critical dependencies are down
- **HTTP client wrapper**: Automatic health reporting for outbound HTTP calls
- **Dedicated router support**: Optionally expose health endpoints on a separate port
- **Per-route dependency mapping**: Routes declare their dependencies for targeted gating

## Installation

```bash
go get github.com/kryovyx/rextension-health
```

## Quick Start

```go
package main

import (
    "github.com/kryovyx/rex"
    "github.com/kryovyx/rex/route"
    health "github.com/kryovyx/rextension-health"
)

func main() {
    app := rex.New()

    // Add health extension with default config
    app.WithOptions(
        health.WithHealth(nil),
    )

    // Register your routes
    app.RegisterRoute(route.New("GET", "/hello", func(ctx route.Context) {
        ctx.Text(200, "Hello, World!")
    }))

    // Run the application
    // Health endpoints available at :9091/live, /ready, /status
    if err := app.Run(); err != nil {
        panic(err)
    }
}
```

## Core Concepts

### Health Checks

Health checks are functions that verify the state of a dependency or subsystem. Each check has a name, a check function, and configurable behavior.

```go
var registry health.Registry
app.Container().Resolve(&registry)

// Register a database health check
registry.Register(health.NewCheck("database",
    func(ctx context.Context, resolver dix.Resolver) *health.CheckResult {
        if err := db.Ping(); err != nil {
            return health.NewCheckResult(health.StatusDown, err.Error(), 0)
        }
        return health.NewCheckResult(health.StatusUp, "Connected", 0)
    },
    health.WithTimeout(5*time.Second),
    health.WithReadiness(true),
))
```

#### Check Modes

| Mode | Behavior |
|------|----------|
| `CheckModeActive` | Runs periodically via concurrent polling |
| `CheckModePassive` | Runs on-demand with result caching |
| `CheckModeOnDemand` | Runs fresh on every request (no caching) |

#### Check Options

```go
registry.Register(health.NewCheck("redis",
    redisCheckFunc,
    health.WithTimeout(3*time.Second),        // Check timeout
    health.WithReadiness(true),               // Affects readiness
    health.WithTags("cache", "critical"),     // Tags for filtering
    health.WithMode(health.CheckModeActive),  // Check mode
    health.WithCacheTTL(10*time.Second),      // Cache duration (passive)
))
```

### Dependency States

The extension tracks dependency states across the application:

| Status | Description | Readiness Impact |
|--------|-------------|------------------|
| `UP` | Fully operational | Ready |
| `DEGRADED` | Operational with issues | Ready (with warning) |
| `DOWN` | Not operational | Not Ready |
| `UNKNOWN` | State not yet determined | Not Ready |

```go
var stateStore health.DepStateStore
app.Container().Resolve(&stateStore)

// Register and update dependency state
stateStore.Register("external-api")
stateStore.Update("external-api", health.StatusUp, "Response time: 50ms")
stateStore.Update("external-api", health.StatusDegraded, "Slow response: 2s")
stateStore.Update("external-api", health.StatusDown, "Connection refused")

// Query state
state := stateStore.Get("external-api")
fmt.Printf("Status: %s, Message: %s\n", state.Status, state.Message)

// Get all states
allStates := stateStore.All()
```

### Circuit Breaker

Protect against cascading failures with the circuit breaker pattern:

```go
// Create circuit breaker
cb := health.NewCircuitBreaker(health.CircuitBreakerConfig{
    FailureThreshold: 5,              // Open after 5 failures
    SuccessThreshold: 2,              // Close after 2 successes in half-open
    Timeout:          30*time.Second,  // Time before half-open
    HalfOpenMaxCalls: 1,              // Max concurrent calls in half-open
})

// With state store integration
cb := health.NewCircuitBreakerWithStore(cfg, stateStore, "external-api")

// Use the circuit breaker
if cb.Allow() {
    err := callExternalAPI()
    if err != nil {
        cb.Failure()
    } else {
        cb.Success()
    }
}

// Check state
state := cb.State() // CircuitClosed, CircuitOpen, or CircuitHalfOpen
```

#### Circuit States

| State | Description | Behavior |
|-------|-------------|----------|
| `CircuitClosed` | Normal operation | All requests allowed |
| `CircuitOpen` | Failures exceeded threshold | All requests rejected |
| `CircuitHalfOpen` | Testing recovery | Limited requests allowed |

### Middleware (Dependency Gate)

The dependency gate middleware blocks requests when critical dependencies are down. Routes can declare their dependencies by implementing `HealthDepRoute`:

```go
// Define a route with dependencies
type MyRoute struct {
    route.Route
    deps []string
}

func (r *MyRoute) Dependencies() []string {
    return r.deps
}

// Register the route
app.RegisterRoute(&MyRoute{
    Route: route.New("GET", "/api/orders", ordersHandler),
    deps:  []string{"database", "inventory-service"},
})
```

**Middleware behavior:**

1. **All UP**: Request proceeds normally
2. **Some DEGRADED**: Request proceeds; degraded deps available in context
3. **Any DOWN**: Request rejected with `503 Service Unavailable`

**Accessing dependency state in handlers:**

```go
func handler(ctx route.Context) {
    if health.IsDegraded(ctx, "cache") {
        // Use fallback logic
    }

    depCtx := health.GetDepStateContext(ctx)
    if depCtx != nil {
        for _, degraded := range depCtx.DegradedDeps {
            log.Printf("Warning: %s is degraded", degraded)
        }
    }

    ctx.JSON(200, response)
}
```

## Configuration Reference

```go
type Config struct {
    LivePath            string              // Liveness endpoint path (default: "/live")
    ReadyPath           string              // Readiness endpoint path (default: "/ready")
    StatusPath          string              // Status endpoint path (default: "/status")
    AtDefaultAddr       bool                // Serve on default router instead of dedicated
    Router              rextension.RouterConfig // Dedicated router config (default: ":9091")
    SnapshotTTL         time.Duration       // Cache duration for health snapshots (default: 5s)
    CheckInterval       time.Duration       // How often active checks run (default: 10s)
    StateStoreConfig    DepStateStoreConfig // Dependency state store configuration
    EnableDependencyGate bool               // Enable the dependency gate middleware (default: true)
}
```

### Config Options

```go
health.WithLivePath("/healthz")              // Custom liveness path
health.WithReadyPath("/readyz")              // Custom readiness path
health.WithStatusPath("/healthz/status")     // Custom status path
health.WithAtDefaultAddr(true)               // Serve on default router
health.WithHealthRouter(cfg)                 // Custom dedicated router config
health.WithSnapshotTTL(10*time.Second)       // Snapshot cache TTL
health.WithCheckInterval(15*time.Second)     // Active check interval
health.WithStateStoreConfig(storeCfg)        // State store configuration
health.WithDependencyGate(false)             // Disable dependency gate
```

### Default Configuration

```go
cfg := health.NewDefaultConfig()
// LivePath:             "/live"
// ReadyPath:            "/ready"
// StatusPath:           "/status"
// AtDefaultAddr:        false
// Router.Addr:          ":9091"
// SnapshotTTL:          5 * time.Second
// CheckInterval:        10 * time.Second
// EnableDependencyGate: true
```

## Endpoints

| Endpoint | Method | Default Port | Description | Success | Failure |
|----------|--------|-------------|-------------|---------|---------|
| `/live` | GET | 9091 | Liveness probe | `200 OK` | — |
| `/ready` | GET | 9091 | Readiness probe | `200 OK` | `503 Service Unavailable` |
| `/status` | GET | 9091 | Detailed health report (JSON) | `200` with report | `503` with report |

### Status Response Format

```json
{
  "status": "UP",
  "timestamp": "2026-01-21T10:30:00Z",
  "dependencies": {
    "database": {
      "status": "UP",
      "message": "Connected",
      "last_check": "2026-01-21T10:29:55Z"
    },
    "cache": {
      "status": "DEGRADED",
      "message": "High latency",
      "last_check": "2026-01-21T10:29:55Z"
    }
  }
}
```

## HTTP Client Wrapper

Automatically track health for outbound HTTP calls:

```go
client := health.WrapHTTPClient(
    http.DefaultClient,
    "payment-service",
    stateStore,
    health.WithHTTPTimeout(10*time.Second),
    health.WithRetries(3, 100*time.Millisecond),
    health.WithCircuitBreaker(cb),
)

// State store automatically updated on success/failure
resp, err := client.Get(ctx, "https://api.payment.com/status")
```

### HTTP Client Options

```go
health.WithHTTPTimeout(timeout)            // Request timeout
health.WithRetries(maxRetries, backoff)    // Retry configuration
health.WithCircuitBreaker(cb)              // Attach circuit breaker
```

## Complete Example

```go
package main

import (
    "context"
    "database/sql"
    "time"

    "github.com/kryovyx/dix"
    "github.com/kryovyx/rex"
    health "github.com/kryovyx/rextension-health"
)

func main() {
    app := rex.New()

    // Add health extension
    app.WithOptions(
        health.WithHealth(&health.Config{
            CheckInterval: 15 * time.Second,
            SnapshotTTL:   10 * time.Second,
        }),
    )

    // Get components via DI
    var (
        registry   health.Registry
        stateStore health.DepStateStore
    )
    app.Container().Resolve(&registry)
    app.Container().Resolve(&stateStore)

    // Register database health check
    registry.Register(health.NewCheck("database",
        func(ctx context.Context, resolver dix.Resolver) *health.CheckResult {
            var db *sql.DB
            resolver.Resolve(&db)

            start := time.Now()
            if err := db.PingContext(ctx); err != nil {
                return health.NewCheckResult(health.StatusDown, err.Error(), time.Since(start))
            }
            return health.NewCheckResult(health.StatusUp, "OK", time.Since(start))
        },
        health.WithTimeout(5*time.Second),
        health.WithReadiness(true),
    ))

    // Register external API with circuit breaker
    cb := health.NewCircuitBreakerWithStore(
        health.DefaultCircuitBreakerConfig(),
        stateStore,
        "payment-api",
    )

    apiClient := health.WrapHTTPClient(
        nil, // use default client
        "payment-api",
        stateStore,
        health.WithCircuitBreaker(cb),
        health.WithHTTPTimeout(10*time.Second),
    )

    // Make client available via DI
    app.Container().Instance(apiClient)

    app.Run()
}
```

## Kubernetes Integration

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: app
    ports:
    - containerPort: 8080
      name: http
    - containerPort: 9091
      name: health
    livenessProbe:
      httpGet:
        path: /live
        port: health
      initialDelaySeconds: 5
      periodSeconds: 10
    readinessProbe:
      httpGet:
        path: /ready
        port: health
      initialDelaySeconds: 5
      periodSeconds: 5
```

## Best Practices

1. **Use appropriate check modes**: Active for critical dependencies, Passive for expensive checks, OnDemand for real-time requirements
2. **Set reasonable timeouts**: Health check timeouts should be shorter than check intervals
3. **Implement graceful degradation**: Handle `DEGRADED` state in your handlers rather than failing
4. **Use circuit breakers**: Protect against cascading failures for all external service calls
5. **Separate health port**: Use the dedicated router (default `:9091`) for health endpoints in production
6. **Tag your checks**: Use tags to categorize and filter health checks by domain
7. **Cache appropriately**: Balance freshness vs. performance with `SnapshotTTL`
8. **Declare route dependencies**: Implement `HealthDepRoute` on routes to enable targeted dependency gating

## Contributing

**At this time, this project is in active development and is not open for external contributions.** The framework is still being refined and major interfaces may change.

Once the framework reaches a stable architecture and API, contributions from the community will be welcome. Please check back later or open an issue if you have feature requests or feedback.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Copyright

© 2026 Kryovyx
