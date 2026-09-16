package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestAdminTranslations_ModalInitiallyHidden(t *testing.T) {
	data := pages.AdminTranslationsData{
		Translations: []*platformadmin.Translation{
			{
				ID:        1,
				Namespace: "admin",
				Key:       "test.key",
				TextAR:    "اختبار",
				TextEN:    "Test",
			},
		},
		Pagination: components.PaginationProps{
			CurrentPage: 1,
			PageSize:    20,
			TotalCount:  1,
			BaseURL:     "/admin/translations",
		},
	}

	var buf bytes.Buffer
	err := pages.AdminTranslations(data, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render AdminTranslations: %v", err)
	}

	html := buf.String()

	// Modal must have style="display: none;" so it does not open automatically on page load
	if !strings.Contains(html, `id="edit-translation-modal" class="modal-backdrop" style="display: none;"`) {
		t.Errorf("edit-translation-modal must have style=\"display: none;\" to prevent auto-opening on page load")
	}

	// Must contain close handler
	if !strings.Contains(html, "closeEditModal()") {
		t.Errorf("expected closeEditModal() to be present")
	}
}

func TestAdminCities_CoverageRadiusWired(t *testing.T) {
	govID := int64(10)
	gov := &platformadmin.Governorate{
		ID:                   govID,
		Name:                 i18n.New("أسوان", "Aswan"),
		Latitude:             24.0889,
		Longitude:            32.8998,
		CoverageRadiusMeters: 30000,
		IsActive:             true,
	}

	city := &platformadmin.City{
		ID:                   101,
		GovernorateID:        &govID,
		GovernorateName:      &gov.Name,
		Name:                 i18n.New("إدفو", "Edfu"),
		Latitude:             24.9785,
		Longitude:            32.8753,
		CoverageRadiusMeters: 5000,
		IsActive:             true,
	}

	data := pages.AdminCitiesData{
		Governorates:          []*platformadmin.Governorate{gov},
		Cities:                []*platformadmin.City{city},
		SelectedGovernorateID: 0,
		TotalCities:           1,
		TotalGovernorates:     1,
		TotalFiltered:         1,
		Page:                  1,
		Limit:                 20,
		TotalPages:            1,
	}

	var buf bytes.Buffer
	err := pages.AdminCities(data, "ar", "rtl", false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render AdminCities: %v", err)
	}

	html := buf.String()

	// 1. Table header must have coverage radius column
	if !strings.Contains(html, "نطاق التغطية") {
		t.Errorf("expected cities table to have 'نطاق التغطية' column")
	}

	// 2. City coverage radius rendered in table row (5.0 كم (5000 م))
	if !strings.Contains(html, "5.0 كم (5000 م)") {
		t.Errorf("expected city coverage radius to be rendered in table, got html: %s", html)
	}

	// 3. Governorate coverage radius rendered in card list (30 كم)
	if !strings.Contains(html, "30 كم") {
		t.Errorf("expected governorate coverage radius to be rendered in card list")
	}

	// 4. Add city form must have coverage_radius_meters input
	if !strings.Contains(html, `name="coverage_radius_meters"`) {
		t.Errorf("expected Add City form to contain coverage_radius_meters input")
	}

	// 5. Add governorate form must have gov_coverage_radius_meters input
	if !strings.Contains(html, `name="gov_coverage_radius_meters"`) {
		t.Errorf("expected Add Governorate form to contain gov_coverage_radius_meters input")
	}

	// 6. Edit city modal must have coverage_radius_meters input bound to editForm.radius
	if !strings.Contains(html, `x-model="editForm.radius"`) {
		t.Errorf("expected Edit City modal to bind radius to editForm.radius")
	}

	// 7. Edit governorate modal must have gov_coverage_radius_meters input bound to govEditForm.radius
	if !strings.Contains(html, `x-model="govEditForm.radius"`) {
		t.Errorf("expected Edit Governorate modal to bind radius to govEditForm.radius")
	}
}

func TestAdminBranches_CoordsValidJSONAndEditMode(t *testing.T) {
	cityID := int64(101)
	lat := 24.0889
	lon := 32.8998
	branch := &org.Branch{
		ID:             55,
		OrganizationID: 10,
		Name:           i18n.Text{"en": "Aswan Branch"}, // Empty Arabic name test
		Code:           "ASW-01",
		WarehouseType:  "branch",
		CityID:         &cityID,
		Address:        "شارع المطار، أسوان",
		Phone:          "01012345678",
		Latitude:       &lat,
		Longitude:      &lon,
		IsMain:         true,
		Status:         "active",
	}

	city := &platformadmin.City{
		ID:        101,
		Name:      i18n.New("أسوان", "Aswan"),
		Latitude:  24.0889,
		Longitude: 32.8998,
	}

	data := pages.AdminBranchesPageData{
		Branches:      []*org.Branch{branch},
		Organizations: []*org.Organization{{ID: 10, LegalName: "مؤسسة أسوان"}},
		OrgNames:      map[int64]string{10: "مؤسسة أسوان"},
		OrgTypes:      map[int64]string{10: "vendor"},
		Cities:        []*platformadmin.City{city},
		TotalBranches: 1,
		FilteredCount: 1,
	}

	var buf bytes.Buffer
	err := pages.AdminBranchesPage(data, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render AdminBranchesPage: %v", err)
	}

	html := buf.String()

	// 1. Coordinates must NOT be literal raw Templ syntax
	if strings.Contains(html, "{ templ.Raw(") {
		t.Errorf("Found unparsed '{ templ.Raw(' inside rendered HTML! Templ did not execute Go expression.")
	}

	// 2. data-coords on #admin-branch-cities-coords must be present and valid JSON
	if !strings.Contains(html, `id="admin-branch-cities-coords"`) {
		t.Fatalf("expected #admin-branch-cities-coords to exist")
	}

	startTag := `id="admin-branch-cities-coords" class="d-none" data-coords="`
	idx := strings.Index(html, startTag)
	if idx == -1 {
		t.Fatalf("expected admin-branch-cities-coords to have data-coords attribute")
	}
	sub := html[idx+len(startTag):]
	endIdx := strings.Index(sub, `"`)
	if endIdx == -1 {
		t.Fatalf("could not find closing quote of data-coords")
	}
	rawCoords := strings.ReplaceAll(sub[:endIdx], "&#34;", "\"")
	var coordsMap map[string][]any
	if err := json.Unmarshal([]byte(rawCoords), &coordsMap); err != nil {
		t.Fatalf("data-coords failed to parse as valid JSON: %v, raw: %s", err, rawCoords)
	}
	if pos, ok := coordsMap["101"]; !ok || len(pos) < 2 {
		t.Fatalf("expected city 101 in coords map, got: %v", coordsMap)
	}

	// 3. Edit button must have data-branch-data with valid JSON
	btnTag := fmt.Sprintf(`data-edit-branch-btn="%d"`, branch.ID)
	if !strings.Contains(html, btnTag) {
		t.Fatalf("expected branch edit button with %s", btnTag)
	}

	// 4. Form submit validation must check both form.city_id and input
	if !strings.Contains(html, "const cid = form.city_id") {
		t.Errorf("expected form @submit to check cid fallback")
	}
}

func TestAdminBranches_OrgTypeWarehouseToggleAndParity(t *testing.T) {
	cityID := int64(101)
	lat := 24.0889
	lon := 32.8998

	pharmacyBranch := &org.Branch{
		ID:                 55,
		OrganizationID:     10, // customer (pharmacy)
		Name:               i18n.New("صيدلية الأمل", "Al Amal Pharmacy"),
		Code:               "PH-01",
		WarehouseType:      "pharmacy",
		CityID:             &cityID,
		Address:            "شارع الجمهورية، أسوان",
		Phone:              "01012345678",
		Latitude:           &lat,
		Longitude:          &lon,
		IsMain:             true,
		Status:             "active",
		InstitutionalWorks: []string{"1"},
	}

	vendorBranch := &org.Branch{
		ID:                 56,
		OrganizationID:     20, // vendor
		Name:               i18n.New("مستودع الأدوية الرئيسي", "Main Medical Warehouse"),
		Code:               "WH-01",
		WarehouseType:      "warehouse",
		CapacitySQM:        1250,
		CityID:             &cityID,
		Address:            "المنطقة الصناعية، أسوان",
		Phone:              "01098765432",
		Latitude:           &lat,
		Longitude:          &lon,
		IsMain:             false,
		Status:             "active",
		InstitutionalWorks: []string{"2"},
	}

	city := &platformadmin.City{
		ID:        101,
		Name:      i18n.New("أسوان", "Aswan"),
		Latitude:  24.0889,
		Longitude: 32.8998,
	}

	instWorks := []*org.InstitutionalWork{
		{
			ID:    1,
			Title: i18n.New("التأمين الصحي", "Health Insurance"),
			Icon:  "🏥",
		},
		{
			ID:    2,
			Title: i18n.New("المناقصات الحكومية", "Gov Tenders"),
			Icon:  "📋",
		},
	}

	data := pages.AdminBranchesPageData{
		Branches: []*org.Branch{pharmacyBranch, vendorBranch},
		Organizations: []*org.Organization{
			{ID: 10, LegalName: "صيدليات النور", Type: org.TypeCustomer},
			{ID: 20, LegalName: "شركة الفا للتوريدات", Type: org.TypeVendor},
		},
		OrgNames: map[int64]string{
			10: "صيدليات النور",
			20: "شركة الفا للتوريدات",
		},
		OrgTypes: map[int64]string{
			10: "customer",
			20: "vendor",
		},
		Cities:             []*platformadmin.City{city},
		InstitutionalWorks: instWorks,
		TotalBranches:      2,
		FilteredCount:      2,
		PharmacyBranches:   1,
		VendorWarehouses:   1,
	}

	var buf bytes.Buffer
	err := pages.AdminBranchesPage(data, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render AdminBranchesPage: %v", err)
	}

	html := buf.String()

	// 1. Verify data-org-types is present and correctly mapped
	if !strings.Contains(html, `id="admin-branch-org-types"`) {
		t.Fatalf("expected #admin-branch-org-types to exist in HTML")
	}
	otTag := `id="admin-branch-org-types" class="d-none" data-org-types="`
	idx := strings.Index(html, otTag)
	if idx == -1 {
		t.Fatalf("expected admin-branch-org-types to contain data-org-types attribute")
	}
	sub := html[idx+len(otTag):]
	endIdx := strings.Index(sub, `"`)
	rawTypes := strings.ReplaceAll(sub[:endIdx], "&#34;", "\"")
	var typesMap map[string]string
	if err := json.Unmarshal([]byte(rawTypes), &typesMap); err != nil {
		t.Fatalf("failed to parse data-org-types JSON: %v, raw: %s", err, rawTypes)
	}
	if typesMap["10"] != "customer" || typesMap["20"] != "vendor" {
		t.Errorf("expected org 10=customer and 20=vendor, got: %v", typesMap)
	}

	// 2. Dynamic vendor controls must be present and conditioned on isVendorSelected()
	if !strings.Contains(html, `x-show="isVendorSelected()"`) {
		t.Errorf("expected vendor warehouse controls to use x-show=\"isVendorSelected()\"")
	}
	if !strings.Contains(html, `name="capacity_sqm"`) {
		t.Errorf("expected capacity_sqm input for vendors")
	}

	// 3. Customer pharmacy notice & automatic type must be conditioned on isCustomerSelected()
	if !strings.Contains(html, `x-show="isCustomerSelected()"`) {
		t.Errorf("expected customer pharmacy notice to use x-show=\"isCustomerSelected()\"")
	}
	if !strings.Contains(html, `name="warehouse_type" value="pharmacy"`) {
		t.Errorf("expected hidden warehouse_type input with value pharmacy for customers")
	}

	// 4. Institutional works section must be rendered
	if !strings.Contains(html, "الأعمال المؤسسية المغطاة (Institutional Works)") {
		t.Errorf("expected Institutional Works section in form")
	}
	if !strings.Contains(html, "التأمين الصحي") || !strings.Contains(html, "المناقصات الحكومية") {
		t.Errorf("expected institutional works items to be rendered")
	}

	// 5. Branch cards must show correct localized badges
	if !strings.Contains(html, "فرع صيدلية") {
		t.Errorf("expected customer branch to display 'فرع صيدلية' badge")
	}
	if !strings.Contains(html, "مخزن رئيسي") {
		t.Errorf("expected vendor warehouse to display 'مخزن رئيسي' badge")
	}
	if !strings.Contains(html, "1250 م²") {
		t.Errorf("expected vendor warehouse card to display capacity '1250 م²'")
	}
}

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
		req := httptest.NewRequest("POST", "/admin/branches/new", strings.NewReader(form.Encode()))
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
		req := httptest.NewRequest("POST", "/admin/branches/new", strings.NewReader(form.Encode()))
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
		req := httptest.NewRequest("POST", "/admin/branches/101/edit", strings.NewReader(form.Encode()))
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
		req := httptest.NewRequest("POST", "/admin/branches/201/edit", strings.NewReader(form.Encode()))
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
