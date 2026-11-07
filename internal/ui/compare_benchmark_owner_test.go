package ui_test

import (
	"context"
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

// Batch 3 (S05): the market benchmark page shows only the caller's own compare
// files. Another organization's file, named by id in the query, must not leak.
func TestMarketBenchmarkHidesOtherOrgsFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	org115 := int64(115)
	repo.files[1] = &compare.CompareFile{ID: 1, UserID: 50, OrganizationID: &org115, SupplierName: "VDATA-SECRET-SUPPLIER-A", Status: compare.FileReady}
	repo.fileRows[1] = []*compare.CompareFileRow{{
		ID: 1, FileID: 1, OrganizationID: &org115, RawName: "VDATA-SECRET-DRUG-A",
		Price: money.MustParse("100.00"), Discount: 25, PriceAfterDiscount: money.MustParse("75.00"),
	}}
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
	h.SetCompareService(compare.NewService(repo, logger))

	get := func(actor authctx.Actor) string {
		req := httptest.NewRequestWithContext(context.Background(), "GET", "/compare/market-benchmark?file=1", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.CompareMarketBenchmarkPage(rec, req)
		return rec.Body.String()
	}

	// The file selector is fed by ListFiles, which is scoped to the caller in
	// production; this double does not scope it, so the assertion is on the
	// benchmark rows, the data S05 exposed.
	other := get(authctx.Actor{UserID: 60, OrganizationID: 116, OrgType: "vendor"})
	if strings.Contains(other, "VDATA-SECRET-DRUG-A") {
		t.Fatal("org 116 read org 115's benchmark rows")
	}

	owner := get(authctx.Actor{UserID: 50, OrganizationID: 115, OrgType: "vendor"})
	if !strings.Contains(owner, "VDATA-SECRET-DRUG-A") {
		t.Fatal("the owning organization should still see its own benchmark rows")
	}
}
