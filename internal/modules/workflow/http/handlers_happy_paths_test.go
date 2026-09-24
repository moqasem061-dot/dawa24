package http_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"CreatePriorityRequest", http.MethodPost, "/api/v1/workflow/priority-requests", `{"user_id":1,"priority_lowest_price":true,"budget_constraint":"1000.00"}`, http.StatusCreated},
		{"SetWeeklyCoverage", http.MethodPost, "/api/v1/workflow/branches/1/coverage", `{"branch_id":1,"day_of_week":1,"distance_meters":5000,"is_active":true}`, http.StatusOK},
		{"GetBranchCoverage", http.MethodGet, "/api/v1/workflow/branches/1/coverage", "", http.StatusOK},
		{"ReportIssue", http.MethodPost, "/api/v1/workflow/issues", `{"reported_by":1,"issue_type":"quality","description":"Broken seal","priority":"high"}`, http.StatusCreated},
		{"ListIssues", http.MethodGet, "/api/v1/workflow/issues?limit=10&offset=0", "", http.StatusOK},
		{"AdminIssues", http.MethodGet, "/api/v1/admin/workflow/issues", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequestWithContext(context.Background(), tt.method, tt.path, bodyReader)
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
