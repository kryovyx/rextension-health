// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package health contains tests for configuration utilities.
package health

import (
	"testing"
	"time"

	"github.com/kryovyx/rextension"
)

// --------------------------------------------------------------------------
// NewDefaultConfig tests
// --------------------------------------------------------------------------

func TestNewDefaultConfig(t *testing.T) {
	t.Run("returns_expected_defaults", func(t *testing.T) {
		// Returns_expected_defaults should populate all fields with expected values.
		cfg := NewDefaultConfig()

		if cfg.LivePath != "/live" {
			t.Fatalf("expected LivePath=/live, got %s", cfg.LivePath)
		}
		if cfg.ReadyPath != "/ready" {
			t.Fatalf("expected ReadyPath=/ready, got %s", cfg.ReadyPath)
		}
		if cfg.StatusPath != "/status" {
			t.Fatalf("expected StatusPath=/status, got %s", cfg.StatusPath)
		}
		if cfg.AtDefaultAddr {
			t.Fatal("expected AtDefaultAddr=false")
		}
		if cfg.SnapshotTTL != 5*time.Second {
			t.Fatalf("expected SnapshotTTL=5s, got %v", cfg.SnapshotTTL)
		}
		if cfg.CheckInterval != 10*time.Second {
			t.Fatalf("expected CheckInterval=10s, got %v", cfg.CheckInterval)
		}
		if !cfg.EnableDependencyGate {
			t.Fatal("expected EnableDependencyGate=true")
		}
	})
}

// --------------------------------------------------------------------------
// ConfigOption tests
// --------------------------------------------------------------------------

func TestConfigOptions(t *testing.T) {
	t.Run("WithLivePath_sets_path", func(t *testing.T) {
		// WithLivePath_sets_path should override the default live path.
		cfg := NewConfig(WithLivePath("/healthz"))
		if cfg.LivePath != "/healthz" {
			t.Fatalf("expected /healthz, got %s", cfg.LivePath)
		}
	})

	t.Run("WithReadyPath_sets_path", func(t *testing.T) {
		// WithReadyPath_sets_path should override the default ready path.
		cfg := NewConfig(WithReadyPath("/readyz"))
		if cfg.ReadyPath != "/readyz" {
			t.Fatalf("expected /readyz, got %s", cfg.ReadyPath)
		}
	})

	t.Run("WithStatusPath_sets_path", func(t *testing.T) {
		// WithStatusPath_sets_path should override the default status path.
		cfg := NewConfig(WithStatusPath("/healthcheck"))
		if cfg.StatusPath != "/healthcheck" {
			t.Fatalf("expected /healthcheck, got %s", cfg.StatusPath)
		}
	})

	t.Run("WithAtDefaultAddr_enables_flag", func(t *testing.T) {
		// WithAtDefaultAddr_enables_flag should set AtDefaultAddr to true.
		cfg := NewConfig(WithAtDefaultAddr(true))
		if !cfg.AtDefaultAddr {
			t.Fatal("expected AtDefaultAddr=true")
		}
	})

	t.Run("WithHealthRouter_sets_router_config", func(t *testing.T) {
		// WithHealthRouter_sets_router_config should override router settings.
		routerCfg := rextension.RouterConfig{Addr: ":9999", BaseURL: "/api"}
		cfg := NewConfig(WithHealthRouter(routerCfg))
		if cfg.Router.Addr != ":9999" {
			t.Fatalf("expected addr :9999, got %s", cfg.Router.Addr)
		}
	})

	t.Run("WithSnapshotTTL_sets_ttl", func(t *testing.T) {
		// WithSnapshotTTL_sets_ttl should override snapshot cache duration.
		cfg := NewConfig(WithSnapshotTTL(30 * time.Second))
		if cfg.SnapshotTTL != 30*time.Second {
			t.Fatalf("expected 30s, got %v", cfg.SnapshotTTL)
		}
	})

	t.Run("WithCheckInterval_sets_interval", func(t *testing.T) {
		// WithCheckInterval_sets_interval should override check interval.
		cfg := NewConfig(WithCheckInterval(1 * time.Minute))
		if cfg.CheckInterval != 1*time.Minute {
			t.Fatalf("expected 1m, got %v", cfg.CheckInterval)
		}
	})

	t.Run("WithStateStoreConfig_sets_store_config", func(t *testing.T) {
		// WithStateStoreConfig_sets_store_config should override state store settings.
		storeCfg := DepStateStoreConfig{FailureThreshold: 10}
		cfg := NewConfig(WithStateStoreConfig(storeCfg))
		if cfg.StateStoreConfig.FailureThreshold != 10 {
			t.Fatalf("expected FailureThreshold=10, got %d", cfg.StateStoreConfig.FailureThreshold)
		}
	})

	t.Run("WithDependencyGate_disables_gate", func(t *testing.T) {
		// WithDependencyGate_disables_gate should disable dependency gate.
		cfg := NewConfig(WithDependencyGate(false))
		if cfg.EnableDependencyGate {
			t.Fatal("expected EnableDependencyGate=false")
		}
	})
}

// --------------------------------------------------------------------------
// NewConfig tests
// --------------------------------------------------------------------------

func TestNewConfig(t *testing.T) {
	t.Run("applies_multiple_options", func(t *testing.T) {
		// Applies_multiple_options should apply all provided options.
		cfg := NewConfig(
			WithLivePath("/l"),
			WithReadyPath("/r"),
			WithCheckInterval(5*time.Second),
		)
		if cfg.LivePath != "/l" {
			t.Fatalf("expected /l, got %s", cfg.LivePath)
		}
		if cfg.ReadyPath != "/r" {
			t.Fatalf("expected /r, got %s", cfg.ReadyPath)
		}
		if cfg.CheckInterval != 5*time.Second {
			t.Fatalf("expected 5s, got %v", cfg.CheckInterval)
		}
	})
}
