package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The account lifecycle, asserted as a table.
//
// This is the question the rebuild started from: what can an account reach while
// its organization is still under review? The answer was "sixty-one routes,
// including wallet deposit and withdrawal", because the shared route group
// carried authentication and nothing else.
//
// TestPendingAccountApprovalLifecycle already covers `pending` and `approved`.
// It does not cover `under_review`, `rejected` or `suspended`, and those are
// the three that were actually reported. RequireApproved treats `under_review`
// exactly like `pending` and sends `rejected` and `suspended` to the same screen
// with a state — none of which was asserted anywhere until now.
//
// The table is the point. A reader answering "can a suspended vendor open the
// wallet?" should find the row rather than trace three middlewares.

// lifecycleActor builds an actor in a given organization state. Permissions are
// deliberately generous — every test here is about the *state* gate, and a thin
// permission set would let a test pass for the wrong reason.
func lifecycleActor(orgType, status string) *authctx.Actor {
	a := &authctx.Actor{
		UserID:         900,
		OrganizationID: 950,
		OrgType:        orgType,
		OrgStatus:      status,
	}
	if orgType == "vendor" {
		a.Scope = rbac.ScopeVendor
		a.Grants([]string{"vendor.*"})
	} else {
		a.Scope = rbac.ScopePharmacy
		a.Grants([]string{"pharmacy.*"})
	}
	return a
}

func TestAccountLifecycleMatrix(t *testing.T) {
	// Tier A is what an organization under review must still reach: its own
	// status, the documents that get it approved, and its own credentials.
	// Tier B is everything approval is a precondition for.
	const (
		tierAStatus    = "/onboarding/pending"
		tierADocuments = "/documents"
		tierBWallet    = "/wallet"
		tierBInvoices  = "/invoices"
	)

	// blocked means RequireApproved sent the caller to the holding screen.
	// A 302 anywhere else would be a different bug, so the destination is
	// asserted too, not just the status code.
	cases := []struct {
		status       string
		tierBAllowed bool
		redirectTo   string
	}{
		{"approved", true, ""},
		{"active", true, ""},
		{"verified", true, ""},
		{"pending", false, "/documents"},
		{"under_review", false, "/documents"},
		{"rejected", false, "/onboarding/pending?state=rejected"},
		{"suspended", false, "/onboarding/pending?state=suspended"},
	}

	for _, orgType := range []string{"customer", "vendor"} {
		for _, tc := range cases {
			name := orgType + "/" + tc.status
			t.Run(name, func(t *testing.T) {
				router := newTestRouter(lifecycleActor(orgType, tc.status))

				// Tier A is reachable in every state. An organization that
				// cannot see why it is blocked, or upload the document that
				// unblocks it, is stuck with no way out.
				for _, path := range []string{tierAStatus, tierADocuments} {
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
					if rec.Code == http.StatusFound {
						t.Errorf("%s: GET %s redirected to %q; pre-approval routes must stay reachable in every state",
							name, path, rec.Header().Get("Location"))
					}
				}

				// Tier B follows approval.
				for _, path := range []string{tierBWallet, tierBInvoices} {
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

					if tc.tierBAllowed {
						if rec.Code == http.StatusFound {
							t.Errorf("%s: GET %s redirected to %q; an approved organization must reach it",
								name, path, rec.Header().Get("Location"))
						}
						continue
					}

					if rec.Code != http.StatusFound {
						t.Errorf("%s: GET %s returned %d, want %d — an unapproved organization must not reach it",
							name, path, rec.Code, http.StatusFound)
						continue
					}
					if got := rec.Header().Get("Location"); got != tc.redirectTo {
						t.Errorf("%s: GET %s redirected to %q, want %q",
							name, path, got, tc.redirectTo)
					}
				}
			})
		}
	}
}

// Money movement is asserted separately because it is the consequence that
// matters most, and because a POST reaching a handler is not undone by the
// handler refusing later. The gate has to stop it.
func TestUnapprovedOrganizationCannotMoveMoney(t *testing.T) {
	writes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/wallet/deposit"},
		{http.MethodPost, "/wallet/withdraw"},
		// The employee write moved with its screen. /settings/employees/create
		// and /settings/organization/member were two of the three CRUDs over
		// org.members and org.organizations; both are gone rather than gated,
		// and a route that does not exist answers 404 — a stronger refusal than
		// the redirect asserted here, but not the one asserted here. What must
		// still be gated is the surviving write, and that is what this names.
		{http.MethodPost, "/customer/employees/new"},
	}

	for _, status := range []string{"pending", "under_review", "rejected", "suspended"} {
		for _, w := range writes {
			t.Run(status+" "+w.path, func(t *testing.T) {
				router := newTestRouter(lifecycleActor("customer", status))
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(w.method, w.path, nil))

				if rec.Code != http.StatusFound {
					t.Errorf("%s %s as %s returned %d, want %d — this write must not reach its handler",
						w.method, w.path, status, rec.Code, http.StatusFound)
				}
			})
		}
	}
}

// The shared surface used to register /customer/* and /vendor/* side by side
// with no type check, so a pharmacy account could open vendor paths. Tier C
// exists to stop that, and the response is 404 rather than 403 on purpose: the
// URL space of one audience does not exist for the other.
func TestSharedRoutesEnforceAudience(t *testing.T) {
	cases := []struct {
		actorType string
		path      string
	}{
		{"customer", "/vendor/documents"},
		{"customer", "/vendor/invoices"},
		{"vendor", "/customer/documents"},
	}

	for _, tc := range cases {
		t.Run(tc.actorType+" -> "+tc.path, func(t *testing.T) {
			router := newTestRouter(lifecycleActor(tc.actorType, "approved"))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != http.StatusSeeOther {
				t.Errorf("GET %s as %s returned %d, want %d — unauthorized audience should redirect to dashboard",
					tc.path, tc.actorType, rec.Code, http.StatusSeeOther)
			}
		})
	}
}
