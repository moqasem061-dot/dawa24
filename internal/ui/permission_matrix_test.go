package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// TestRoleRoutePermissionMatrix asserts the complete Role x Route access matrix.
// Every cell in this table specifies the exact expected HTTP status code and redirect target.
func TestRoleRoutePermissionMatrix(t *testing.T) {
	type testActor struct {
		name  string
		actor *authctx.Actor
	}

	actors := []testActor{
		{
			name:  "anonymous",
			actor: nil,
		},
		{
			name: "super_admin",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
		},
		{
			name: "platform_support",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Role:        "support",
				Permissions: []string{"admin.dashboard", "admin.support"},
			},
		},
		{
			name: "vendor_approved",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID:         10,
					OrganizationID: 100,
					OrgType:        "vendor",
					OrgStatus:      "approved",
					Scope:          rbac.ScopeVendor,
				}
				a.Grants([]string{"vendor.*"})
				return a
			}(),
		},
		{
			name: "vendor_pending",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID:         11,
					OrganizationID: 101,
					OrgType:        "vendor",
					OrgStatus:      "pending",
					Scope:          rbac.ScopeVendor,
				}
				a.Grants([]string{"vendor.*"})
				return a
			}(),
		},
		{
			name: "customer_approved",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID:         20,
					OrganizationID: 200,
					OrgType:        "customer",
					OrgStatus:      "approved",
					Scope:          rbac.ScopePharmacy,
				}
				a.Grants([]string{"pharmacy.*"})
				return a
			}(),
		},
		{
			name: "customer_pending",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID:         21,
					OrganizationID: 201,
					OrgType:        "customer",
					OrgStatus:      "pending",
					Scope:          rbac.ScopePharmacy,
				}
				a.Grants([]string{"pharmacy.*"})
				return a
			}(),
		},
	}

	type routeExpectation struct {
		path             string
		expectedStatuses map[string]int
	}

	matrix := []routeExpectation{
		{
			path: "/auth/login",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusOK,
				"super_admin":       http.StatusSeeOther,
				"platform_support":  http.StatusSeeOther,
				"vendor_approved":   http.StatusSeeOther,
				"vendor_pending":    http.StatusSeeOther,
				"customer_approved": http.StatusSeeOther,
				"customer_pending":  http.StatusSeeOther,
			},
		},
		{
			path: "/onboarding/pending",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusOK,
				"super_admin":       http.StatusSeeOther, // redirects approved to dashboard
				"platform_support":  http.StatusSeeOther,
				"vendor_approved":   http.StatusSeeOther,
				"vendor_pending":    http.StatusOK, // pending org reaches holding page
				"customer_approved": http.StatusSeeOther,
				"customer_pending":  http.StatusOK, // pending org reaches holding page
			},
		},
		{
			path: "/documents",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusOK, // tier A available for pre-approval verification
				"super_admin":       http.StatusOK,
				"platform_support":  http.StatusOK,
				"vendor_approved":   http.StatusOK,
				"vendor_pending":    http.StatusOK,
				"customer_approved": http.StatusOK,
				"customer_pending":  http.StatusOK,
			},
		},
		{
			path: "/admin/dashboard",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusSeeOther, // redirects non-staff to login
				"super_admin":       http.StatusOK,
				"platform_support":  http.StatusOK,
				"vendor_approved":   http.StatusSeeOther, // redirects to /vendor/dashboard with flash notice
				"vendor_pending":    http.StatusSeeOther,
				"customer_approved": http.StatusSeeOther, // redirects to /customer/dashboard with flash notice
				"customer_pending":  http.StatusSeeOther,
			},
		},
		{
			path: "/admin/users",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusSeeOther,
				"super_admin":       http.StatusOK,
				"platform_support":  http.StatusSeeOther, // missing identity.user.view permission -> redirects to admin dashboard
				"vendor_approved":   http.StatusSeeOther,
				"vendor_pending":    http.StatusSeeOther,
				"customer_approved": http.StatusSeeOther,
				"customer_pending":  http.StatusSeeOther,
			},
		},
		{
			path: "/vendor/dashboard",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusSeeOther,
				"super_admin":       http.StatusSeeOther, // non-vendor redirects to admin dashboard
				"platform_support":  http.StatusSeeOther,
				"vendor_approved":   http.StatusOK,
				"vendor_pending":    http.StatusFound,    // redirects to /onboarding/pending
				"customer_approved": http.StatusSeeOther, // redirects to /customer/dashboard
				"customer_pending":  http.StatusSeeOther,
			},
		},
		{
			path: "/customer/dashboard",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusSeeOther,
				"super_admin":       http.StatusSeeOther, // non-customer redirects to admin dashboard
				"platform_support":  http.StatusSeeOther,
				"vendor_approved":   http.StatusSeeOther, // redirects to /vendor/dashboard
				"vendor_pending":    http.StatusSeeOther,
				"customer_approved": http.StatusOK,
				"customer_pending":  http.StatusFound, // redirects to /onboarding/pending
			},
		},
		{
			path: "/wallet",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusSeeOther,
				"super_admin":       http.StatusSeeOther, // Tier B 303 redirect to /customer/wallet (no permanent cache)
				"platform_support":  http.StatusSeeOther,
				"vendor_approved":   http.StatusSeeOther, // Tier B 303 redirect to /vendor/wallet
				"vendor_pending":    http.StatusFound,    // unapproved org redirects to /onboarding/pending
				"customer_approved": http.StatusSeeOther,
				"customer_pending":  http.StatusFound, // unapproved org redirects to /onboarding/pending
			},
		},
		{
			path: "/notifications",
			expectedStatuses: map[string]int{
				"anonymous":         http.StatusSeeOther,
				"super_admin":       http.StatusOK,
				"platform_support":  http.StatusOK,
				"vendor_approved":   http.StatusOK,
				"vendor_pending":    http.StatusOK,
				"customer_approved": http.StatusOK,
				"customer_pending":  http.StatusOK,
			},
		},
	}

	for _, entry := range matrix {
		for _, act := range actors {
			expectedCode, ok := entry.expectedStatuses[act.name]
			if !ok {
				t.Fatalf("missing expectation for route %s and actor %s", entry.path, act.name)
			}

			testName := act.name + " -> " + entry.path
			t.Run(testName, func(t *testing.T) {
				router := newTestRouter(act.actor)
				req := httptest.NewRequest(http.MethodGet, entry.path, nil)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != expectedCode {
					t.Errorf("%s: got status %d, want %d", testName, rec.Code, expectedCode)
				}
			})
		}
	}
}
