package layouts

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// An unapproved organization sees its documents, its notifications, and its
// members' own account settings — and nothing else.
//
// Account settings is the deliberate third item. A member whose company is
// still under review must be able to change their own password and revoke a
// session on a lost device; withholding that is not a safety property, it is a
// way to strand someone. It carries no permission for the same reason (see
// rbac.NavItem.AlwaysVisible) and its routes sit in the pre-approval tier.
func TestVisibleNav_UnapprovedUser_ShowsOnlyOwnAccountAndDocuments(t *testing.T) {
	actor := authctx.Actor{
		UserID:         42,
		OrganizationID: 10,
		Role:           "customer_owner",
		Scope:          rbac.ScopePharmacy,
		OrgStatus:      "pending",
		Permissions: []string{
			"pharmacy.dashboard.view",
			"pharmacy.purchase_request.view",
			"pharmacy.order.view",
			"pharmacy.invoice.view",
			"pharmacy.documents.view",
		},
	}
	ctx := authctx.WithActor(context.Background(), actor)

	sections := visibleNav(ctx, rbac.ScopePharmacy)
	if len(sections) == 0 {
		t.Fatalf("expected unapproved user to see documents/notifications section, got none")
	}

	for _, sec := range sections {
		for _, item := range sec.Items {
			allowed := item.Key == "documents" ||
				item.Key == "notifications" ||
				item.Key == "account_settings" ||
				item.Href == "/customer/documents" ||
				item.Href == "/documents" ||
				item.Href == "/notifications" ||
				item.Href == "/settings"
			if !allowed {
				t.Errorf("unapproved user saw non-allowed item: key=%s href=%s", item.Key, item.Href)
			}
		}
	}
}

func TestVisibleNav_ApprovedUser_ShowsFullNavigation(t *testing.T) {
	actor := authctx.Actor{
		UserID:         42,
		OrganizationID: 10,
		Role:           "customer_owner",
		Scope:          rbac.ScopePharmacy,
		OrgStatus:      "approved",
		Permissions: []string{
			"pharmacy.dashboard.view",
			"pharmacy.purchase_request.view",
			"pharmacy.order.view",
		},
	}
	ctx := authctx.WithActor(context.Background(), actor)

	sections := visibleNav(ctx, rbac.ScopePharmacy)
	if len(sections) == 0 {
		t.Fatalf("expected approved user to have visible nav sections, got none")
	}

	foundDashboard := false
	for _, sec := range sections {
		for _, item := range sec.Items {
			if item.Key == "dashboard" {
				foundDashboard = true
			}
		}
	}
	if !foundDashboard {
		t.Errorf("approved user should see dashboard nav item")
	}
}

// TestVisibleNav_UnapprovedUser_KeepsAccountSettings states the positive half
// of the rule above: the item must actually be there, not merely tolerated.
func TestVisibleNav_UnapprovedUser_KeepsAccountSettings(t *testing.T) {
	actor := authctx.Actor{
		UserID:         42,
		OrganizationID: 10,
		Scope:          rbac.ScopePharmacy,
		OrgStatus:      "pending",
		Permissions:    []string{"pharmacy.dashboard.view"},
	}
	ctx := authctx.WithActor(context.Background(), actor)

	for _, sec := range visibleNav(ctx, rbac.ScopePharmacy) {
		for _, item := range sec.Items {
			if item.Key == "account_settings" && item.Href == "/settings" {
				return
			}
		}
	}
	t.Error("a member of a pending organization cannot reach their own account settings")
}
