package ui_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

func TestHeadToHeadExportsAllFilteredRowsBeyondPage(t *testing.T) {
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
	for _, tc := range []struct {
		query string
		count int
	}{{"", 137}, {"&q=Product+136", 1}, {"&tab=your_better", 0}} {
		req := httptest.NewRequestWithContext(context.Background(), "GET", "/compare/head-to-head?source=1&target=1&limit=100&page=2&export=csv"+tc.query, nil)
		req = req.WithContext(authctx.WithActor(context.Background(), authctx.Actor{UserID: 10, OrganizationID: 100, OrgType: "vendor"}))
		rec := httptest.NewRecorder()
		h.CompareHeadToHeadPage(rec, req)
		if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
			t.Fatalf("expected CSV: %d %s", rec.Code, rec.Body.String())
		}
		rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(rec.Body.String(), "\ufeff"))).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(rows)-1 != tc.count {
			t.Fatalf("query=%s got %d expected %d", tc.query, len(rows)-1, tc.count)
		}
	}
}
