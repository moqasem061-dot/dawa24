package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

func renderBaseWithActor(actor authctx.Actor) (string, error) {
	ctx := context.Background()
	if actor.UserID > 0 {
		ctx = authctx.WithActor(ctx, actor)
	}
	var buf bytes.Buffer
	comp := Base("Test Page", "ar", "rtl")
	err := comp.Render(ctx, &buf)
	return buf.String(), err
}

func TestBaseAssistantTrigger_PermissionGating(t *testing.T) {
	tests := []struct {
		name       string
		actor      authctx.Actor
		wantRender bool
	}{
		{
			name:       "Anonymous user",
			actor:      authctx.Actor{},
			wantRender: false,
		},
		{
			name: "Logged-in pharmacy employee without assistant permission",
			actor: authctx.Actor{
				UserID:  10,
				OrgID:   1,
				OrgType: "customer",
				Scope:   rbac.ScopePharmacy,
				Permissions: []string{
					"pharmacy.dashboard.view",
					"pharmacy.order.view",
				},
			},
			wantRender: false,
		},
		{
			name: "Pharmacy user with explicit assistant permission",
			actor: authctx.Actor{
				UserID:  10,
				OrgID:   1,
				OrgType: "customer",
				Scope:   rbac.ScopePharmacy,
				Permissions: []string{
					"pharmacy.assistant.use",
				},
			},
			wantRender: true,
		},
		{
			name: "Pharmacy owner with wildcard permission",
			actor: authctx.Actor{
				UserID:      10,
				OrgID:       1,
				OrgType:     "customer",
				Scope:       rbac.ScopePharmacy,
				IsOwner:     true,
				Permissions: []string{"pharmacy.*"},
			},
			wantRender: true,
		},
		{
			name: "Logged-in vendor employee without assistant permission",
			actor: authctx.Actor{
				UserID:  20,
				OrgID:   2,
				OrgType: "vendor",
				Scope:   rbac.ScopeVendor,
				Permissions: []string{
					"vendor.order.view",
				},
			},
			wantRender: false,
		},
		{
			name: "Vendor user with explicit assistant permission",
			actor: authctx.Actor{
				UserID:  20,
				OrgID:   2,
				OrgType: "vendor",
				Scope:   rbac.ScopeVendor,
				Permissions: []string{
					"vendor.assistant.use",
				},
			},
			wantRender: true,
		},
		{
			name: "Platform admin with platform.assistant.use",
			actor: authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "admin",
				Scope:       rbac.ScopeAdmin,
				Permissions: []string{"platform.assistant.use"},
			},
			wantRender: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := renderBaseWithActor(tt.actor)
			if err != nil {
				t.Fatalf("render error: %v", err)
			}
			hasTrigger := strings.Contains(html, "capsule-assistant-host")
			if hasTrigger != tt.wantRender {
				t.Errorf("got hasTrigger = %v, want %v", hasTrigger, tt.wantRender)
			}
		})
	}
}
