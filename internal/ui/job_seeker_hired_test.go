package ui

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func TestJobSeekerHired_ActorAndSessionGating(t *testing.T) {
	// 1. Unhired job seeker: must be recognized as job seeker
	unhired := authctx.Actor{
		UserID:         100,
		Role:           identity.RoleJobSeeker,
		OrganizationID: 0,
	}
	if !unhired.IsJobSeeker() {
		t.Errorf("expected unhired job seeker to return IsJobSeeker() == true")
	}

	// 2. Hired job seeker into a customer (pharmacy) org: must NOT be treated as job seeker
	hiredCustomer := authctx.Actor{
		UserID:         100,
		Role:           identity.RoleJobSeeker,
		OrganizationID: 10,
		OrgType:        "customer",
		OrgStatus:      "approved",
	}
	if hiredCustomer.IsJobSeeker() {
		t.Errorf("expected hired job seeker to return IsJobSeeker() == false")
	}
	if !hiredCustomer.IsCustomer() {
		t.Errorf("expected hired job seeker in customer org to return IsCustomer() == true")
	}

	// 3. Hired job seeker into a vendor org: must NOT be treated as job seeker
	hiredVendor := authctx.Actor{
		UserID:         100,
		Role:           identity.RoleJobSeeker,
		OrganizationID: 20,
		OrgType:        "vendor",
		OrgStatus:      "approved",
	}
	if hiredVendor.IsJobSeeker() {
		t.Errorf("expected hired job seeker to return IsJobSeeker() == false")
	}
	if !hiredVendor.IsVendor() {
		t.Errorf("expected hired job seeker in vendor org to return IsVendor() == true")
	}

	// 4. Test landingPathForSession
	sessUnhired := &identity.Session{
		UserID:      100,
		Role:        identity.RoleJobSeeker,
		ActiveOrgID: 0,
	}
	if landingPathForSession(sessUnhired) != "/jobs" {
		t.Errorf("expected unhired session to land on /jobs, got %s", landingPathForSession(sessUnhired))
	}

	sessHiredCustomer := &identity.Session{
		UserID:      100,
		Role:        identity.RoleJobSeeker,
		ActiveOrgID: 10,
		OrgType:     "customer",
		OrgStatus:   "approved",
	}
	if landingPathForSession(sessHiredCustomer) != "/customer/dashboard" {
		t.Errorf("expected hired customer session to land on /customer/dashboard, got %s", landingPathForSession(sessHiredCustomer))
	}

	sessHiredVendor := &identity.Session{
		UserID:      100,
		Role:        identity.RoleJobSeeker,
		ActiveOrgID: 20,
		OrgType:     "vendor",
		OrgStatus:   "approved",
	}
	if landingPathForSession(sessHiredVendor) != "/vendor/dashboard" {
		t.Errorf("expected hired vendor session to land on /vendor/dashboard, got %s", landingPathForSession(sessHiredVendor))
	}
}
