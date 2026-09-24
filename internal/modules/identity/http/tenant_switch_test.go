package http

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// Switching to an organization where the user is a junior employee must not
// carry the owner authority they hold in their own organization.
func TestSwitchActorOrgDropsPreviousOrgAuthority(t *testing.T) {
	svc := identity.NewService(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	branch := int64(9)
	a := authctx.Actor{UserID: 5, OrganizationID: 1, OrgID: 1, IsOwner: true, BranchID: &branch}
	a.Grants([]string{"vendor.wallet.manage", "vendor.team.manage"})

	got := switchActorOrg(context.Background(), svc, a, 5, 2, "vendor", "approved", []string{"vendor.order.view"})

	if got.Can("vendor.wallet.manage") || got.Can("vendor.team.manage") {
		t.Error("permissions from organization 1 survived the switch to organization 2")
	}
	if !got.Can("vendor.order.view") {
		t.Error("organization 2 permission missing after switch")
	}
	if got.IsOwner || got.BranchID != nil {
		t.Errorf("owner/branch carried over: owner=%v branch=%v", got.IsOwner, got.BranchID)
	}
	if got.OrgID != 2 || got.OrganizationID != 2 {
		t.Errorf("org ids = %d/%d; want 2/2", got.OrgID, got.OrganizationID)
	}
}
