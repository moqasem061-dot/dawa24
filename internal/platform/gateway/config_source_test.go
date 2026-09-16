package gateway

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

type mockSettingsSource struct {
	mu       sync.Mutex
	settings Settings
	err      error
}

func (m *mockSettingsSource) GatewaySettings(ctx context.Context) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings, m.err
}

func (m *mockSettingsSource) update(s Settings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = s
}

func TestPhase1_AdminSettingsResolution(t *testing.T) {
	envCfg := config.Gateway{
		BaseURL:    "https://env.muhiya.com",
		VirtualKey: "sk-env-key-12345",
		ClientApp:  "Dawa24StoreEnv",
		Enabled:    true,
		Timeout:    10 * time.Second,
	}

	src := &mockSettingsSource{
		settings: Settings{
			BaseURL:    "https://admin.muhiya.com",
			VirtualKey: "sk-admin-key-67890",
			ClientApp:  "Dawa24StoreAdmin",
			Enabled:    true,
		},
	}

	client := New(envCfg, slog.Default()).WithSettingsSource(src)

	// T1.1: admin settings present and active -> resolve returns them
	s := client.resolve(context.Background())
	if s.BaseURL != "https://admin.muhiya.com" || s.VirtualKey != "sk-admin-key-67890" {
		t.Fatalf("T1.1 failed: expected admin settings, got %+v", s)
	}

	// T1.2: admin settings disabled -> falls back to env
	src.update(Settings{
		BaseURL:    "https://admin.muhiya.com",
		VirtualKey: "",
		Enabled:    false,
	})
	// Reset cache to test fallback resolution
	client.cache = newSettingsCache(30 * time.Second)

	s2 := client.resolve(context.Background())
	if s2.BaseURL != "https://env.muhiya.com" || s2.VirtualKey != "sk-env-key-12345" {
		t.Fatalf("T1.2 failed: expected env settings fallback, got %+v", s2)
	}
}

func TestPhase1_CacheTTL(t *testing.T) {
	// T1.3: changing settings takes effect when cache expires
	src := &mockSettingsSource{
		settings: Settings{
			BaseURL:    "https://v1.muhiya.com",
			VirtualKey: "sk-key-v1",
			Enabled:    true,
		},
	}

	client := New(config.Gateway{Enabled: true}, slog.Default()).WithSettingsSource(src)
	client.cache = newSettingsCache(50 * time.Millisecond) // Short TTL for test

	s1 := client.resolve(context.Background())
	if s1.BaseURL != "https://v1.muhiya.com" {
		t.Fatalf("expected v1 settings, got %+v", s1)
	}

	// Update source
	src.update(Settings{
		BaseURL:    "https://v2.muhiya.com",
		VirtualKey: "sk-key-v2",
		Enabled:    true,
	})

	// Before expiry -> still returns cached v1
	sCached := client.resolve(context.Background())
	if sCached.BaseURL != "https://v1.muhiya.com" {
		t.Fatalf("expected cached v1 settings, got %+v", sCached)
	}

	// Wait for cache TTL
	time.Sleep(60 * time.Millisecond)

	// After expiry -> returns updated v2
	s2 := client.resolve(context.Background())
	if s2.BaseURL != "https://v2.muhiya.com" || s2.VirtualKey != "sk-key-v2" {
		t.Fatalf("T1.3 failed: expected updated v2 settings after TTL, got %+v", s2)
	}
}

func TestPhase1_KeyNeverLogged(t *testing.T) {
	// T1.4: API key never appears in log output
	secretKey := "sk-super-secret-key-9999"

	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	src := &mockSettingsSource{
		settings: Settings{
			BaseURL:    "https://api.muhiya.com",
			VirtualKey: secretKey,
			Enabled:    true,
		},
	}

	client := New(config.Gateway{Enabled: true}, logger).WithSettingsSource(src)
	_ = client.resolve(context.Background())

	logs := logBuf.String()
	if strings.Contains(logs, secretKey) {
		t.Fatalf("T1.4 failed: secret key was leaked into logs: %s", logs)
	}
}
