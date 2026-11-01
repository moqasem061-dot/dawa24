package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

// headToHeadExportBody runs the comparison download and returns the workbook it
// produced, opened and ready to read.
func headToHeadExportBody(t *testing.T, query string) *excelize.File {
	t.Helper()
	repo := newMockCompareRepoE2E()
	repo.files[1] = &compare.CompareFile{ID: 1, UserID: 10, SupplierName: "Supplier", Status: compare.FileReady}
	for i := range 137 {
		productID := int64(i + 1)
		repo.fileRows[1] = append(repo.fileRows[1], &compare.CompareFileRow{
			ID: productID, FileID: 1, MatchedProductID: &productID, RawName: fmt.Sprintf("Product %d", i), SKU: fmt.Sprint(i), Price: money.FromMajor(10), Discount: 15,
		})
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
	h.SetCompareService(compare.NewService(repo, logger))

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/compare/head-to-head?source=1&target=1&limit=100&page=2&export=xlsx"+query, nil)
	req = req.WithContext(authctx.WithActor(context.Background(), authctx.Actor{UserID: 10, OrganizationID: 100, OrgType: "vendor"}))
	rec := httptest.NewRecorder()
	h.CompareHeadToHeadPage(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "spreadsheetml.sheet") {
		t.Fatalf("query=%s: expected an Excel download, got %d %q", query, rec.Code, ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".xlsx") {
		t.Fatalf("query=%s: expected an attached .xlsx, got %q", query, cd)
	}
	book, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("query=%s: exported body is not a readable workbook: %v", query, err)
	}
	t.Cleanup(func() { _ = book.Close() })
	return book
}

// The download is the whole filtered report, not the page on screen: a reader
// who filters to 137 rows gets 137 rows even though the page shows 25.
func TestHeadToHeadExportsAllFilteredRowsBeyondPage(t *testing.T) {
	for _, tc := range []struct {
		query string
		count int
	}{{"", 137}, {"&q=Product+136", 1}, {"&tab=your_better", 0}} {
		book := headToHeadExportBody(t, tc.query)
		rows, err := book.GetRows(book.GetSheetName(0))
		if err != nil {
			t.Fatalf("query=%s: read rows: %v", tc.query, err)
		}
		if len(rows)-1 != tc.count {
			t.Fatalf("query=%s got %d rows expected %d", tc.query, len(rows)-1, tc.count)
		}
	}
}

// Prices and discounts are numbers, not text: a CSV left them as strings, and a
// reader could not total a column of them without converting every cell.
func TestHeadToHeadWorkbookStoresMoneyAndDiscountsAsNumbers(t *testing.T) {
	book := headToHeadExportBody(t, "")
	sheet := book.GetSheetName(0)
	for _, cell := range []string{"A2", "D2", "E2", "F2", "G2", "H2", "I2", "J2"} {
		kind, err := book.GetCellType(sheet, cell)
		if err != nil {
			t.Fatalf("cell %s: %v", cell, err)
		}
		// excelize leaves a number's type attribute off, which is how a
		// spreadsheet marks a numeric cell. A string cell is the failure.
		if kind == excelize.CellTypeSharedString || kind == excelize.CellTypeInlineString {
			t.Errorf("cell %s holds text, want a number", cell)
		}
		raw, err := book.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatalf("cell %s: %v", cell, err)
		}
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			t.Errorf("cell %s = %q, which is not a number a reader could total", cell, raw)
		}
	}
	price, err := book.GetCellValue(sheet, "D2", excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	if price != "10" {
		t.Errorf("public price cell = %q, want the exact quoted price 10", price)
	}
}

// The workbook carries a summary sheet as well as the rows, so a reader who only
// received the file still sees who won.
func TestHeadToHeadWorkbookHasSummarySheet(t *testing.T) {
	book := headToHeadExportBody(t, "")
	sheets := book.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("expected the rows sheet and a summary sheet, got %v", sheets)
	}
	summary, err := book.GetRows(sheets[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) < 4 {
		t.Fatalf("expected the summary to carry the report's statistics, got %d rows", len(summary))
	}
}
