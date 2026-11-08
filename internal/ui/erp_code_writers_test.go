package ui

import (
	"net/url"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// S5: the four writers of the customer number (customer and vendor, create and
// update) refuse a code another customer holds, and write a free one. A supplier
// without ERP is never refused (SC-009).

const (
	writerVendor int64 = 700
	writerPlain  int64 = 800
)

// writerStore holds one ERP supplier, one plain supplier, and a holder link with
// code C-1 for each.
func writerStore() *codeStore {
	s := newCodeStore()
	s.erpSuppliers[writerVendor] = true
	holderOrg := int64(200)
	s.addCodeLink(501, 40, &holderOrg, writerVendor, "C-1", org.UserOrgStatusApproved)
	s.addCodeLink(601, 40, &holderOrg, writerPlain, "C-1", org.UserOrgStatusApproved)
	return s
}

func customerActor(userID, orgID int64) authctx.Actor {
	return authctx.Actor{UserID: userID, OrganizationID: orgID, OrgID: orgID}
}

func vendorActor(orgID int64) authctx.Actor {
	return authctx.Actor{UserID: 50, OrganizationID: orgID, OrgID: orgID, OrgType: "vendor"}
}

func TestErpCodeWriterCustomerCreate(t *testing.T) {
	post := func(s *codeStore, vendor, code string) (string, int) {
		h := codeHandler(s, true)
		rec := codeRequest(url.Values{"vendor_org_id": {vendor}, "organization_number": {code}}, customerActor(42, 301), nil, h.CustomerUserOrganizationCreateSubmit)
		return rec.Header().Get("Location"), s.creates
	}
	t.Run("a taken code redirects with the notice and writes nothing", func(t *testing.T) {
		loc, creates := post(writerStore(), "700", "C-1")
		if !takenNotice(loc) || creates != 0 {
			t.Fatalf("Location=%q creates=%d, want the taken notice and no write", loc, creates)
		}
	})
	t.Run("a free code is written", func(t *testing.T) {
		_, creates := post(writerStore(), "700", "C-9")
		if creates != 1 {
			t.Fatalf("creates = %d, want 1", creates)
		}
	})
	t.Run("a supplier without ERP is unchanged", func(t *testing.T) {
		loc, creates := post(writerStore(), "800", "C-1")
		if takenNotice(loc) || creates != 1 {
			t.Fatalf("Location=%q creates=%d, want the write to go ahead", loc, creates)
		}
	})
}

func TestErpCodeWriterCustomerUpdate(t *testing.T) {
	// Link 502 belongs to user 42 (org 301) and holds C-2. Editing it to C-1
	// must be refused while 501 holds C-1 for another customer.
	store := func() *codeStore {
		s := writerStore()
		s.addCodeLink(502, 42, pointerOrNil(301), writerVendor, "C-2", org.UserOrgStatusApproved)
		return s
	}
	update := func(s *codeStore, code string) string {
		h := codeHandler(s, true)
		rec := codeRequest(url.Values{"organization_number": {code}}, customerActor(42, 301), map[string]string{"id": "502"}, h.CustomerUserOrganizationUpdateSubmit)
		return rec.Header().Get("Location")
	}
	t.Run("a taken code redirects with the notice and writes nothing", func(t *testing.T) {
		s := store()
		if loc := update(s, "C-1"); !takenNotice(loc) || s.updates != 0 {
			t.Fatalf("Location=%q updates=%d, want the taken notice and no write", loc, s.updates)
		}
	})
	t.Run("a free code is written", func(t *testing.T) {
		s := store()
		if loc := update(s, "C-7"); takenNotice(loc) || s.updates != 1 || s.links[502].OrganizationNumber != "C-7" {
			t.Fatalf("Location=%q updates=%d, want the write of C-7", loc, s.updates)
		}
	})
}

func TestErpCodeWriterVendorCreate(t *testing.T) {
	post := func(s *codeStore, vendor int64, code string) (string, int) {
		h := codeHandler(s, true)
		rec := codeRequest(url.Values{"user_id": {"43"}, "organization_number": {code}}, vendorActor(vendor), nil, h.VendorUserOrganizationCreateSubmit)
		return rec.Header().Get("Location"), s.creates
	}
	t.Run("a taken code redirects with the notice and writes nothing", func(t *testing.T) {
		loc, creates := post(writerStore(), writerVendor, "C-1")
		if !takenNotice(loc) || creates != 0 {
			t.Fatalf("Location=%q creates=%d, want the taken notice and no write", loc, creates)
		}
	})
	t.Run("a supplier without ERP is unchanged", func(t *testing.T) {
		loc, creates := post(writerStore(), writerPlain, "C-1")
		if takenNotice(loc) || creates != 1 {
			t.Fatalf("Location=%q creates=%d, want the write to go ahead", loc, creates)
		}
	})
}

func TestErpCodeWriterVendorUpdate(t *testing.T) {
	store := func() *codeStore {
		s := writerStore()
		s.addCodeLink(502, 42, pointerOrNil(301), writerVendor, "C-2", org.UserOrgStatusApproved)
		return s
	}
	t.Run("a taken code redirects with the notice and writes nothing", func(t *testing.T) {
		s := store()
		h := codeHandler(s, true)
		rec := codeRequest(url.Values{"organization_number": {"C-1"}}, vendorActor(writerVendor), map[string]string{"id": "502"}, h.VendorUserOrganizationUpdateSubmit)
		if loc := rec.Header().Get("Location"); !takenNotice(loc) || s.updates != 0 {
			t.Fatalf("Location=%q updates=%d, want the taken notice and no write", loc, s.updates)
		}
	})
}
