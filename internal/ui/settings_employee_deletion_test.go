package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockIdentityDeletionRepo struct {
	identity.Repository
}

func (m *mockIdentityDeletionRepo) GetUserByID(_ context.Context, id int64) (*identity.User, error) {
	return &identity.User{
		ID:     id,
		Email:  "emp@example.com",
		Name:   i18n.New("موظف تجريبي", "Test Employee"),
		Role:   "employee",
		Status: identity.StatusActive,
	}, nil
}

func (m *mockIdentityDeletionRepo) GetPermissionsForUser(_ context.Context, _, _ int64) ([]string, error) {
	return nil, nil
}

func (m *mockIdentityDeletionRepo) ListSessions(_ context.Context, _ int64) ([]*identity.Session, error) {
	return nil, nil
}

func (m *mockIdentityDeletionRepo) ListSessionPlans(_ context.Context) ([]*identity.SessionPlan, error) {
	return nil, nil
}

func (m *mockIdentityDeletionRepo) RequestAccountDeletion(_ context.Context, _ int64, _ *int64, _ string) error {
	return nil
}

func (m *mockIdentityDeletionRepo) GetPendingAccountDeletionRequest(_ context.Context, _ int64) (*identity.AccountDeletionRequest, error) {
	return nil, nil
}

func TestEmployeeCannotRequestAccountDeletion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	idSvc := identity.NewService(&mockIdentityDeletionRepo{}, nil, logger)
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, idSvc, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	employeeActor := authctx.Actor{
		UserID:         100,
		OrganizationID: 10,
		OrgID:          10,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "employee",
		IsOwner:        false,
		Scope:          rbac.ScopeVendor,
	}

	ownerActor := authctx.Actor{
		UserID:         200,
		OrganizationID: 10,
		OrgID:          10,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		IsOwner:        true,
		Scope:          rbac.ScopeVendor,
	}

	t.Run("Employee settings page hides deletion danger zone and modal", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), employeeActor))
		rr := httptest.NewRecorder()

		handler.SettingsIndex(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if strings.Contains(body, "delete-account-modal") {
			t.Error("employee settings page must NOT contain delete-account-modal")
		}
		if strings.Contains(body, "data-modal-open=\"delete-account-modal\"") {
			t.Error("employee settings page must NOT contain trigger button for delete modal")
		}
	})

	t.Run("Owner settings page displays deletion danger zone and modal", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), ownerActor))
		rr := httptest.NewRecorder()

		handler.SettingsIndex(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "delete-account-modal") {
			t.Error("owner settings page should contain delete-account-modal")
		}
	})

	t.Run("Employee POST to delete-request is rejected", func(t *testing.T) {
		form := url.Values{"reason": {"أرغب بحذف الحساب"}}
		req := httptest.NewRequest(http.MethodPost, "/settings/delete-request", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), employeeActor))
		rr := httptest.NewRecorder()

		handler.SettingsDeleteRequestSubmit(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "notice=error") {
			t.Errorf("expected error notice on deletion request by employee, got: %s", loc)
		}
	})
}
