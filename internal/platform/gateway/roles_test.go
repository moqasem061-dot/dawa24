package gateway

import (
	"context"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

func TestResolveRoleModel(t *testing.T) {
	// 1. Default fallback
	settings := Settings{}
	m := ResolveRoleModel(settings, RoleMatchVendorImport)
	if m != "qwen3.7-flash" {
		t.Errorf("expected default qwen3.7-flash, got %s", m)
	}

	// 2. Fallback to RoleMatching when sub-role is unset
	settingsWithMatching := Settings{
		RoleModels: map[string]string{
			string(RoleMatching): "custom-matching-fallback",
		},
	}
	m = ResolveRoleModel(settingsWithMatching, RoleMatchVendorImport)
	if m != "custom-matching-fallback" {
		t.Errorf("expected custom-matching-fallback, got %s", m)
	}

	// 3. Sub-role explicit in DB overrides RoleMatching
	settingsWithSubRole := Settings{
		RoleModels: map[string]string{
			string(RoleMatching):            "custom-matching-fallback",
			string(RoleMatchVendorImport):   "vendor-specific-model",
			string(RoleMatchSavingProducts): "savings-specific-model",
			string(RoleMatchSmartOrder):     "order-specific-model",
			string(RoleMatchAdminCatalog):   "catalog-specific-model",
		},
	}
	if got := ResolveRoleModel(settingsWithSubRole, RoleMatchVendorImport); got != "vendor-specific-model" {
		t.Errorf("expected vendor-specific-model, got %s", got)
	}
	if got := ResolveRoleModel(settingsWithSubRole, RoleMatchSavingProducts); got != "savings-specific-model" {
		t.Errorf("expected savings-specific-model, got %s", got)
	}
	if got := ResolveRoleModel(settingsWithSubRole, RoleMatchSmartOrder); got != "order-specific-model" {
		t.Errorf("expected order-specific-model, got %s", got)
	}
	if got := ResolveRoleModel(settingsWithSubRole, RoleMatchAdminCatalog); got != "catalog-specific-model" {
		t.Errorf("expected catalog-specific-model, got %s", got)
	}

	// 4. Environment variable override
	t.Setenv("GATEWAY_MODEL_MATCHING_VENDOR_IMPORT", "env-vendor-model")
	emptySettings := Settings{}
	if got := ResolveRoleModel(emptySettings, RoleMatchVendorImport); got != "env-vendor-model" {
		t.Errorf("expected env-vendor-model, got %s", got)
	}

	// 5. DB settings override environment variable
	if got := ResolveRoleModel(settingsWithSubRole, RoleMatchVendorImport); got != "vendor-specific-model" {
		t.Errorf("expected DB setting vendor-specific-model to override env, got %s", got)
	}
}

func TestRoleForRequest(t *testing.T) {
	cases := []struct {
		req      Request
		expected Role
	}{
		{
			req:      Request{Role: RolePrimary},
			expected: RolePrimary,
		},
		{
			req:      Request{Feature: "variant_match"},
			expected: RoleMatchVendorImport,
		},
		{
			req:      Request{Feature: "savings_import"},
			expected: RoleMatchSavingProducts,
		},
		{
			req:      Request{Feature: "smart_order"},
			expected: RoleMatchSmartOrder,
		},
		{
			req:      Request{Feature: "catalog_import"},
			expected: RoleMatchAdminCatalog,
		},
		{
			req:      Request{Capability: CapMatchEnhance},
			expected: RoleMatching,
		},
		{
			req:      Request{Capability: CapColumnDetect},
			expected: RoleColumns,
		},
		{
			req:      Request{Capability: CapSearchExpand},
			expected: RoleExpand,
		},
	}

	for _, tc := range cases {
		got := RoleForRequest(tc.req)
		if got != tc.expected {
			t.Errorf("RoleForRequest(%+v) = %s; want %s", tc.req, got, tc.expected)
		}
	}
}

func TestRoleDisabledAndMaxTokens(t *testing.T) {
	settings := Settings{
		RoleDisabled: map[string]bool{
			string(RoleMatchVendorImport): true,
		},
		RoleMaxTokens: map[string]int{
			string(RoleMatchSmartOrder): 4096,
		},
	}

	if !IsRoleDisabled(settings, RoleMatchVendorImport) {
		t.Error("expected RoleMatchVendorImport to be disabled")
	}
	if IsRoleDisabled(settings, RoleMatchSmartOrder) {
		t.Error("expected RoleMatchSmartOrder to be enabled")
	}

	if tokens := GetRoleMaxTokens(settings, RoleMatchSmartOrder); tokens != 4096 {
		t.Errorf("expected max tokens 4096, got %d", tokens)
	}
	if tokens := GetRoleMaxTokens(settings, RoleMatchVendorImport); tokens != 0 {
		t.Errorf("expected max tokens 0, got %d", tokens)
	}
}

type staticSettingsSource struct {
	settings Settings
}

func (s staticSettingsSource) GatewaySettings(ctx context.Context) (Settings, error) {
	return s.settings, nil
}

func TestHTTPClientRoleDisabledReturnsErrDisabled(t *testing.T) {
	settings := Settings{
		BaseURL:    "http://127.0.0.1:9999",
		VirtualKey: "sk-virt-test",
		Enabled:    true,
		RoleDisabled: map[string]bool{
			string(RoleMatchVendorImport): true,
		},
	}

	client := New(config.Gateway{}, slog.Default()).WithSettingsSource(staticSettingsSource{settings: settings})
	_, err := client.Invoke(context.Background(), Request{
		Capability: CapMatchEnhance,
		Feature:    "variant_match",
	})
	if err != ErrDisabled {
		t.Errorf("expected ErrDisabled when role is disabled, got %v", err)
	}
}
