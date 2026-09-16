package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminTempWarehouse_100Files_BulkUpload_WithMappingWizardQueue(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	// Build a multipart form with 100 distinct warehouse files (.csv and .xlsx)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	totalFiles := 100
	itemsPerFile := 15
	expectedTotalItems := totalFiles * itemsPerFile

	startGen := time.Now()
	for i := 1; i <= totalFiles; i++ {
		filename := fmt.Sprintf("مستودع_المورد_المعتمد_%03d.csv", i)
		part, err := writer.CreateFormFile("files", filename)
		if err != nil {
			t.Fatalf("failed to create form file part %d: %v", i, err)
		}

		var csvContent strings.Builder
		csvContent.WriteString("كود الصنف (SKU),اسم المنتج,سعر الجمهور,نسبة الخصم %\n")
		for j := 1; j <= itemsPerFile; j++ {
			csvContent.WriteString(fmt.Sprintf("SKU-%03d-%02d,صنف تجريبي رقم %d-%d,120.00,18.0%%\n", i, j, i, j))
		}
		_, _ = part.Write([]byte(csvContent.String()))
	}
	_ = writer.Close()
	t.Logf("Generated 100 files multipart payload in %v (size: %.2f KB)", time.Since(startGen), float64(body.Len())/1024.0)

	// 1. Send bulk upload request with AJAX/JSON header
	req := httptest.NewRequest(http.MethodPost, "/admin/temporary-warehouses/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	adminActor := authctx.Actor{
		UserID:      1,
		Email:       "admin@dawa24.com",
		Role:        "super_admin",
		IsStaff:     true,
		IsOwner:     true,
		Permissions: []string{"inventory.warehouse.update", "inventory.temp_warehouse.view"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), adminActor))

	rec := httptest.NewRecorder()
	startProc := time.Now()
	r.ServeHTTP(rec, req)
	elapsed := time.Since(startProc)

	t.Logf("Processed 100 files bulk upload in %v", elapsed)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var jsonResp struct {
		Success         bool     `json:"success"`
		TotalFiles      int      `json:"total_files"`
		SuccessfulFiles int      `json:"successful_files"`
		FailedFiles     int      `json:"failed_files"`
		TotalItems      int64    `json:"total_items"`
		UploadedIDs     []string `json:"uploaded_ids"`
		SetupQueue      string   `json:"setup_queue"`
		Message         string   `json:"message"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &jsonResp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v, raw: %s", err, rec.Body.String())
	}

	if !jsonResp.Success {
		t.Fatalf("expected success=true, got false")
	}
	if jsonResp.TotalFiles != 100 || jsonResp.SuccessfulFiles != 100 {
		t.Errorf("expected 100 successful files, got %d", jsonResp.SuccessfulFiles)
	}
	if len(jsonResp.UploadedIDs) != 100 {
		t.Fatalf("expected 100 uploaded IDs in setup queue, got %d", len(jsonResp.UploadedIDs))
	}
	// Rows arrive AFTER the response: the parse is detached.
	if staged := waitForTempWarehouseStaging(t, mockRepo, jsonResp.UploadedIDs); staged != expectedTotalItems {
		t.Errorf("expected %d total items once staging finished, got %d", expectedTotalItems, staged)
	}

	// 2. Step-by-step setup: GET /admin/temporary-warehouses/{id}/mapping-json
	firstID := jsonResp.UploadedIDs[0]
	reqMappingJSON := httptest.NewRequest(http.MethodGet, "/admin/temporary-warehouses/"+firstID+"/mapping-json", nil)
	reqMappingJSON = reqMappingJSON.WithContext(authctx.WithActor(reqMappingJSON.Context(), adminActor))
	recMappingJSON := httptest.NewRecorder()
	r.ServeHTTP(recMappingJSON, reqMappingJSON)

	if recMappingJSON.Code != http.StatusOK {
		t.Fatalf("expected status 200 for mapping JSON, got %d", recMappingJSON.Code)
	}

	var mappingResp struct {
		Success      bool     `json:"success"`
		ID           int64    `json:"id"`
		SupplierName string   `json:"supplier_name"`
		RowCount     int      `json:"row_count"`
		Headers      []string `json:"headers"`
		CodeCol      int      `json:"code_col"`
		NameCol      int      `json:"name_col"`
		PriceCol     int      `json:"price_col"`
		DiscountCol  int      `json:"discount_col"`
	}
	if err := json.Unmarshal(recMappingJSON.Body.Bytes(), &mappingResp); err != nil {
		t.Fatalf("failed to unmarshal mapping JSON: %v", err)
	}
	if !mappingResp.Success || mappingResp.RowCount != itemsPerFile {
		t.Errorf("expected successful mapping info with %d items, got %+v", itemsPerFile, mappingResp)
	}

	// 3. Test Step Submission: POST /admin/temporary-warehouses/{id}/mapping
	formSubmit := url.Values{}
	formSubmit.Set("supplier_name", "مستودع المورد الأول المحدّث")
	formSubmit.Set("col_code", "0")
	formSubmit.Set("col_name", "1")
	formSubmit.Set("col_price", "2")
	formSubmit.Set("col_discount", "3")
	formSubmit.Set("setup_queue", strings.Join(jsonResp.UploadedIDs[1:], ","))
	formSubmit.Set("step", "1")
	formSubmit.Set("total", "100")

	reqSubmit := httptest.NewRequest(http.MethodPost, "/admin/temporary-warehouses/"+firstID+"/mapping", strings.NewReader(formSubmit.Encode()))
	reqSubmit.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSubmit.Header.Set("Accept", "application/json")
	reqSubmit.Header.Set("X-Requested-With", "XMLHttpRequest")
	reqSubmit = reqSubmit.WithContext(authctx.WithActor(reqSubmit.Context(), adminActor))
	recSubmit := httptest.NewRecorder()
	r.ServeHTTP(recSubmit, reqSubmit)

	if recSubmit.Code != http.StatusOK {
		t.Fatalf("expected status 200 for mapping submit, got %d", recSubmit.Code)
	}

	var submitResp struct {
		Success        bool   `json:"success"`
		RowCount       int    `json:"row_count"`
		NextFileID     int64  `json:"next_file_id"`
		RemainingQueue string `json:"remaining_queue"`
		Step           int    `json:"step"`
	}
	if err := json.Unmarshal(recSubmit.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("failed to unmarshal mapping submit response: %v", err)
	}
	if !submitResp.Success || submitResp.Step != 2 {
		t.Errorf("expected step=2 and success=true, got %+v", submitResp)
	}
}

type mockBillingRepoForCompare struct {
	billing.Repository
	plan *billing.Plan
}

func (m *mockBillingRepoForCompare) GetActiveSubscription(ctx context.Context, userID int64) (*billing.Subscription, error) {
	return nil, nil
}

func (m *mockBillingRepoForCompare) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*billing.Subscription, error) {
	return &billing.Subscription{
		ID:             1,
		OrganizationID: &orgID,
		PlanID:         m.plan.ID,
		Status:         billing.SubActive,
	}, nil
}

func (m *mockBillingRepoForCompare) GetPlanByID(ctx context.Context, id int64) (*billing.Plan, error) {
	return m.plan, nil
}

func (m *mockBillingRepoForCompare) GetDefaultPlan(ctx context.Context) (*billing.Plan, error) {
	return m.plan, nil
}

func TestCompareUpload_SubscriptionLimit_Enforcement(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	// Create plan with limit of 2 files
	limitedPlan := &billing.Plan{
		ID:        10,
		Slug:      "silver-pharmacy",
		Features:  map[string]string{"max_compare_files": "2"},
		IsActive:  true,
		IsDefault: true,
	}
	mockBillRepo := &mockBillingRepoForCompare{plan: limitedPlan}
	billSvc := billing.NewService(mockBillRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, billSvc, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)

	customerActor := authctx.Actor{
		UserID:         15,
		OrganizationID: 25,
		Role:           "customer",
		Permissions:    []string{"pharmacy.compare.view"},
	}

	// 1. Upload 3 files in one batch (exceeds limit 2) -> the batch is trimmed
	//    to what the plan allows and the surplus is named in a warning. It is
	//    NOT refused: refusing told the vendor to archive their old lists by
	//    hand, which is what the upload now does for them.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for i := 1; i <= 3; i++ {
		part, _ := writer.CreateFormFile("files", fmt.Sprintf("supplier_%d.csv", i))
		_, _ = part.Write([]byte("كود,اسم,سعر,خصم\nSKU1,دواء,100,10%\n"))
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/compare/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(authctx.WithActor(req.Context(), customerActor))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 after a trimmed batch, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice=success") {
		t.Errorf("expected the two files that fit to be staged, got %s", loc)
	}
	if !strings.Contains(loc, "warning=") || !strings.Contains(loc, "supplier_3.csv") {
		t.Errorf("expected the third file to be named as skipped, got %s", loc)
	}
	if staged, _ := mockRepo.CountActiveFiles(context.Background(), customerActor.UserID, &customerActor.OrganizationID); staged != 2 {
		t.Errorf("expected exactly 2 files staged from the trimmed batch, got %d", staged)
	}

	// 2. Upload 1 file (within limit 2) -> Should succeed
	bodyOk := &bytes.Buffer{}
	writerOk := multipart.NewWriter(bodyOk)
	partOk, _ := writerOk.CreateFormFile("files", "supplier_valid.csv")
	_, _ = partOk.Write([]byte("كود,اسم,سعر,خصم\nSKU1,دواء,100,10%\n"))
	_ = writerOk.Close()

	reqOk := httptest.NewRequest(http.MethodPost, "/compare/upload", bodyOk)
	reqOk.Header.Set("Content-Type", writerOk.FormDataContentType())
	reqOk = reqOk.WithContext(authctx.WithActor(reqOk.Context(), customerActor))

	recOk := httptest.NewRecorder()
	r.ServeHTTP(recOk, reqOk)

	if recOk.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 on success, got %d", recOk.Code)
	}
	locOk := recOk.Header().Get("Location")
	if strings.Contains(locOk, "notice_type=error") {
		t.Errorf("expected success redirect, got error notice: %s", locOk)
	}
}

func TestAdminTempWarehouse_BulkActions_Archive_Unarchive_Delete(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	adminActor := authctx.Actor{
		UserID:      1,
		Email:       "admin@dawa24.com",
		Role:        "super_admin",
		IsStaff:     true,
		IsOwner:     true,
		Permissions: []string{"*"},
	}

	// 1. Create test files in mockRepo
	f1 := &compare.CompareFile{
		UserID:       1,
		SupplierName: "مستودع 1",
		Status:       compare.FileReady,
	}
	_ = mockRepo.CreateFile(context.Background(), f1)

	f2 := &compare.CompareFile{
		UserID:       1,
		SupplierName: "مستودع 2",
		Status:       compare.FileReady,
	}
	_ = mockRepo.CreateFile(context.Background(), f2)

	f3 := &compare.CompareFile{
		UserID:       1,
		SupplierName: "مستودع 3",
		Status:       compare.FileReady,
	}
	_ = mockRepo.CreateFile(context.Background(), f3)

	// Test 1: Bulk Archive (Super Admin)
	formArchive := url.Values{
		"bulk_action":  {"archive"},
		"selected_ids": {fmt.Sprintf("%d", f1.ID), fmt.Sprintf("%d", f2.ID)},
	}
	reqArchive := httptest.NewRequest(http.MethodPost, "/admin/temporary-warehouses/bulk", strings.NewReader(formArchive.Encode()))
	reqArchive.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqArchive = reqArchive.WithContext(authctx.WithActor(reqArchive.Context(), adminActor))

	recArchive := httptest.NewRecorder()
	r.ServeHTTP(recArchive, reqArchive)

	if recArchive.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on bulk archive, got %d", recArchive.Code)
	}

	// Check status of f1 & f2
	f1Check, _ := mockRepo.GetFileByID(context.Background(), f1.ID)
	if f1Check.Status != compare.FileArchived {
		t.Errorf("expected f1 to be archived, got %s", f1Check.Status)
	}
	f2Check, _ := mockRepo.GetFileByID(context.Background(), f2.ID)
	if f2Check.Status != compare.FileArchived {
		t.Errorf("expected f2 to be archived, got %s", f2Check.Status)
	}

	// Test 2: Bulk Unarchive (Super Admin)
	formUnarchive := url.Values{
		"bulk_action":  {"unarchive"},
		"selected_ids": {fmt.Sprintf("%d", f1.ID), fmt.Sprintf("%d", f2.ID)},
	}
	reqUnarchive := httptest.NewRequest(http.MethodPost, "/admin/temporary-warehouses/bulk", strings.NewReader(formUnarchive.Encode()))
	reqUnarchive.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqUnarchive = reqUnarchive.WithContext(authctx.WithActor(reqUnarchive.Context(), adminActor))

	recUnarchive := httptest.NewRecorder()
	r.ServeHTTP(recUnarchive, reqUnarchive)

	if recUnarchive.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on bulk unarchive, got %d", recUnarchive.Code)
	}

	f1Unarchived, _ := mockRepo.GetFileByID(context.Background(), f1.ID)
	if f1Unarchived.Status != compare.FileReady {
		t.Errorf("expected f1 to be unarchived (ready), got %s", f1Unarchived.Status)
	}

	// Test 3: Bulk Delete (My Temp Warehouses)
	formDelete := url.Values{
		"bulk_action":  {"delete"},
		"selected_ids": {fmt.Sprintf("%d", f1.ID), fmt.Sprintf("%d", f3.ID)},
	}
	reqDelete := httptest.NewRequest(http.MethodPost, "/admin/my/temparte-warehouses/bulk", strings.NewReader(formDelete.Encode()))
	reqDelete.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqDelete = reqDelete.WithContext(authctx.WithActor(reqDelete.Context(), adminActor))
	recDelete := httptest.NewRecorder()
	r.ServeHTTP(recDelete, reqDelete)
	if recDelete.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on bulk delete, got %d", recDelete.Code)
	}

	deletedF1, _ := mockRepo.GetFileByID(context.Background(), f1.ID)
	if deletedF1 != nil {
		t.Errorf("expected f1 to be deleted from repo, got %+v", deletedF1)
	}
}

func TestAdminTempWarehouse_SortingAndColumns(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	adminActor := authctx.Actor{
		UserID:         1,
		IsStaff:        true,
		Email:          "admin@dawa24.com",
		Role:           "super_admin",
		OrganizationID: 0,
		Permissions:    []string{"*"},
	}

	// 1. Test Super Admin GET with sorting
	req1 := httptest.NewRequest(http.MethodGet, "/admin/user/temparte-warehouses?sort=supplier&order=asc", nil)
	req1 = req1.WithContext(authctx.WithActor(req1.Context(), adminActor))
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec1.Code)
	}
	body1 := rec1.Body.String()
	if !strings.Contains(body1, "sort=supplier") {
		t.Errorf("expected HTML to contain sort=supplier link")
	}

	// 2. Test My Warehouses GET with sorting
	req2 := httptest.NewRequest(http.MethodGet, "/admin/my/temparte-warehouses?sort=rows&order=desc", nil)
	req2 = req2.WithContext(authctx.WithActor(req2.Context(), adminActor))
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec2.Code)
	}

	// 3. Test Team Warehouses GET with sorting
	req3 := httptest.NewRequest(http.MethodGet, "/admin/team/temparte-warehouses?sort=date&order=asc", nil)
	req3 = req3.WithContext(authctx.WithActor(req3.Context(), adminActor))
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec3.Code)
	}
}
