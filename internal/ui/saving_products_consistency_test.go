package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

type mockCatalogMatchingRepo struct {
	*mockCatalogImageRepo
	matchProducts []catalog.MatchProduct
}

func (m *mockCatalogMatchingRepo) ListMatchProducts(_ context.Context) ([]catalog.MatchProduct, error) {
	return m.matchProducts, nil
}

func (m *mockCatalogMatchingRepo) CreateImportProducts(_ context.Context, _ int64, _ []*catalog.Product) ([]int64, error) {
	return nil, nil
}

func (m *mockCatalogMatchingRepo) RetireVariantsExcept(_ context.Context, _, _ int64, _ []int64) ([]catalog.RetiredVariant, error) {
	return nil, nil
}

func (m *mockCatalogMatchingRepo) DefaultCatalogOrg(_ context.Context) (int64, error) {
	return 1, nil
}

func (m *mockCatalogMatchingRepo) ListVariantKeys(_ context.Context, _ int64) ([]catalog.VariantKey, error) {
	return nil, nil
}

func (m *mockCatalogMatchingRepo) BulkWriteVariants(_ context.Context, _ int64, _ []catalog.VariantWriteRow) (catalog.VariantWriteResult, error) {
	return catalog.VariantWriteResult{}, nil
}

func createConsistencyWorkbook(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	f := excelize.NewFile()
	rows := [][]any{
		{"اسم الصنف", "كود SKU", "الكمية", "سعر الجمهور"},
		{"بانادول إكسترا 24 قرص", "PAN-EXT-24", 10, 35.50},
		{"كونجستال 20 قرص", "CONG-20", 5, 29.00},
		{"صنف غير معروف بتاتا بالدليل", "UNK-999", 3, 50.00},
		{"أوجمنتين 1 جم 14 قرص", "AUG-1G", 12, 110.00},
	}
	for i, row := range rows {
		for j, val := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			_ = f.SetCellValue("Sheet1", cell, val)
		}
	}
	var raw bytes.Buffer
	if err := f.Write(&raw); err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "savings.xlsx")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write(raw.Bytes()); err != nil {
		t.Fatalf("copy workbook: %v", err)
	}
	_ = w.Close()
	return &body, w.FormDataContentType()
}

// TestSavingProductsConsistency_EqualContextAcrossEntryPoints verifies User Story 3
// acceptance scenario 1: Given equal evidence, visibility, identifier declarations,
// and policy, when any savings path (customer, vendor, admin-assisted) runs,
// identity outcomes (product_id, match state/type, commercial totals) agree identically.
func TestSavingProductsConsistency_EqualContextAcrossEntryPoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &mockCatalogMatchingRepo{
		mockCatalogImageRepo: newMockCatalogImageRepo(),
		matchProducts: []catalog.MatchProduct{
			{
				ID:          101,
				NameAR:      "بانادول إكسترا 24 قرص",
				NameEN:      "Panadol Extra 24 Tablets",
				SKU:         "PAN-EXT-24",
				Barcode:     "6221234567891",
				PublicPrice: "35.50",
			},
			{
				ID:          102,
				NameAR:      "كونجستال 20 قرص",
				NameEN:      "Congestal 20 Tablets",
				SKU:         "CONG-20",
				Barcode:     "6221234567892",
				PublicPrice: "29.00",
			},
			{
				ID:          103,
				NameAR:      "أوجمنتين 1 جم 14 قرص",
				NameEN:      "Augmentin 1g 14 Tablets",
				SKU:         "AUG-1G",
				Barcode:     "6221234567893",
				PublicPrice: "110.00",
			},
		},
	}
	catSvc := catalog.NewService(repo, logger)
	h := NewUIHandler(catSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	h.RegisterCustomerRoutes(r)
	h.RegisterVendorRoutes(r)
	h.RegisterAdminRoutes(r)

	customerActor := authctx.Actor{
		UserID:         100,
		OrganizationID: 10,
		OrgID:          10,
		Role:           "customer",
		Permissions:    []string{"pharmacy.saving_product.manage", "pharmacy.*"},
	}
	vendorActor := authctx.Actor{
		UserID:         200,
		OrganizationID: 20,
		OrgID:          20,
		Role:           "vendor",
		Permissions:    []string{"vendor.saving_product.manage", "vendor.*"},
	}
	adminActor := authctx.Actor{
		UserID:         1,
		OrganizationID: 999,
		OrgID:          999,
		Role:           "super_admin",
		IsStaff:        true,
		Permissions:    []string{"catalog.org_import.run", "catalog.org_import.view"},
	}
	const targetAdminOrgID int64 = 30

	mapForm := url.Values{
		"col_name":  {"اسم الصنف"},
		"col_sku":   {"كود SKU"},
		"col_qty":   {"الكمية"},
		"col_price": {"سعر الجمهور"},
	}

	// 1. Run Customer savings import
	custBody, custCT := createConsistencyWorkbook(t)
	custReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), customerActor), http.MethodPost, "/customer/saving-products/import/upload", custBody)
	custReq.Header.Set("Content-Type", custCT)
	custRec := httptest.NewRecorder()
	r.ServeHTTP(custRec, custReq)
	if custRec.Code != http.StatusSeeOther {
		t.Fatalf("customer upload: expected 303, got %d: %s", custRec.Code, custRec.Body.String())
	}
	custLoc := custRec.Header().Get("Location")
	custUploadSessID := custLoc[strings.LastIndex(custLoc, "/")+1:]

	custMapReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), customerActor), http.MethodPost, "/customer/saving-products/import/"+custUploadSessID+"/map", strings.NewReader(mapForm.Encode()))
	custMapReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	custMapRec := httptest.NewRecorder()
	r.ServeHTTP(custMapRec, custMapReq)
	if custMapRec.Code != http.StatusSeeOther {
		t.Fatalf("customer map: expected 303, got %d: %s", custMapRec.Code, custMapRec.Body.String())
	}
	custMapLoc := custMapRec.Header().Get("Location")
	custRunID := custMapLoc[strings.LastIndex(custMapLoc, "/")+1:]

	var custSess *SavingImportSession
	for i := 0; i < 50; i++ {
		if s, ok := globalSavingImportSessionStore.GetSession(custRunID, customerActor.OrganizationID); ok && s.Status == SessionStateReady {
			custSess = s
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if custSess == nil {
		t.Fatalf("customer import did not finish processing")
	}

	// 2. Run Vendor savings import
	vendBody, vendCT := createConsistencyWorkbook(t)
	vendReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), vendorActor), http.MethodPost, "/vendor/saving-products/import/upload", vendBody)
	vendReq.Header.Set("Content-Type", vendCT)
	vendRec := httptest.NewRecorder()
	r.ServeHTTP(vendRec, vendReq)
	if vendRec.Code != http.StatusSeeOther {
		t.Fatalf("vendor upload: expected 303, got %d: %s", vendRec.Code, vendRec.Body.String())
	}
	vendLoc := vendRec.Header().Get("Location")
	vendUploadSessID := vendLoc[strings.LastIndex(vendLoc, "/")+1:]

	vendMapReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), vendorActor), http.MethodPost, "/vendor/saving-products/import/"+vendUploadSessID+"/map", strings.NewReader(mapForm.Encode()))
	vendMapReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	vendMapRec := httptest.NewRecorder()
	r.ServeHTTP(vendMapRec, vendMapReq)
	if vendMapRec.Code != http.StatusSeeOther {
		t.Fatalf("vendor map: expected 303, got %d: %s", vendMapRec.Code, vendMapRec.Body.String())
	}
	vendMapLoc := vendMapRec.Header().Get("Location")
	vendRunID := vendMapLoc[strings.LastIndex(vendMapLoc, "/")+1:]

	var vendSess *SavingImportSession
	for i := 0; i < 50; i++ {
		if s, ok := globalSavingImportSessionStore.GetSession(vendRunID, vendorActor.OrganizationID); ok && s.Status == SessionStateReady {
			vendSess = s
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if vendSess == nil {
		t.Fatalf("vendor import did not finish processing")
	}

	// 3. Run Admin-assisted savings import
	adminBody, adminCT := createConsistencyWorkbook(t)
	adminUploadPath := fmt.Sprintf("/admin/organizations/import/%d/saving/upload", targetAdminOrgID)
	adminReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), adminActor), http.MethodPost, adminUploadPath, adminBody)
	adminReq.Header.Set("Content-Type", adminCT)
	adminRec := httptest.NewRecorder()
	r.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusSeeOther {
		t.Fatalf("admin upload: expected 303, got %d: %s", adminRec.Code, adminRec.Body.String())
	}
	adminLoc := adminRec.Header().Get("Location")
	t.Logf("adminLoc: %q", adminLoc)
	if !strings.Contains(adminLoc, "/runs/") || !strings.Contains(adminLoc, "/mapping") {
		t.Fatalf("unexpected admin redirect location: %q", adminLoc)
	}
	adminUploadSessID := adminLoc[strings.LastIndex(adminLoc, "/runs/")+6 : strings.LastIndex(adminLoc, "/mapping")]

	adminMapPath := fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", targetAdminOrgID, adminUploadSessID)
	adminMapReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), adminActor), http.MethodPost, adminMapPath, strings.NewReader(mapForm.Encode()))
	adminMapReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	adminMapRec := httptest.NewRecorder()
	r.ServeHTTP(adminMapRec, adminMapReq)
	if adminMapRec.Code != http.StatusSeeOther {
		t.Fatalf("admin map: expected 303, got %d: %s", adminMapRec.Code, adminMapRec.Body.String())
	}

	adminSess, ok := globalSavingImportSessionStore.GetSession(adminUploadSessID, targetAdminOrgID)
	if !ok || adminSess == nil {
		t.Fatalf("admin import session not found")
	}

	// 4. Assert identical results across customer, vendor, and admin paths
	custItems := custSess.Items
	vendItems := vendSess.Items
	adminItems := adminSess.Items

	if len(custItems) != 4 || len(vendItems) != 4 || len(adminItems) != 4 {
		t.Fatalf("expected 4 items each, got cust=%d vend=%d admin=%d", len(custItems), len(vendItems), len(adminItems))
	}

	expectedMatches := []struct {
		hasProduct bool
		productID  int64
		matchType  string
	}{
		{hasProduct: true, productID: 101, matchType: "exact_name"},
		{hasProduct: true, productID: 102, matchType: "exact_name"},
		{hasProduct: false, productID: 0, matchType: "unlinked"},
		{hasProduct: true, productID: 103, matchType: "exact_name"},
	}

	for i := 0; i < 4; i++ {
		ci, vi, ai := custItems[i], vendItems[i], adminItems[i]
		exp := expectedMatches[i]

		// Cross-entrypoint consistency checks
		if (ci.ProductID == nil) != (vi.ProductID == nil) || (ci.ProductID == nil) != (ai.ProductID == nil) {
			t.Errorf("row %d: ProductID nilness mismatch: cust=%v vend=%v admin=%v", i+1, ci.ProductID, vi.ProductID, ai.ProductID)
		}
		if ci.ProductID != nil && vi.ProductID != nil && ai.ProductID != nil {
			if *ci.ProductID != *vi.ProductID || *ci.ProductID != *ai.ProductID {
				t.Errorf("row %d: ProductID value mismatch: cust=%d vend=%d admin=%d", i+1, *ci.ProductID, *vi.ProductID, *ai.ProductID)
			}
		}
		if ci.MatchType != vi.MatchType || ci.MatchType != ai.MatchType {
			t.Errorf("row %d: MatchType mismatch: cust=%q vend=%q admin=%q", i+1, ci.MatchType, vi.MatchType, ai.MatchType)
		}

		// Expected resolution checks
		if exp.hasProduct {
			if ci.ProductID == nil || *ci.ProductID != exp.productID {
				t.Errorf("row %d: expected ProductID %d, got cust=%v", i+1, exp.productID, ci.ProductID)
			}
		} else {
			if ci.ProductID != nil {
				t.Errorf("row %d: expected unlinked product, got %v", i+1, *ci.ProductID)
			}
		}
		if ci.MatchType != exp.matchType {
			t.Errorf("row %d: expected MatchType %q, got %q", i+1, exp.matchType, ci.MatchType)
		}

		// Input preservation checks
		if ci.NameProduct != vi.NameProduct || ci.NameProduct != ai.NameProduct {
			t.Errorf("row %d: NameProduct mismatch", i+1)
		}
		if ci.SKU != vi.SKU || ci.SKU != ai.SKU {
			t.Errorf("row %d: SKU mismatch", i+1)
		}
		if ci.Quantity != vi.Quantity || ci.Quantity != ai.Quantity {
			t.Errorf("row %d: Quantity mismatch", i+1)
		}
		if ci.Price.Minor() != vi.Price.Minor() || ci.Price.Minor() != ai.Price.Minor() {
			t.Errorf("row %d: Price mismatch", i+1)
		}
	}
}
