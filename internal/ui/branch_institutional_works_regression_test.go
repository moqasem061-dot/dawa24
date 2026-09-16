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

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type branchWorksMockRepo struct {
	org.Repository
	branch        *org.Branch
	updatedBranch *org.Branch
}

func (m *branchWorksMockRepo) GetBranchByID(_ context.Context, id int64) (*org.Branch, error) {
	if m.branch != nil && m.branch.ID == id {
		return m.branch, nil
	}
	return nil, nil
}

func (m *branchWorksMockRepo) UpdateBranch(_ context.Context, b *org.Branch) error {
	m.updatedBranch = b
	return nil
}

func (m *branchWorksMockRepo) GetOrganizationByID(_ context.Context, id int64) (*org.Organization, error) {
	return &org.Organization{ID: id, Type: org.TypeCustomer}, nil
}

func (m *branchWorksMockRepo) UnsetMainBranches(_ context.Context, _ int64) error {
	return nil
}

// TestBranchEdit_RetainsInstitutionalWorks (R5 regression test)
// verifies that editing a branch without modifying its institutional works
// (the form resubmits the branch's existing institutional_works) does not clear them.
func TestBranchEdit_RetainsInstitutionalWorks(t *testing.T) {
	existingWorks := []string{"101", "102"}
	branch := &org.Branch{
		ID:                 55,
		OrganizationID:     12,
		Name:               i18n.New("فرع رئيسي", "Main Branch"),
		Code:               "BR-55",
		WarehouseType:      "pharmacy",
		Address:            "شارع التحرير",
		Phone:              "01000000000",
		Status:             "active",
		InstitutionalWorks: existingWorks,
	}

	repo := &branchWorksMockRepo{branch: branch}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, logger)
	handler := ui.NewUIHandler(nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Post("/admin/branches/{id}/edit", handler.AdminBranchEditSubmit)

	// Admin edits address and phone, resubmitting the existing institutional works rendered by the form
	form := url.Values{
		"org_id":              {"12"},
		"name_ar":             {"فرع رئيسي معدل"},
		"address":             {"شارع النصر الجديد"},
		"phone":               {"01011112222"},
		"institutional_works": existingWorks,
	}

	req := httptest.NewRequest("POST", "/admin/branches/55/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", rr.Code)
	}
	if repo.updatedBranch == nil {
		t.Fatalf("expected branch to be updated")
	}

	// Verify works were preserved and not cleared
	if len(repo.updatedBranch.InstitutionalWorks) != 2 {
		t.Fatalf("expected 2 institutional works retained, got %d", len(repo.updatedBranch.InstitutionalWorks))
	}
	if repo.updatedBranch.InstitutionalWorks[0] != "101" || repo.updatedBranch.InstitutionalWorks[1] != "102" {
		t.Errorf("expected institutional works %v, got %v", existingWorks, repo.updatedBranch.InstitutionalWorks)
	}
	if repo.updatedBranch.Address != "شارع النصر الجديد" {
		t.Errorf("expected updated address, got %s", repo.updatedBranch.Address)
	}

	// Also test CustomerBranchEditSubmit
	r2 := chi.NewRouter()
	r2.Post("/customer/branches/{id}/edit", handler.CustomerBranchEditSubmit)

	repo.updatedBranch = nil
	custForm := url.Values{
		"name_ar":             {"فرع العميل"},
		"address":             {"عنوان العميل 456"},
		"phone":               {"01033334444"},
		"institutional_works": existingWorks,
	}
	custReq := httptest.NewRequest("POST", "/customer/branches/55/edit", strings.NewReader(custForm.Encode()))
	custReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Authenticate as owner
	custActor := authctx.Actor{UserID: 1, OrgID: 12, Role: "customer"}
	custReq = custReq.WithContext(authctx.WithActor(custReq.Context(), custActor))

	custRR := httptest.NewRecorder()
	r2.ServeHTTP(custRR, custReq)

	if custRR.Code != http.StatusSeeOther {
		t.Fatalf("expected customer redirect 303, got %d", custRR.Code)
	}
	if repo.updatedBranch == nil {
		t.Fatalf("expected branch to be updated by customer")
	}
	if len(repo.updatedBranch.InstitutionalWorks) != 2 {
		t.Fatalf("expected 2 institutional works retained in customer edit, got %d", len(repo.updatedBranch.InstitutionalWorks))
	}
}
