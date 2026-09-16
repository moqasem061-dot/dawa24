package authctx_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func TestRequirePagePermission(t *testing.T) {
	gate := authctx.RequirePagePermission("catalog.product.view")
	developerGate := authctx.RequirePagePermission("platform.developer.sql")

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	tests := []struct {
		name       string
		gate       func(http.Handler) http.Handler
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous request redirects to login",
			gate:       gate,
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Non-staff user (customer) gets 404",
			gate: gate,
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: false,
				Role:    "customer",
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Staff user lacking permission gets 404 (T5a/T5b)",
			gate: developerGate,
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Role:        "support",
				Permissions: []string{"catalog.product.view"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Staff user with required permission gets 200 (T5c)",
			gate: gate,
			actor: &authctx.Actor{
				UserID:      3,
				IsStaff:     true,
				Role:        "support",
				Permissions: []string{"catalog.product.view"},
			},
			wantStatus: http.StatusOK,
		},
		{
			// A role name is no longer a bypass. super_admin passes because it
			// holds every permission, which the resolver gives it — not
			// because this middleware knows the string "super_admin". That
			// difference is what lets an operator create a new staff role and
			// have it work without a code change.
			name: "Super admin holds the permission and passes",
			gate: developerGate,
			actor: &authctx.Actor{
				UserID:      4,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "A staff role with no grants is refused, whatever it is called",
			gate: developerGate,
			actor: &authctx.Actor{
				UserID:  5,
				IsStaff: true,
				Role:    "developer",
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Wildcard permission bypasses check",
			gate: developerGate,
			actor: &authctx.Actor{
				UserID:      6,
				IsStaff:     true,
				Role:        "admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/admin/test", nil)
			if tt.actor != nil {
				req = req.WithContext(authctx.WithActor(req.Context(), *tt.actor))
			}
			rec := httptest.NewRecorder()
			handler := tt.gate(okHandler)
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestOrgStatusMatrix(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tierBGate := authctx.RequireApproved(log)
	tierAGate := func(next http.Handler) http.Handler {
		// Tier A has no RequireApproved gate; authenticated callers proceed.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authctx.From(r.Context()); !ok {
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	statuses := []struct {
		status         string
		tierBWantsCode int
		tierAWantsCode int
	}{
		{"pending", http.StatusFound, http.StatusOK},
		{"under_review", http.StatusFound, http.StatusOK},
		{"rejected", http.StatusFound, http.StatusOK},
		{"suspended", http.StatusFound, http.StatusOK},
		{"approved", http.StatusOK, http.StatusOK},
	}

	for _, s := range statuses {
		t.Run("Status_"+s.status, func(t *testing.T) {
			actor := authctx.Actor{
				UserID:         10,
				OrganizationID: 100,
				OrgType:        "customer",
				OrgStatus:      s.status,
			}

			// Tier B check
			reqB := httptest.NewRequest("GET", "/wallet", nil)
			reqB = reqB.WithContext(authctx.WithActor(reqB.Context(), actor))
			recB := httptest.NewRecorder()
			tierBGate(okHandler).ServeHTTP(recB, reqB)
			if recB.Code != s.tierBWantsCode {
				t.Errorf("Tier B status %q: got %d, want %d", s.status, recB.Code, s.tierBWantsCode)
			}

			// Tier A check
			reqA := httptest.NewRequest("GET", "/documents", nil)
			reqA = reqA.WithContext(authctx.WithActor(reqA.Context(), actor))
			recA := httptest.NewRecorder()
			tierAGate(okHandler).ServeHTTP(recA, reqA)
			if recA.Code != s.tierAWantsCode {
				t.Errorf("Tier A status %q: got %d, want %d", s.status, recA.Code, s.tierAWantsCode)
			}
		})
	}
}

func TestCrossAudienceGates(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	customerGate := authctx.RequireCustomer(log)
	vendorGate := authctx.RequireVendor(log)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	customerActor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgType:        "customer",
		OrgStatus:      "approved",
	}

	vendorActor := authctx.Actor{
		UserID:         2,
		OrganizationID: 20,
		OrgType:        "vendor",
		OrgStatus:      "approved",
	}

	// 1. Customer accessing /vendor/* path gets 303 redirect
	reqCV := httptest.NewRequest("GET", "/vendor/documents", nil)
	reqCV = reqCV.WithContext(authctx.WithActor(reqCV.Context(), customerActor))
	recCV := httptest.NewRecorder()
	vendorGate(okHandler).ServeHTTP(recCV, reqCV)
	if recCV.Code != http.StatusSeeOther {
		t.Errorf("Customer on vendor gate: got %d, want 303", recCV.Code)
	}

	// 2. Vendor accessing /customer/* path gets 303 redirect
	reqVC := httptest.NewRequest("GET", "/customer/documents", nil)
	reqVC = reqVC.WithContext(authctx.WithActor(reqVC.Context(), vendorActor))
	recVC := httptest.NewRecorder()
	customerGate(okHandler).ServeHTTP(recVC, reqVC)
	if recVC.Code != http.StatusSeeOther {
		t.Errorf("Vendor on customer gate: got %d, want 303", recVC.Code)
	}

	// 3. Customer accessing customer gate gets 200
	reqCC := httptest.NewRequest("GET", "/customer/documents", nil)
	reqCC = reqCC.WithContext(authctx.WithActor(reqCC.Context(), customerActor))
	recCC := httptest.NewRecorder()
	customerGate(okHandler).ServeHTTP(recCC, reqCC)
	if recCC.Code != http.StatusOK {
		t.Errorf("Customer on customer gate: got %d, want 200", recCC.Code)
	}

	// 4. Vendor accessing vendor gate gets 200
	reqVV := httptest.NewRequest("GET", "/vendor/documents", nil)
	reqVV = reqVV.WithContext(authctx.WithActor(reqVV.Context(), vendorActor))
	recVV := httptest.NewRecorder()
	vendorGate(okHandler).ServeHTTP(recVV, reqVV)
	if recVV.Code != http.StatusOK {
		t.Errorf("Vendor on vendor gate: got %d, want 200", recVV.Code)
	}
}
