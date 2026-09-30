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

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

type mockScopeSavingRepo struct {
	*mockCatalogMatchingRepo
	committedOrgID   int64
	committedUserID  *int64
	committedProduct []*catalog.SavingProduct
}

func (m *mockScopeSavingRepo) BatchUpsertSavingProducts(_ context.Context, orgID int64, userID *int64, prods []*catalog.SavingProduct) (int, int, error) {
	m.committedOrgID = orgID
	m.committedUserID = userID
	m.committedProduct = prods
	return len(prods), 0, nil
}

func createAdminScopeWorkbook(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	f := excelize.NewFile()
	rows := [][]any{
		{"اسم الصنف", "كود SKU", "الكمية", "سعر الجمهور"},
		{"بانادول إكسترا 24 قرص", "PAN-EXT-24", 10, 35.50},
		{"كونجستال 20 قرص", "CONG-20", 5, 29.00},
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
	part, err := w.CreateFormFile("file", "admin_scope.xlsx")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write(raw.Bytes()); err != nil {
		t.Fatalf("copy workbook: %v", err)
	}
	_ = w.Close()
	return &body, w.FormDataContentType()
}

// TestAdminAssistedScope verifies User Story 3 acceptance scenario 5:
// Given an administrator from Org X imports for a target organization Y,
// when any step (upload, mapping, manual match, commit) runs,
// the run's organization_id is Y, and all decisions/items are scoped to Y,
// never to the administrator's unrelated organization X.
func TestAdminAssistedScope(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &mockScopeSavingRepo{
		mockCatalogMatchingRepo: &mockCatalogMatchingRepo{
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
			},
		},
	}
	catSvc := catalog.NewService(mockRepo, logger)
	h := NewUIHandler(catSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	h.RegisterAdminRoutes(r)

	const adminOrgX int64 = 999
	const targetOrgY int64 = 55
	adminActor := authctx.Actor{
		UserID:         42,
		OrganizationID: adminOrgX,
		OrgID:          adminOrgX,
		Role:           "super_admin",
		IsStaff:        true,
		Permissions:    []string{"catalog.org_import.run", "catalog.org_import.view"},
	}

	// 1. Upload for target organization Y
	body, ct := createAdminScopeWorkbook(t)
	uploadPath := fmt.Sprintf("/admin/organizations/import/%d/saving/upload", targetOrgY)
	req := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), adminActor), http.MethodPost, uploadPath, body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/runs/") || !strings.Contains(loc, "/mapping") {
		t.Fatalf("unexpected redirect location: %s", loc)
	}
	sessionID := loc[strings.LastIndex(loc, "/runs/")+6 : strings.LastIndex(loc, "/mapping")]

	// Assert session organization is target Org Y (55), NOT admin Org X (999)
	sessionY, ok := globalSavingImportSessionStore.GetSession(sessionID, targetOrgY)
	if !ok || sessionY == nil {
		t.Fatalf("session not found under target organization Y (%d)", targetOrgY)
	}
	if sessionY.OrgID != targetOrgY {
		t.Errorf("expected session.OrgID to be target Org Y (%d), got %d", targetOrgY, sessionY.OrgID)
	}
	if sessionY.OrgID == adminOrgX {
		t.Errorf("session.OrgID leaked administrator's organization X (%d)", adminOrgX)
	}

	// Assert session appears in target Org Y's sessions list, but NOT in Org X's sessions list
	sessionsForY := globalSavingImportSessionStore.ListSessions(targetOrgY)
	foundInY := false
	for _, s := range sessionsForY {
		if s.ID == sessionID {
			foundInY = true
			break
		}
	}
	if !foundInY {
		t.Errorf("expected session %s to appear in ListSessions(%d)", sessionID, targetOrgY)
	}

	sessionsForX := globalSavingImportSessionStore.ListSessions(adminOrgX)
	for _, s := range sessionsForX {
		if s.ID == sessionID {
			t.Errorf("session %s improperly appeared in administrator Org X (%d) session list", sessionID, adminOrgX)
		}
	}

	// 2. Submit Column Mapping for Target Org Y
	mapForm := url.Values{
		"col_name":  {"اسم الصنف"},
		"col_sku":   {"كود SKU"},
		"col_qty":   {"الكمية"},
		"col_price": {"سعر الجمهور"},
	}
	mappingPath := fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", targetOrgY, sessionID)
	mapReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), adminActor), http.MethodPost, mappingPath, strings.NewReader(mapForm.Encode()))
	mapReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mapRec := httptest.NewRecorder()
	r.ServeHTTP(mapRec, mapReq)

	if mapRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 on mapping submit, got %d: %s", mapRec.Code, mapRec.Body.String())
	}

	sessionY, _ = globalSavingImportSessionStore.GetSession(sessionID, targetOrgY)
	if sessionY == nil || len(sessionY.Items) != 2 {
		t.Fatalf("expected 2 staged items under target Org Y (%d)", targetOrgY)
	}

	// 3. Admin manually updates / links a row on review
	matchForm := url.Values{
		"product_id":  {"101"},
		"master_name": {"بانادول إكسترا 24 قرص"},
		"master_sku":  {"PAN-EXT-24"},
	}
	matchPath := fmt.Sprintf("/admin/organizations/import/%d/saving/%s/items/1/match", targetOrgY, sessionID)
	matchReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), adminActor), http.MethodPost, matchPath, strings.NewReader(matchForm.Encode()))
	matchReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	matchRec := httptest.NewRecorder()
	r.ServeHTTP(matchRec, matchReq)

	if matchRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 on item match submit, got %d: %s", matchRec.Code, matchRec.Body.String())
	}

	// 4. Commit session and verify records are saved for target Org Y, not Org X
	commitPath := fmt.Sprintf("/admin/organizations/import/%d/saving/%s/commit", targetOrgY, sessionID)
	commitReq := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), adminActor), http.MethodPost, commitPath, nil)
	commitRec := httptest.NewRecorder()
	r.ServeHTTP(commitRec, commitReq)

	if commitRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 on commit submit, got %d: %s", commitRec.Code, commitRec.Body.String())
	}

	if mockRepo.committedOrgID != targetOrgY {
		t.Errorf("committed organization ID: expected target Org Y (%d), got %d", targetOrgY, mockRepo.committedOrgID)
	}
	if mockRepo.committedOrgID == adminOrgX {
		t.Errorf("committed records improperly assigned to administrator Org X (%d)", adminOrgX)
	}
	if len(mockRepo.committedProduct) != 2 {
		t.Errorf("expected 2 committed products, got %d", len(mockRepo.committedProduct))
	}
	for i, cp := range mockRepo.committedProduct {
		if cp.OrganizationID != targetOrgY {
			t.Errorf("product %d OrganizationID: expected %d, got %d", i+1, targetOrgY, cp.OrganizationID)
		}
	}
}
