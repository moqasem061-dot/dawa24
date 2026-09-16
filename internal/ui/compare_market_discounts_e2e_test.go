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

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestCompareUploadSubmit_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	compareSvc := compare.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	h.SetCompareService(compareSvc)

	// Prepare multipart form with CSV file
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("supplier_name", "شركة الفتح لتوزيع الأدوية")

	part, err := writer.CreateFormFile("compare_file", "prices.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	csvData := "كود الصنف,اسم الصنف الدوائي,السعر الرسمي,نسبة الخصم\n1001,بانادول اكسترا 24 قرص,45.00,18.5\n1002,اوجمنتين 1 جم 14 قرص,135.00,12.0\n"
	_, _ = part.Write([]byte(csvData))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/compare/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Attach authenticated vendor actor
	actor := authctx.Actor{
		UserID:         100,
		OrganizationID: 200,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.*"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	h.CompareUploadSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other redirect, got %d", res.StatusCode)
	}

	loc := res.Header.Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("invalid redirect url: %v", err)
	}
	if u.Path != "/compare/tool" {
		t.Errorf("expected redirect to /compare/tool, got %s", u.Path)
	}
	if u.Query().Get("notice") != "success" {
		t.Errorf("expected success notice, got notice=%s msg=%s", u.Query().Get("notice"), u.Query().Get("msg"))
	}

	// Verify file and rows in repository.
	//
	// The rows arrive AFTER the redirect. Reading a whole workbook and writing
	// every row of it used to happen inside this POST, which is why a ten-file
	// batch held the browser for minutes; the upload now records the file and
	// hands the parse to a goroutine that outlives the request. So the handler
	// returning is the start of the work, not the end of it, and the test waits
	// for the outcome exactly as the screen does.
	if len(repo.files) != 1 {
		t.Fatalf("expected 1 file stored in repo, got %d", len(repo.files))
	}
	var uploadedFile *compare.CompareFile
	for _, f := range repo.files {
		uploadedFile = f
	}
	if uploadedFile.SupplierName != "شركة الفتح لتوزيع الأدوية" {
		t.Errorf("unexpected supplier name: %s", uploadedFile.SupplierName)
	}

	waitForStaging(t, repo, uploadedFile.ID)

	if got := repo.fileOf(uploadedFile.ID).RowCount; got != 2 {
		t.Errorf("expected 2 rows extracted, got %d", got)
	}
	if len(repo.rowsOf(uploadedFile.ID)) != 2 {
		t.Errorf("expected 2 file rows stored, got %d", len(repo.rowsOf(uploadedFile.ID)))
	}

	// Test CompareToolPage renders the file and success notice
	reqTool := httptest.NewRequest("GET", "/compare/tool?"+u.RawQuery, nil)
	reqTool = reqTool.WithContext(authctx.WithActor(reqTool.Context(), actor))
	recTool := httptest.NewRecorder()
	h.CompareToolPage(recTool, reqTool)

	toolBody := recTool.Body.String()
	if !strings.Contains(toolBody, "شركة الفتح لتوزيع الأدوية") {
		t.Errorf("expected CompareToolPage to contain supplier name, got: %s", toolBody)
	}
	// The row-count badge on the file card, not the notice text.
	//
	// This used to assert against the redirect's success message, which quoted
	// a row count the upload knew because it had just parsed everything inside
	// the request. It no longer knows: the parse is detached, so the message
	// counts files and the ROW count is whatever the staged file ended up with.
	// Reading it off the rendered card is the stronger assertion anyway — it is
	// the number the pharmacy actually sees.
	if !strings.Contains(toolBody, "2 صنف") {
		t.Errorf("expected CompareToolPage to show the staged row count badge")
	}
	if !strings.Contains(toolBody, "شركة الفتح لتوزيع الأدوية") {
		t.Errorf("expected the supplier card to be listed")
	}
}

func TestCompareSampleDownload_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	req := httptest.NewRequest("GET", "/compare/sample", nil)
	rec := httptest.NewRecorder()
	h.CompareSampleDownload(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "spreadsheetml") {
		t.Errorf("expected excel spreadsheet Content-Type, got %s", ct)
	}
	if rec.Body.Len() == 0 {
		t.Errorf("expected non-empty Excel template response body")
	}
}

func TestCompareFileMappingModal_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	compareSvc := compare.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	h.SetCompareService(compareSvc)

	// Create test file with rows
	nameCol := 1
	priceCol := 2
	file := &compare.CompareFile{
		SupplierName:     "شركة الدواء الحديث للتوزيع",
		OriginalFilename: "prices.xlsx",
		UserID:           100,
		RowCount:         3,
		Status:           compare.FileReady,
		MappingConfig: compare.MappingConfig{
			NameCol:  &nameCol,
			PriceCol: &priceCol,
		},
	}
	_ = repo.CreateFile(context.Background(), file)

	// Add sample rows to repo
	_ = repo.InsertFileRows(context.Background(), []*compare.CompareFileRow{
		{FileID: file.ID, SKU: "101", RawName: "بانادول 24 قرص", NormalizedName: "بانادول 24 قرص", Price: money.FromMinor(4500), Discount: 15.0},
		{FileID: file.ID, SKU: "102", RawName: "كونجستال 20 قرص", NormalizedName: "كونجستال 20 قرص", Price: money.FromMinor(3100), Discount: 20.0},
	})

	actor := authctx.Actor{UserID: 100, OrganizationID: 200, OrgType: "customer", Permissions: []string{"pharmacy.*"}}

	// Test GET /compare/files/{id}/mapping-modal
	reqModal := httptest.NewRequest("GET", fmt.Sprintf("/compare/files/%d/mapping-modal", file.ID), nil)
	reqModal = reqModal.WithContext(authctx.WithActor(reqModal.Context(), actor))
	recModal := httptest.NewRecorder()

	// Use chi routing context to provide URL param {id}
	rCtx := chi.NewRouteContext()
	rCtx.URLParams.Add("id", fmt.Sprintf("%d", file.ID))
	reqModal = reqModal.WithContext(context.WithValue(reqModal.Context(), chi.RouteCtxKey, rCtx))

	h.CompareFileMappingModal(recModal, reqModal)

	if recModal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for mapping modal, got %d", recModal.Code)
	}

	body := recModal.Body.String()
	if !strings.Contains(body, "شركة الدواء الحديث للتوزيع") {
		t.Errorf("expected modal to contain supplier name")
	}
	if !strings.Contains(body, "اسم الصنف الدوائي") {
		t.Errorf("expected modal to contain product name field")
	}
	if !strings.Contains(body, "حفظ") {
		t.Errorf("expected modal to contain submit button")
	}
}

func TestCompareQuickSearch_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	compareSvc := compare.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	h.SetCompareService(compareSvc)

	// Create test file and rows
	file := &compare.CompareFile{
		ID:               10,
		UserID:           100,
		OrganizationID:   &[]int64{200}[0],
		SupplierName:     "شركة النصر للأدوية",
		OriginalFilename: "nasr.xlsx",
		Status:           compare.FileReady,
	}
	_ = repo.CreateFile(context.Background(), file)

	_ = repo.InsertFileRows(context.Background(), []*compare.CompareFileRow{
		{
			FileID:             file.ID,
			SKU:                "1001",
			RawName:            "بانادول اكسترا 24 قرص",
			NormalizedName:     "بانادول اكسترا 24 قرص",
			Price:              money.FromMajor(45),
			Discount:           18.5,
			PriceAfterDiscount: money.FromMinor(3668),
			MatchedProductID:   &[]int64{45}[0],
		},
	})

	actor := authctx.Actor{UserID: 100, OrganizationID: 200, OrgType: "customer", Permissions: []string{"pharmacy.*"}}

	// Test GET /compare/search?q=بانادول
	req := httptest.NewRequest("GET", "/compare/search?q=بانادول", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	h.CompareQuickSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected application/json Content-Type, got %s", contentType)
	}

	var res compare.CompareSearchResults
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if res.TotalMatches == 0 {
		t.Fatalf("expected search results for 'بانادول', got 0")
	}

	item := res.Items[0]
	if item.BestSupplier != "شركة النصر للأدوية" {
		t.Errorf("expected best supplier 'شركة النصر للأدوية', got %s", item.BestSupplier)
	}
	if item.CatalogStatus != compare.StatusCatalogAndSuppliers {
		t.Errorf("expected catalog status %s, got %s", compare.StatusCatalogAndSuppliers, item.CatalogStatus)
	}
}

// خصومات السوق العامة renders the temporary warehouses and refuses everything
// else.
//
// Both halves matter and the second is the one worth a test: an ordinary
// Compare Tool upload belongs to the supplier who uploaded it, and this board
// is read by every other supplier on the platform. The fixture below puts one
// of each into the repository and asserts that exactly one of them reaches the
// page.
func TestMarketDiscountsPage_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	compareSvc := compare.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	h.SetCompareService(compareSvc)

	// A temporary warehouse: uploaded by a moderator, no organization behind it.
	warehouse := &compare.CompareFile{
		ID:               25,
		UserID:           1,
		SupplierName:     "مخزن المتحدة بلقيس",
		OriginalFilename: "united.xlsx",
		Status:           compare.FileReady,
		IsTempWarehouse:  true,
	}
	_ = repo.CreateFile(context.Background(), warehouse)

	// A supplier's own Compare Tool upload, which must not appear.
	private := &compare.CompareFile{
		ID:               26,
		UserID:           100,
		OrganizationID:   &[]int64{200}[0],
		SupplierName:     "قائمة أسعار خاصة",
		OriginalFilename: "private.xlsx",
		Status:           compare.FileReady,
	}
	_ = repo.CreateFile(context.Background(), private)

	_ = repo.InsertFileRows(context.Background(), []*compare.CompareFileRow{
		{
			FileID:             warehouse.ID,
			SKU:                "9901",
			RawName:            "اماريل 1مجم اقراص",
			NormalizedName:     "اماريل 1مجم اقراص",
			Price:              money.FromMajor(40),
			Discount:           36.0,
			PriceAfterDiscount: money.FromMinor(2560),
			MatchedProductID:   &[]int64{101}[0],
		},
		{
			FileID:             private.ID,
			SKU:                "7777",
			RawName:            "صنف من قائمة خاصة",
			NormalizedName:     "صنف من قائمة خاصة",
			Price:              money.FromMajor(50),
			Discount:           10.0,
			PriceAfterDiscount: money.FromMinor(4500),
		},
	})

	actor := authctx.Actor{UserID: 100, OrganizationID: 200, OrgType: "vendor", Permissions: []string{"vendor.*"}}

	req := httptest.NewRequest("GET", "/market-discounts", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	h.MarketDiscountsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "خصومات السوق العامة") {
		t.Errorf("expected page title 'خصومات السوق العامة'")
	}
	if !strings.Contains(body, "اماريل 1مجم اقراص") {
		t.Errorf("expected the temporary warehouse's line to render")
	}
	if !strings.Contains(body, "مخزن المتحدة بلقيس") {
		t.Errorf("expected the warehouse name to render")
	}
	if !strings.Contains(body, "36%") || !strings.Contains(body, "market-disc-pill") {
		t.Errorf("expected the discount to render as a .market-disc-pill")
	}

	// The supplier's own upload must not be on a board other suppliers read —
	// not even for the supplier who owns it, because there is no per-caller
	// slice of this page.
	if strings.Contains(body, "صنف من قائمة خاصة") || strings.Contains(body, "قائمة أسعار خاصة") {
		t.Error("a Compare Tool upload reached خصومات السوق العامة; only temporary warehouses belong there")
	}

	// The product code is gone from both card layouts: the uploads carry
	// whatever the sheet's code column held, and on files mapped before the
	// header detection was fixed that is a price.
	if strings.Contains(body, "market-code") {
		t.Error("the cards still render a product code")
	}
	// "السعر بعد الخصم" is explicitly rendered per user design specification.
	if !strings.Contains(body, "السعر بعد الخصم") {
		t.Error("the cards must render السعر بعد الخصم")
	}
	// And the upload date is present, which the grid card never showed.
	if !strings.Contains(body, "market-list-date") {
		t.Error("the cards do not show the upload date")
	}
}
