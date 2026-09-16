package ui_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func makeTestXLSX(t *testing.T, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Sheet1"
	for rIdx, row := range rows {
		for cIdx, val := range row {
			cell, err := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(sheet, cell, val))
		}
	}
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

func doMultipartPOST(t *testing.T, h http.Handler, path string, fieldName, filename string, content []byte, extraFields map[string]string, actor authctx.Actor) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)

	for k, v := range extraFields {
		require.NoError(t, writer.WriteField(k, v))
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if actor.UserID != 0 || actor.OrganizationID != 0 || actor.Role != "" {
		req = req.WithContext(authctx.WithActor(req.Context(), actor))
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSpreadsheetSecurity_CustomerSavingUploadRejection(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	actor := authctx.Actor{
		UserID:         20,
		OrganizationID: 200,
		OrgID:          200,
		OrgType:        "customer",
		Permissions:    []string{"pharmacy.saving_product.manage", "pharmacy.saving_product.view"},
	}

	// 1. XLSX with http URL
	maliciousXLSX := makeTestXLSX(t, [][]any{
		{"اسم الدواء", "الباركود", "السعر", "الكمية"},
		{"بنادول اكسترا", "123456", 50.0, 10},
		{"باراسيتامول", "http://evil-attacker.com/malware", 25.0, 5},
	})

	rec := doMultipartPOST(t, r, "/customer/saving-products/import/upload", "file", "products.xlsx", maliciousXLSX, nil, actor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	decodedLoc, _ := url.QueryUnescape(loc)
	assert.Contains(t, decodedLoc, filesecurity.SecurityErrorMessage)

	// 2. CSV with domain
	maliciousCSV := []byte("اسم الدواء,الباركود,السعر,الكمية\nكونجستال,phishing-login.com,30.0,15\n")
	rec = doMultipartPOST(t, r, "/customer/saving-products/import/upload", "file", "products.csv", maliciousCSV, nil, actor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc = rec.Header().Get("Location")
	decodedLoc, _ = url.QueryUnescape(loc)
	assert.Contains(t, decodedLoc, filesecurity.SecurityErrorMessage)
}

func TestSpreadsheetSecurity_CustomerSavingPreviewJSONRejection(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	actor := authctx.Actor{
		UserID:         20,
		OrganizationID: 200,
		OrgID:          200,
		OrgType:        "customer",
		Permissions:    []string{"pharmacy.saving_product.manage", "pharmacy.saving_product.view"},
	}

	maliciousXLSX := makeTestXLSX(t, [][]any{
		{"اسم الدواء", "الباركود", "السعر", "الكمية"},
		{"فيتامين سي", "www.hack-pharma.org", 45.0, 20},
	})

	rec := doMultipartPOST(t, r, "/customer/saving-products/preview-columns", "file", "preview.xlsx", maliciousXLSX, nil, actor)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, false, resp["success"])
	assert.Equal(t, filesecurity.SecurityErrorMessage, resp["error"])
}

func TestSpreadsheetSecurity_CustomerSavingStartJSONRejection(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	actor := authctx.Actor{
		UserID:         20,
		OrganizationID: 200,
		OrgID:          200,
		OrgType:        "customer",
		Permissions:    []string{"pharmacy.saving_product.manage", "pharmacy.saving_product.view"},
	}

	maliciousXLSX := makeTestXLSX(t, [][]any{
		{"اسم الدواء", "الباركود", "السعر", "الكمية"},
		{"أوجمنتين", "192.168.1.100", 90.0, 5},
	})

	rec := doMultipartPOST(t, r, "/customer/saving-products/import/start", "file", "start.xlsx", maliciousXLSX, nil, actor)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, false, resp["success"])
	assert.Equal(t, filesecurity.SecurityErrorMessage, resp["error"])
}

func TestSpreadsheetSecurity_VendorSavingUploadRejection(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	actor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgID:          100,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.saving_product.manage", "vendor.saving_product.view"},
	}

	maliciousXLSX := makeTestXLSX(t, [][]any{
		{"اسم المنتج", "كود", "السعر", "الكمية"},
		{"أوميجا 3", "https://trojan-download.xyz", 120.0, 30},
	})

	rec := doMultipartPOST(t, r, "/vendor/saving-products/import/upload", "file", "vendor_items.xlsx", maliciousXLSX, nil, actor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	decodedLoc, _ := url.QueryUnescape(loc)
	assert.Contains(t, decodedLoc, filesecurity.SecurityErrorMessage)
}

func TestSpreadsheetSecurity_TeamImportEmailsAllowedVsUrlsRejected(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	actor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgID:          100,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.team.view", "vendor.team.create"},
	}

	// A. Valid employee emails are ALLOWED in team import
	cleanTeamXLSX := makeTestXLSX(t, [][]any{
		{"اسم الموظف", "البريد الإلكتروني", "الموبايل", "الدور"},
		{"محمد محمود", "mohamed@pharmacy.com", "01012345678", "مدير"},
		{"سارة أحمد", "sara@company.eg", "01112345678", "صيدلي"},
	})

	rec := doMultipartPOST(t, r, "/vendor/team/import/upload", "file", "team.xlsx", cleanTeamXLSX, nil, actor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	assert.Contains(t, loc, "/vendor/team/import/")
	assert.NotContains(t, loc, filesecurity.SecurityErrorMessage)

	// B. Web URLs and raw domains are strictly REJECTED in team import
	maliciousTeamXLSX := makeTestXLSX(t, [][]any{
		{"اسم الموظف", "البريد الإلكتروني", "الموبايل", "الدور"},
		{"مخترق", "https://malicious-phish.com", "01099999999", "مدير"},
	})

	rec = doMultipartPOST(t, r, "/vendor/team/import/upload", "file", "team_bad.xlsx", maliciousTeamXLSX, nil, actor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc = rec.Header().Get("Location")
	decodedLoc, _ := url.QueryUnescape(loc)
	assert.Contains(t, decodedLoc, filesecurity.SecurityErrorMessage)
}

func TestSpreadsheetSecurity_AdminOrgImportRejection(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	actor := authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "superadmin",
		Permissions: []string{"catalog.org_import.run", "catalog.org_import.view"},
	}

	maliciousXLSX := makeTestXLSX(t, [][]any{
		{"اسم المنتج", "الكود", "السعر", "الكمية"},
		{"انسولين", "http://c2-server.ru", 150.0, 10},
	})

	extra := map[string]string{"org_id": "100"}
	rec := doMultipartPOST(t, r, "/admin/organizations/import/saving/upload", "file", "org_bad.xlsx", maliciousXLSX, extra, actor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	decodedLoc, _ := url.QueryUnescape(loc)
	assert.Contains(t, decodedLoc, filesecurity.SecurityErrorMessage)
}

func TestSpreadsheetSecurity_CompareUploadRejection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	compareSvc := compare.NewService(repo, logger)

	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
	h.SetCompareService(compareSvc)

	maliciousXLSX := makeTestXLSX(t, [][]any{
		{"كود الصنف", "اسم الصنف", "السعر", "الخصم"},
		{"101", "أسبوسيد", 15.0, 10.0},
		{"102", "https://bad-site.xyz/phish", 20.0, 5.0},
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("supplier_name", "مورد تجريبي")
	part, _ := writer.CreateFormFile("compare_file", "compare_bad.xlsx")
	_, _ = part.Write(maliciousXLSX)
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/compare/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	actor := authctx.Actor{
		UserID:         100,
		OrganizationID: 200,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.*"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	h.CompareUploadSubmit(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	decodedLoc, _ := url.QueryUnescape(loc)
	assert.Contains(t, decodedLoc, filesecurity.SecurityErrorMessage)
}

func TestSpreadsheetSecurity_AdminTempWarehouseRejection(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	maliciousXLSX := makeTestXLSX(t, [][]any{
		{"كود الصنف", "اسم الصنف", "السعر", "الخصم"},
		{"101", "أدول", "www.c2-command.com", 10.0},
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "temp_bad.xlsx")
	_, _ = part.Write(maliciousXLSX)
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/admin/temporary-warehouses/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	actor := authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "superadmin",
		Permissions: []string{"inventory.warehouse.manage"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	h.AdminTempWarehouseUploadSubmit(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	decodedLoc, _ := url.QueryUnescape(loc)
	assert.Contains(t, decodedLoc, filesecurity.SecurityErrorMessage)
}
