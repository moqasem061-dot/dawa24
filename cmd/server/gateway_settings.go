package main

import (
	"context"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// adminGatewaySettings reads the credentials an operator saved in
// /admin/settings. It lives here, not in platform/, because platform packages
// must not import business modules.
type adminGatewaySettings struct {
	svc  *platformadmin.Service
	keys *adminKeyProvisioner
}

func newAdminGatewaySettings(svc *platformadmin.Service, keys *adminKeyProvisioner) *adminGatewaySettings {
	return &adminGatewaySettings{svc: svc, keys: keys}
}

// GatewaySettings resolves the runtime's Bearer credentials and model choices.
//
// The key it returns is always a virtual key. The previous version returned
// whatever was in the APIKey field, which holds an administrator credential for
// the Gateway's management API — a value /v1 answers with 401. Combined with a
// model alias the Gateway does not publish, that made every AI call fail and
// every caller fall back silently, which is how the AI features came to look
// switched on while doing nothing at all.
func (a *adminGatewaySettings) GatewaySettings(ctx context.Context) (gateway.Settings, error) {
	if a == nil || a.svc == nil {
		return gateway.Settings{}, nil
	}
	// database.AsSystem is justified: Gateway settings are platform-wide,
	// not scoped to any single organization tenant.
	sysCtx := database.AsSystem(ctx)

	gw, err := a.svc.GetGatewaySettings(sysCtx)
	if err != nil || gw == nil {
		return gateway.Settings{}, err
	}

	settings := gateway.Settings{
		BaseURL:      strings.TrimSpace(gw.EndpointURL),
		ClientApp:    "Dawa24Store",
		Enabled:      gw.IsActive,
		FastModel:    gw.FastModel,
		QualityModel: gw.QualityModel,
	}

	// The admin panel's own key, provisioned on demand from the administrator
	// credentials the operator configured.
	settings.VirtualKey = a.keys.Key(sysCtx)

	if settings.VirtualKey == "" {
		// The AI settings screen is the other place an operator can paste a key
		// directly, which is the escape hatch when provisioning is unavailable.
		if ai, aiErr := a.svc.GetAISettings(sysCtx); aiErr == nil && ai != nil && ai.APIKey != "" {
			if isVirtualKey(ai.APIKey) {
				settings.VirtualKey = ai.APIKey
				if settings.BaseURL == "" {
					settings.BaseURL = ai.EndpointURL
				}
				settings.Enabled = settings.Enabled || ai.IsActive
			}
		}
	}

	roleModels, err := a.svc.ListAIRoleModels(sysCtx)
	if err == nil && len(roleModels) > 0 {
		settings.RoleModels = make(map[string]string, len(roleModels))
		settings.RoleDisabled = make(map[string]bool, len(roleModels))
		settings.RoleMaxTokens = make(map[string]int, len(roleModels))
		for _, rm := range roleModels {
			if rm.Model != "" {
				settings.RoleModels[rm.Role] = strings.TrimSpace(rm.Model)
			}
			if !rm.IsActive {
				settings.RoleDisabled[rm.Role] = true
			}
			if rm.MaxTokens != nil && *rm.MaxTokens > 0 {
				settings.RoleMaxTokens[rm.Role] = *rm.MaxTokens
			}
		}
	}

	return settings, nil
}

// isVirtualKey reports whether a configured value looks like a Bearer key the
// Gateway issued rather than an administrator credential.
//
// Operators paste the wrong one into these fields often enough that guarding is
// worth more than the guess costs: a "user:password" value in the Bearer slot
// disables AI silently, while rejecting it here leaves a log line saying so.
func isVirtualKey(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.Contains(trimmed, ":") {
		return false
	}
	return strings.HasPrefix(trimmed, "sk-")
}
