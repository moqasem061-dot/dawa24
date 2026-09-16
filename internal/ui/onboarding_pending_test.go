package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOnboardingPendingFourDistinctStates(t *testing.T) {
	states := []struct {
		state         string
		expectedClass string
		expectedLink  string
	}{
		{
			state:         "pending",
			expectedClass: "status-pending",
			expectedLink:  "/documents",
		},
		{
			state:         "under_review",
			expectedClass: "status-review",
			expectedLink:  "/documents",
		},
		{
			state:         "rejected",
			expectedClass: "status-rejected",
			expectedLink:  "/report-issue",
		},
		{
			state:         "suspended",
			expectedClass: "status-suspended",
			expectedLink:  "/report-issue",
		},
	}

	router := newTestRouter(nil)

	for _, s := range states {
		t.Run("state="+s.state, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/onboarding/pending?state="+s.state, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			body := rec.Body.String()
			if !strings.Contains(body, s.expectedClass) {
				t.Errorf("state %s: body missing expected class %q", s.state, s.expectedClass)
			}
			if !strings.Contains(body, s.expectedLink) {
				t.Errorf("state %s: body missing actionable link %q", s.state, s.expectedLink)
			}
		})
	}
}
