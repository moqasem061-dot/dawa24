package main

import (
	"log"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func reviewFixture(w http.ResponseWriter, req *http.Request) {
	lang := i18n.ParseLang(req.URL.Query().Get("lang"))
	view := reviewFixtureView(string(lang))
	ctx := httpx.WithLang(layouts.WithPath(req.Context(), req.URL.Path), lang)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngestPage(view, string(lang), lang.Dir()).Render(ctx, w); err != nil {
		log.Print(err)
	}
}

func reviewFixtureView(lang string) pages.VendorImportView {
	rows := make([]*ingest.RowOutcome, 0, 4)
	for index, code := range []string{"word_mismatch", "similarity", "ambiguous_siblings", "strength_conflict"} {
		productID := int64(index + 1)
		row := &ingest.RowOutcome{
			ID: productID, SourceRow: 103 + index*57, SourceCode: "23045",
			DisplayName: "إلكسوليت شراب 360 مل", MatchedProductName: "إلكسوليت محلول جفاف 360 مل",
			ProductID: &productID, MatchLevel: "strong", MatchScore: 0.85,
			MatchDecision: &matchflow.RowDecision{Reasons: []matchflow.Reason{{Code: code}}},
			Payload: &productmatch.Row{
				PublicPrice: money.MustParse("12345.67"), CostPrice: money.MustParse("650.00"),
				DiscountBps: 3200, CostDiscountBps: 1000, Quantity: 123456, HasQuantity: true,
			},
		}
		if lang == "en" {
			row.DisplayName = "Electrolyte oral solution 360 ml"
			row.MatchedProductName = "Oral rehydration electrolyte solution 360 ml"
		}
		if index == 1 {
			row.MatchDecision.Flags = []string{"retrieval_incomplete"}
		}
		if index == 3 {
			row.ProductID = nil
			row.MatchLevel = "none"
			row.MatchDecision.Reasons[0].Params = map[string]string{"attr": "strength", "expected": "60 mg", "actual": "30 mg"}
		}
		rows = append(rows, row)
	}
	return pages.VendorImportView{
		Lang: lang, Rows: rows, RowTotal: len(rows), PerPage: 25, Page: 1,
		Session: &ingest.Session{
			PublicID: "review-fixture", Phase: ingest.PhaseReview, TotalRows: len(rows), MatchedRows: 3, UnmatchedRows: 1,
			Settings: ingest.Settings{Mode: ingest.ModeUpsert, StockMode: ingest.StockKeep},
		},
	}
}
