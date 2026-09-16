package authctx

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

func TestFromGrantCarriesTheWholeHolding(t *testing.T) {
	branch := int64(9)
	keys := []string{"pharmacy.order.view", "pharmacy.assistant.use"}
	a := FromGrant(rbac.Grant{
		UserID: 3, OrganizationID: 4, Active: true, Scope: rbac.ScopePharmacy,
		OrgType: "customer", OrgStatus: "approved", BranchID: &branch, Name: "n",
		Keys: keys, Permissions: rbac.NewSet(keys),
	})
	if a.UserID != 3 || a.OrgID != 4 || a.OrganizationID != 4 || a.BranchID == nil || *a.BranchID != 9 || a.BoundBranchID == nil || *a.BoundBranchID != 9 {
		t.Fatalf("identity not carried: %+v", a)
	}
	if !a.Can("pharmacy.order.view") || a.Can("vendor.order.view") || a.DashboardScope() != rbac.ScopePharmacy {
		t.Fatalf("holding not carried: %+v", a)
	}
}

func TestFromGrantForNonMemberHasNoAuthority(t *testing.T) {
	a := FromGrant(rbac.Grant{UserID: 3, OrganizationID: 4, Active: true})
	if a.DashboardScope() != "" || a.Can("pharmacy.order.view") || a.IsOrgApproved() {
		t.Fatalf("a grant with no membership produced authority: %+v", a)
	}
}
