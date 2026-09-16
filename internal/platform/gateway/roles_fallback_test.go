package gateway

import (
	"context"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// TestEveryRoleDeterministicFallbackWhenGatewayDisabled proves that for each
// configured role, when the Gateway is disabled globally, the invocation
// returns ErrDisabled so the caller can trigger its deterministic fallback.
func TestEveryRoleDeterministicFallbackWhenGatewayDisabled(t *testing.T) {
	roles := []struct {
		role    Role
		feature string
		cap     Capability
	}{
		{role: RoleMatchVendorImport, feature: "variant_match", cap: CapMatchEnhance},
		{role: RoleMatchSavingProducts, feature: "savings_import", cap: CapMatchEnhance},
		{role: RoleMatchSmartOrder, feature: "smart_order", cap: CapMatchEnhance},
		{role: RoleMatchAdminCatalog, feature: "catalog_import", cap: CapMatchEnhance},
		{role: RoleMatching, feature: "", cap: CapProductMatch},
		{role: RoleColumns, feature: "column_detect", cap: CapColumnDetect},
		{role: RoleExpand, feature: "search_expand", cap: CapSearchExpand},
		{role: RoleClassify, feature: "classify", cap: CapMemoryExtract},
	}

	// 1. Gateway disabled globally
	disabledSettings := Settings{
		Enabled: false,
	}
	client := New(config.Gateway{}, slog.Default()).WithSettingsSource(staticSettingsSource{settings: disabledSettings})

	for _, tc := range roles {
		t.Run(string(tc.role)+"_global_disabled", func(t *testing.T) {
			_, err := client.Invoke(context.Background(), Request{
				Capability: tc.cap,
				Feature:    tc.feature,
				Role:       tc.role,
			})
			if err != ErrDisabled {
				t.Fatalf("expected ErrDisabled for role %s, got %v", tc.role, err)
			}
		})
	}

	// 2. Gateway enabled globally, but individual role is disabled
	for _, tc := range roles {
		t.Run(string(tc.role)+"_role_disabled", func(t *testing.T) {
			roleDisabledSettings := Settings{
				BaseURL:    "http://127.0.0.1:9999",
				VirtualKey: "sk-virt-test",
				Enabled:    true,
				RoleDisabled: map[string]bool{
					string(tc.role): true,
				},
			}
			clientRoleDisabled := New(config.Gateway{}, slog.Default()).WithSettingsSource(staticSettingsSource{settings: roleDisabledSettings})

			_, err := clientRoleDisabled.Invoke(context.Background(), Request{
				Capability: tc.cap,
				Feature:    tc.feature,
				Role:       tc.role,
			})
			if err != ErrDisabled {
				t.Fatalf("expected ErrDisabled when role %s is individually disabled, got %v", tc.role, err)
			}
		})
	}
}
