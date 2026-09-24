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

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

type mockAdminBranchRepo struct {
	org.Repository
	orgs          map[int64]*org.Organization
	branches      map[int64]*org.Branch
	createdBranch *org.Branch
	updatedBranch *org.Branch
}

func (m *mockAdminBranchRepo) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	if o, ok := m.orgs[id]; ok {
		return o, nil
	}
	return nil, nil
}

func (m *mockAdminBranchRepo) CreateBranch(ctx context.Context, b *org.Branch) error {
	b.ID = 1001
	m.createdBranch = b
	return nil
}

func (m *mockAdminBranchRepo) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	if b, ok := m.branches[id]; ok {
		return b, nil
	}
	return nil, nil
}

func (m *mockAdminBranchRepo) UpdateBranch(ctx context.Context, b *org.Branch) error {
	m.updatedBranch = b
	return nil
}

func (m *mockAdminBranchRepo) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	return nil, nil
}

func (m *mockAdminBranchRepo) UnsetMainBranches(ctx context.Context, orgID int64) error {
	return nil
}

func TestAdminBranchSubmit_CustomerVsVendorParity(t *testing.T) {
	repo := &mockAdminBranchRepo{
		orgs: map[int64]*org.Organization{
			10: {ID: 10, LegalName: "صيدليات النور", Type: org.TypeCustomer},
			20: {ID: 20, LegalName: "شركة الفا للتوريدات", Type: org.TypeVendor},
		},
		branches: map[int64]*org.Branch{
			101: {
				ID:             101,
				OrganizationID: 10,
				Name:           i18n.New("فرع صيدلية قديم", "Old Pharmacy"),
				WarehouseType:  "pharmacy",
				CityID:         nil,
			},
			201: {
				ID:             201,
				OrganizationID: 20,
				Name:           i18n.New("مستودع قديم", "Old Warehouse"),
				WarehouseType:  "warehouse",
				CapacitySQM:    500,
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, logger)
	handler := ui.NewUIHandler(nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	// 1. Submit new branch for Customer (pharmacy) without specifying warehouse_type
	{
		form := url.Values{
			"org_id":              {"10"},
			"name_ar":             {"صيدلية الزهور"},
			"name_en":             {"Al Zohoor Pharmacy"},
			"city_id":             {"1"},
			"address":             {"شارع النيل"},
			"institutional_works": {"1", "3"},
		}
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/branches/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		handler.AdminBranchNewSubmit(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect 303, got: %d", rr.Code)
		}
		if repo.createdBranch == nil {
			t.Fatalf("expected branch to be created")
		}
		if repo.createdBranch.WarehouseType != "pharmacy" {
			t.Errorf("expected customer branch warehouse_type to be 'pharmacy', got: %s", repo.createdBranch.WarehouseType)
		}
		if repo.createdBranch.CapacitySQM != 0 {
			t.Errorf("expected customer branch capacity to be 0, got: %f", repo.createdBranch.CapacitySQM)
		}
		if len(repo.createdBranch.InstitutionalWorks) != 2 {
			t.Errorf("expected 2 institutional works, got: %v", repo.createdBranch.InstitutionalWorks)
		}
	}

	// 2. Submit new branch for Vendor with logistics warehouse type & capacity_sqm
	{
		form := url.Values{
			"org_id":              {"20"},
			"name_ar":             {"مستودع التبريد السريع"},
			"warehouse_type":      {"cold_depot"},
			"capacity_sqm":        {"850.5"},
			"city_id":             {"2"},
			"address":             {"المنطقة الصناعية"},
			"has_cold_storage":    {"true"},
			"institutional_works": {"2"},
		}
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/branches/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		handler.AdminBranchNewSubmit(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect 303, got: %d", rr.Code)
		}
		if repo.createdBranch == nil {
			t.Fatalf("expected branch to be created")
		}
		if repo.createdBranch.WarehouseType != "cold_depot" {
			t.Errorf("expected vendor branch warehouse_type to be 'cold_depot', got: %s", repo.createdBranch.WarehouseType)
		}
		if repo.createdBranch.CapacitySQM != 850.5 {
			t.Errorf("expected vendor branch capacity to be 850.5, got: %f", repo.createdBranch.CapacitySQM)
		}
		if !repo.createdBranch.HasColdStorage {
			t.Errorf("expected has_cold_storage to be true")
		}
	}

	// 3. Edit existing branch for Customer (pharmacy): enforces warehouse_type = "pharmacy"
	{
		r := chi.NewRouter()
		r.Post("/admin/branches/{id}/edit", handler.AdminBranchEditSubmit)

		form := url.Values{
			"org_id":         {"10"},
			"name_ar":        {"صيدلية الزهور - معدلة"},
			"warehouse_type": {"warehouse"}, // Tampering attempt: should be forced to "pharmacy" because org is customer
			"city_id":        {"1"},
			"address":        {"شارع النيل الجديد"},
		}
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/branches/101/edit", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect 303, got: %d", rr.Code)
		}
		if repo.updatedBranch == nil {
			t.Fatalf("expected branch to be updated")
		}
		if repo.updatedBranch.WarehouseType != "pharmacy" {
			t.Errorf("expected edited customer branch warehouse_type to be 'pharmacy', got: %s", repo.updatedBranch.WarehouseType)
		}
	}

	// 4. Edit existing branch for Vendor: updates warehouse_type & capacity_sqm
	{
		r := chi.NewRouter()
		r.Post("/admin/branches/{id}/edit", handler.AdminBranchEditSubmit)

		form := url.Values{
			"org_id":         {"20"},
			"name_ar":        {"مستودع الأدوية المطور"},
			"warehouse_type": {"fast_hub"},
			"capacity_sqm":   {"1200"},
			"city_id":        {"2"},
			"address":        {"المنطقة الصناعية الجديدة"},
		}
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/branches/201/edit", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect 303, got: %d", rr.Code)
		}
		if repo.updatedBranch == nil {
			t.Fatalf("expected branch to be updated")
		}
		if repo.updatedBranch.WarehouseType != "fast_hub" {
			t.Errorf("expected edited vendor branch warehouse_type to be 'fast_hub', got: %s", repo.updatedBranch.WarehouseType)
		}
		if repo.updatedBranch.CapacitySQM != 1200 {
			t.Errorf("expected edited vendor branch capacity to be 1200, got: %f", repo.updatedBranch.CapacitySQM)
		}
	}
}
