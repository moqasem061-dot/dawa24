package http_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
)

func TestOrgHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"RegisterOrg", http.MethodPost, "/api/v1/org/organizations", `{"legal_name":"Al-Amal","commercial_register":"CR-101","type":"customer","credit_limit":"1000.00","payment_terms_days":30}`, http.StatusCreated},
		{"GetOrg", http.MethodGet, "/api/v1/org/organizations/1", "", http.StatusOK},
		{"UpdateOrg", http.MethodPut, "/api/v1/org/organizations/1", `{"legal_name":"Al-Amal Updated","commercial_register":"CR-101"}`, http.StatusOK},
		{"DeleteOrg", http.MethodDelete, "/api/v1/org/organizations/1", "", http.StatusOK},
		{"ListOrgs", http.MethodGet, "/api/v1/org/organizations?limit=10&offset=0", "", http.StatusOK},
		{"UpdateStatus", http.MethodPost, "/api/v1/org/organizations/1/status", `{"status":"approved"}`, http.StatusOK},
		{"CreateBranch", http.MethodPost, "/api/v1/org/organizations/1/branches", `{"code":"BR-01","name":{"en":"Main Branch"}}`, http.StatusCreated},
		{"ListBranches", http.MethodGet, "/api/v1/org/organizations/1/branches", "", http.StatusOK},
		{"UpdateBranch", http.MethodPut, "/api/v1/org/organizations/1/branches/1", `{"code":"BR-01","name":{"en":"Main Branch Updated"}}`, http.StatusOK},
		{"DeleteBranch", http.MethodDelete, "/api/v1/org/organizations/1/branches/1", "", http.StatusOK},
		{"AddMember", http.MethodPost, "/api/v1/org/organizations/1/members", `{"user_id":2,"role_id":1}`, http.StatusCreated},
		{"ListMembers", http.MethodGet, "/api/v1/org/organizations/1/members", "", http.StatusOK},
		{"UpdateMemberRole", http.MethodPut, "/api/v1/org/organizations/1/members/1", `{"role":"manager"}`, http.StatusOK},
		{"RemoveMember", http.MethodDelete, "/api/v1/org/organizations/1/members/1", "", http.StatusOK},
		{"AddReview", http.MethodPost, "/api/v1/org/organizations/1/reviews", `{"rating":5,"review_text":"Great service"}`, http.StatusCreated},
		{"ListReviews", http.MethodGet, "/api/v1/org/organizations/1/reviews?limit=10&offset=0", "", http.StatusOK},
		{"ToggleFollow", http.MethodPost, "/api/v1/org/organizations/1/follow", `{"user_id":1}`, http.StatusOK},
		{"AdminPending", http.MethodGet, "/api/v1/admin/org/pending", "", http.StatusOK},
		{"AdminApprove", http.MethodPost, "/api/v1/admin/org/1/approve", "", http.StatusOK},
		{"AdminReject", http.MethodPost, "/api/v1/admin/org/1/reject", "", http.StatusOK},
		{"AdminSuspend", http.MethodPost, "/api/v1/admin/org/1/suspend", "", http.StatusOK},
		{"AdminUpdate", http.MethodPut, "/api/v1/admin/org/1", `{"legal_name":"Admin Org Name","commercial_register":"CR-101"}`, http.StatusOK},
		{"AdminMembers", http.MethodGet, "/api/v1/admin/org/members", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func (r stubRepo) CountOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus) (int, error) {
	r.fail("CountOrganizations")
	return 0, nil
}

func (r stubRepo) UpdateOrganizationAICredentials(_ context.Context, _ int64, _, _ string) error {
	r.fail("UpdateOrganizationAICredentials")
	return nil
}

func (happyRepo) CountOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus) (int, error) {
	return 2, nil
}

func (happyRepo) UpdateOrganizationAICredentials(_ context.Context, _ int64, _, _ string) error {
	return nil
}
