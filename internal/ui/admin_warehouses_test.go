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

	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

type mockAdminWarehouseRepo struct {
	inventory.Repository
	warehouses map[int64]*inventory.Warehouse
	nextID     int64
}

func newMockAdminWarehouseRepo() *mockAdminWarehouseRepo {
	return &mockAdminWarehouseRepo{
		warehouses: make(map[int64]*inventory.Warehouse),
		nextID:     1,
	}
}

func (m *mockAdminWarehouseRepo) CreateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	if w.ID == 0 {
		w.ID = m.nextID
		m.nextID++
	}
	m.warehouses[w.ID] = w
	return nil
}

func (m *mockAdminWarehouseRepo) GetWarehouseByID(_ context.Context, id int64) (*inventory.Warehouse, error) {
	w, ok := m.warehouses[id]
	if !ok {
		return nil, nil
	}
	cpy := *w
	return &cpy, nil
}

func (m *mockAdminWarehouseRepo) UpdateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	m.warehouses[w.ID] = w
	return nil
}

// Implement inventory.AdminWarehouseBackend
func (m *mockAdminWarehouseRepo) ListAdminWarehouseRows(_ context.Context, _ inventory.AdminWarehouseFilter) ([]*inventory.AdminWarehouseRow, int, error) {
	return nil, 0, nil
}
func (m *mockAdminWarehouseRepo) AdminWarehouseOwners(_ context.Context) ([]inventory.AdminWarehouseOwner, error) {
	return nil, nil
}
func (m *mockAdminWarehouseRepo) SetWarehouseActive(_ context.Context, id int64, active bool) error {
	if w, ok := m.warehouses[id]; ok {
		w.IsActive = active
	}
	return nil
}

type mockAdminWarehouseOrgRepo struct {
	org.Repository
	branches map[int64]*org.Branch
}

func (m *mockAdminWarehouseOrgRepo) GetBranchByID(_ context.Context, id int64) (*org.Branch, error) {
	b, ok := m.branches[id]
	if !ok {
		return nil, nil
	}
	return b, nil
}

func (m *mockAdminWarehouseOrgRepo) ListOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus, _, _ int) ([]*org.Organization, error) {
	return []*org.Organization{
		{
			ID:        1,
			LegalName: "صيدلية النور",
			Type:      org.TypeCustomer,
		},
	}, nil
}

func (m *mockAdminWarehouseOrgRepo) ListBranchesByOrg(_ context.Context, orgID int64) ([]*org.Branch, error) {
	var res []*org.Branch
	for _, b := range m.branches {
		if orgID == 0 || b.OrganizationID == orgID {
			res = append(res, b)
		}
	}
	return res, nil
}

func setupAdminWarehouseTestHandler() (*ui.UIHandler, *mockAdminWarehouseRepo, *mockAdminWarehouseOrgRepo) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	invRepo := newMockAdminWarehouseRepo()
	invSvc := inventory.NewService(invRepo, logger)

	branchLat := 30.0444
	branchLng := 31.2357
	orgRepo := &mockAdminWarehouseOrgRepo{
		branches: map[int64]*org.Branch{
			10: {
				ID:             10,
				OrganizationID: 1,
				Name:           i18n.New("فرع التحرير", "Tahrir Branch"),
				Address:        "ميدان التحرير، القاهرة",
				Phone:          "01000000001",
				Latitude:       &branchLat,
				Longitude:      &branchLng,
			},
		},
	}
	orgSvc := org.NewService(orgRepo, logger)

	handler := ui.NewUIHandler(nil, orgSvc, nil, nil, invSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
	return handler, invRepo, orgRepo
}

func TestAdminWarehouseCreateSubmit_Validation(t *testing.T) {
	handler, _, _ := setupAdminWarehouseTestHandler()

	form := url.Values{
		"organization_id": {""},
		"name":            {""},
	}
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/warehouses/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AdminWarehouseCreateSubmit(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "notice=error") {
		t.Errorf("expected error notice in location, got: %s", loc)
	}
}

func TestAdminWarehouseCreateSubmit_WithExplicitDetails(t *testing.T) {
	handler, invRepo, _ := setupAdminWarehouseTestHandler()

	form := url.Values{
		"organization_id": {"1"},
		"branch_id":       {"10"},
		"name":            {"مستودع الأدوية المركزي"},
		"code":            {"WH-CUSTOM-01"},
		"phone":           {"01234567890"},
		"address":         {"مدينة نصر، القاهرة"},
		"latitude":        {"30.0500"},
		"longitude":       {"31.3000"},
		"is_active":       {"1"},
	}
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/warehouses/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AdminWarehouseCreateSubmit(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "notice=success") {
		t.Errorf("expected success notice in location, got: %s", loc)
	}

	wh := invRepo.warehouses[1]
	if wh == nil {
		t.Fatalf("expected warehouse to be saved in repo")
	}
	if wh.OrganizationID != 1 {
		t.Errorf("expected org 1, got %d", wh.OrganizationID)
	}
	if wh.BranchID == nil || *wh.BranchID != 10 {
		t.Errorf("expected branch 10, got %v", wh.BranchID)
	}
	if wh.Code != "WH-CUSTOM-01" {
		t.Errorf("expected code WH-CUSTOM-01, got %s", wh.Code)
	}
	if wh.Latitude == nil || *wh.Latitude != 30.05 {
		t.Errorf("expected lat 30.05, got %v", wh.Latitude)
	}
	if wh.Longitude == nil || *wh.Longitude != 31.3 {
		t.Errorf("expected lng 31.3, got %v", wh.Longitude)
	}
	if !wh.IsActive {
		t.Errorf("expected isActive true, got %v", wh.IsActive)
	}
}

func TestAdminWarehouseCreateSubmit_BranchFallbackAndAutoCode(t *testing.T) {
	handler, invRepo, _ := setupAdminWarehouseTestHandler()

	form := url.Values{
		"organization_id": {"1"},
		"branch_id":       {"10"},
		"name":            {"مستودع الفرع"},
		"code":            {""}, // empty: should autogenerate
		"phone":           {""}, // empty: should fallback to branch
		"address":         {""}, // empty: should fallback to branch
		"latitude":        {""}, // empty: should fallback to branch
		"longitude":       {""}, // empty: should fallback to branch
		"is_active":       {"1"},
	}
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/warehouses/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AdminWarehouseCreateSubmit(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rr.Code)
	}

	wh := invRepo.warehouses[1]
	if wh == nil {
		t.Fatalf("expected warehouse to be saved")
	}
	if !strings.HasPrefix(wh.Code, "WH-1-") {
		t.Errorf("expected autogenerated code starting with WH-1-, got %s", wh.Code)
	}
	if wh.Address != "ميدان التحرير، القاهرة" {
		t.Errorf("expected branch address fallback, got %s", wh.Address)
	}
	if wh.Phone != "01000000001" {
		t.Errorf("expected branch phone fallback, got %s", wh.Phone)
	}
	if wh.Latitude == nil || *wh.Latitude != 30.0444 {
		t.Errorf("expected branch lat fallback, got %v", wh.Latitude)
	}
	if wh.Longitude == nil || *wh.Longitude != 31.2357 {
		t.Errorf("expected branch lng fallback, got %v", wh.Longitude)
	}
}

func TestAdminWarehouseEditSubmit_SuccessAndOrgImmutability(t *testing.T) {
	handler, invRepo, _ := setupAdminWarehouseTestHandler()

	// Seed existing warehouse for org 1
	bID := int64(10)
	lat := 30.0
	lng := 31.0
	invRepo.warehouses[5] = &inventory.Warehouse{
		ID:             5,
		OrganizationID: 1,
		BranchID:       &bID,
		Name:           "الاسم القديم",
		Code:           "WH-OLD",
		Address:        "العنوان القديم",
		Phone:          "01000000000",
		Latitude:       &lat,
		Longitude:      &lng,
		IsActive:       false,
	}

	form := url.Values{
		"organization_id": {"999"}, // Attempt to hijack/move to org 999
		"branch_id":       {""},    // General warehouse (detach branch)
		"name":            {"الاسم الجديد المعدل"},
		"code":            {"WH-NEW-05"},
		"phone":           {"01111111111"},
		"address":         {"العنوان الجديد المحدث"},
		"latitude":        {"30.1234"},
		"longitude":       {"31.5678"},
		"is_active":       {"1"},
	}

	req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/warehouses/5/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.AdminWarehouseEditSubmit(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rr.Code)
	}

	wh := invRepo.warehouses[5]
	if wh.OrganizationID != 1 {
		t.Errorf("SECURITY FAULT: warehouse org was altered to %d! Must remain 1", wh.OrganizationID)
	}
	if wh.Name != "الاسم الجديد المعدل" {
		t.Errorf("expected updated name, got %s", wh.Name)
	}
	if wh.BranchID != nil {
		t.Errorf("expected branchID to be cleared (nil), got %v", wh.BranchID)
	}
	if wh.Latitude == nil || *wh.Latitude != 30.1234 {
		t.Errorf("expected lat 30.1234, got %v", wh.Latitude)
	}
	if !wh.IsActive {
		t.Errorf("expected isActive true, got %v", wh.IsActive)
	}
}

func TestAdminWarehouseToggleSubmit(t *testing.T) {
	handler, invRepo, _ := setupAdminWarehouseTestHandler()

	invRepo.warehouses[8] = &inventory.Warehouse{
		ID:             8,
		OrganizationID: 1,
		Name:           "مستودع للاختبار",
		IsActive:       false,
	}

	form := url.Values{"active": {"1"}}
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/admin/warehouses/8/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "8")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.AdminWarehouseToggleSubmit(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rr.Code)
	}
	if !invRepo.warehouses[8].IsActive {
		t.Errorf("expected warehouse 8 to be toggled active")
	}
}

func TestAdminWarehousePage_SearchableOrgAndBranchesRender(t *testing.T) {
	handler, _, _ := setupAdminWarehouseTestHandler()

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/admin/warehouses", nil)
	rr := httptest.NewRecorder()

	handler.AdminWarehousesPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "adminWarehouseCreateState") {
		t.Error("expected page to register adminWarehouseCreateState for Alpine CSP")
	}
	if !strings.Contains(body, `name="organization_id"`) {
		t.Error("expected hidden organization_id input")
	}
	if !strings.Contains(body, `x-ref="branchSelect"`) {
		t.Error("expected branchSelect ref for dynamic options")
	}
}
